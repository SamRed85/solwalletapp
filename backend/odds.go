package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// GameOdds is the stored 1X2 line for one game. The percents are 1/odds with the
// bookmaker margin removed, rounded to a whole percent. They never affect a payout.
type GameOdds struct {
	Bookmaker   string `json:"bookmaker"`
	One         string `json:"one"`
	Draw        string `json:"draw"`
	Two         string `json:"two"`
	OnePercent  int    `json:"onePercent"`
	DrawPercent int    `json:"drawPercent"`
	TwoPercent  int    `json:"twoPercent"`
}

// PickShare is the share of rows on each outcome. It is published only once the
// event has started. Revealed and Coupons are coupons, not rows.
type PickShare struct {
	Revealed int             `json:"revealed"`
	Coupons  int             `json:"coupons"`
	Games    []SharePercents `json:"games"`
}

// SharePercents is the whole-percent share of rows for 1, X and 2 on one game.
type SharePercents struct {
	One  int `json:"one"`
	Draw int `json:"draw"`
	Two  int `json:"two"`
}

type storedOdds struct {
	ID          int       `bson:"_id"`
	BookmakerID int       `bson:"bookmakerId"`
	Bookmaker   string    `bson:"bookmaker"`
	One         float64   `bson:"one"`
	Draw        float64   `bson:"draw"`
	Two         float64   `bson:"two"`
	Updated     time.Time `bson:"updatedAt"`
}

// impliedPercents turns decimal odds into whole percents. Each raw share is
// 1/odds. Dividing by the sum takes out the bookmaker margin. Rounding each
// share can leave the three a percent away from 100.
func impliedPercents(one, draw, two float64) (int, int, int, error) {
	if one <= 0 || draw <= 0 || two <= 0 {
		return 0, 0, 0, errors.New("odds must be positive")
	}
	rawOne, rawDraw, rawTwo := 1/one, 1/draw, 1/two
	sum := rawOne + rawDraw + rawTwo
	return int(math.Round(rawOne / sum * 100)), int(math.Round(rawDraw / sum * 100)), int(math.Round(rawTwo / sum * 100)), nil
}

func (row storedOdds) view() *GameOdds {
	one, draw, two, err := impliedPercents(row.One, row.Draw, row.Two)
	if err != nil {
		return nil
	}
	return &GameOdds{
		Bookmaker:  row.Bookmaker,
		One:        strconv.FormatFloat(row.One, 'f', 2, 64),
		Draw:       strconv.FormatFloat(row.Draw, 'f', 2, 64),
		Two:        strconv.FormatFloat(row.Two, 'f', 2, 64),
		OnePercent: one, DrawPercent: draw, TwoPercent: two,
	}
}

func sharePercents(weights [3]uint64) SharePercents {
	total := weights[0] + weights[1] + weights[2]
	if total == 0 {
		return SharePercents{}
	}
	return SharePercents{
		One:  int(math.Round(float64(weights[0]) * 100 / float64(total))),
		Draw: int(math.Round(float64(weights[1]) * 100 / float64(total))),
		Two:  int(math.Round(float64(weights[2]) * 100 / float64(total))),
	}
}

// systemFromMask reads a reveal mask. Bit 0 is 1, bit 1 is X and bit 2 is 2.
// The row count is the product of the selected outcomes, which is the surviving
// rows of a reduced coupon: each placed piece is already that piece's system.
func systemFromMask(picks []byte) ([]string, int) {
	rows := 1
	system := make([]string, len(picks))
	for i, mask := range picks {
		var b strings.Builder
		if mask&0b001 != 0 {
			b.WriteByte('1')
		}
		if mask&0b010 != 0 {
			b.WriteByte('X')
		}
		if mask&0b100 != 0 {
			b.WriteByte('2')
		}
		if b.Len() == 0 {
			return nil, 0
		}
		system[i] = b.String()
		rows *= b.Len()
	}
	return system, rows
}

// publishPickShare clears the share until the event start, and keeps it from then on.
func publishPickShare(e *Event, now time.Time, share *PickShare) {
	if e.StartTime.IsZero() || now.Before(e.StartTime) {
		e.PickShare = nil
		return
	}
	e.PickShare = share
}

func (a *App) decorateEvent(ctx context.Context, e *Event, now time.Time) {
	a.attachOdds(ctx, e)
	var share *PickShare
	if !e.StartTime.IsZero() && !now.Before(e.StartTime) {
		share = a.computePickShare(ctx, e)
	}
	publishPickShare(e, now, share)
}

func (a *App) attachOdds(ctx context.Context, e *Event) {
	if a.odds == nil || a.cfg.OddsBookmakerID <= 0 {
		return
	}
	ids := make([]int, 0, len(e.Games))
	for _, game := range e.Games {
		if game.FixtureID > 0 {
			ids = append(ids, game.FixtureID)
		}
	}
	if len(ids) == 0 {
		return
	}
	cur, err := a.odds.Find(ctx, bson.M{"_id": bson.M{"$in": ids}, "bookmakerId": a.cfg.OddsBookmakerID})
	if err != nil {
		return
	}
	defer cur.Close(ctx)
	byID := map[int]storedOdds{}
	for cur.Next(ctx) {
		var row storedOdds
		if cur.Decode(&row) == nil {
			byID[row.ID] = row
		}
	}
	for i := range e.Games {
		if row, ok := byID[e.Games[i].FixtureID]; ok {
			e.Games[i].Odds = row.view()
		}
	}
}

func (a *App) computePickShare(ctx context.Context, e *Event) *PickShare {
	weights := make([][3]uint64, len(e.Games))
	revealed, coupons := 0, 0
	if e.Chain != nil && e.Chain.Address != "" {
		revealed, coupons = a.addSlipShare(ctx, e.Chain.Address, len(e.Games), weights)
	} else if !e.hasVault {
		entries, err := a.loadEventEntries(ctx, e.ID)
		if err != nil {
			entries = nil
		}
		coupons = len(entries)
		revealed = coupons
		for _, entry := range entries {
			entry.addPickWeights(weights)
		}
	}
	share := &PickShare{Revealed: revealed, Coupons: coupons, Games: make([]SharePercents, len(weights))}
	for i, weight := range weights {
		share.Games[i] = sharePercents(weight)
	}
	return share
}

func (a *App) addSlipShare(ctx context.Context, event string, gameCount int, weights [][3]uint64) (revealed, coupons int) {
	if a.slips == nil || event == "" {
		return 0, 0
	}
	n, err := a.slips.CountDocuments(ctx, bson.M{"event": event, "kind": "entry"})
	if err != nil {
		return 0, 0
	}
	coupons = int(n)
	cur, err := a.slips.Find(ctx, bson.M{"event": event, "kind": "reveal"})
	if err != nil {
		return 0, coupons
	}
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		var record slipRecord
		if cur.Decode(&record) != nil {
			continue
		}
		picks, err := hex.DecodeString(record.Picks)
		if err != nil || len(picks) < gameCount {
			continue
		}
		colors, err := parseColorHex(record.Colors)
		if err != nil {
			continue
		}
		if len(colors) > 0 {
			addColoredWeights(weights, picks[:gameCount], colors)
			revealed++
			continue
		}
		system, rows := systemFromMask(picks[:gameCount])
		if rows == 0 {
			continue
		}
		Entry{System: system, Combinations: rows}.addPickWeights(weights)
		revealed++
	}
	return revealed, coupons
}

func (a *App) handleRefreshOdds(w http.ResponseWriter, r *http.Request) {
	if a.feed == nil || a.feed.key == "" {
		writeError(w, http.StatusServiceUnavailable, "the score feed is not configured")
		return
	}
	if a.cfg.OddsBookmakerID <= 0 || a.odds == nil {
		writeError(w, http.StatusServiceUnavailable, "the odds bookmaker is not configured")
		return
	}
	e, ok := a.loadEvent(w, r)
	if !ok {
		return
	}
	linked := 0
	for _, game := range e.Games {
		if game.FixtureID > 0 {
			linked++
		}
	}
	if linked == 0 {
		writeError(w, http.StatusBadRequest, "no match is linked to a fixture")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	saved := 0
	var missing []string
	for _, game := range e.Games {
		if game.FixtureID <= 0 {
			continue
		}
		label := strings.TrimSpace(game.HomeTeam + " vs " + game.AwayTeam)
		row, err := a.feed.matchWinner(ctx, game.FixtureID, a.cfg.OddsBookmakerID)
		if err != nil {
			missing = append(missing, label)
			if strings.Contains(err.Error(), "quota") {
				break
			}
			continue
		}
		row.ID = game.FixtureID
		if err = a.storeOdds(ctx, row); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to save odds")
			return
		}
		saved++
	}
	if saved == 0 {
		writeError(w, http.StatusBadGateway, "no Match Winner odds for "+strings.Join(missing, ", "))
		return
	}
	shown := []Event{*e}
	a.countCoupons(ctx, shown)
	a.decorateEvent(ctx, &shown[0], time.Now())
	writeJSON(w, http.StatusOK, map[string]any{"event": shown[0], "missing": missing})
}

func (a *App) storeOdds(ctx context.Context, row storedOdds) error {
	if a.odds == nil {
		return errors.New("odds are not configured")
	}
	_, err := a.odds.UpdateOne(ctx, bson.M{"_id": row.ID}, bson.M{"$set": bson.M{
		"bookmakerId": row.BookmakerID,
		"bookmaker":   row.Bookmaker,
		"one":         row.One,
		"draw":        row.Draw,
		"two":         row.Two,
		"updatedAt":   time.Now().UTC(),
	}}, options.UpdateOne().SetUpsert(true))
	return err
}

func (f *footballFeed) matchWinner(ctx context.Context, fixtureID, bookmakerID int) (storedOdds, error) {
	path := fmt.Sprintf("/odds?fixture=%d&bookmaker=%d&bet=1", fixtureID, bookmakerID)
	body, _, err := f.get(ctx, path)
	if err != nil {
		return storedOdds{}, err
	}
	return parseMatchWinner(body, bookmakerID)
}

func parseMatchWinner(body []byte, bookmakerID int) (storedOdds, error) {
	var rows []struct {
		Bookmakers []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
			Bets []struct {
				ID     int        `json:"id"`
				Name   string     `json:"name"`
				Values []oddValue `json:"values"`
			} `json:"bets"`
		} `json:"bookmakers"`
	}
	if err := json.Unmarshal(body, &rows); err != nil {
		return storedOdds{}, err
	}
	for _, row := range rows {
		for _, book := range row.Bookmakers {
			if bookmakerID > 0 && book.ID != bookmakerID {
				continue
			}
			for _, bet := range book.Bets {
				if bet.ID != 1 && !strings.EqualFold(bet.Name, "Match Winner") {
					continue
				}
				parsed, err := winnerOdds(book.ID, book.Name, bet.Values)
				if err != nil {
					return storedOdds{}, err
				}
				return parsed, nil
			}
		}
	}
	return storedOdds{}, errors.New("no Match Winner odds")
}

type oddValue struct {
	Value string `json:"value"`
	Odd   string `json:"odd"`
}

func winnerOdds(bookmakerID int, name string, values []oddValue) (storedOdds, error) {
	var one, draw, two float64
	for _, value := range values {
		odd, err := strconv.ParseFloat(value.Odd, 64)
		if err != nil || odd <= 0 {
			continue
		}
		switch strings.ToLower(value.Value) {
		case "home", "1":
			one = odd
		case "draw", "x":
			draw = odd
		case "away", "2":
			two = odd
		}
	}
	if one <= 0 || draw <= 0 || two <= 0 {
		return storedOdds{}, errors.New("the Match Winner line is incomplete")
	}
	return storedOdds{BookmakerID: bookmakerID, Bookmaker: name, One: one, Draw: draw, Two: two}, nil
}

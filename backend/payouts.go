package main

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Payout rules (Phase 1 decisions, see .cursor/plans/master_plan.md):
//   - The house keeps houseCutPercent of ticket sales; the tiers share the rest.
//   - Each tier's pot is split equally between the coupons with exactly that many correct picks.
//   - A tier without winners rolls over into the top tier of the next event that has not started.
//   - Rollover received from earlier events is added to the top tier (all games correct).
//   - Integer division leftovers (at most winners-1 lamports per tier) go to the house.
const (
	defaultGameCount   = 13
	houseCutPercent    = 5
	tierPercentTotal   = 100 - houseCutPercent
	cancelledResult    = "C"
	cancelledDrawDelay = time.Hour
	maxCombinations    = 44000
)

// Percentages for "all correct", "one wrong", "two wrong" and "three wrong".
var defaultTierPercents = []int{50, 25, 15, 5}

type Tier struct {
	Correct int `json:"correct" bson:"correct"`
	Percent int `json:"percent" bson:"percent"`
}

type TierPayout struct {
	Correct           int    `json:"correct" bson:"correct"`
	Percent           int    `json:"percent" bson:"percent"`
	PotLamports       uint64 `json:"potLamports" bson:"potLamports"`
	Winners           int    `json:"winners" bson:"winners"`
	PerWinnerLamports uint64 `json:"perWinnerLamports" bson:"perWinnerLamports"`
}

// Settlement is computed once, when the results are saved, and never changes afterwards.
type Settlement struct {
	Coupons             int            `json:"coupons" bson:"coupons"`
	SalesLamports       uint64         `json:"salesLamports" bson:"salesLamports"`
	RolloverInLamports  uint64         `json:"rolloverInLamports" bson:"rolloverInLamports"`
	HouseLamports       uint64         `json:"houseLamports" bson:"houseLamports"`
	PaidLamports        uint64         `json:"paidLamports" bson:"paidLamports"`
	RolloverOutLamports uint64         `json:"rolloverOutLamports" bson:"rolloverOutLamports"`
	RolloverToEventID   *bson.ObjectID `json:"rolloverToEventId,omitempty" bson:"rolloverToEventId,omitempty"`
	Tiers               []TierPayout   `json:"tiers" bson:"tiers"`
}

// Draw records how the outcome of a cancelled game was chosen, so anyone can recompute it:
// seed = sha256("sol-draw:<eventId>:<gameIndex>:<blockhash>"), r = first 8 bytes (big endian) mod total weight,
// and the outcome is the first of 1, X, 2 whose cumulative weight exceeds r.
type Draw struct {
	Slot      uint64    `json:"slot" bson:"slot"`
	Blockhash string    `json:"blockhash" bson:"blockhash"`
	BlockTime time.Time `json:"blockTime" bson:"blockTime"`
	Weights   [3]uint64 `json:"weights" bson:"weights"`
}

func defaultTiers(gameCount int) []Tier {
	tiers := make([]Tier, 0, len(defaultTierPercents))
	for i, p := range defaultTierPercents {
		if gameCount-i > 0 {
			tiers = append(tiers, Tier{Correct: gameCount - i, Percent: p})
		}
	}
	return tiers
}

func validateTiers(tiers []Tier, gameCount int) string {
	if len(tiers) == 0 {
		return "at least one prize tier is required"
	}
	seen := map[int]bool{}
	total, hasTop := 0, false
	for _, t := range tiers {
		switch {
		case t.Correct < 1 || t.Correct > gameCount:
			return "tier correct picks must be between 1 and the number of games"
		case seen[t.Correct]:
			return "each number of correct picks can only have one tier"
		case t.Percent < 1:
			return "tier percentages must be positive"
		}
		seen[t.Correct] = true
		total += t.Percent
		hasTop = hasTop || t.Correct == gameCount
	}
	if !hasTop {
		return "there must be a tier for all games correct (it receives the rollover)"
	}
	if total != tierPercentTotal {
		return fmt.Sprintf("tier percentages must add up to %d%% (the house keeps %d%%)", tierPercentTotal, houseCutPercent)
	}
	return ""
}

func sortTiers(tiers []Tier) {
	sort.Slice(tiers, func(i, j int) bool { return tiers[i].Correct > tiers[j].Correct })
}

// correctCounts returns how many combinations have exactly n correct picks.
// It multiplies one game at a time, so a 44,000-row coupon is never expanded.
func correctCounts(system []string, results string) []int {
	counts := []int{1}
	for i, picks := range system {
		correct, wrong := 0, 0
		for j := 0; j < len(picks); j++ {
			if i < len(results) && picks[j] == results[i] {
				correct++
			} else {
				wrong++
			}
		}
		next := make([]int, len(counts)+1)
		for k, n := range counts {
			if wrong > 0 {
				next[k] += n * wrong
			}
			if correct > 0 {
				next[k+1] += n * correct
			}
		}
		counts = next
	}
	return counts
}

func countCorrect(coupon, results string) int {
	n := 0
	for i := 0; i < len(coupon) && i < len(results); i++ {
		if coupon[i] == results[i] {
			n++
		}
	}
	return n
}

func settle(coupons []string, results string, salesLamports, rolloverIn uint64, tiers []Tier) Settlement {
	winners := map[int]int{}
	for _, c := range coupons {
		winners[countCorrect(c, results)]++
	}
	return settleCounts(len(coupons), winners, len(results), salesLamports, rolloverIn, tiers)
}

func settleCounts(rows int, winners map[int]int, resultCount int, salesLamports, rolloverIn uint64, tiers []Tier) Settlement {
	s := Settlement{Coupons: rows, SalesLamports: salesLamports, RolloverInLamports: rolloverIn}
	var allocated uint64
	for _, t := range tiers {
		pot := salesLamports * uint64(t.Percent) / 100
		allocated += pot
		if t.Correct == resultCount {
			pot += rolloverIn
		}
		tp := TierPayout{Correct: t.Correct, Percent: t.Percent, PotLamports: pot, Winners: winners[t.Correct]}
		if tp.Winners == 0 {
			s.RolloverOutLamports += pot
		} else {
			tp.PerWinnerLamports = pot / uint64(tp.Winners)
			paid := tp.PerWinnerLamports * uint64(tp.Winners)
			s.PaidLamports += paid
			s.HouseLamports += pot - paid
		}
		s.Tiers = append(s.Tiers, tp)
	}
	s.HouseLamports += salesLamports - allocated
	return s
}

func (s *Settlement) payoutFor(correct int) uint64 {
	if s == nil {
		return 0
	}
	for _, t := range s.Tiers {
		if t.Correct == correct {
			return t.PerWinnerLamports
		}
	}
	return 0
}

// pickWeights counts how many coupons picked 1, X and 2 for one game.
func pickWeights(coupons []string, game int) [3]uint64 {
	var w [3]uint64
	for _, c := range coupons {
		if game < len(c) {
			switch c[game] {
			case '1':
				w[0]++
			case 'X':
				w[1]++
			case '2':
				w[2]++
			}
		}
	}
	return w
}

func drawOutcome(eventID bson.ObjectID, game int, blockhash string, weights [3]uint64) string {
	seed := sha256.Sum256([]byte("sol-draw:" + eventID.Hex() + ":" + strconv.Itoa(game) + ":" + blockhash))
	total := weights[0] + weights[1] + weights[2]
	if total == 0 {
		weights, total = [3]uint64{1, 1, 1}, 3
	}
	r := binary.BigEndian.Uint64(seed[:8]) % total
	for i, o := range []string{"1", "X", "2"} {
		if r < weights[i] {
			return o
		}
		r -= weights[i]
	}
	return "2"
}

func (a *App) loadEventEntries(ctx context.Context, eventID bson.ObjectID) ([]Entry, error) {
	cur, err := a.entries.Find(ctx, bson.M{"eventId": eventID})
	if err != nil {
		return nil, err
	}
	var entries []Entry
	return entries, cur.All(ctx, &entries)
}

func entryTotals(entries []Entry, results string) (rows int, sales uint64, winners map[int]int) {
	winners = map[int]int{}
	for _, en := range entries {
		sales += en.Lamports
		rows += en.rowCount()
		en.addWinners(results, winners)
	}
	return rows, sales, winners
}

func settleEntries(entries []Entry, results string, salesLamports, rolloverIn uint64, tiers []Tier) Settlement {
	rows, _, winners := entryTotals(entries, results)
	return settleCounts(rows, winners, len(results), salesLamports, rolloverIn, tiers)
}

// rolloverTarget is the next event that has not started yet, earliest first.
func (a *App) rolloverTarget(ctx context.Context, exclude bson.ObjectID, now time.Time) (*bson.ObjectID, error) {
	var next Event
	err := a.events.FindOne(ctx,
		bson.M{"_id": bson.M{"$ne": exclude}, "startTime": bson.M{"$gt": now}, "resultsSetAt": bson.M{"$exists": false}, "chain": bson.M{"$exists": false}},
		options.FindOne().SetSort(bson.M{"startTime": 1}).SetProjection(bson.M{"_id": 1}),
	).Decode(&next)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &next.ID, nil
}

const pendingRolloverID = "pending"

func (a *App) applyRollover(ctx context.Context, target *bson.ObjectID, lamports uint64) error {
	if lamports == 0 {
		return nil
	}
	inc := bson.M{"$inc": bson.M{"rolloverInLamports": int64(lamports)}}
	if target != nil {
		_, err := a.events.UpdateByID(ctx, *target, inc)
		return err
	}
	_, err := a.rollover.UpdateByID(ctx, pendingRolloverID, bson.M{"$inc": bson.M{"lamports": int64(lamports)}}, options.UpdateOne().SetUpsert(true))
	return err
}

// takePendingRollover hands rollover that had no target event to a newly created event.
func (a *App) takePendingRollover(ctx context.Context) uint64 {
	var doc struct {
		Lamports int64 `bson:"lamports"`
	}
	err := a.rollover.FindOneAndUpdate(ctx,
		bson.M{"_id": pendingRolloverID, "lamports": bson.M{"$gt": 0}},
		bson.M{"$set": bson.M{"lamports": 0}},
	).Decode(&doc)
	if err != nil {
		if !errors.Is(err, mongo.ErrNoDocuments) {
			log.Printf("failed to take pending rollover: %v", err)
		}
		return 0
	}
	return uint64(doc.Lamports)
}

type WalletPayout struct {
	WalletAddress    string `json:"walletAddress"`
	WinningCoupons   int    `json:"winningCoupons"`
	WinningsLamports uint64 `json:"winningsLamports"`
}

// handlePayouts lists what each wallet should receive. Payment is manual in Phase 1.
func (a *App) handlePayouts(w http.ResponseWriter, r *http.Request) {
	e, ok := a.loadEvent(w, r)
	if !ok {
		return
	}
	if e.Settlement == nil {
		writeError(w, http.StatusConflict, "results have not been set yet")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	entries, err := a.loadEventEntries(ctx, e.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load entries")
		return
	}
	byWallet := map[string]*WalletPayout{}
	for _, en := range entries {
		rows, won := en.payout(e)
		if won == 0 {
			continue
		}
		p := byWallet[en.WalletAddress]
		if p == nil {
			p = &WalletPayout{WalletAddress: en.WalletAddress}
			byWallet[en.WalletAddress] = p
		}
		p.WinningCoupons += rows
		p.WinningsLamports += won
	}
	payouts := make([]WalletPayout, 0, len(byWallet))
	for _, p := range byWallet {
		payouts = append(payouts, *p)
	}
	sort.Slice(payouts, func(i, j int) bool { return payouts[i].WinningsLamports > payouts[j].WinningsLamports })
	writeJSON(w, http.StatusOK, map[string]any{"settlement": e.Settlement, "payouts": payouts})
}

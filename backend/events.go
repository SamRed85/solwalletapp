package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	minGames       = 7
	maxGames       = 13
	maxTitleLen    = 120
	maxDescLen     = 1000
	maxTeamNameLen = 60

	maxCouponPriceLamports = 10_000_000_000
)

// Game results and coupon picks use the same encoding: home win, draw, away win.
var validOutcomes = map[byte]bool{'1': true, 'X': true, '2': true}

// A cancelled game keeps Result set to the drawn outcome, so scoring never needs special cases.
type Game struct {
	HomeTeam  string    `json:"homeTeam" bson:"homeTeam"`
	AwayTeam  string    `json:"awayTeam" bson:"awayTeam"`
	Kickoff   time.Time `json:"kickoff" bson:"kickoff"`
	Result    string    `json:"result" bson:"result"`
	Cancelled bool      `json:"cancelled,omitempty" bson:"cancelled,omitempty"`
	Draw      *Draw     `json:"draw,omitempty" bson:"draw,omitempty"`
}

// Event status is derived, never stored:
// draft (no games yet) -> open (entries allowed until startTime) -> closed -> completed (results set).
type Event struct {
	ID                  bson.ObjectID `json:"id" bson:"_id,omitempty"`
	Title               string        `json:"title" bson:"title"`
	Description         string        `json:"description" bson:"description"`
	GameCount           int           `json:"gameCount" bson:"gameCount"`
	CouponPriceLamports uint64        `json:"couponPriceLamports" bson:"couponPriceLamports"`
	Tiers               []Tier        `json:"tiers" bson:"tiers"`
	RolloverInLamports  uint64        `json:"rolloverInLamports" bson:"rolloverInLamports"`
	StartTime           time.Time     `json:"startTime" bson:"startTime"`
	EndTime             time.Time     `json:"endTime" bson:"endTime"`
	Games               []Game        `json:"games" bson:"games"`
	CreatedBy           string        `json:"createdBy" bson:"createdBy"`
	CreatedAt           time.Time     `json:"createdAt" bson:"createdAt"`
	ResultsSetAt        *time.Time    `json:"resultsSetAt,omitempty" bson:"resultsSetAt,omitempty"`
	Settlement          *Settlement   `json:"settlement,omitempty" bson:"settlement,omitempty"`
	Status              string        `json:"status" bson:"-"`
	CouponsSold         int           `json:"couponsSold" bson:"-"`
}

func (e *Event) results() string {
	b := make([]byte, len(e.Games))
	for i, g := range e.Games {
		if g.Result != "" {
			b[i] = g.Result[0]
		}
	}
	return string(b)
}

// prepare fills derived fields and defaults for events created before per-event rules existed.
func (a *App) prepare(e *Event, now time.Time) {
	e.Status = e.statusAt(now)
	if e.CouponPriceLamports == 0 {
		e.CouponPriceLamports = a.cfg.CouponPriceLamports
	}
	if len(e.Tiers) == 0 {
		e.Tiers = defaultTiers(e.GameCount)
	}
	for i := range e.Games {
		if e.Games[i].Kickoff.IsZero() {
			e.Games[i].Kickoff = e.StartTime
		}
	}
}

func (a *App) countCoupons(ctx context.Context, events []Event) {
	if len(events) == 0 {
		return
	}
	ids := make([]bson.ObjectID, len(events))
	for i, e := range events {
		ids[i] = e.ID
	}
	cur, err := a.entries.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"eventId": bson.M{"$in": ids}}}},
		{{Key: "$group", Value: bson.M{"_id": "$eventId", "coupons": bson.M{"$sum": bson.M{"$cond": bson.A{
			bson.M{"$gt": bson.A{"$combinations", 0}},
			"$combinations",
			bson.M{"$size": bson.M{"$ifNull": bson.A{"$coupons", bson.A{}}}},
		}}}}}},
	})
	if err != nil {
		return
	}
	var counts []struct {
		ID      bson.ObjectID `bson:"_id"`
		Coupons int           `bson:"coupons"`
	}
	if cur.All(ctx, &counts) != nil {
		return
	}
	byID := map[bson.ObjectID]int{}
	for _, c := range counts {
		byID[c.ID] = c.Coupons
	}
	for i := range events {
		events[i].CouponsSold = byID[events[i].ID]
	}
}

func (e *Event) statusAt(now time.Time) string {
	switch {
	case e.ResultsSetAt != nil:
		return "completed"
	case len(e.Games) == 0:
		return "draft"
	case now.Before(e.StartTime):
		return "open"
	default:
		return "closed"
	}
}

func (a *App) findEvent(ctx context.Context, id bson.ObjectID) (*Event, error) {
	var e Event
	if err := a.events.FindOne(ctx, bson.M{"_id": id}).Decode(&e); err != nil {
		return nil, err
	}
	a.prepare(&e, time.Now())
	return &e, nil
}

func eventIDFrom(w http.ResponseWriter, r *http.Request) (bson.ObjectID, bool) {
	id, err := bson.ObjectIDFromHex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid event id")
		return id, false
	}
	return id, true
}

func (a *App) loadEvent(w http.ResponseWriter, r *http.Request) (*Event, bool) {
	id, ok := eventIDFrom(w, r)
	if !ok {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	e, err := a.findEvent(ctx, id)
	if errors.Is(err, mongo.ErrNoDocuments) {
		writeError(w, http.StatusNotFound, "event not found")
		return nil, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load event")
		return nil, false
	}
	return e, true
}

func (a *App) handleListEvents(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	cur, err := a.events.Find(ctx, bson.M{}, options.Find().SetSort(bson.M{"startTime": -1}))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load events")
		return
	}
	events := []Event{}
	if err := cur.All(ctx, &events); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load events")
		return
	}
	now := time.Now()
	for i := range events {
		a.prepare(&events[i], now)
	}
	a.countCoupons(ctx, events)
	writeJSON(w, http.StatusOK, events)
}

func (a *App) handleGetEvent(w http.ResponseWriter, r *http.Request) {
	if e, ok := a.loadEvent(w, r); ok {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		events := []Event{*e}
		a.countCoupons(ctx, events)
		writeJSON(w, http.StatusOK, events[0])
	}
}

func (a *App) handleCreateEvent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title       string    `json:"title"`
		Description string    `json:"description"`
		GameCount   int       `json:"gameCount"`
		StartTime   time.Time `json:"startTime"`
		EndTime     time.Time `json:"endTime"`
		// Required for special events (not 13 games); optional overrides for standard events.
		CouponPriceLamports uint64 `json:"couponPriceLamports"`
		Tiers               []Tier `json:"tiers"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	req.Description = strings.TrimSpace(req.Description)
	now := time.Now()
	if req.GameCount == 0 {
		req.GameCount = defaultGameCount
	}
	special := req.GameCount != defaultGameCount
	if req.CouponPriceLamports == 0 && !special {
		req.CouponPriceLamports = a.cfg.CouponPriceLamports
	}
	if len(req.Tiers) == 0 && !special {
		req.Tiers = defaultTiers(req.GameCount)
	}
	sortTiers(req.Tiers)

	switch {
	case req.Title == "" || len(req.Title) > maxTitleLen:
		writeError(w, http.StatusBadRequest, "title is required (max 120 characters)")
		return
	case len(req.Description) > maxDescLen:
		writeError(w, http.StatusBadRequest, "description is too long (max 1000 characters)")
		return
	case req.GameCount < minGames || req.GameCount > maxGames:
		writeError(w, http.StatusBadRequest, "an event must have between 7 and 13 games")
		return
	case !req.StartTime.After(now):
		writeError(w, http.StatusBadRequest, "start time must be in the future")
		return
	case !req.EndTime.After(req.StartTime):
		writeError(w, http.StatusBadRequest, "end time must be after start time")
		return
	case req.CouponPriceLamports == 0 || req.CouponPriceLamports > maxCouponPriceLamports:
		writeError(w, http.StatusBadRequest, "a coupon price between 1 lamport and 10 SOL is required for special events")
		return
	}
	if msg := validateTiers(req.Tiers, req.GameCount); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	e := Event{
		ID:                  bson.NewObjectID(),
		Title:               req.Title,
		Description:         req.Description,
		GameCount:           req.GameCount,
		CouponPriceLamports: req.CouponPriceLamports,
		Tiers:               req.Tiers,
		RolloverInLamports:  a.takePendingRollover(ctx),
		StartTime:           req.StartTime.UTC(),
		EndTime:             req.EndTime.UTC(),
		Games:               []Game{},
		CreatedBy:           walletFrom(r),
		CreatedAt:           now,
	}
	if _, err := a.events.InsertOne(ctx, e); err != nil {
		_ = a.applyRollover(ctx, nil, e.RolloverInLamports)
		writeError(w, http.StatusInternalServerError, "failed to create event")
		return
	}
	a.prepare(&e, now)
	writeJSON(w, http.StatusCreated, e)
}

// Games can be set once, before entries open. Changing games after people have
// placed coupons would change what their picks mean.
func (a *App) handleSetGames(w http.ResponseWriter, r *http.Request) {
	e, ok := a.loadEvent(w, r)
	if !ok {
		return
	}
	var req struct {
		Games []struct {
			HomeTeam string    `json:"homeTeam"`
			AwayTeam string    `json:"awayTeam"`
			Kickoff  time.Time `json:"kickoff"`
		} `json:"games"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if len(req.Games) != e.GameCount {
		writeError(w, http.StatusBadRequest, "number of games does not match the event")
		return
	}
	games := make([]Game, len(req.Games))
	for i, g := range req.Games {
		home, away := strings.TrimSpace(g.HomeTeam), strings.TrimSpace(g.AwayTeam)
		if home == "" || away == "" || len(home) > maxTeamNameLen || len(away) > maxTeamNameLen {
			writeError(w, http.StatusBadRequest, "every game needs a home and away team (max 60 characters)")
			return
		}
		if strings.EqualFold(home, away) {
			writeError(w, http.StatusBadRequest, "a team cannot play itself")
			return
		}
		kickoff := g.Kickoff.UTC()
		if kickoff.IsZero() {
			kickoff = e.StartTime
		}
		if kickoff.Before(e.StartTime) || kickoff.After(e.EndTime) {
			writeError(w, http.StatusBadRequest, "every kickoff must be between the event start (entry deadline) and end time")
			return
		}
		games[i] = Game{HomeTeam: home, AwayTeam: away, Kickoff: kickoff}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	res, err := a.events.UpdateOne(ctx,
		bson.M{"_id": e.ID, "games": bson.M{"$size": 0}, "startTime": bson.M{"$gt": time.Now()}},
		bson.M{"$set": bson.M{"games": games}},
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save games")
		return
	}
	if res.MatchedCount == 0 {
		writeError(w, http.StatusConflict, "games can only be set once, before the event starts")
		return
	}
	a.handleGetEvent(w, r)
}

// Results can be set once, after the event has started. A result of "C" marks a cancelled
// game; its outcome is drawn from a public blockhash, weighted by how the coupons picked it.
// Saving results also settles the event and passes unwon prize money on as rollover.
func (a *App) handleSetResults(w http.ResponseWriter, r *http.Request) {
	e, ok := a.loadEvent(w, r)
	if !ok {
		return
	}
	var req struct {
		Results []string `json:"results"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if e.Status != "closed" {
		writeError(w, http.StatusConflict, "results can only be set after the event has started and before results exist")
		return
	}
	if len(req.Results) != len(e.Games) {
		writeError(w, http.StatusBadRequest, "a result is required for every game")
		return
	}
	now := time.Now()
	for i, res := range req.Results {
		if res == cancelledResult {
			if drawAt := e.Games[i].Kickoff.Add(cancelledDrawDelay); now.Before(drawAt) {
				writeError(w, http.StatusConflict, fmt.Sprintf("game %d can be drawn from %s (1 hour after kickoff)", i+1, drawAt.Format(time.RFC3339)))
				return
			}
			continue
		}
		if len(res) != 1 || !validOutcomes[res[0]] {
			writeError(w, http.StatusBadRequest, "results must be 1, X, 2 or C (cancelled)")
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	entries, err := a.loadEventEntries(ctx, e.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load entries")
		return
	}
	var sales uint64
	weights := make([][3]uint64, len(e.Games))
	for _, en := range entries {
		sales += en.Lamports
		en.addPickWeights(weights)
	}

	for i, res := range req.Results {
		if res != cancelledResult {
			e.Games[i].Result = res
			continue
		}
		block, err := a.findDrawBlock(ctx, e.Games[i].Kickoff.Add(cancelledDrawDelay))
		if err != nil {
			writeError(w, http.StatusBadGateway, fmt.Sprintf("could not draw game %d: %v", i+1, err))
			return
		}
		e.Games[i].Cancelled = true
		e.Games[i].Result = drawOutcome(e.ID, i, block.Blockhash, weights[i])
		e.Games[i].Draw = &Draw{Slot: block.Slot, Blockhash: block.Blockhash, BlockTime: block.Time, Weights: weights[i]}
	}

	settlement := settleEntries(entries, e.results(), sales, e.RolloverInLamports, e.Tiers)
	if settlement.RolloverOutLamports > 0 {
		target, err := a.rolloverTarget(ctx, e.ID, now)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to find the rollover event")
			return
		}
		settlement.RolloverToEventID = target
	}

	res, err := a.events.UpdateOne(ctx,
		bson.M{"_id": e.ID, "resultsSetAt": bson.M{"$exists": false}, "startTime": bson.M{"$lte": now}},
		bson.M{"$set": bson.M{"games": e.Games, "resultsSetAt": now, "settlement": settlement}},
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save results")
		return
	}
	if res.MatchedCount == 0 {
		writeError(w, http.StatusConflict, "results have already been set")
		return
	}
	if err := a.applyRollover(ctx, settlement.RolloverToEventID, settlement.RolloverOutLamports); err != nil {
		log.Printf("event %s: failed to apply rollover of %d lamports: %v", e.ID.Hex(), settlement.RolloverOutLamports, err)
	}
	a.handleGetEvent(w, r)
}

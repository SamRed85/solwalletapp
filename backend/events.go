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
	FixtureID int       `json:"fixtureId,omitempty" bson:"fixtureId,omitempty"`
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
	Chain               *ChainVault   `json:"chain,omitempty" bson:"chain,omitempty"`

	// hasVault stays true when prepare hides a vault from another program deployment:
	// such an event still never takes treasury money.
	hasVault bool
}

// ChainVault is the on-chain event created by the admin wallet. The program holds the funds.
// Status mirrors the on-chain lifecycle (open, tallying, resolved, cancelled) for display.
type ChainVault struct {
	ProgramID string `json:"programId" bson:"programId"`
	Admin     string `json:"admin" bson:"admin"`
	Cosigner  string `json:"cosigner" bson:"cosigner"`
	Nonce     uint64 `json:"nonce" bson:"nonce"`
	Address   string `json:"address" bson:"address"`
	Status    string `json:"status,omitempty" bson:"status,omitempty"`
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
	e.hasVault = e.Chain != nil
	if e.Chain != nil && e.Chain.ProgramID != a.cfg.ProgramID {
		e.Chain = nil
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
	case e.Chain != nil && e.Chain.Status == "cancelled":
		return "cancelled"
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

// handleDeleteEvent removes an event that has not been published on chain.
// Coupons and results make the event a record of money already taken, so those stay.
// Rollover that was waiting on the event is handed to the next treasury event.
func (a *App) handleDeleteEvent(w http.ResponseWriter, r *http.Request) {
	e, ok := a.loadEvent(w, r)
	if !ok {
		return
	}
	if e.hasVault {
		writeError(w, http.StatusConflict, "this event is on chain and cannot be deleted")
		return
	}
	if e.ResultsSetAt != nil {
		writeError(w, http.StatusConflict, "results are already set, so this event cannot be deleted")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	activity, err := a.eventActivity(ctx, e.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check coupons")
		return
	}
	if activity > 0 {
		writeError(w, http.StatusConflict, "this event already has coupons or payments, so it cannot be deleted")
		return
	}

	res, err := a.events.DeleteOne(ctx, bson.M{
		"_id":          e.ID,
		"chain":        bson.M{"$exists": false},
		"resultsSetAt": bson.M{"$exists": false},
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete event")
		return
	}
	if res.DeletedCount == 0 {
		writeError(w, http.StatusConflict, "this event changed and can no longer be deleted")
		return
	}

	activity, err = a.eventActivity(ctx, e.ID)
	if err != nil || activity > 0 {
		if _, insErr := a.events.InsertOne(ctx, e); insErr != nil {
			log.Printf("event %s was deleted during a coupon race and could not be restored: %v", e.ID.Hex(), insErr)
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to check coupons")
		} else {
			writeError(w, http.StatusConflict, "this event already has coupons or payments, so it cannot be deleted")
		}
		return
	}

	if e.RolloverInLamports > 0 {
		target, targetErr := a.rolloverTarget(ctx, e.ID, time.Now())
		if targetErr != nil || a.applyRolloverFrom(ctx, e.ID, target, e.RolloverInLamports) != nil {
			if _, insErr := a.events.InsertOne(ctx, e); insErr != nil {
				log.Printf("event %s was deleted but %d lamports of rollover could not be returned or restored: %v", e.ID.Hex(), e.RolloverInLamports, insErr)
			}
			writeError(w, http.StatusInternalServerError, "failed to return rollover from this event")
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) eventActivity(ctx context.Context, eventID bson.ObjectID) (int64, error) {
	payments, err := a.payments.CountDocuments(ctx, bson.M{"eventId": eventID})
	if err != nil {
		return 0, err
	}
	entries, err := a.entries.CountDocuments(ctx, bson.M{"eventId": eventID})
	return payments + entries, err
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
			HomeTeam  string    `json:"homeTeam"`
			AwayTeam  string    `json:"awayTeam"`
			Kickoff   time.Time `json:"kickoff"`
			FixtureID int       `json:"fixtureId"`
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
		if g.FixtureID < 0 {
			writeError(w, http.StatusBadRequest, "a fixture id must be a positive number")
			return
		}
		games[i] = Game{HomeTeam: home, AwayTeam: away, Kickoff: kickoff, FixtureID: g.FixtureID}
	}
	seenFixture := map[int]bool{}
	for _, g := range games {
		if g.FixtureID == 0 {
			continue
		}
		if seenFixture[g.FixtureID] {
			writeError(w, http.StatusBadRequest, "two games cannot use the same fixture")
			return
		}
		seenFixture[g.FixtureID] = true
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

func solanaPubkey(s string) bool {
	raw, err := base58Decode(s)
	return err == nil && len(raw) == 32
}

func validateChainVault(vault ChainVault, games int, start, now time.Time) string {
	if games == 0 {
		return "set the games before opening the on-chain vault"
	}
	if !start.After(now) {
		return "the on-chain vault can only be opened before entries close"
	}
	if !solanaPubkey(vault.Admin) || !solanaPubkey(vault.Cosigner) || vault.Admin == vault.Cosigner {
		return "the event needs two different Solana result signers"
	}
	if !solanaPubkey(vault.Address) {
		return "the on-chain event address is missing"
	}
	return ""
}

// handleSetChain attaches an on-chain vault to an event after reading it from the program.
// From then on the event takes money only through the vault.
func (a *App) handleSetChain(w http.ResponseWriter, r *http.Request) {
	e, ok := a.loadEvent(w, r)
	if !ok {
		return
	}
	var req struct {
		ChainVault
		Signers string `json:"signers"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	vault := ChainVault{
		ProgramID: a.cfg.ProgramID,
		Admin:     req.Admin,
		Cosigner:  req.Cosigner,
		Nonce:     req.Nonce,
		Address:   req.Address,
		Status:    "open",
	}
	if msg := validateChainVault(vault, len(e.Games), e.StartTime, time.Now()); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	// An admin-chosen cosigner could be a second wallet of the same person, which would turn
	// the two-signer approval into one.
	if vault.Cosigner != a.cfg.ResultCosigner {
		writeError(w, http.StatusBadRequest, "the second result signer must be the configured cosigner "+a.cfg.ResultCosigner)
		return
	}
	if vault.Admin != walletFrom(r) {
		writeError(w, http.StatusForbidden, "publish the vault from the admin wallet you are signed in with")
		return
	}
	if !solanaPubkey(req.Signers) {
		writeError(w, http.StatusBadRequest, "the on-chain result signer address is missing")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	eventAccount, err := a.fetchAccount(ctx, vault.Address)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	signersAccount, err := a.fetchAccount(ctx, req.Signers)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	onChain, err := decodeChainEvent(eventAccount, a.cfg.ProgramID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	signers, err := decodeChainSigners(signersAccount, a.cfg.ProgramID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if msg := checkVaultMatches(e, vault, onChain, signers); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	// Mark the event first so a treasury entry that starts now sees the vault, then make
	// sure no treasury payment arrived before. Either order of the two requests ends with
	// one money path.
	res, err := a.events.UpdateOne(ctx,
		bson.M{"_id": e.ID, "startTime": bson.M{"$gt": time.Now()}, "$or": bson.A{
			bson.M{"chain": bson.M{"$exists": false}},
			bson.M{"chain.programId": bson.M{"$ne": a.cfg.ProgramID}},
		}},
		bson.M{"$set": bson.M{"chain": vault}},
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save the on-chain vault")
		return
	}
	if res.MatchedCount == 0 {
		writeError(w, http.StatusConflict, "this event already has an on-chain vault or entries have closed")
		return
	}
	payments, err := a.payments.CountDocuments(ctx, bson.M{"eventId": e.ID})
	if err == nil && payments == 0 {
		var entries int64
		entries, err = a.entries.CountDocuments(ctx, bson.M{"eventId": e.ID})
		payments += entries
	}
	if err != nil || payments > 0 {
		_, _ = a.events.UpdateOne(ctx, bson.M{"_id": e.ID, "chain.address": vault.Address}, bson.M{"$unset": bson.M{"chain": ""}})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to check treasury payments")
		} else {
			writeError(w, http.StatusConflict, "this event already has treasury payments, so it must stay a treasury event")
		}
		return
	}
	// Treasury rollover cannot reach an on-chain vault; hand it to the next treasury event.
	var before Event
	if err := a.events.FindOneAndUpdate(ctx,
		bson.M{"_id": e.ID, "rolloverInLamports": bson.M{"$gt": 0}},
		bson.M{"$set": bson.M{"rolloverInLamports": 0}},
	).Decode(&before); err == nil {
		if err := a.applyRollover(ctx, nil, before.RolloverInLamports); err != nil {
			log.Printf("event %s: failed to return %d lamports of treasury rollover: %v", e.ID.Hex(), before.RolloverInLamports, err)
		}
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
	if e.hasVault {
		writeError(w, http.StatusConflict, "this event is settled by its on-chain vault; approve the results there")
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
		settlement.RolloverApplied = false
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
	if settlement.RolloverOutLamports > 0 {
		if err := a.commitRollover(ctx, e.ID, settlement); err != nil {
			log.Printf("event %s: failed to apply rollover of %d lamports: %v", e.ID.Hex(), settlement.RolloverOutLamports, err)
		}
	}
	a.handleGetEvent(w, r)
}

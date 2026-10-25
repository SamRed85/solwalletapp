package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

const footballBase = "https://v3.football.api-sports.io"

// A projected result is the score as it stands. It is never written as the event result.
type gameProjection struct {
	Linked    bool   `json:"linked"`
	Status    string `json:"status,omitempty"`
	Elapsed   *int   `json:"elapsed,omitempty"`
	Extra     *int   `json:"extra,omitempty"`
	HomeGoals *int   `json:"homeGoals,omitempty"`
	AwayGoals *int   `json:"awayGoals,omitempty"`
	Projected string `json:"projected"`
	Final     bool   `json:"final"`
}

type projectionView struct {
	Games              []gameProjection `json:"games"`
	Complete           bool             `json:"complete"`
	RevealClosed       bool             `json:"revealClosed"`
	Maximum            bool             `json:"maximum"`
	SalesLamports      uint64           `json:"salesLamports"`
	RolloverInLamports uint64           `json:"rolloverInLamports"`
	Tiers              []TierPayout     `json:"tiers"`
}

type fixtureState struct {
	Status   string
	Elapsed  *int
	Extra    *int
	Home     *int
	Away     *int
	FullHome *int
	FullAway *int
}

type footballHit struct {
	ID      int       `json:"id"`
	Home    string    `json:"home"`
	Away    string    `json:"away"`
	Kickoff time.Time `json:"kickoff"`
	League  string    `json:"league"`
	Status  string    `json:"status"`
}

type footballFeed struct {
	key        string
	client     *http.Client
	mu         sync.Mutex
	scores     map[int]fixtureState
	search     map[string]cachedSearch
	days       map[string]cachedDay
	lastCall   time.Time
	pauseUntil time.Time
	refreshed  map[string]bool
	forced     map[string]time.Time
	counts     map[string]cachedCounts
}

type cachedSearch struct {
	at   time.Time
	hits []footballHit
}

type cachedDay struct {
	at   time.Time
	rows []apiFixture
}

type cachedCounts struct {
	at      time.Time
	winners map[int]int
}

func newFootballFeed(key string) *footballFeed {
	return &footballFeed{
		key:       key,
		client:    &http.Client{Timeout: 15 * time.Second},
		scores:    map[int]fixtureState{},
		search:    map[string]cachedSearch{},
		days:      map[string]cachedDay{},
		refreshed: map[string]bool{},
		forced:    map[string]time.Time{},
		counts:    map[string]cachedCounts{},
	}
}

const (
	scoreRefreshHalf   = 45 * time.Minute
	scoreRefreshFull   = 90 * time.Minute
	scoreRefreshWindow = 2 * time.Minute
)

// dueScoreMarks returns the scheduled refreshes that are due now. Each event is
// fetched once at 45 minutes after it starts and once at 90 minutes, and not otherwise.
func dueScoreMarks(start, now time.Time, done map[string]bool, eventID string) []string {
	var marks []string
	elapsed := now.Sub(start)
	for _, mark := range []struct {
		name string
		at   time.Duration
	}{{"45", scoreRefreshHalf}, {"90", scoreRefreshFull}} {
		if elapsed < mark.at || elapsed >= mark.at+scoreRefreshWindow || done[eventID+":"+mark.name] {
			continue
		}
		marks = append(marks, mark.name)
	}
	return marks
}

// projectFixture maps a fixture to 1, X or 2. Penalties and extra time do not
// change a draw after regular time. A match that has not started counts as X.
func projectFixture(state fixtureState) (outcome string, home, away *int, final bool) {
	switch state.Status {
	case "NS", "TBD":
		return "X", nil, nil, false
	case "1H", "HT", "2H", "LIVE", "SUSP", "INT":
		if state.Home == nil || state.Away == nil {
			return "", nil, nil, false
		}
		return scoreOutcome(*state.Home, *state.Away), state.Home, state.Away, false
	case "FT":
		h, a := state.FullHome, state.FullAway
		if h == nil || a == nil {
			h, a = state.Home, state.Away
		}
		if h == nil || a == nil {
			return "", nil, nil, false
		}
		return scoreOutcome(*h, *a), h, a, true
	case "ET", "BT", "P", "AET", "PEN":
		if state.FullHome == nil || state.FullAway == nil {
			return "", nil, nil, false
		}
		return scoreOutcome(*state.FullHome, *state.FullAway), state.FullHome, state.FullAway, state.Status == "AET" || state.Status == "PEN"
	default:
		return "", nil, nil, false
	}
}

func scoreOutcome(home, away int) string {
	switch {
	case home > away:
		return "1"
	case home < away:
		return "2"
	default:
		return "X"
	}
}

func buildProjection(games []Game, scores map[int]fixtureState, now, end time.Time, sales, rollover uint64, tiers []Tier, winners map[int]int) projectionView {
	view := projectionView{
		Games:              make([]gameProjection, len(games)),
		RevealClosed:       !now.Before(end),
		SalesLamports:      sales,
		RolloverInLamports: rollover,
	}
	results := make([]byte, len(games))
	complete := len(games) > 0
	for i, game := range games {
		row := gameProjection{Linked: game.FixtureID > 0}
		if game.FixtureID > 0 {
			if state, ok := scores[game.FixtureID]; ok {
				outcome, home, away, final := projectFixture(state)
				row.Status = state.Status
				row.Elapsed = state.Elapsed
				row.Extra = state.Extra
				row.HomeGoals = home
				row.AwayGoals = away
				row.Projected = outcome
				row.Final = final
			}
		}
		if row.Projected == "" {
			complete = false
		} else {
			results[i] = row.Projected[0]
		}
		view.Games[i] = row
	}
	view.Complete = complete
	if !complete {
		return view
	}
	view.Maximum = !view.RevealClosed
	if view.Maximum {
		winners = map[int]int{}
		for _, tier := range tiers {
			winners[tier.Correct] = 1
		}
	}
	view.Tiers = settleCounts(0, winners, len(games), sales, rollover, tiers).Tiers
	return view
}

func addMaskCounts(winners map[int]int, picks []byte, results string) {
	if len(picks) < len(results) || results == "" {
		return
	}
	counts := []int{1}
	for game := 0; game < len(results); game++ {
		var bits byte
		switch results[game] {
		case '1':
			bits = 0b001
		case 'X':
			bits = 0b010
		case '2':
			bits = 0b100
		case 'C':
			bits = 0b111
		default:
			return
		}
		mask := picks[game] & 0b111
		choices := bitsSet(mask)
		correct := bitsSet(mask & bits)
		next := make([]int, len(counts)+1)
		for score, rows := range counts {
			next[score] += rows * (choices - correct)
			next[score+1] += rows * correct
		}
		counts = next
	}
	for score, rows := range counts {
		if rows > 0 {
			winners[score] += rows
		}
	}
}

func bitsSet(mask byte) int {
	return int(mask&1) + int((mask>>1)&1) + int((mask>>2)&1)
}

func (a *App) footballLoop() {
	if a.feed == nil || a.feed.key == "" {
		log.Print("football feed is not configured")
		return
	}
	a.refreshFixtures()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		a.refreshFixtures()
	}
}

func (a *App) refreshFixtures() {
	if a.feed == nil || a.feed.key == "" || a.events == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cur, err := a.events.Find(ctx, bson.M{
		"resultsSetAt":    bson.M{"$exists": false},
		"games.fixtureId": bson.M{"$gt": 0},
	})
	if err != nil {
		log.Printf("football feed: %v", err)
		return
	}
	defer cur.Close(ctx)
	var events []Event
	if err = cur.All(ctx, &events); err != nil {
		log.Printf("football feed: %v", err)
		return
	}
	now := time.Now()
	a.feed.mu.Lock()
	done := a.feed.refreshed
	a.feed.mu.Unlock()
	var games []Game
	var keys []string
	seen := map[int]bool{}
	for _, event := range events {
		marks := dueScoreMarks(event.StartTime, now, done, event.ID.Hex())
		if len(marks) == 0 {
			continue
		}
		pending := false
		for _, game := range event.Games {
			if game.FixtureID <= 0 || seen[game.FixtureID] {
				continue
			}
			if state, ok := a.feed.score(game.FixtureID); ok {
				if _, _, _, final := projectFixture(state); final {
					continue
				}
			}
			seen[game.FixtureID] = true
			games = append(games, game)
			pending = true
		}
		for _, mark := range marks {
			keys = append(keys, event.ID.Hex()+":"+mark)
		}
		if !pending {
			a.feed.markRefreshed(keys[len(keys)-len(marks):])
			keys = keys[:len(keys)-len(marks)]
		}
	}
	if len(games) == 0 {
		return
	}
	if err = a.feed.loadFixtures(ctx, games); err != nil {
		log.Printf("football feed: %v", err)
		return
	}
	a.feed.markRefreshed(keys)
}

func (f *footballFeed) markRefreshed(keys []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, key := range keys {
		f.refreshed[key] = true
	}
}

func (f *footballFeed) score(id int) (fixtureState, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	state, ok := f.scores[id]
	return state, ok
}

// The free plan rejects fixtures?ids. A day's list includes the live score, and one fixture id is the fallback.
func (f *footballFeed) loadFixtures(ctx context.Context, games []Game) error {
	dates := map[string]struct{}{}
	want := map[int]struct{}{}
	for _, game := range games {
		if game.FixtureID <= 0 {
			continue
		}
		want[game.FixtureID] = struct{}{}
		if !game.Kickoff.IsZero() {
			dates[game.Kickoff.UTC().Format("2006-01-02")] = struct{}{}
		}
	}
	found := map[int]fixtureState{}
	for date := range dates {
		rows, err := f.rowsOnDate(ctx, date, "")
		if err != nil {
			return err
		}
		for _, row := range rows {
			if _, ok := want[row.Fixture.ID]; ok {
				found[row.Fixture.ID] = row.state()
			}
		}
	}
	for id := range want {
		if _, ok := found[id]; ok {
			continue
		}
		state, err := f.fixtureByID(ctx, id)
		if err != nil {
			return err
		}
		if state.Status != "" {
			found[id] = state
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for id, state := range found {
		f.scores[id] = state
	}
	return nil
}

func (f *footballFeed) fixtureByID(ctx context.Context, id int) (fixtureState, error) {
	body, _, err := f.get(ctx, "/fixtures?id="+strconv.Itoa(id))
	if err != nil {
		return fixtureState{}, err
	}
	var rows []apiFixture
	if err = json.Unmarshal(body, &rows); err != nil {
		return fixtureState{}, err
	}
	if len(rows) == 0 {
		return fixtureState{}, nil
	}
	return rows[0].state(), nil
}

func (a *App) handleFixtureSearch(w http.ResponseWriter, r *http.Request) {
	if a.feed == nil || a.feed.key == "" {
		writeError(w, http.StatusServiceUnavailable, "the score feed is not configured")
		return
	}
	team := strings.TrimSpace(r.URL.Query().Get("team"))
	away := strings.TrimSpace(r.URL.Query().Get("away"))
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	if len(team) < 3 || len(date) != 10 {
		writeError(w, http.StatusBadRequest, "a team and a kickoff date are required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	hits, message, err := a.feed.searchFixtures(ctx, team, away, date, cleanZone(r.URL.Query().Get("timezone")))
	if err != nil {
		log.Printf("football search: %v", err)
		writeError(w, http.StatusBadGateway, feedMessage(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"fixtures": hits, "message": message})
}

func (f *footballFeed) searchFixtures(ctx context.Context, team, away, date, timezone string) ([]footballHit, string, error) {
	day, err := f.fixturesOnDate(ctx, date, timezone)
	if err != nil {
		return nil, "", err
	}
	hits := rankFixtures(day, team, away)
	message := searchMessage(team, away, date, hits)
	if len(hits) > 12 {
		message = fmt.Sprintf("Showing 12 of %d matches on %s. Pick the right one.", len(hits), date)
		hits = hits[:12]
	}
	return hits, message, nil
}

func (f *footballFeed) fixturesOnDate(ctx context.Context, date, timezone string) ([]footballHit, error) {
	key := date + "|" + timezone
	f.mu.Lock()
	if cached, ok := f.search[key]; ok && time.Since(cached.at) < 10*time.Minute {
		f.mu.Unlock()
		return cached.hits, nil
	}
	f.mu.Unlock()

	rows, err := f.rowsOnDate(ctx, date, timezone)
	if err != nil {
		return nil, err
	}
	hits := make([]footballHit, 0, len(rows))
	for _, row := range rows {
		kickoff, _ := time.Parse(time.RFC3339, row.Fixture.Date)
		hits = append(hits, footballHit{
			ID: row.Fixture.ID, Home: row.Teams.Home.Name, Away: row.Teams.Away.Name,
			Kickoff: kickoff, League: row.League.Name, Status: row.Fixture.Status.Short,
		})
	}
	f.mu.Lock()
	f.search[key] = cachedSearch{at: time.Now(), hits: hits}
	f.mu.Unlock()
	return hits, nil
}

func (f *footballFeed) rowsOnDate(ctx context.Context, date, timezone string) ([]apiFixture, error) {
	key := date + "|" + timezone
	f.mu.Lock()
	if cached, ok := f.days[key]; ok && time.Since(cached.at) < 10*time.Minute {
		f.mu.Unlock()
		return cached.rows, nil
	}
	f.mu.Unlock()

	path := "/fixtures?date=" + date
	if timezone != "" {
		path += "&timezone=" + url.QueryEscape(timezone)
	}
	body, _, err := f.get(ctx, path)
	if err != nil {
		return nil, err
	}
	var rows []apiFixture
	if err = json.Unmarshal(body, &rows); err != nil {
		return nil, err
	}
	f.mu.Lock()
	f.days[key] = cachedDay{at: time.Now(), rows: rows}
	f.mu.Unlock()
	return rows, nil
}

func rankFixtures(rows []footballHit, team, away string) []footballHit {
	team = strings.ToLower(strings.TrimSpace(team))
	away = strings.ToLower(strings.TrimSpace(away))
	type scored struct {
		hit  footballHit
		rank int
	}
	matched := make([]scored, 0)
	for _, hit := range rows {
		names := strings.ToLower(hit.Home + " " + hit.Away)
		if !strings.Contains(names, team) {
			continue
		}
		rank := 1
		if away != "" && strings.Contains(names, away) {
			rank = 0
		}
		matched = append(matched, scored{hit, rank})
	}
	sort.SliceStable(matched, func(i, j int) bool { return matched[i].rank < matched[j].rank })
	out := make([]footballHit, len(matched))
	for i, item := range matched {
		out[i] = item.hit
	}
	return out
}

func searchMessage(team, away, date string, hits []footballHit) string {
	if len(hits) == 0 {
		return fmt.Sprintf("No match on %s includes %s. Check the spelling and the kickoff date.", date, team)
	}
	if away != "" {
		needle := strings.ToLower(away)
		both := false
		for _, hit := range hits {
			if strings.Contains(strings.ToLower(hit.Home+" "+hit.Away), needle) {
				both = true
				break
			}
		}
		if !both {
			return fmt.Sprintf("No match on %s includes both names. These matches include %s.", date, team)
		}
	}
	noun := "matches"
	if len(hits) == 1 {
		noun = "match"
	}
	return fmt.Sprintf("%d %s on %s. Pick the right one.", len(hits), noun, date)
}

func cleanZone(zone string) string {
	zone = strings.TrimSpace(zone)
	if zone == "" || len(zone) > 64 {
		return ""
	}
	for _, r := range zone {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '/' && r != '_' && r != '-' && r != '+' {
			return ""
		}
	}
	return zone
}

func footballError(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "[]" || string(raw) == "{}" || string(raw) == "null" {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil && text != "" {
		return text
	}
	var list []string
	if json.Unmarshal(raw, &list) == nil && len(list) > 0 {
		return strings.Join(list, "; ")
	}
	var obj map[string]string
	if json.Unmarshal(raw, &obj) == nil && len(obj) > 0 {
		parts := make([]string, 0, len(obj))
		for _, value := range obj {
			if value != "" {
				parts = append(parts, value)
			}
		}
		sort.Strings(parts)
		if len(parts) > 0 {
			return strings.Join(parts, "; ")
		}
	}
	return "the score feed rejected the request"
}

func feedMessage(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "quota"):
		return "The score feed has reached its request limit. Try again later."
	case strings.Contains(msg, "unreachable"):
		return "The score feed could not be reached. Try again in a moment."
	default:
		if len(msg) > 180 {
			msg = msg[:180]
		}
		return msg
	}
}

func (a *App) handleRefreshScores(w http.ResponseWriter, r *http.Request) {
	if a.feed == nil || a.feed.key == "" {
		writeError(w, http.StatusServiceUnavailable, "the score feed is not configured")
		return
	}
	e, ok := a.loadEvent(w, r)
	if !ok {
		return
	}
	var ids []int
	for _, game := range e.Games {
		if game.FixtureID > 0 {
			ids = append(ids, game.FixtureID)
		}
	}
	if len(ids) == 0 {
		writeError(w, http.StatusBadRequest, "no match is linked to a live score")
		return
	}
	a.feed.mu.Lock()
	if time.Since(a.feed.forced[e.ID.Hex()]) < 15*time.Second {
		a.feed.mu.Unlock()
		writeError(w, http.StatusTooManyRequests, "scores were just refreshed")
		return
	}
	a.feed.forced[e.ID.Hex()] = time.Now()
	a.feed.mu.Unlock()
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	if err := a.feed.loadFixtures(ctx, e.Games); err != nil {
		log.Printf("football refresh: %v", err)
		writeError(w, http.StatusBadGateway, feedMessage(err))
		return
	}
	a.handleProjection(w, r)
}

func (a *App) handleProjection(w http.ResponseWriter, r *http.Request) {
	e, ok := a.loadEvent(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	sales, rollover := a.projectionPool(ctx, e)
	scores := map[int]fixtureState{}
	if a.feed != nil {
		a.feed.mu.Lock()
		for _, game := range e.Games {
			if state, ok := a.feed.scores[game.FixtureID]; ok {
				scores[game.FixtureID] = state
			}
		}
		a.feed.mu.Unlock()
	}
	now := time.Now()
	view := buildProjection(e.Games, scores, now, e.EndTime, sales, rollover, e.Tiers, nil)
	if view.RevealClosed && view.Complete {
		view = buildProjection(e.Games, scores, now, e.EndTime, sales, rollover, e.Tiers, a.projectionWinners(ctx, e))
	}
	writeJSON(w, http.StatusOK, view)
}

func (a *App) projectionPool(ctx context.Context, e *Event) (sales, rollover uint64) {
	rollover = e.RolloverInLamports
	if e.Chain != nil && e.Chain.Address != "" && a.fetchAccount != nil {
		account, err := a.fetchAccount(ctx, e.Chain.Address)
		if err == nil {
			if decoded, err := decodeChainEvent(account, a.cfg.ProgramID); err == nil {
				return decoded.Sales, decoded.RolloverIn
			}
		}
	}
	if a.payments == nil {
		return 0, rollover
	}
	cur, err := a.payments.Find(ctx, bson.M{"eventId": e.ID, "status": paymentAccepted})
	if err != nil {
		return 0, rollover
	}
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		var payment PaymentRecord
		if cur.Decode(&payment) == nil {
			sales += payment.Lamports
		}
	}
	return sales, rollover
}

func (a *App) projectionWinners(ctx context.Context, e *Event) map[int]int {
	if a.feed != nil {
		a.feed.mu.Lock()
		if cached, ok := a.feed.counts[e.ID.Hex()]; ok && time.Since(cached.at) < time.Minute {
			a.feed.mu.Unlock()
			return cached.winners
		}
		a.feed.mu.Unlock()
	}
	winners := map[int]int{}
	results := projectedResults(e.Games, a.feed)
	if e.Chain != nil && e.Chain.Address != "" {
		a.addSlipCounts(ctx, e.Chain.Address, results, winners)
		if len(winners) == 0 {
			a.addChainCouponCounts(ctx, e.Chain.Address, len(e.Games), results, winners)
		}
	} else {
		a.addEntryCounts(ctx, e.ID, results, winners)
	}
	if a.feed != nil {
		a.feed.mu.Lock()
		a.feed.counts[e.ID.Hex()] = cachedCounts{at: time.Now(), winners: winners}
		a.feed.mu.Unlock()
	}
	return winners
}

func projectedResults(games []Game, feed *footballFeed) string {
	out := make([]byte, len(games))
	if feed == nil {
		return string(out)
	}
	feed.mu.Lock()
	defer feed.mu.Unlock()
	for i, game := range games {
		outcome, _, _, _ := projectFixture(feed.scores[game.FixtureID])
		if outcome != "" {
			out[i] = outcome[0]
		}
	}
	return string(out)
}

func (a *App) addSlipCounts(ctx context.Context, event, results string, winners map[int]int) {
	if a.slips == nil || strings.Contains(results, "\x00") {
		return
	}
	cur, err := a.slips.Find(ctx, bson.M{"event": event, "kind": "reveal"})
	if err != nil {
		return
	}
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		var record slipRecord
		if cur.Decode(&record) != nil {
			continue
		}
		picks, err := hex.DecodeString(record.Picks)
		if err != nil {
			continue
		}
		addMaskCounts(winners, picks, results)
	}
}

func (a *App) addChainCouponCounts(ctx context.Context, event string, gameCount int, results string, winners map[int]int) {
	if a.rpc == nil || strings.Contains(results, "\x00") {
		return
	}
	want, err := base58Decode(event)
	if err != nil || len(want) != 32 {
		return
	}
	rows, err := a.rpc.programAccounts(ctx, a.cfg.ProgramID, couponAccountSize)
	if err != nil {
		return
	}
	for _, row := range rows {
		if len(row.Data) < 123 || string(row.Data[8:40]) != string(want) || row.Data[122] != 1 {
			continue
		}
		addMaskCounts(winners, row.Data[77:77+gameCount], results)
	}
}

func (a *App) addEntryCounts(ctx context.Context, eventID bson.ObjectID, results string, winners map[int]int) {
	if a.entries == nil || strings.Contains(results, "\x00") {
		return
	}
	entries, err := a.loadEventEntries(ctx, eventID)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if len(entry.System) == 0 {
			for _, coupon := range entry.Coupons {
				winners[countCorrect(coupon, results)]++
			}
			continue
		}
		picks := make([]byte, len(results))
		for i := 0; i < len(results) && i < len(entry.System); i++ {
			for _, outcome := range entry.System[i] {
				switch outcome {
				case '1':
					picks[i] |= 0b001
				case 'X':
					picks[i] |= 0b010
				case '2':
					picks[i] |= 0b100
				}
			}
		}
		addMaskCounts(winners, picks, results)
	}
}

func (f *footballFeed) get(ctx context.Context, path string) (json.RawMessage, int, error) {
	f.mu.Lock()
	if time.Now().Before(f.pauseUntil) {
		f.mu.Unlock()
		return nil, 0, errors.New("football feed quota is spent for today")
	}
	if !f.lastCall.IsZero() && time.Since(f.lastCall) < 2*time.Second {
		wait := 2*time.Second - time.Since(f.lastCall)
		f.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, 0, ctx.Err()
		case <-time.After(wait):
		}
		f.mu.Lock()
	}
	f.lastCall = time.Now()
	f.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, footballBase+path, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("x-apisports-key", f.key)
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, 0, errors.New("football feed unreachable")
	}
	defer resp.Body.Close()
	if remaining := resp.Header.Get("x-ratelimit-requests-remaining"); remaining == "0" {
		f.mu.Lock()
		f.pauseUntil = time.Now().UTC().Truncate(24 * time.Hour).Add(24 * time.Hour)
		f.mu.Unlock()
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, 0, errors.New("football feed quota reached")
	}
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, 0, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, fmt.Errorf("football feed status %d", resp.StatusCode)
	}
	var envelope struct {
		Response json.RawMessage `json:"response"`
		Errors   json.RawMessage `json:"errors"`
	}
	if err = json.Unmarshal(payload, &envelope); err != nil {
		return nil, resp.StatusCode, err
	}
	if reason := footballError(envelope.Errors); reason != "" {
		return nil, resp.StatusCode, errors.New(reason)
	}
	return envelope.Response, resp.StatusCode, nil
}

type apiFixture struct {
	Fixture struct {
		ID     int    `json:"id"`
		Date   string `json:"date"`
		Status struct {
			Short   string `json:"short"`
			Elapsed *int   `json:"elapsed"`
			Extra   *int   `json:"extra"`
		} `json:"status"`
	} `json:"fixture"`
	League struct {
		Name string `json:"name"`
	} `json:"league"`
	Teams struct {
		Home struct {
			Name string `json:"name"`
		} `json:"home"`
		Away struct {
			Name string `json:"name"`
		} `json:"away"`
	} `json:"teams"`
	Goals struct {
		Home *int `json:"home"`
		Away *int `json:"away"`
	} `json:"goals"`
	Score struct {
		Fulltime struct {
			Home *int `json:"home"`
			Away *int `json:"away"`
		} `json:"fulltime"`
	} `json:"score"`
}

func (row apiFixture) state() fixtureState {
	return fixtureState{
		Status: row.Fixture.Status.Short, Elapsed: row.Fixture.Status.Elapsed, Extra: row.Fixture.Status.Extra, Home: row.Goals.Home, Away: row.Goals.Away,
		FullHome: row.Score.Fulltime.Home, FullAway: row.Score.Fulltime.Away,
	}
}

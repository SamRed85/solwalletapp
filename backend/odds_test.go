package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestImpliedPercentsRemoveTheMargin(t *testing.T) {
	// 1/1.85 + 1/3.40 + 1/4.20 is about 1.07. The shares of that sum are 50, 27 and 22.
	one, draw, two, err := impliedPercents(1.85, 3.40, 4.20)
	if err != nil || one != 50 || draw != 27 || two != 22 {
		t.Fatalf("margin odds = %d %d %d %v", one, draw, two, err)
	}
	// A fair line has no margin, so the percents are the raw shares.
	one, draw, two, err = impliedPercents(2, 4, 4)
	if err != nil || one != 50 || draw != 25 || two != 25 {
		t.Fatalf("fair odds = %d %d %d %v", one, draw, two, err)
	}
	if _, _, _, err = impliedPercents(0, 3, 3); err == nil {
		t.Fatal("a zero odd was accepted")
	}
}

func TestSystemCouponWeightsEachOutcomeByItsRows(t *testing.T) {
	weights := make([][3]uint64, 2)
	Entry{System: []string{"1X", "2"}, Combinations: 2}.addPickWeights(weights)
	if got := sharePercents(weights[0]); got != (SharePercents{One: 50, Draw: 50}) {
		t.Fatalf("game 1 shares %+v", got)
	}
	if got := sharePercents(weights[1]); got != (SharePercents{Two: 100}) {
		t.Fatalf("game 2 shares %+v", got)
	}
	system, rows := systemFromMask([]byte{0b011, 0b100})
	masked := make([][3]uint64, 2)
	Entry{System: system, Combinations: rows}.addPickWeights(masked)
	if rows != 2 || masked[0] != weights[0] || masked[1] != weights[1] {
		t.Fatalf("mask weights %+v rows %d, system %+v", masked, rows, weights)
	}
}

func TestReducedCouponCountsOnlySurvivingRows(t *testing.T) {
	// The frame 1X / 12 is four rows. The placed piece kept 1 and 2 only.
	frame := make([][3]uint64, 2)
	Entry{System: []string{"1X", "12"}, Combinations: 4}.addPickWeights(frame)
	if got := sharePercents(frame[0]); got != (SharePercents{One: 50, Draw: 50}) {
		t.Fatalf("frame game 1 %+v", got)
	}
	system, rows := systemFromMask([]byte{0b001, 0b100})
	piece := make([][3]uint64, 2)
	Entry{System: system, Combinations: rows}.addPickWeights(piece)
	if rows != 1 || system[0] != "1" || system[1] != "2" {
		t.Fatalf("piece %v rows %d", system, rows)
	}
	if got := sharePercents(piece[0]); got != (SharePercents{One: 100}) {
		t.Fatalf("surviving game 1 %+v", got)
	}
	if got := sharePercents(piece[1]); got != (SharePercents{Two: 100}) {
		t.Fatalf("surviving game 2 %+v", got)
	}
}

func TestPickShareStaysHiddenBeforeTheStart(t *testing.T) {
	start := time.Now().Add(time.Hour)
	event := &Event{StartTime: start, Games: []Game{{}, {}}}
	share := &PickShare{Revealed: 1, Coupons: 2, Games: []SharePercents{{One: 100}, {Two: 100}}}
	publishPickShare(event, start.Add(-time.Minute), share)
	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "pickShare") {
		t.Fatalf("share was returned before the start: %s", raw)
	}
	publishPickShare(event, start, share)
	raw, err = json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"pickShare"`) {
		t.Fatalf("share was missing at the start: %s", raw)
	}
}

func TestOddsRefreshAndHiddenShare(t *testing.T) {
	app := testApp(t, testDatabase(t))
	app.cfg.OddsBookmakerID = 8
	app.odds = app.db.Collection("fixtureOdds")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "fixture=42") || !strings.Contains(r.URL.RawQuery, "bookmaker=8") || !strings.Contains(r.URL.RawQuery, "bet=1") {
			t.Errorf("odds query %s", r.URL.RawQuery)
		}
		if r.Header.Get("x-apisports-key") == "" {
			t.Error("odds request had no API key header")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"response": []any{map[string]any{
				"bookmakers": []any{map[string]any{
					"id": 8, "name": "Bet365",
					"bets": []any{map[string]any{
						"id": 1, "name": "Match Winner",
						"values": []any{
							map[string]string{"value": "Home", "odd": "1.85"},
							map[string]string{"value": "Draw", "odd": "3.40"},
							map[string]string{"value": "Away", "odd": "4.20"},
						},
					}},
				}},
			}},
			"errors": []any{},
		})
	}))
	defer server.Close()
	app.feed = newFootballFeed("test-key")
	app.feed.base = server.URL
	app.feed.client = server.Client()

	start := time.Now().Add(time.Hour)
	res, err := app.events.InsertOne(t.Context(), Event{
		Title: "weekend", GameCount: 2, CouponPriceLamports: 1_000_000,
		Games:     []Game{{HomeTeam: "Arsenal", AwayTeam: "Chelsea", FixtureID: 42}, {HomeTeam: "Inter", AwayTeam: "Milan"}},
		StartTime: start, EndTime: start.Add(3 * time.Hour), CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	eventID := res.InsertedID.(bson.ObjectID)
	if _, err = app.entries.InsertOne(t.Context(), Entry{
		EventID: eventID, WalletAddress: "player", System: []string{"1X", "2"}, Combinations: 2, Signature: "sig",
	}); err != nil {
		t.Fatal(err)
	}

	admin := loginCookie(t, app, defaultWallet)
	refreshed := cookieCall(app, http.MethodPost, "/api/events/"+eventID.Hex()+"/odds/refresh", "", admin)
	if refreshed.Code != http.StatusOK {
		t.Fatalf("refresh %d %s", refreshed.Code, refreshed.Body.String())
	}
	var saved struct {
		Event   Event    `json:"event"`
		Missing []string `json:"missing"`
	}
	if err = json.Unmarshal(refreshed.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Event.PickShare != nil {
		t.Fatalf("refresh returned a share before the start: %+v", saved.Event.PickShare)
	}
	if saved.Event.Games[0].Odds == nil || saved.Event.Games[0].Odds.One != "1.85" || saved.Event.Games[0].Odds.OnePercent != 50 {
		t.Fatalf("stored odds %+v", saved.Event.Games[0].Odds)
	}
	if len(saved.Missing) != 0 || saved.Event.Games[1].Odds != nil {
		t.Fatalf("unlinked game missing=%v odds=%+v", saved.Missing, saved.Event.Games[1].Odds)
	}

	body := cookieCall(app, http.MethodGet, "/api/events/"+eventID.Hex(), "", nil).Body.String()
	if strings.Contains(body, "pickShare") {
		t.Fatalf("GET returned a share before the start: %s", body)
	}
	if !strings.Contains(body, `"one":"1.85"`) {
		t.Fatalf("GET did not return the stored odds: %s", body)
	}

	if _, err = app.events.UpdateOne(t.Context(), bson.M{"_id": eventID}, bson.M{"$set": bson.M{"startTime": time.Now().Add(-time.Minute)}}); err != nil {
		t.Fatal(err)
	}
	got := cookieCall(app, http.MethodGet, "/api/events/"+eventID.Hex(), "", nil)
	var event Event
	if err = json.Unmarshal(got.Body.Bytes(), &event); err != nil {
		t.Fatal(err)
	}
	if event.PickShare == nil || event.PickShare.Revealed != 1 || event.PickShare.Coupons != 1 {
		t.Fatalf("share after the start %+v", event.PickShare)
	}
	if event.PickShare.Games[0] != (SharePercents{One: 50, Draw: 50}) || event.PickShare.Games[1] != (SharePercents{Two: 100}) {
		t.Fatalf("shares %+v", event.PickShare.Games)
	}
}

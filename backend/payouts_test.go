package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestDefaultTiers(t *testing.T) {
	tiers := defaultTiers(13)
	want := []Tier{{13, 50}, {12, 25}, {11, 15}, {10, 5}}
	if len(tiers) != len(want) {
		t.Fatalf("got %v", tiers)
	}
	for i := range want {
		if tiers[i] != want[i] {
			t.Fatalf("got %v, want %v", tiers, want)
		}
	}
	if msg := validateTiers(tiers, 13); msg != "" {
		t.Fatalf("default tiers rejected: %s", msg)
	}
}

func TestValidateTiers(t *testing.T) {
	bad := map[string][]Tier{
		"empty":          nil,
		"wrong total":    {{7, 50}, {6, 40}},
		"no top tier":    {{6, 60}, {5, 35}},
		"duplicate":      {{7, 50}, {7, 45}},
		"too many picks": {{8, 95}},
		"zero percent":   {{7, 95}, {6, 0}},
		"negative picks": {{7, 95}, {0, 0}},
		"includes house": {{7, 100}},
	}
	for name, tiers := range bad {
		if validateTiers(tiers, 7) == "" {
			t.Errorf("%s: invalid tiers accepted: %v", name, tiers)
		}
	}
	if msg := validateTiers([]Tier{{7, 60}, {6, 35}}, 7); msg != "" {
		t.Fatalf("valid special event tiers rejected: %s", msg)
	}
}

func checkConservation(t *testing.T, s Settlement) {
	t.Helper()
	if s.HouseLamports+s.PaidLamports+s.RolloverOutLamports != s.SalesLamports+s.RolloverInLamports {
		t.Fatalf("money not conserved: %+v", s)
	}
}

func TestSettle(t *testing.T) {
	results := "1111111111111"
	coupons := []string{
		"1111111111111", // 13
		"1111111111112", // 12
		"1111111111122", // 11
		"111111111112X", // 11
		"2222222222222", // 0
	}
	price := uint64(1_000_000)
	sales := price * uint64(len(coupons))
	s := settle(coupons, results, sales, 2_000_000, defaultTiers(13))
	checkConservation(t, s)

	pots := map[int]TierPayout{}
	for _, tp := range s.Tiers {
		pots[tp.Correct] = tp
	}
	if pots[13].PotLamports != 2_500_000+2_000_000 || pots[13].PerWinnerLamports != 4_500_000 {
		t.Fatalf("top tier should get 50%% of sales plus the rollover: %+v", pots[13])
	}
	if pots[12].PerWinnerLamports != 1_250_000 {
		t.Fatalf("12 correct: %+v", pots[12])
	}
	if pots[11].Winners != 2 || pots[11].PerWinnerLamports != 375_000 {
		t.Fatalf("11 correct should be split between two coupons: %+v", pots[11])
	}
	if pots[10].Winners != 0 || s.RolloverOutLamports != 250_000 {
		t.Fatalf("unwon 10-correct tier should roll over: %+v rollover=%d", pots[10], s.RolloverOutLamports)
	}
	if s.HouseLamports != 250_000 {
		t.Fatalf("house should keep 5%%, got %d", s.HouseLamports)
	}
}

func TestSettleNoTopWinnerRollsOverJackpot(t *testing.T) {
	s := settle([]string{"2222222222222"}, "1111111111111", 1_000_000, 7_000_000, defaultTiers(13))
	checkConservation(t, s)
	if s.PaidLamports != 0 || s.RolloverOutLamports != 950_000+7_000_000 {
		t.Fatalf("everything except the house cut should roll over: %+v", s)
	}
}

func TestSettleRoundingGoesToHouse(t *testing.T) {
	coupons := []string{"1111111", "1111111", "1111111"}
	tiers := []Tier{{7, 95}}
	s := settle(coupons, "1111111", 1_000_001, 0, tiers)
	checkConservation(t, s)
	if s.Tiers[0].PerWinnerLamports != 950_000/3 {
		t.Fatalf("per winner: %+v", s.Tiers[0])
	}
	if s.PaidLamports > 950_000 {
		t.Fatalf("paid more than the pot: %+v", s)
	}
}

func TestSettleEmptyEvent(t *testing.T) {
	s := settle(nil, "1111111111111", 0, 5_000, defaultTiers(13))
	checkConservation(t, s)
	if s.RolloverOutLamports != 5_000 {
		t.Fatalf("rollover must pass through an event without entries: %+v", s)
	}
}

func TestDrawOutcome(t *testing.T) {
	id := bson.NewObjectID()
	w := [3]uint64{3, 0, 1}
	first := drawOutcome(id, 2, "hash", w)
	for i := 0; i < 5; i++ {
		if drawOutcome(id, 2, "hash", w) != first {
			t.Fatal("draw must be deterministic for the same inputs")
		}
	}
	counts := map[string]int{}
	for i := 0; i < 4000; i++ {
		counts[drawOutcome(id, i, "hash", w)]++
	}
	if counts["X"] != 0 {
		t.Fatal("an outcome nobody picked must never be drawn")
	}
	if ratio := float64(counts["1"]) / float64(counts["2"]); ratio < 2.5 || ratio > 3.5 {
		t.Fatalf("draw should follow the 3:1 weights, got %v", counts)
	}
	if o := drawOutcome(id, 0, "hash", [3]uint64{}); !validOutcomes[o[0]] {
		t.Fatalf("draw without coupons must still produce an outcome, got %q", o)
	}
}

func TestSystemCouponMatchesExpandedRows(t *testing.T) {
	system, n, msg := canonicalizeSystem([]string{"X1", "12", "1"}, 3)
	if msg != "" || n != 4 || system[0] != "1X" || system[1] != "12" {
		t.Fatalf("canonical system: %v %d %s", system, n, msg)
	}
	results := "111"
	counts := map[int]int{}
	for correct, rows := range correctCounts(system, results) {
		counts[correct] = rows
	}
	expanded := []string{"111", "121", "X11", "X21"}
	for _, row := range expanded {
		counts[countCorrect(row, results)]--
	}
	for correct, left := range counts {
		if left != 0 {
			t.Fatalf("correct %d differs from expanded rows by %d", correct, left)
		}
	}

	entry := Entry{System: system, Combinations: n}
	var weights [][3]uint64
	weights = append(weights, [3]uint64{}, [3]uint64{}, [3]uint64{})
	entry.addPickWeights(weights)
	if weights[0] != [3]uint64{2, 2, 0} || weights[1] != [3]uint64{2, 0, 2} || weights[2] != [3]uint64{4, 0, 0} {
		t.Fatalf("weights: %v", weights)
	}

	raw := make([]string, 13)
	for i := range raw {
		raw[i] = "1"
		if i < 10 {
			raw[i] = "1X2"
		}
	}
	if _, _, msg := canonicalizeSystem(raw, 13); msg == "" {
		t.Fatal("59049 combinations were accepted")
	}
	raw[9] = "1X"
	if _, got, msg := canonicalizeSystem(raw, 13); msg != "" || got != 39366 {
		t.Fatalf("39366 combinations: got %d, %s", got, msg)
	}
}

func TestPickWeights(t *testing.T) {
	w := pickWeights([]string{"1X2", "1XX", "222"}, 1)
	if w != [3]uint64{0, 2, 1} {
		t.Fatalf("got %v", w)
	}
}

// fakeChain serves a devnet-like RPC: one slot per 400 ms, with every 7th slot skipped.
func fakeChain(t *testing.T, genesis time.Time, latest uint64) *solanaRPC {
	t.Helper()
	exists := func(s uint64) bool { return s%7 != 3 }
	blockTime := func(s uint64) int64 { return genesis.Add(time.Duration(s) * 400 * time.Millisecond).Unix() }
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		var slot uint64
		if len(req.Params) > 0 {
			_ = json.Unmarshal(req.Params[0], &slot)
		}
		var result any
		switch req.Method {
		case "getSlot":
			result = latest
		case "getBlocksWithLimit":
			for !exists(slot) && slot <= latest {
				slot++
			}
			result = []uint64{}
			if slot <= latest {
				result = []uint64{slot}
			}
		case "getBlockTime":
			if !exists(slot) {
				_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1,
					"error": map[string]any{"code": -32007, "message": fmt.Sprintf("Slot %d was skipped", slot)}})
				return
			}
			result = blockTime(slot)
		case "getBlock":
			result = map[string]any{"blockhash": fmt.Sprintf("hash%d", slot), "blockTime": blockTime(slot)}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": result})
	}))
	t.Cleanup(srv.Close)
	return newSolanaRPC(srv.URL)
}

func TestFindDrawBlock(t *testing.T) {
	genesis := time.Unix(1_700_000_000, 0)
	latest := uint64(200_002)
	rpc := fakeChain(t, genesis, latest)
	ctx := context.Background()

	for _, offset := range []time.Duration{time.Minute, 3 * time.Hour, 20*time.Hour + 1300*time.Millisecond} {
		want := genesis.Add(offset)
		got, err := rpc.findDrawBlock(ctx, want)
		if err != nil {
			t.Fatal(err)
		}
		if got.Time.Unix() < want.Unix() {
			t.Fatalf("block %d at %v is before the draw time %v", got.Slot, got.Time, want)
		}
		if got.Slot%7 == 3 {
			t.Fatalf("returned a skipped slot %d", got.Slot)
		}
		for prev := got.Slot - 1; prev > got.Slot-10; prev-- {
			if prev%7 != 3 {
				if genesis.Add(time.Duration(prev)*400*time.Millisecond).Unix() >= want.Unix() {
					t.Fatalf("slot %d is not the first block at or after the draw time (slot %d is too)", got.Slot, prev)
				}
				break
			}
		}
	}

	if _, err := rpc.findDrawBlock(ctx, genesis.Add(time.Duration(latest+1000)*400*time.Millisecond)); err == nil {
		t.Fatal("a draw time in the future must be rejected")
	}
}

// Run with SOLANA_LIVE_RPC=https://api.devnet.solana.com go test -run TestFindDrawBlockLive -v
func TestFindDrawBlockLive(t *testing.T) {
	url := os.Getenv("SOLANA_LIVE_RPC")
	if url == "" {
		t.Skip("SOLANA_LIVE_RPC not set")
	}
	want := time.Now().Add(-2 * time.Hour).Truncate(time.Second)
	got, err := newSolanaRPC(url).findDrawBlock(context.Background(), want)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("draw time %v -> slot %d, block time %v, blockhash %s", want, got.Slot, got.Time, got.Blockhash)
	if got.Time.Before(want) || got.Time.Sub(want) > 10*time.Second {
		t.Fatalf("block time %v is not just after %v", got.Time, want)
	}
}

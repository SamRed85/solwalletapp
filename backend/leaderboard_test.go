package main

import (
	"encoding/binary"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestLeaderboardKeepsWinnersOnly(t *testing.T) {
	event := &Event{
		Title:     "Stryket",
		GameCount: 1,
		Games:     []Game{{Result: "1"}},
		Settlement: &Settlement{Tiers: []TierPayout{
			{Correct: 1, PerWinnerLamports: 50},
		}},
	}
	rows := map[string]*leaderAcc{}
	addLeaderEntry(rows, Entry{WalletAddress: "winner", System: []string{"1"}, Combinations: 1}, event)
	addLeaderEntry(rows, Entry{WalletAddress: "miss", System: []string{"2"}, Combinations: 1}, event)
	addLeaderEntry(rows, Entry{WalletAddress: "winner", System: []string{"1"}, Combinations: 1}, event)

	out := leaderboardRows(rows)
	if len(out) != 1 {
		t.Fatalf("winners = %d, want 1", len(out))
	}
	if out[0].Wallet != "winner" || out[0].WinningsLamports != 100 || out[0].Coupons != 2 {
		t.Fatalf("row = %+v", out[0])
	}
	if out[0].BestCorrect != 1 || out[0].GameCount != 1 || out[0].EventTitle != "Stryket" {
		t.Fatalf("hit = %+v", out[0])
	}
}

func TestFinishedEventWithoutSettlementStillCounts(t *testing.T) {
	when := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	event := &Event{
		ID:           bson.NewObjectID(),
		Title:        "Rad",
		GameCount:    1,
		Games:        []Game{{Result: "1"}},
		ResultsSetAt: &when,
		EndTime:      when,
	}
	if !eventInSpan(event, time.Time{}, false, event.ID) {
		t.Fatal("a finished event with no stored settlement was left out")
	}
	data := make([]byte, perWinnerOffset+8)
	binary.LittleEndian.PutUint64(data[perWinnerOffset:], 80)
	event.Settlement = payoutSettlement(data, 1)
	rows := map[string]*leaderAcc{}
	addLeaderEntry(rows, Entry{WalletAddress: "winner", System: []string{"1"}, Combinations: 1}, event)
	out := leaderboardRows(rows)
	if len(out) != 1 || out[0].WinningsLamports != 80 {
		t.Fatalf("row = %+v", out)
	}
}

func TestLeaderSpan(t *testing.T) {
	if _, _, ok := leaderSpan("year", time.Now()); ok {
		t.Fatal("year should be rejected")
	}
	_, round, ok := leaderSpan("round", time.Now())
	if !ok || !round {
		t.Fatal("round should be a single event")
	}
}

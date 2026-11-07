package main

import (
	"crypto/ed25519"
	"encoding/hex"
	"testing"
)

func testEntry(index uint32, revealed bool) (slipRecord, slipRecord) {
	owner := base58Encode(bytesRepeat(1, 32))
	commitment := hex.EncodeToString(bytesRepeat(byte(index+2), 32))
	entry := slipRecord{Index: index, Owner: owner, Combinations: 1, Commitment: commitment}
	if !revealed {
		return entry, slipRecord{}
	}
	picks := make([]byte, 13)
	picks[0] = 0b001
	return entry, slipRecord{Index: index, Owner: owner, Combinations: 1, Commitment: commitment, Picks: hex.EncodeToString(picks)}
}

func bytesRepeat(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}

func TestScoreSlipFitsAndKeepsAnUnrevealedCoupon(t *testing.T) {
	entries := make([]slipRecord, 8)
	reveals := make([]slipRecord, 0, 7)
	for i := uint32(0); i < 8; i++ {
		entry, reveal := testEntry(i, i != 3)
		entries[i] = entry
		if i != 3 {
			reveals = append(reveals, reveal)
		}
	}
	hidden, err := scoreSlipData(3, slipArchiveView{Entries: entries, Reveals: reveals})
	if err != nil {
		t.Fatal(err)
	}
	if hidden[8+4+32+4+13+8+32+4] != 0 {
		t.Fatal("an unrevealed coupon was sent as revealed")
	}
	shown, err := scoreSlipData(7, slipArchiveView{Entries: entries, Reveals: reveals})
	if err != nil {
		t.Fatal(err)
	}
	if shown[8+4+32+4+13+8+32+4] != 1 {
		t.Fatal("a revealed coupon was sent as hidden")
	}
	_, keeper, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := signLegacy(keeper, [][]byte{bytesRepeat(2, 32), bytesRepeat(3, 32), bytesRepeat(4, 32), bytesRepeat(5, 32), bytesRepeat(6, 32), bytesRepeat(7, 32)}, [][]byte{bytesRepeat(8, 32)}, []ixSpec{{
		program: 7, accounts: []int{0, 1, 2, 3, 4, 5, 6}, data: shown,
	}}, bytesRepeat(9, 32))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) > maxTransactionBytes {
		t.Fatalf("score transaction is %d bytes", len(raw))
	}
	if _, err = scoreSlipData(4, slipArchiveView{Entries: entries[:4]}); err == nil {
		t.Fatal("a missing coupon was scored")
	}
}

func TestChainTalliedReadsTheCounter(t *testing.T) {
	data := make([]byte, eventTalliedOffset+4)
	data[eventTalliedOffset] = 8
	tallied, ok := chainTallied(data)
	if !ok || tallied != 8 {
		t.Fatalf("tallied %d ok %v", tallied, ok)
	}
	if _, ok = chainTallied(data[:10]); ok {
		t.Fatal("a short account reported a counter")
	}
}

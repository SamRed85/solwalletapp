package main

import (
	"context"
	"testing"
)

func TestParseSlipLogKeepsTheEventOnTheLine(t *testing.T) {
	commitment := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	entry, ok := parseSlipLog("Program log: slip-v1:Event111:0:Owner111:1:" + commitment)
	if !ok || entry.Kind != "entry" || entry.Event != "Event111" || entry.Index != 0 || entry.Combinations != 1 {
		t.Fatalf("entry log: %+v %v", entry, ok)
	}
	reveal, ok := parseSlipLog("Program log: reveal-v1:Event111:0:Owner111:1:" + commitment + ":0102")
	if !ok || reveal.Kind != "reveal" || reveal.Picks != "0102" {
		t.Fatalf("reveal log: %+v %v", reveal, ok)
	}
	if _, ok := parseSlipLog("Program log: slip-v1:too:few"); ok {
		t.Fatal("a short line must be ignored")
	}
}

func TestSlipArchiveStoresACouponOnce(t *testing.T) {
	db := testDatabase(t)
	app := &App{slips: db.Collection("slipArchive")}
	commitment := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	logs := []string{
		"Program log: slip-v1:Event222:0:Owner222:2:" + commitment,
		"Program log: reveal-v1:Event222:0:Owner222:2:" + commitment + ":010203",
		"Program log: not a slip",
	}
	ctx := context.Background()
	stored, err := app.ingestSlipLogs(ctx, "sig-1", logs)
	if err != nil || stored != 2 {
		t.Fatalf("stored %d: %v", stored, err)
	}
	again, err := app.ingestSlipLogs(ctx, "sig-1", logs)
	if err != nil || again != 0 {
		t.Fatalf("repeat stored %d: %v", again, err)
	}
	view, err := app.slipArchive(ctx, "Event222")
	if err != nil || len(view.Entries) != 1 || len(view.Reveals) != 1 {
		t.Fatalf("archive entries %d reveals %d: %v", len(view.Entries), len(view.Reveals), err)
	}
	if view.Entries[0].Combinations != 2 || view.Reveals[0].Picks != "010203" {
		t.Fatalf("stored fields %+v %+v", view.Entries[0], view.Reveals[0])
	}
	marked, ok := parseSlipLog("Program log: claim-v1:Event222:0")
	if !ok || marked.Kind != "claim" || marked.Index != 0 {
		t.Fatalf("claim log: %+v %v", marked, ok)
	}
}

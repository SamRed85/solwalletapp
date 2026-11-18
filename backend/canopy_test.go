package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"testing"
)

func foldProof(index uint32, leaf []byte, proof [][]byte) []byte {
	node := leaf
	for _, sibling := range proof {
		if index&1 == 0 {
			node = slipHash(node, sibling)
		} else {
			node = slipHash(sibling, node)
		}
		index >>= 1
	}
	return node
}

// The same six coupons are pinned in the program and browser tests: indexes 1024 to 1029,
// owner of 32 bytes of 1, one row each, and a commitment of 32 bytes of the index's low byte.
func canopyVector() []slipRecord {
	owner := base58Encode(bytesRepeat(1, 32))
	records := make([]slipRecord, 0, 6)
	for index := uint32(1024); index < 1030; index++ {
		records = append(records, slipRecord{
			Index: index, Owner: owner, Combinations: 1, Commitment: hex.EncodeToString(bytesRepeat(byte(index), 32)),
		})
	}
	return records
}

func TestBottomProofStaysInsideTheChunk(t *testing.T) {
	records := canopyVector()
	proof := bottomProof(records, 1027, 10, entryLeafOf)
	if len(proof) != 10 {
		t.Fatalf("a canopy proof has 10 hashes, got %d", len(proof))
	}
	leaf := entryLeafOf(records[3])
	chunk := foldProof(1027, leaf, proof)
	if got := hex.EncodeToString(chunk); got != "6dc83f88ac175a33282aed66fd901da954f9fd9bd97b66c62a78d24bf78ab65d" {
		t.Fatalf("chunk node drifted: %s", got)
	}
	if got := hex.EncodeToString(proof[1]); got != "8f181aec88bc669959be78fe4878bdf1a7b7cfec7eb7da2a16df616ca7f8b93d" {
		t.Fatalf("second proof hash drifted: %s", got)
	}

	// The whole-event proof of an older event starts with the same ten hashes, and the
	// coupons of other chunks do not change them.
	all := append([]slipRecord{}, records...)
	owner := base58Encode(bytesRepeat(1, 32))
	for index := uint32(0); index < 1024; index += 97 {
		all = append(all, slipRecord{Index: index, Owner: owner, Combinations: 2, Commitment: hex.EncodeToString(bytesRepeat(9, 32))})
	}
	full := sparseProof(entryNodes(all), 1027)
	for level := 0; level < 10; level++ {
		if !bytes.Equal(full[level], proof[level]) {
			t.Fatalf("hash %d differs from the full proof", level)
		}
	}
	if bytes.Equal(full[10], slipZero(10)) {
		t.Fatal("the full proof must reach into the first chunk above the canopy")
	}
	if from, to := canopyChunk(1027, 10); from != 1024 || to != 2048 {
		t.Fatalf("chunk of 1027 is %d to %d", from, to)
	}
	if from, to := canopyChunk(999_999, 10); from != 999_424 || to != 1_000_448 {
		t.Fatalf("chunk of the last coupon is %d to %d", from, to)
	}
}

// The last coupon of a full event, with four colors, counted through the canopy.
func TestScoreCanopySlipFitsAtCouponOneMillion(t *testing.T) {
	const index = 999_999
	owner := base58Encode(bytesRepeat(1, 32))
	commitment := hex.EncodeToString(bytesRepeat(7, 32))
	picks := hex.EncodeToString(bytesRepeat(0b111, 13))
	colors := hex.EncodeToString(bytesRepeat(1, 4*slipColorBytes))
	view := slipArchiveView{}
	for at := uint32(999_424); at <= index; at++ {
		entry := slipRecord{Index: at, Owner: owner, Combinations: 14, Commitment: commitment}
		view.Entries = append(view.Entries, entry)
		if at != 999_500 {
			reveal := entry
			reveal.Picks, reveal.Colors = picks, colors
			view.Reveals = append(view.Reveals, reveal)
		}
	}
	shown, err := scoreCanopyData(index, view, 10)
	if err != nil {
		t.Fatal(err)
	}
	argLen := 4 + 32 + 4 + 13 + 8 + 32 + 4 + 4*slipColorBytes
	if !bytes.Equal(shown[:8], ixDisc("score_canopy_slip")) || shown[8+argLen] != 1 {
		t.Fatal("a revealed coupon was not sent as revealed to score_canopy_slip")
	}
	if binary.LittleEndian.Uint32(shown[8+argLen+1:]) != 10 || len(shown) != 8+argLen+1+4+320 {
		t.Fatalf("the score sends one proof of ten hashes, got %d bytes", len(shown))
	}
	hidden, err := scoreCanopyData(999_500, view, 10)
	if err != nil {
		t.Fatal(err)
	}
	if hidden[8+4+32+4+13+8+32+4] != 0 {
		t.Fatal("an unrevealed coupon was sent as revealed")
	}
	_, keeper, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	key := func(fill byte) []byte { return bytesRepeat(fill, 32) }
	raw, err := scoreCanopyTx(keeper, key(2), key(3), key(4), key(5), key(6), key(10), key(11), shown, key(9))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) > maxTransactionBytes {
		t.Fatalf("score transaction is %d bytes", len(raw))
	}
	// One signature, the header, nine keys, then the compute limit before the score.
	budget := mustDecode(computeBudgetProgram)
	if at := 1 + 64 + 3 + 1 + 7*32; !bytes.Equal(raw[at:at+32], budget) || raw[1+64+2] != 3 {
		t.Fatal("the score transaction does not carry the compute limit program read-only")
	}

	// The same coupon on an event without a canopy needs three depth-20 proofs.
	older := 8 + argLen + 1 + 3*(4+20*32)
	if older <= maxTransactionBytes {
		t.Fatalf("the full proofs were expected not to fit, got %d bytes", older)
	}

	arg := revealArg(index, bytesRepeat(1, 32), 14, bytesRepeat(0b111, 13), 5, bytesRepeat(7, 32), bytesRepeat(1, 4*slipColorBytes))
	reveal := revealCanopyData(arg, index, view, 10)
	if !bytes.Equal(reveal[:8], ixDisc("reveal_canopy_slip")) || len(reveal) != 8+argLen+2*(4+320) {
		t.Fatalf("the reveal sends two proofs of ten hashes, got %d bytes", len(reveal))
	}
	raw, err = revealCanopyTx(keeper, key(2), key(3), key(11), reveal, key(9))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) > maxTransactionBytes {
		t.Fatalf("reveal transaction is %d bytes", len(raw))
	}
	if _, err = scoreCanopyData(1_000_000, view, 10); err == nil {
		t.Fatal("a missing coupon was scored")
	}
}

func TestChainEventReadsTheCanopy(t *testing.T) {
	data := make([]byte, 456)
	copy(data, anchorDiscriminator("Event"))
	data[eventGameCountOffset] = 13
	account := &chainAccount{Owner: "program", Data: data}
	older, err := decodeChainEvent(account, "program")
	if err != nil || older.Canopy != 0 {
		t.Fatalf("an event from before the canopy read %+v: %v", older, err)
	}
	data[eventCanopyOffset] = 10
	current, err := decodeChainEvent(account, "program")
	if err != nil || current.Canopy != 10 {
		t.Fatalf("canopy read %+v: %v", current, err)
	}
	data[eventCanopyOffset] = 20
	if _, err = decodeChainEvent(account, "program"); err == nil {
		t.Fatal("a canopy as deep as the tree was accepted")
	}
}

func TestArchiveRangeReadsTheQuery(t *testing.T) {
	if from, to, ok := archiveRange("", ""); !ok || from != 0 || to != 0 {
		t.Fatal("no range must mean the whole event")
	}
	if from, to, ok := archiveRange("1024", "2048"); !ok || from != 1024 || to != 2048 {
		t.Fatalf("range read %d %d %v", from, to, ok)
	}
	for _, bad := range [][2]string{{"10", "10"}, {"10", "5"}, {"-1", ""}, {"a", "5"}, {"0", "x"}} {
		if _, _, ok := archiveRange(bad[0], bad[1]); ok {
			t.Fatalf("range %v was accepted", bad)
		}
	}
}

func TestSlipArchiveRangeReturnsOneChunk(t *testing.T) {
	db := testDatabase(t)
	app := &App{slips: db.Collection("slipArchive")}
	ctx := context.Background()
	commitment := "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	for _, index := range []int{0, 1023, 1024, 1500, 2047, 2048} {
		logs := []string{
			fmt.Sprintf("Program log: slip-v1:Event333:%d:Owner333:1:%s", index, commitment),
			fmt.Sprintf("Program log: reveal-v1:Event333:%d:Owner333:1:%s:01", index, commitment),
		}
		if _, err := app.ingestSlipLogs(ctx, fmt.Sprintf("sig-%d", index), logs); err != nil {
			t.Fatal(err)
		}
	}
	from, to := canopyChunk(1500, 10)
	view, err := app.slipArchiveRange(ctx, "Event333", from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Entries) != 3 || len(view.Reveals) != 3 || view.Entries[0].Index != 1024 || view.Entries[2].Index != 2047 {
		t.Fatalf("chunk returned %d entries and %d reveals", len(view.Entries), len(view.Reveals))
	}
	whole, err := app.slipArchive(ctx, "Event333")
	if err != nil || len(whole.Entries) != 6 {
		t.Fatalf("whole event returned %d entries: %v", len(whole.Entries), err)
	}
}

func TestSlipArchiveOwnerReturnsOneWallet(t *testing.T) {
	db := testDatabase(t)
	app := &App{slips: db.Collection("slipArchive")}
	ctx := context.Background()
	commitment := "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	owners := []string{"OwnerA", "OwnerB", "OwnerA", "OwnerB"}
	for index, owner := range owners {
		logs := []string{
			fmt.Sprintf("Program log: slip-v1:Event444:%d:%s:1:%s", index, owner, commitment),
			fmt.Sprintf("Program log: reveal-v1:Event444:%d:%s:1:%s:01", index, owner, commitment),
			fmt.Sprintf("Program log: claim-v1:Event444:%d", index),
		}
		if _, err := app.ingestSlipLogs(ctx, fmt.Sprintf("own-%d", index), logs); err != nil {
			t.Fatal(err)
		}
	}
	view, err := app.slipArchiveOwner(ctx, "Event444", "OwnerA")
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Entries) != 2 || len(view.Reveals) != 2 || len(view.Claims) != 2 {
		t.Fatalf("got %d entries, %d reveals, %d claims", len(view.Entries), len(view.Reveals), len(view.Claims))
	}
	if view.Entries[1].Index != 2 || view.Claims[0].Index != 0 || view.Claims[1].Index != 2 {
		t.Fatalf("wrong coupons: %+v %+v", view.Entries, view.Claims)
	}
	none, err := app.slipArchiveOwner(ctx, "Event444", "OwnerC")
	if err != nil || len(none.Entries) != 0 || len(none.Claims) != 0 {
		t.Fatalf("a wallet with no coupons got %+v: %v", none, err)
	}
}

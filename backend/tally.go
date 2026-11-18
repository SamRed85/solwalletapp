package main

import (
	"context"
	"crypto/ed25519"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	eventTalliedOffset  = 305
	maxScoresPerPass    = 12
	maxTransactionBytes = 1232
)

func chainTallied(data []byte) (uint32, bool) {
	if len(data) < eventTalliedOffset+4 {
		return 0, false
	}
	return binary.LittleEndian.Uint32(data[eventTalliedOffset : eventTalliedOffset+4]), true
}

func slipMark() []byte {
	return slipHash([]byte("slip-marked"))
}

func (a *App) scoreLoop() {
	if a.keeper == nil || a.rpc == nil || a.fetchAccount == nil {
		return
	}
	log.Print("automatic count is on")
	if a.cfg.TotalsWatch {
		log.Print("totals watcher is on: this process counts its own archive and challenges, and it does not post totals")
	}
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	a.countDue()
	for range ticker.C {
		a.countDue()
	}
}

func (a *App) countDue() {
	if a.events == nil || a.slips == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cursor, err := a.events.Find(ctx, bson.M{
		"chain.address": bson.M{"$exists": true, "$ne": ""},
		"chain.status":  bson.M{"$nin": []string{"resolved", "cancelled"}},
	})
	if err != nil {
		log.Printf("automatic count: %v", err)
		return
	}
	defer cursor.Close(ctx)
	for cursor.Next(ctx) {
		var event Event
		if err = cursor.Decode(&event); err != nil || event.Chain == nil || event.Chain.Address == "" {
			continue
		}
		if err = a.countEvent(ctx, &event); err != nil {
			log.Printf("automatic count %s: %v", event.Chain.Address, err)
		}
	}
}

// countEvent advances one vault. A signed-totals event is posted or finalized here.
// The coupon-by-coupon count runs for an older event, after the 24-hour fallback, or during a recount.
func (a *App) countEvent(ctx context.Context, event *Event) error {
	account, err := a.fetchAccount(ctx, event.Chain.Address)
	if err != nil {
		return err
	}
	if account == nil || account.Owner != a.cfg.ProgramID {
		return nil
	}
	chain, err := decodeChainEvent(account, a.cfg.ProgramID)
	if err != nil {
		return err
	}
	switch chain.Status {
	case "window":
		if a.cfg.TotalsWatch {
			return a.watchTotals(ctx, event, chain)
		}
		return a.finalizeTotals(ctx, event, chain)
	case "tallying":
		if chain.SettlementPath == pathSigned && chain.CountStarted == 0 {
			if a.cfg.TotalsWatch {
				return nil
			}
			if !totalsFallbackDue(chain, time.Now().Unix()) {
				return a.postTotals(ctx, event, chain)
			}
			log.Printf("totals: %s had no signed totals after 24 hours; counting on chain", event.Chain.Address)
		}
	case "recount":
		if a.cfg.TotalsWatch {
			return nil
		}
	default:
		return nil
	}
	var pay *recountPay
	if chain.Status == "recount" {
		totals, err := a.readTotals(ctx, event.Chain.Address)
		if err != nil {
			return err
		}
		if totals == nil {
			return errors.New("the recount has no signed totals")
		}
		pay = &recountPay{poster: totals.PosterKey, challenger: totals.ChallengerKey}
	}
	for scored := 0; scored < maxScoresPerPass; scored++ {
		account, err = a.fetchAccount(ctx, event.Chain.Address)
		if err != nil {
			return err
		}
		if account == nil || account.Owner != a.cfg.ProgramID {
			return nil
		}
		chain, err = decodeChainEvent(account, a.cfg.ProgramID)
		if err != nil {
			return err
		}
		if chain.Status != "tallying" && chain.Status != "recount" {
			return nil
		}
		tallied, ok := chainTallied(account.Data)
		if !ok {
			return errors.New("the event account has no tally counter")
		}
		if tallied >= chain.CouponCount {
			return nil
		}
		before := tallied
		if err = a.scoreOne(ctx, event.Chain.Admin, event.Chain.Address, tallied, chain.Canopy, pay); err != nil {
			return err
		}
		account, err = a.fetchAccount(ctx, event.Chain.Address)
		if err != nil {
			return err
		}
		chain, err = decodeChainEvent(account, a.cfg.ProgramID)
		if err != nil {
			return err
		}
		tallied, _ = chainTallied(account.Data)
		log.Printf("counted coupon %d of %d on %s", before+1, chain.CouponCount, event.Chain.Address)
		if chain.Status == "resolved" {
			log.Printf("event %s is settled", event.Chain.Address)
			_ = a.mirrorOne(ctx, event)
			return nil
		}
		if tallied <= before {
			return errors.New("the count did not advance")
		}
	}
	return nil
}

// scoreOne counts one coupon. levels is the event's canopy: zero sends the whole proofs of
// an older event, and anything else sends the bottom hashes built from the coupon's chunk.
func (a *App) scoreOne(ctx context.Context, admin, eventAddress string, index uint32, levels int, pay *recountPay) error {
	var view slipArchiveView
	var err error
	if levels > 0 {
		from, to := canopyChunk(index, levels)
		view, err = a.slipArchiveRange(ctx, eventAddress, from, to)
	} else {
		view, err = a.slipArchive(ctx, eventAddress)
	}
	if err != nil {
		return err
	}
	raw, err := a.scoreTransaction(ctx, admin, eventAddress, index, view, levels, pay)
	if err != nil {
		return err
	}
	if len(raw) > maxTransactionBytes {
		return errors.New("the score transaction is too large; the count stopped")
	}
	if err = a.rpc.simulate(ctx, raw); err != nil {
		return err
	}
	signature, err := a.rpc.send(ctx, raw)
	if err != nil {
		return err
	}
	if err = a.rpc.confirm(ctx, signature); err != nil {
		return err
	}
	if logs, err := a.rpc.transactionLogs(ctx, signature, "confirmed"); err == nil {
		_, _ = a.ingestSlipLogs(ctx, signature, logs)
	}
	return nil
}

func (a *App) scoreTransaction(ctx context.Context, admin, eventAddress string, index uint32, view slipArchiveView, levels int, pay *recountPay) ([]byte, error) {
	var data []byte
	var err error
	if levels > 0 {
		data, err = scoreCanopyData(index, view, levels)
	} else {
		data, err = scoreSlipData(index, view)
	}
	if err != nil {
		return nil, err
	}
	program, err := base58Decode(a.cfg.ProgramID)
	eventKey, err2 := base58Decode(eventAddress)
	adminKey, err3 := base58Decode(admin)
	if err != nil || err2 != nil || err3 != nil {
		return nil, errors.New("the event address is invalid")
	}
	seed := "slips"
	if levels > 0 {
		seed = "canopy"
	}
	slips, err := findPDA([][]byte{[]byte(seed), eventKey}, program)
	if err != nil {
		return nil, err
	}
	vault, err := findPDA([][]byte{[]byte("vault"), eventKey}, program)
	if err != nil {
		return nil, err
	}
	schedule, err := findPDA([][]byte{[]byte("schedule"), adminKey}, program)
	if err != nil {
		return nil, err
	}
	kinds, err := findPDA([][]byte{[]byte("schedule-kinds"), adminKey}, program)
	if err != nil {
		return nil, err
	}
	league, err := findPDA([][]byte{[]byte("league")}, program)
	if err != nil {
		return nil, err
	}
	hash, err := a.rpc.latestBlockhash(ctx)
	if err != nil {
		return nil, err
	}
	if levels > 0 {
		if pay != nil {
			return scoreRecountCanopyTx(a.keeper, eventKey, vault, schedule, kinds, league, slips, program, data, hash, pay)
		}
		return scoreCanopyTx(a.keeper, eventKey, vault, schedule, kinds, league, slips, program, data, hash)
	}
	if pay != nil {
		return scoreRecountSlipTx(a.keeper, eventKey, vault, slips, schedule, kinds, league, program, data, hash, pay)
	}
	// 0 keeper, 1 event, 2 vault, 3 slips, 4 schedule, 5 kinds, 6 league, 7 program.
	return signLegacy(a.keeper, [][]byte{eventKey, vault, slips, schedule, kinds, league}, [][]byte{program}, []ixSpec{{
		program: 7, accounts: []int{0, 1, 2, 3, 4, 5, 6}, data: data,
	}}, hash)
}

const computeBudgetProgram = "ComputeBudget111111111111111111111111111111"

// unitLimit sets the compute limit of a transaction to 400,000 units. Recounting the kept
// rows of a colored coupon can pass the default 200,000, and the count must not stop there.
var unitLimit = []byte{2, 0x80, 0x1a, 0x06, 0x00}

// scoreCanopyTx signs score_canopy_slip behind the compute limit. Scoring only reads the
// canopy, so it is not writable.
func scoreCanopyTx(keeper ed25519.PrivateKey, event, vault, schedule, kinds, league, canopy, program, data, blockhash []byte) ([]byte, error) {
	// 0 keeper, 1 event, 2 vault, 3 schedule, 4 kinds, 5 league, 6 canopy, 7 compute budget, 8 program.
	return signLegacy(keeper, [][]byte{event, vault, schedule, kinds, league}, [][]byte{canopy, mustDecode(computeBudgetProgram), program}, []ixSpec{
		{program: 7, data: unitLimit},
		{program: 8, accounts: []int{0, 1, 2, 6, 3, 4, 5}, data: data},
	}, blockhash)
}

// scoredCoupon is the RevealArg of one coupon for a score, and whether it was revealed.
// An unrevealed coupon sends empty picks. The reveal nonce is not needed to count.
func scoredCoupon(index uint32, view slipArchiveView) (arg []byte, revealed bool, err error) {
	var entry *slipRecord
	for i := range view.Entries {
		if view.Entries[i].Index == index {
			entry = &view.Entries[i]
			break
		}
	}
	if entry == nil {
		return nil, false, errors.New("the coupon list is still catching up")
	}
	owner, err := base58Decode(entry.Owner)
	commitment, err2 := hex.DecodeString(entry.Commitment)
	if err != nil || err2 != nil || len(owner) != 32 || len(commitment) != 32 {
		return nil, false, errors.New("the stored coupon is invalid")
	}
	var reveal *slipRecord
	for i := range view.Reveals {
		if view.Reveals[i].Index == index {
			reveal = &view.Reveals[i]
			break
		}
	}
	picks := make([]byte, 13)
	var colors []byte
	if reveal != nil {
		raw, err := hex.DecodeString(reveal.Picks)
		if err != nil {
			return nil, false, errors.New("the stored picks are invalid")
		}
		copy(picks, raw)
		parsed, err := parseColorHex(reveal.Colors)
		if err != nil {
			return nil, false, err
		}
		colors = colorsBytes(parsed)
	}
	return revealArg(index, owner, entry.Combinations, picks, 0, commitment, colors), reveal != nil, nil
}

func revealedFlag(revealed bool) byte {
	if revealed {
		return 1
	}
	return 0
}

// scoreSlipData is score_slip on an event from before the canopy: three whole proofs.
func scoreSlipData(index uint32, view slipArchiveView) ([]byte, error) {
	arg, revealed, err := scoredCoupon(index, view)
	if err != nil {
		return nil, err
	}
	entryProof := compactProof(sparseProof(entryNodes(view.Entries), index))
	revealProof := compactProof(sparseProof(revealNodes(view.Reveals), index))
	scored := make([][]byte, index)
	mark := slipMark()
	for i := range scored {
		scored[i] = mark
	}
	scoredProof := compactProof(sparseProof(scored, index))
	data := append(ixDisc("score_slip"), arg...)
	data = append(data, revealedFlag(revealed))
	for _, proof := range [][][]byte{entryProof, revealProof, scoredProof} {
		data = appendProof(data, proof)
	}
	return data, nil
}

// scoreCanopyData is score_canopy_slip: the bottom hashes of the reveal proof, built from
// the coupon's chunk. The reveal leaf was written only after the entry was checked, and
// the count runs in index order, so no entry proof and no scored proof are sent.
func scoreCanopyData(index uint32, view slipArchiveView, levels int) ([]byte, error) {
	arg, revealed, err := scoredCoupon(index, view)
	if err != nil {
		return nil, err
	}
	data := append(ixDisc("score_canopy_slip"), arg...)
	data = append(data, revealedFlag(revealed))
	return appendProof(data, bottomProof(view.Reveals, index, levels, revealLeafOf)), nil
}

// entryLeafOf is the entry leaf of one archive record, or nil when the record is unusable.
func entryLeafOf(entry slipRecord) []byte {
	owner, err := base58Decode(entry.Owner)
	commitment, err2 := hex.DecodeString(entry.Commitment)
	if err != nil || err2 != nil || len(owner) != 32 || len(commitment) != 32 {
		return nil
	}
	return slipLeaf(entry.Index, owner, entry.Combinations, commitment)
}

// revealLeafOf is the reveal leaf of one archive record, or nil when the record is unusable.
func revealLeafOf(reveal slipRecord) []byte {
	owner, err := base58Decode(reveal.Owner)
	commitment, err2 := hex.DecodeString(reveal.Commitment)
	picks, err3 := hex.DecodeString(reveal.Picks)
	colors, err4 := hex.DecodeString(reveal.Colors)
	if err != nil || err2 != nil || err3 != nil || err4 != nil || len(owner) != 32 || len(commitment) != 32 {
		return nil
	}
	padded := make([]byte, 13)
	copy(padded, picks)
	return slipRevealLeaf(reveal.Index, owner, reveal.Combinations, commitment, padded, colors)
}

func leafNodes(records []slipRecord, leaf func(slipRecord) []byte) [][]byte {
	nodes := [][]byte{}
	for _, record := range records {
		node := leaf(record)
		if node == nil {
			continue
		}
		for int(record.Index) >= len(nodes) {
			nodes = append(nodes, nil)
		}
		nodes[record.Index] = node
	}
	return nodes
}

func entryNodes(entries []slipRecord) [][]byte {
	return leafNodes(entries, entryLeafOf)
}

func revealNodes(reveals []slipRecord) [][]byte {
	return leafNodes(reveals, revealLeafOf)
}

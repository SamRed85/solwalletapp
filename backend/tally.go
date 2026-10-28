package main

import (
	"context"
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

// countEvent scores the next coupons of one tallying vault. It reads tallied from the
// chain, so a restart continues where the last count stopped.
func (a *App) countEvent(ctx context.Context, event *Event) error {
	for scored := 0; scored < maxScoresPerPass; scored++ {
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
		if chain.Status != "tallying" {
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
		if err = a.scoreOne(ctx, event.Chain.Admin, event.Chain.Address, tallied); err != nil {
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

func (a *App) scoreOne(ctx context.Context, admin, eventAddress string, index uint32) error {
	view, err := a.slipArchive(ctx, eventAddress)
	if err != nil {
		return err
	}
	raw, err := a.scoreTransaction(ctx, admin, eventAddress, index, view)
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

func (a *App) scoreTransaction(ctx context.Context, admin, eventAddress string, index uint32, view slipArchiveView) ([]byte, error) {
	data, err := scoreSlipData(index, view)
	if err != nil {
		return nil, err
	}
	program, err := base58Decode(a.cfg.ProgramID)
	eventKey, err2 := base58Decode(eventAddress)
	adminKey, err3 := base58Decode(admin)
	if err != nil || err2 != nil || err3 != nil {
		return nil, errors.New("the event address is invalid")
	}
	slips, err := findPDA([][]byte{[]byte("slips"), eventKey}, program)
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
	hash, err := a.rpc.latestBlockhash(ctx)
	if err != nil {
		return nil, err
	}
	// 0 keeper, 1 event, 2 slips, 3 schedule, 4 vault, 5 program.
	return signLegacy(a.keeper, [][]byte{eventKey, slips, schedule}, [][]byte{vault, program}, []ixSpec{{
		program: 5, accounts: []int{0, 1, 4, 2, 3}, data: data,
	}}, hash)
}

func scoreSlipData(index uint32, view slipArchiveView) ([]byte, error) {
	var entry *slipRecord
	for i := range view.Entries {
		if view.Entries[i].Index == index {
			entry = &view.Entries[i]
			break
		}
	}
	if entry == nil {
		return nil, errors.New("the coupon list is still catching up")
	}
	owner, err := base58Decode(entry.Owner)
	commitment, err2 := hex.DecodeString(entry.Commitment)
	if err != nil || err2 != nil || len(owner) != 32 || len(commitment) != 32 {
		return nil, errors.New("the stored coupon is invalid")
	}
	var revealed *slipRecord
	for i := range view.Reveals {
		if view.Reveals[i].Index == index {
			revealed = &view.Reveals[i]
			break
		}
	}
	picks := make([]byte, 13)
	if revealed != nil {
		raw, err := hex.DecodeString(revealed.Picks)
		if err != nil {
			return nil, errors.New("the stored picks are invalid")
		}
		copy(picks, raw)
	}
	entryProof := compactProof(sparseProof(entryNodes(view.Entries), index))
	revealProof := compactProof(sparseProof(revealNodes(view.Reveals), index))
	scored := make([][]byte, index)
	mark := slipMark()
	for i := range scored {
		scored[i] = mark
	}
	scoredProof := compactProof(sparseProof(scored, index))
	data := append(ixDisc("score_slip"), putU32(index)...)
	data = append(data, owner...)
	data = append(data, putU32(entry.Combinations)...)
	data = append(data, picks...)
	data = append(data, putU64(0)...)
	data = append(data, commitment...)
	if revealed != nil {
		data = append(data, 1)
	} else {
		data = append(data, 0)
	}
	for _, proof := range [][][]byte{entryProof, revealProof, scoredProof} {
		data = append(data, putU32(uint32(len(proof)))...)
		for _, sibling := range proof {
			data = append(data, sibling...)
		}
	}
	return data, nil
}

func entryNodes(entries []slipRecord) [][]byte {
	nodes := [][]byte{}
	for _, entry := range entries {
		owner, err := base58Decode(entry.Owner)
		commitment, err2 := hex.DecodeString(entry.Commitment)
		if err != nil || err2 != nil || len(owner) != 32 || len(commitment) != 32 {
			continue
		}
		for int(entry.Index) >= len(nodes) {
			nodes = append(nodes, nil)
		}
		nodes[entry.Index] = slipLeaf(entry.Index, owner, entry.Combinations, commitment)
	}
	return nodes
}

func revealNodes(reveals []slipRecord) [][]byte {
	nodes := [][]byte{}
	for _, reveal := range reveals {
		owner, err := base58Decode(reveal.Owner)
		commitment, err2 := hex.DecodeString(reveal.Commitment)
		picks, err3 := hex.DecodeString(reveal.Picks)
		if err != nil || err2 != nil || err3 != nil || len(owner) != 32 || len(commitment) != 32 {
			continue
		}
		padded := make([]byte, 13)
		copy(padded, picks)
		for int(reveal.Index) >= len(nodes) {
			nodes = append(nodes, nil)
		}
		nodes[reveal.Index] = slipRevealLeaf(reveal.Index, owner, reveal.Combinations, commitment, padded)
	}
	return nodes
}

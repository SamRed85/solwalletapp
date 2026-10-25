package main

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const slipDepth = 20

func slipHash(parts ...[]byte) []byte {
	h := sha256.New()
	for _, part := range parts {
		h.Write(part)
	}
	return h.Sum(nil)
}

func slipZero(level int) []byte {
	node := slipHash([]byte("slip-empty"))
	for i := 0; i < level; i++ {
		node = slipHash(node, node)
	}
	return node
}

func slipLeaf(index uint32, owner []byte, combinations uint32, commitment []byte) []byte {
	return slipHash([]byte("slip-leaf"), putU32(index), owner, putU32(combinations), commitment)
}

func slipRevealLeaf(index uint32, owner []byte, combinations uint32, commitment, picks []byte) []byte {
	return slipHash([]byte("slip-reveal"), putU32(index), owner, putU32(combinations), commitment, picks)
}

func putU32(value uint32) []byte {
	out := make([]byte, 4)
	binary.LittleEndian.PutUint32(out, value)
	return out
}

func putU64(value uint64) []byte {
	out := make([]byte, 8)
	binary.LittleEndian.PutUint64(out, value)
	return out
}

func nodeAt(level [][]byte, index uint32, depth int) []byte {
	if int(index) < len(level) && level[index] != nil {
		return level[index]
	}
	return slipZero(depth)
}

func sparseProof(nodes [][]byte, index uint32) [][]byte {
	level := nodes
	cursor := index
	proof := make([][]byte, 0, slipDepth)
	for depth := 0; depth < slipDepth; depth++ {
		proof = append(proof, nodeAt(level, cursor^1, depth))
		nextCount := (len(level) + 1) / 2
		if need := int(cursor>>1) + 1; need > nextCount {
			nextCount = need
		}
		next := make([][]byte, nextCount)
		for i := 0; i < nextCount; i++ {
			leftMissing := int(i*2) >= len(level) || level[i*2] == nil
			rightMissing := int(i*2+1) >= len(level) || level[i*2+1] == nil
			if leftMissing && rightMissing {
				continue
			}
			next[i] = slipHash(nodeAt(level, uint32(i*2), depth), nodeAt(level, uint32(i*2+1), depth))
		}
		level = next
		cursor >>= 1
	}
	return proof
}

func (a *App) armSlipReveal(w http.ResponseWriter, r *http.Request, wallet, coupon, ciphertext string) {
	parts := strings.Split(coupon, ":")
	if len(parts) != 3 || !isValidWallet(parts[1]) {
		writeError(w, http.StatusBadRequest, "invalid coupon")
		return
	}
	index64, err := strconv.ParseUint(parts[2], 10, 32)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid coupon")
		return
	}
	if len(ciphertext) < 200 || len(ciphertext) > 8000 {
		writeError(w, http.StatusBadRequest, "the time-locked reveal has an unexpected size")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	view, err := a.slipArchive(ctx, parts[1])
	if err != nil {
		writeError(w, http.StatusBadGateway, "could not read the slip archive")
		return
	}
	var entry *slipRecord
	for i := range view.Entries {
		if view.Entries[i].Index == uint32(index64) {
			entry = &view.Entries[i]
			break
		}
	}
	if entry == nil {
		writeError(w, http.StatusBadRequest, "the coupon was not found in the archive")
		return
	}
	if entry.Owner != wallet {
		writeError(w, http.StatusForbidden, "this coupon belongs to another wallet")
		return
	}
	event, err := a.fetchAccount(ctx, parts[1])
	if err != nil || event == nil || len(event.Data) < eventAccountMinimumSize {
		writeError(w, http.StatusBadGateway, "the vault event could not be read")
		return
	}
	if event.Data[eventStatusOffset] != 0 {
		writeError(w, http.StatusConflict, "this event is no longer open")
		return
	}
	deadline := int64(binary.LittleEndian.Uint64(event.Data[eventDeadlineOffset:]))
	revealBy := int64(binary.LittleEndian.Uint64(event.Data[eventRevealOffset:]))
	if time.Now().Unix() >= revealBy {
		writeError(w, http.StatusConflict, "the reveal window has closed")
		return
	}
	if err := ciphertextWindow(ciphertext, deadline, revealBy); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	_, err = a.reveals.UpdateOne(ctx, bson.M{"coupon": coupon}, bson.M{"$set": scheduledReveal{
		Coupon: coupon, Owner: wallet, Event: parts[1], Deadline: deadline, RevealBy: revealBy,
		Ciphertext: ciphertext, Status: "pending", NextTry: time.Now(), UpdatedAt: time.Now(),
		Slip: true, Index: uint32(index64), Combinations: entry.Combinations, Commitment: entry.Commitment,
	}}, options.UpdateOne().SetUpsert(true))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not store the scheduled reveal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "armed"})
}

func (a *App) publishSlip(ctx context.Context, item scheduledReveal) {
	if time.Now().Unix() >= item.RevealBy {
		a.finishReveal(ctx, item.Coupon, "expired", "the reveal window closed", true)
		return
	}
	view, err := a.slipArchive(ctx, item.Event)
	if err != nil {
		a.retryReveal(ctx, item, "could not read the slip archive")
		return
	}
	for _, reveal := range view.Reveals {
		if reveal.Index == item.Index {
			a.finishReveal(ctx, item.Coupon, "revealed", "", true)
			return
		}
	}
	plain, err := decryptLocked(item.Ciphertext)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "too early") {
			return
		}
		a.retryReveal(ctx, item, "could not open the time lock")
		return
	}
	defer func() {
		for i := range plain {
			plain[i] = 0
		}
	}()
	picks, revealNonce, err := lockedPicks(plain)
	if err != nil {
		a.finishReveal(ctx, item.Coupon, "rejected", err.Error(), true)
		return
	}
	owner, err := base58Decode(item.Owner)
	commitment, err2 := hex.DecodeString(item.Commitment)
	if err != nil || err2 != nil || len(owner) != 32 || len(commitment) != 32 {
		a.finishReveal(ctx, item.Coupon, "rejected", "the stored slip is invalid", true)
		return
	}
	if hex.EncodeToString(couponCommitment(owner, picks, revealNonce)) != item.Commitment {
		a.finishReveal(ctx, item.Coupon, "rejected", "the stored picks do not match this coupon", true)
		return
	}
	nodes := make([][]byte, 0)
	for _, reveal := range view.Reveals {
		for int(reveal.Index) >= len(nodes) {
			nodes = append(nodes, nil)
		}
		picksBytes, err := hex.DecodeString(reveal.Picks)
		commitBytes, err2 := hex.DecodeString(reveal.Commitment)
		ownerBytes, err3 := base58Decode(reveal.Owner)
		if err != nil || err2 != nil || err3 != nil {
			continue
		}
		nodes[reveal.Index] = slipRevealLeaf(reveal.Index, ownerBytes, reveal.Combinations, commitBytes, picksBytes)
	}
	for int(item.Index) >= len(nodes) {
		nodes = append(nodes, nil)
	}
	proof := sparseProof(nodes, item.Index)
	program, err := base58Decode(a.cfg.ProgramID)
	eventKey, err2 := base58Decode(item.Event)
	if err != nil || err2 != nil {
		a.retryReveal(ctx, item, "the event address is invalid")
		return
	}
	slips, err := findPDA([][]byte{[]byte("slips"), eventKey}, program)
	if err != nil {
		a.retryReveal(ctx, item, err.Error())
		return
	}
	data := append(ixDisc("reveal_slip"), putU32(item.Index)...)
	data = append(data, owner...)
	data = append(data, putU32(item.Combinations)...)
	data = append(data, picks...)
	data = append(data, putU64(revealNonce)...)
	data = append(data, commitment...)
	data = append(data, putU32(uint32(len(proof)))...)
	for _, sibling := range proof {
		data = append(data, sibling...)
	}
	hash, err := a.rpc.latestBlockhash(ctx)
	if err != nil {
		a.retryReveal(ctx, item, "could not read a recent blockhash")
		return
	}
	raw, err := signLegacy(a.keeper, [][]byte{slips}, [][]byte{eventKey, program}, []ixSpec{{
		program: 3, accounts: []int{0, 2, 1}, data: data,
	}}, hash)
	if err != nil {
		a.retryReveal(ctx, item, err.Error())
		return
	}
	if err := a.rpc.simulate(ctx, raw); err != nil {
		a.retryReveal(ctx, item, err.Error())
		return
	}
	signature, err := a.rpc.send(ctx, raw)
	if err != nil {
		a.retryReveal(ctx, item, err.Error())
		return
	}
	if err := a.rpc.confirm(ctx, signature); err != nil {
		a.retryReveal(ctx, item, err.Error())
		return
	}
	if logs, err := a.rpc.transactionLogs(ctx, signature, "confirmed"); err == nil {
		_, _ = a.ingestSlipLogs(ctx, signature, logs)
	}
	a.finishReveal(ctx, item.Coupon, "revealed", "", true)
}

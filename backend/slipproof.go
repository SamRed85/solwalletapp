package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
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

// slipRevealLeaf matches the program. A slip with no colors keeps the leaf it had before colors.
func slipRevealLeaf(index uint32, owner []byte, combinations uint32, commitment, picks, colors []byte) []byte {
	if len(colors) == 0 {
		return slipHash([]byte("slip-reveal"), putU32(index), owner, putU32(combinations), commitment, picks)
	}
	return slipHash([]byte("slip-reveal"), putU32(index), owner, putU32(combinations), commitment, picks, []byte("colors-v1"), colors)
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

func compactProof(proof [][]byte) [][]byte {
	end := len(proof)
	for end > 0 && bytes.Equal(proof[end-1], slipZero(end-1)) {
		end--
	}
	return proof[:end]
}

// canopyChunk is the range of coupons that share the bottom proof hashes with index.
func canopyChunk(index uint32, levels int) (from, to uint32) {
	from = index >> levels << levels
	return from, from + 1<<levels
}

// chunkLeaves lays the records of one chunk out by position. A missing record stays nil,
// which the proof treats as the empty leaf.
func chunkLeaves(records []slipRecord, from uint32, levels int, leaf func(slipRecord) []byte) [][]byte {
	leaves := make([][]byte, 1<<levels)
	for _, record := range records {
		if record.Index < from || record.Index-from >= uint32(len(leaves)) {
			continue
		}
		leaves[record.Index-from] = leaf(record)
	}
	return leaves
}

// bottomProof is what a client sends on an event with a canopy: the bottom hashes of the
// full proof. They never leave the coupon's chunk, and the program stores the rest.
func bottomProof(records []slipRecord, index uint32, levels int, leaf func(slipRecord) []byte) [][]byte {
	from, _ := canopyChunk(index, levels)
	return sparseProof(chunkLeaves(records, from, levels, leaf), index-from)[:levels]
}

// revealArg is the program's RevealArg in wire order.
func revealArg(index uint32, owner []byte, combinations uint32, picks []byte, nonce uint64, commitment, colors []byte) []byte {
	data := append(putU32(index), owner...)
	data = append(data, putU32(combinations)...)
	data = append(data, picks...)
	data = append(data, putU64(nonce)...)
	data = append(data, commitment...)
	return append(data, colorsArg(colors)...)
}

func appendProof(data []byte, proof [][]byte) []byte {
	data = append(data, putU32(uint32(len(proof)))...)
	for _, sibling := range proof {
		data = append(data, sibling...)
	}
	return data
}

// eventCanopy reads how many proof hashes a slip of this event sends. Zero is an event from
// before the canopy.
func (a *App) eventCanopy(ctx context.Context, address string) (int, error) {
	if a.fetchAccount == nil {
		return 0, errors.New("no Solana connection")
	}
	account, err := a.fetchAccount(ctx, address)
	if err != nil {
		return 0, err
	}
	chain, err := decodeChainEvent(account, a.cfg.ProgramID)
	if err != nil {
		return 0, err
	}
	return chain.Canopy, nil
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
	view, err := a.slipArchiveRange(ctx, parts[1], uint32(index64), uint32(index64)+1)
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
	levels, err := a.eventCanopy(ctx, item.Event)
	if err != nil {
		a.retryReveal(ctx, item, "the vault event could not be read")
		return
	}
	var view slipArchiveView
	if levels > 0 {
		from, to := canopyChunk(item.Index, levels)
		view, err = a.slipArchiveRange(ctx, item.Event, from, to)
	} else {
		view, err = a.slipArchive(ctx, item.Event)
	}
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
	picks, revealNonce, colors, err := lockedPicks(plain)
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
	if hex.EncodeToString(slipCommitment(owner, picks, revealNonce, colors)) != item.Commitment {
		a.finishReveal(ctx, item.Coupon, "rejected", "the stored picks do not match this coupon", true)
		return
	}
	program, err := base58Decode(a.cfg.ProgramID)
	eventKey, err2 := base58Decode(item.Event)
	if err != nil || err2 != nil {
		a.retryReveal(ctx, item, "the event address is invalid")
		return
	}
	arg := revealArg(item.Index, owner, item.Combinations, picks, revealNonce, commitment, colors)
	// The tree account is the canopy on a new event and the slip book on an older one.
	// Either way the instruction takes the keeper, the event, and that account.
	seed, data := "slips", revealSlipData(arg, item.Index, view)
	if levels > 0 {
		seed, data = "canopy", revealCanopyData(arg, item.Index, view, levels)
	}
	slips, err := findPDA([][]byte{[]byte(seed), eventKey}, program)
	if err != nil {
		a.retryReveal(ctx, item, err.Error())
		return
	}
	hash, err := a.rpc.latestBlockhash(ctx)
	if err != nil {
		a.retryReveal(ctx, item, "could not read a recent blockhash")
		return
	}
	var raw []byte
	if levels > 0 {
		raw, err = revealCanopyTx(a.keeper, slips, eventKey, program, data, hash)
	} else {
		raw, err = signLegacy(a.keeper, [][]byte{slips}, [][]byte{eventKey, program}, []ixSpec{{
			program: 3, accounts: []int{0, 2, 1}, data: data,
		}}, hash)
	}
	if err != nil {
		a.retryReveal(ctx, item, err.Error())
		return
	}
	if len(raw) > maxTransactionBytes {
		a.retryReveal(ctx, item, "the reveal transaction is too large")
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

// revealSlipData is reveal_slip on an event from before the canopy: one proof, cut where
// the rest of the tree is empty.
func revealSlipData(arg []byte, index uint32, view slipArchiveView) []byte {
	nodes := revealNodes(view.Reveals)
	for int(index) >= len(nodes) {
		nodes = append(nodes, nil)
	}
	data := append(ixDisc("reveal_slip"), arg...)
	return appendProof(data, compactProof(sparseProof(nodes, index)))
}

// revealCanopyTx signs reveal_canopy_slip behind the compute limit.
func revealCanopyTx(keeper ed25519.PrivateKey, canopy, event, program, data, blockhash []byte) ([]byte, error) {
	// 0 keeper, 1 canopy, 2 event, 3 compute budget, 4 program.
	return signLegacy(keeper, [][]byte{canopy}, [][]byte{event, mustDecode(computeBudgetProgram), program}, []ixSpec{
		{program: 3, data: unitLimit},
		{program: 4, accounts: []int{0, 2, 1}, data: data},
	}, blockhash)
}

// revealCanopyData is reveal_canopy_slip. The entry proof shows the coupon was placed and
// the reveal proof shows its reveal leaf is still empty. Each is the bottom hashes only,
// built from the coupon's own chunk.
func revealCanopyData(arg []byte, index uint32, view slipArchiveView, levels int) []byte {
	data := append(ixDisc("reveal_canopy_slip"), arg...)
	data = appendProof(data, bottomProof(view.Entries, index, levels, entryLeafOf))
	return appendProof(data, bottomProof(view.Reveals, index, levels, revealLeafOf))
}

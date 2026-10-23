package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// The vault schedule every published event must use. The program checks the order of these
// times; the backend checks that they match the MongoDB event exactly.
const vaultResolveWindow = 7 * 24 * time.Hour

// maxOpenVaultEvents is the program's per-admin schedule size (MAX_SCHEDULE). An event leaves
// the schedule when it is resolved, cancelled or expired.
const maxOpenVaultEvents = 16

// Byte offsets in the program's Event account, including the 8-byte Anchor discriminator.
const (
	eventAdminOffset        = 8
	eventNonceOffset        = 40
	eventGameCountOffset    = 48
	eventPriceOffset        = 49
	eventTierCountOffset    = 57
	eventTiersOffset        = 58
	eventDeadlineOffset     = 84
	eventRevealOffset       = 92
	eventEndOffset          = 100
	eventTimeoutOffset      = 108
	eventStatusOffset       = 116
	eventResultsOffset      = 117
	eventCouponCountOffset  = 130
	eventAccountMinimumSize = 421

	signersEventOffset     = 8
	signersThresholdOffset = 40
	signersCountOffset     = 41
	signersListOffset      = 42
	signersAccountSize     = 203
)

var chainStatuses = []string{"open", "resolved", "cancelled", "tallying"}

func anchorDiscriminator(account string) []byte {
	sum := sha256.Sum256([]byte("account:" + account))
	return sum[:8]
}

type chainAccount struct {
	Owner string
	Data  []byte
}

type chainEvent struct {
	Admin          string
	Nonce          uint64
	GameCount      int
	Price          uint64
	Tiers          []Tier
	Deadline       int64
	RevealDeadline int64
	EndTime        int64
	ResolveTimeout int64
	Status         string
	Results        string
	CouponCount    uint32
}

func decodeChainEvent(account *chainAccount, programID string) (*chainEvent, error) {
	if account == nil {
		return nil, errors.New("the event account does not exist on chain")
	}
	if account.Owner != programID {
		return nil, errors.New("the event account is not owned by the vault program")
	}
	d := account.Data
	if len(d) < eventAccountMinimumSize || string(d[:8]) != string(anchorDiscriminator("Event")) {
		return nil, errors.New("the account is not a vault event")
	}
	ev := &chainEvent{
		Admin:          base58Encode(d[eventAdminOffset : eventAdminOffset+32]),
		Nonce:          binary.LittleEndian.Uint64(d[eventNonceOffset:]),
		GameCount:      int(d[eventGameCountOffset]),
		Price:          binary.LittleEndian.Uint64(d[eventPriceOffset:]),
		Deadline:       int64(binary.LittleEndian.Uint64(d[eventDeadlineOffset:])),
		RevealDeadline: int64(binary.LittleEndian.Uint64(d[eventRevealOffset:])),
		EndTime:        int64(binary.LittleEndian.Uint64(d[eventEndOffset:])),
		ResolveTimeout: int64(binary.LittleEndian.Uint64(d[eventTimeoutOffset:])),
		CouponCount:    binary.LittleEndian.Uint32(d[eventCouponCountOffset:]),
	}
	status := int(d[eventStatusOffset])
	if status >= len(chainStatuses) {
		return nil, fmt.Errorf("unknown event status %d", status)
	}
	ev.Status = chainStatuses[status]
	tierCount := int(d[eventTierCountOffset])
	if tierCount > maxGames || ev.GameCount > maxGames {
		return nil, errors.New("the event account is malformed")
	}
	for i := 0; i < tierCount; i++ {
		at := eventTiersOffset + 2*i
		ev.Tiers = append(ev.Tiers, Tier{Correct: int(d[at]), Percent: int(d[at+1])})
	}
	if ev.Status == "resolved" || ev.Status == "tallying" {
		ev.Results = string(d[eventResultsOffset : eventResultsOffset+ev.GameCount])
	}
	return ev, nil
}

type chainSigners struct {
	Event     string
	Threshold int
	Members   []string
}

func decodeChainSigners(account *chainAccount, programID string) (*chainSigners, error) {
	if account == nil {
		return nil, errors.New("the result signer account does not exist on chain")
	}
	if account.Owner != programID {
		return nil, errors.New("the result signer account is not owned by the vault program")
	}
	d := account.Data
	if len(d) < signersAccountSize || string(d[:8]) != string(anchorDiscriminator("ResultSigners")) {
		return nil, errors.New("the account is not a result signer list")
	}
	count := int(d[signersCountOffset])
	if count > 5 {
		return nil, errors.New("the result signer account is malformed")
	}
	s := &chainSigners{Event: base58Encode(d[signersEventOffset : signersEventOffset+32]), Threshold: int(d[signersThresholdOffset])}
	for i := 0; i < count; i++ {
		at := signersListOffset + 32*i
		s.Members = append(s.Members, base58Encode(d[at:at+32]))
	}
	return s, nil
}

func sameTiers(a, b []Tier) bool {
	if len(a) != len(b) {
		return false
	}
	x, y := append([]Tier(nil), a...), append([]Tier(nil), b...)
	sort.Slice(x, func(i, j int) bool { return x[i].Correct < x[j].Correct })
	sort.Slice(y, func(i, j int) bool { return y[i].Correct < y[j].Correct })
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

// checkVaultMatches compares the on-chain vault with the MongoDB event it is attached to.
// It returns a message for the admin, or "" when every economic parameter matches.
func checkVaultMatches(e *Event, vault ChainVault, ev *chainEvent, signers *chainSigners) string {
	switch {
	case ev.Admin != vault.Admin || ev.Nonce != vault.Nonce:
		return "the on-chain event belongs to a different admin wallet or nonce"
	case ev.Status != "open":
		return "the on-chain event is no longer open"
	case ev.GameCount != len(e.Games) || ev.GameCount != e.GameCount:
		return "the on-chain event has a different number of games"
	case ev.Price != e.CouponPriceLamports:
		return "the on-chain coupon price differs from the event"
	case !sameTiers(ev.Tiers, e.Tiers):
		return "the on-chain prize tiers differ from the event"
	case ev.Deadline != e.StartTime.Unix():
		return "the on-chain entry deadline differs from the event start"
	case ev.EndTime != e.EndTime.Unix() || ev.RevealDeadline != ev.EndTime:
		return "the on-chain reveal deadline and end time must both equal the event end"
	case ev.ResolveTimeout != e.EndTime.Add(vaultResolveWindow).Unix():
		return "the on-chain resolution timeout must be seven days after the event end"
	}
	if signers.Event != vault.Address || signers.Threshold != 2 || len(signers.Members) != 2 {
		return "the on-chain event must require both result signers"
	}
	want := map[string]bool{vault.Admin: true, vault.Cosigner: true}
	for _, m := range signers.Members {
		if !want[m] {
			return "the on-chain result signers differ from the admin and co-signer"
		}
		delete(want, m)
	}
	return ""
}

func (s *solanaRPC) getAccount(ctx context.Context, address string) (*chainAccount, error) {
	var out struct {
		Value *struct {
			Owner string    `json:"owner"`
			Data  [2]string `json:"data"`
		} `json:"value"`
	}
	if err := s.call(ctx, "getAccountInfo", []any{address, map[string]any{"encoding": "base64", "commitment": "confirmed"}}, &out); err != nil {
		return nil, err
	}
	if out.Value == nil {
		return nil, nil
	}
	data, err := base64.StdEncoding.DecodeString(out.Value.Data[0])
	if err != nil {
		return nil, fmt.Errorf("invalid account data: %w", err)
	}
	return &chainAccount{Owner: out.Value.Owner, Data: data}, nil
}

// handleSyncChain copies the on-chain outcome of a vault event into MongoDB for display.
// The chain is authoritative, so any signed-in user may trigger it.
func (a *App) handleSyncChain(w http.ResponseWriter, r *http.Request) {
	e, ok := a.loadEvent(w, r)
	if !ok {
		return
	}
	if e.Chain == nil || e.Chain.Address == "" {
		writeError(w, http.StatusConflict, "this event has no on-chain vault")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	account, err := a.fetchAccount(ctx, e.Chain.Address)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	ev, err := decodeChainEvent(account, a.cfg.ProgramID)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	set := bson.M{"chain.status": ev.Status}
	filter := bson.M{"_id": e.ID, "chain.address": e.Chain.Address}
	if ev.Status == "resolved" && e.ResultsSetAt == nil {
		games := append([]Game(nil), e.Games...)
		for i := range games {
			if i < len(ev.Results) {
				games[i].Result = string(ev.Results[i])
				games[i].Cancelled = ev.Results[i] == 'C'
			}
		}
		set["games"] = games
		set["resultsSetAt"] = time.Now()
		filter["resultsSetAt"] = bson.M{"$exists": false}
	}
	if _, err := a.events.UpdateOne(ctx, filter, bson.M{"$set": set}); err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		writeError(w, http.StatusInternalServerError, "failed to save the on-chain status")
		return
	}
	a.handleGetEvent(w, r)
}

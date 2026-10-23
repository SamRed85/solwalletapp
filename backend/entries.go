package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// An entry is one paid submission. System stores the selected outcomes for each game,
// in 1, X, 2 order, for example ["1X", "2", "1X2"]. Combinations is their product.
// Coupons holds the expanded rows of entries saved before system coupons existed.
type Entry struct {
	ID               bson.ObjectID  `json:"id" bson:"_id,omitempty"`
	WalletAddress    string         `json:"walletAddress" bson:"walletAddress"`
	EventID          bson.ObjectID  `json:"eventId" bson:"eventId"`
	System           []string       `json:"system,omitempty" bson:"system,omitempty"`
	Combinations     int            `json:"combinations,omitempty" bson:"combinations,omitempty"`
	Coupons          []string       `json:"coupons,omitempty" bson:"coupons,omitempty"`
	Signature        string         `json:"signature" bson:"signature"`
	Lamports         uint64         `json:"lamports" bson:"lamports"`
	CreatedAt        time.Time      `json:"createdAt" bson:"createdAt"`
	EventTitle       string         `json:"eventTitle,omitempty" bson:"-"`
	Event            *Event         `json:"event,omitempty" bson:"-"`
	Results          []CouponResult `json:"results,omitempty" bson:"-"`
	Scores           []ScoreBand    `json:"scores,omitempty" bson:"-"`
	WinningsLamports uint64         `json:"winningsLamports" bson:"-"`
}

type CouponResult struct {
	Correct          int    `json:"correct"`
	WinningsLamports uint64 `json:"winningsLamports"`
}

type ScoreBand struct {
	Correct          int    `json:"correct"`
	Combinations     int    `json:"combinations"`
	WinningsLamports uint64 `json:"winningsLamports"`
}

func (en Entry) rowCount() int {
	if en.Combinations > 0 {
		return en.Combinations
	}
	return len(en.Coupons)
}

func (en Entry) addWinners(results string, winners map[int]int) {
	if len(en.System) > 0 {
		for correct, n := range correctCounts(en.System, results) {
			winners[correct] += n
		}
		return
	}
	for _, c := range en.Coupons {
		winners[countCorrect(c, results)]++
	}
}

func (en Entry) addPickWeights(weights [][3]uint64) {
	if len(en.System) > 0 {
		for game, picks := range en.System {
			if game >= len(weights) || len(picks) == 0 {
				continue
			}
			each := uint64(en.Combinations / len(picks))
			for i := 0; i < len(picks); i++ {
				switch picks[i] {
				case '1':
					weights[game][0] += each
				case 'X':
					weights[game][1] += each
				case '2':
					weights[game][2] += each
				}
			}
		}
		return
	}
	for _, c := range en.Coupons {
		for game := 0; game < len(c) && game < len(weights); game++ {
			switch c[game] {
			case '1':
				weights[game][0]++
			case 'X':
				weights[game][1]++
			case '2':
				weights[game][2]++
			}
		}
	}
}

func (en *Entry) score(e *Event) {
	if e.Settlement == nil {
		return
	}
	results := e.results()
	if len(en.System) > 0 {
		counts := correctCounts(en.System, results)
		for correct := len(counts) - 1; correct >= 0; correct-- {
			n := counts[correct]
			if n == 0 {
				continue
			}
			won := e.Settlement.payoutFor(correct) * uint64(n)
			en.Scores = append(en.Scores, ScoreBand{Correct: correct, Combinations: n, WinningsLamports: won})
			en.WinningsLamports += won
		}
		return
	}
	en.Results = make([]CouponResult, len(en.Coupons))
	for i, c := range en.Coupons {
		correct := countCorrect(c, results)
		won := e.Settlement.payoutFor(correct)
		en.Results[i] = CouponResult{Correct: correct, WinningsLamports: won}
		en.WinningsLamports += won
	}
}

func (en Entry) payout(e *Event) (winningRows int, lamports uint64) {
	results := e.results()
	if len(en.System) > 0 {
		for correct, n := range correctCounts(en.System, results) {
			each := e.Settlement.payoutFor(correct)
			if n == 0 || each == 0 {
				continue
			}
			winningRows += n
			lamports += each * uint64(n)
		}
		return winningRows, lamports
	}
	for _, c := range en.Coupons {
		won := e.Settlement.payoutFor(countCorrect(c, results))
		if won == 0 {
			continue
		}
		winningRows++
		lamports += won
	}
	return winningRows, lamports
}

// canonicalizeSystem sorts each game's picks into 1, X, 2 order and returns the combination count.
func canonicalizeSystem(raw []string, gameCount int) ([]string, int, string) {
	if len(raw) != gameCount {
		return nil, 0, "every game needs a selection"
	}
	system := make([]string, gameCount)
	combinations := 1
	for i, picks := range raw {
		for j := 0; j < len(picks); j++ {
			if !validOutcomes[picks[j]] {
				return nil, 0, "picks must be 1, X or 2"
			}
		}
		var selected strings.Builder
		for _, outcome := range []byte{'1', 'X', '2'} {
			if strings.ContainsRune(picks, rune(outcome)) {
				selected.WriteByte(outcome)
			}
		}
		if selected.Len() == 0 {
			return nil, 0, "every game needs a selection"
		}
		if combinations > maxCombinations/selected.Len() {
			return nil, 0, fmt.Sprintf("a coupon can have at most %d combinations", maxCombinations)
		}
		combinations *= selected.Len()
		system[i] = selected.String()
	}
	return system, combinations, ""
}

func validateCoupons(coupons []string, gameCount int) string {
	if len(coupons) == 0 || len(coupons) > maxCombinations {
		return fmt.Sprintf("an entry must contain between 1 and %d combinations", maxCombinations)
	}
	for _, c := range coupons {
		if len(c) != gameCount {
			return "every coupon needs exactly one pick per game"
		}
		for i := 0; i < len(c); i++ {
			if !validOutcomes[c[i]] {
				return "picks must be 1, X or 2"
			}
		}
	}
	return ""
}

func (a *App) handleCreateEntry(w http.ResponseWriter, r *http.Request) {
	e, ok := a.loadEvent(w, r)
	if !ok {
		return
	}
	var req struct {
		System    []string `json:"system"`
		Coupons   []string `json:"coupons"`
		Signature string   `json:"signature"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if e.hasVault {
		writeError(w, http.StatusConflict, "this event takes entries only through its on-chain vault")
		return
	}
	if e.Status != "open" {
		writeError(w, http.StatusConflict, "this event is not open for entries")
		return
	}
	var system []string
	combinations := len(req.Coupons)
	if len(req.System) > 0 {
		var msg string
		system, combinations, msg = canonicalizeSystem(req.System, len(e.Games))
		if msg != "" {
			writeError(w, http.StatusBadRequest, msg)
			return
		}
	} else if msg := validateCoupons(req.Coupons, len(e.Games)); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	if n, _ := a.entries.CountDocuments(ctx, bson.M{"signature": req.Signature}); n > 0 {
		writeError(w, http.StatusConflict, "this payment has already been used")
		return
	}

	wallet := walletFrom(r)
	lamports := e.CouponPriceLamports * uint64(combinations)
	memo := paymentMemo(e.ID.Hex(), system)
	if err := a.verifyPayment(ctx, req.Signature, wallet, a.cfg.TreasuryWallet, lamports, memo); err != nil {
		writeError(w, http.StatusPaymentRequired, err.Error())
		return
	}
	payment := newPaymentRecord(req.Signature, wallet, e.ID, system, combinations, lamports, memo, paymentProcessing)
	if err := a.reservePayment(ctx, payment); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			writeError(w, http.StatusConflict, "this payment has already been used")
		} else {
			writeError(w, http.StatusConflict, err.Error())
		}
		return
	}

	// The reservation above is visible to handleSetChain, which refuses events with payments.
	// Checking again here covers a vault attached while this payment was being verified.
	if n, err := a.events.CountDocuments(ctx, bson.M{"_id": e.ID, "chain": bson.M{"$exists": true}}); err != nil || n > 0 {
		_ = a.setPaymentStatus(ctx, req.Signature, paymentRecoveryPending, "the event moved to an on-chain vault while the payment was being verified")
		writeError(w, http.StatusConflict, "this event now takes entries only through its on-chain vault; your payment is in the refund queue")
		return
	}
	now := time.Now()
	if !now.Before(e.StartTime) {
		_ = a.setPaymentStatus(ctx, req.Signature, paymentRecoveryPending, "entries closed while the payment was being verified")
		writeError(w, http.StatusConflict, "entries closed while the payment was being verified; request a payment recovery")
		return
	}
	coupons := req.Coupons
	if system != nil {
		coupons = nil
	}
	entry := Entry{
		ID:            bson.NewObjectID(),
		WalletAddress: wallet,
		EventID:       e.ID,
		System:        system,
		Combinations:  combinations,
		Coupons:       coupons,
		Signature:     req.Signature,
		Lamports:      lamports,
		CreatedAt:     now,
	}
	if _, err := a.entries.InsertOne(ctx, entry); err != nil {
		_ = a.setPaymentStatus(ctx, req.Signature, paymentRecoveryPending, "database rejected the entry after payment verification")
		if mongo.IsDuplicateKeyError(err) {
			writeError(w, http.StatusConflict, "this payment has already been used")
		} else {
			writeError(w, http.StatusInternalServerError, "failed to save entry; the payment is in the recovery queue")
		}
		return
	}
	_ = a.setPaymentStatus(ctx, req.Signature, paymentAccepted, "")
	entry.EventTitle = e.Title
	writeJSON(w, http.StatusCreated, entry)
}

func (a *App) handleMyEntries(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	cur, err := a.entries.Find(ctx,
		bson.M{"walletAddress": walletFrom(r)},
		options.Find().SetSort(bson.M{"createdAt": -1}),
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load entries")
		return
	}
	entries := []Entry{}
	if err := cur.All(ctx, &entries); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load entries")
		return
	}

	ids := make([]bson.ObjectID, 0, len(entries))
	for _, en := range entries {
		ids = append(ids, en.EventID)
	}
	events := map[bson.ObjectID]*Event{}
	if len(ids) > 0 {
		evCur, err := a.events.Find(ctx, bson.M{"_id": bson.M{"$in": ids}})
		if err == nil {
			var evs []Event
			if evCur.All(ctx, &evs) == nil {
				now := time.Now()
				for i := range evs {
					a.prepare(&evs[i], now)
					events[evs[i].ID] = &evs[i]
				}
			}
		}
	}
	for i := range entries {
		if e := events[entries[i].EventID]; e != nil {
			entries[i].Event = e
			entries[i].EventTitle = e.Title
			entries[i].score(e)
		}
	}
	writeJSON(w, http.StatusOK, entries)
}

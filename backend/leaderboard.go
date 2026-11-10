package main

import (
	"context"
	"encoding/hex"
	"log"
	"net/http"
	"sort"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// A public ranking of settled winnings. Picks are not included.
type LeaderRow struct {
	Wallet           string `json:"wallet"`
	Name             string `json:"name"`
	WinningsLamports uint64 `json:"winningsLamports"`
	Coupons          int    `json:"coupons"`
	BestCorrect      int    `json:"bestCorrect"`
	GameCount        int    `json:"gameCount"`
	EventTitle       string `json:"eventTitle"`
}

type leaderAcc struct {
	LeaderRow
	topWin uint64
}

func (en Entry) bestCorrect() int {
	best := 0
	for _, band := range en.Scores {
		if band.Combinations > 0 && band.Correct > best {
			best = band.Correct
		}
	}
	for _, result := range en.Results {
		if result.Correct > best {
			best = result.Correct
		}
	}
	return best
}

// addLeaderEntry folds one scored coupon into the wallet totals.
// EventTitle is the round where this wallet won the most.
func addLeaderEntry(rows map[string]*leaderAcc, en Entry, e *Event) {
	if len(en.Scores) > 0 && len(en.System) == 0 && len(en.Coupons) == 0 {
		if e.Settlement != nil {
			for i, band := range en.Scores {
				won := e.Settlement.payoutFor(band.Correct) * uint64(band.Combinations)
				en.Scores[i].WinningsLamports = won
				en.WinningsLamports += won
			}
		}
		addLeaderScored(rows, en, e)
		return
	}
	en.score(e)
	addLeaderScored(rows, en, e)
}

func addLeaderScored(rows map[string]*leaderAcc, en Entry, e *Event) {
	if en.WalletAddress == "" {
		return
	}
	acc := rows[en.WalletAddress]
	if acc == nil {
		acc = &leaderAcc{LeaderRow: LeaderRow{Wallet: en.WalletAddress, GameCount: e.GameCount}}
		rows[en.WalletAddress] = acc
	}
	acc.Coupons++
	acc.WinningsLamports += en.WinningsLamports
	if best := en.bestCorrect(); best > acc.BestCorrect {
		acc.BestCorrect = best
		acc.GameCount = e.GameCount
	}
	if en.WinningsLamports > acc.topWin {
		acc.topWin = en.WinningsLamports
		acc.EventTitle = e.Title
	}
}

// scoreLeaderEvent ranks one settled event. A vault resolved before its settlement was
// stored still has the winner table on the account, and its revealed slips in the archive.
func (a *App) scoreLeaderEvent(ctx context.Context, e *Event, rows map[string]*leaderAcc) {
	if e == nil {
		return
	}
	played := a.leaderEntries(ctx, e)
	settlement := e.Settlement
	if settlement == nil {
		settlement = a.chainSettlement(ctx, e)
	}
	if settlement == nil && len(played) > 0 && !e.League {
		tiers := e.Tiers
		if len(tiers) == 0 {
			tiers = defaultTiers(e.GameCount)
		}
		var sales uint64
		for _, en := range played {
			sales += en.Lamports
		}
		built := settleEntries(played, e.results(), sales, e.RolloverInLamports, tiers)
		settlement = &built
	}
	if settlement == nil {
		return
	}
	scored := *e
	scored.Settlement = settlement
	for _, en := range played {
		addLeaderEntry(rows, en, &scored)
	}
}

func (a *App) chainSettlement(ctx context.Context, e *Event) *Settlement {
	if e.Chain == nil || e.Chain.Address == "" || a.fetchAccount == nil {
		return nil
	}
	account, err := a.fetchAccount(ctx, e.Chain.Address)
	if err != nil || account == nil {
		log.Printf("leaderboard vault %s: %v", e.ID.Hex(), err)
		return nil
	}
	return payoutSettlement(account.Data, e.GameCount)
}

func (a *App) leaderEntries(ctx context.Context, e *Event) []Entry {
	var played []Entry
	if loaded, err := a.loadEventEntries(ctx, e.ID); err != nil {
		log.Printf("leaderboard entries %s: %v", e.ID.Hex(), err)
	} else {
		played = append(played, loaded...)
	}
	reveals := a.revealEntries(ctx, e)
	played = append(played, reveals...)
	if len(reveals) == 0 {
		played = append(played, a.savedCouponEntries(ctx, e)...)
	}
	return played
}

func (a *App) revealEntries(ctx context.Context, e *Event) []Entry {
	if a.slips == nil || e.Chain == nil || e.Chain.Address == "" {
		return nil
	}
	cursor, err := a.slips.Find(ctx, bson.M{"event": e.Chain.Address, "kind": "reveal"})
	if err != nil {
		log.Printf("leaderboard slips %s: %v", e.ID.Hex(), err)
		return nil
	}
	defer cursor.Close(ctx)
	var records []slipRecord
	if err := cursor.All(ctx, &records); err != nil {
		log.Printf("leaderboard slips %s: %v", e.ID.Hex(), err)
		return nil
	}
	results := e.results()
	out := make([]Entry, 0, len(records))
	for _, record := range records {
		if record.Owner == "" || record.Picks == "" {
			continue
		}
		picks, err := hex.DecodeString(record.Picks)
		if err != nil || len(picks) < e.GameCount || e.GameCount == 0 {
			continue
		}
		picks = picks[:e.GameCount]
		colors, err := parseColorHex(record.Colors)
		if record.Colors != "" && err != nil {
			colors = nil
		}
		if len(colors) > 0 {
			if en, ok := coloredEntry(record.Owner, picks, colors, results); ok {
				out = append(out, en)
			}
			continue
		}
		system, rows := systemFromMask(picks)
		if rows == 0 {
			continue
		}
		out = append(out, Entry{WalletAddress: record.Owner, System: system, Combinations: rows})
	}
	return out
}

// coloredEntry carries the rows a reduction kept, so score does not pay the rows it removed.
func coloredEntry(wallet string, picks []byte, colors []slipColor, results string) (Entry, bool) {
	kept := keptCounts(picks, colors, results)
	if len(kept) == 0 {
		return Entry{}, false
	}
	en := Entry{WalletAddress: wallet}
	for correct, n := range kept {
		if n == 0 {
			continue
		}
		en.Scores = append(en.Scores, ScoreBand{Correct: correct, Combinations: int(n)})
		en.Combinations += int(n)
	}
	if en.Combinations == 0 {
		return Entry{}, false
	}
	return en, true
}

func (a *App) savedCouponEntries(ctx context.Context, e *Event) []Entry {
	if a.coupons == nil {
		return nil
	}
	cursor, err := a.coupons.Find(ctx, bson.M{"coupon.eventId": e.ID.Hex()})
	if err != nil {
		log.Printf("leaderboard coupons %s: %v", e.ID.Hex(), err)
		return nil
	}
	defer cursor.Close(ctx)
	var docs []walletCouponDoc
	if err := cursor.All(ctx, &docs); err != nil {
		return nil
	}
	out := make([]Entry, 0, len(docs))
	for _, doc := range docs {
		if doc.Wallet == "" || len(doc.Coupon.System) == 0 {
			continue
		}
		out = append(out, Entry{
			WalletAddress: doc.Wallet,
			System:        doc.Coupon.System,
			Combinations:  int(doc.Coupon.Combinations),
			Lamports:      doc.Coupon.Lamports,
		})
	}
	return out
}

// leaderboardRows keeps wallets that won something, highest winnings first.
func leaderboardRows(rows map[string]*leaderAcc) []LeaderRow {
	out := make([]LeaderRow, 0, len(rows))
	for _, acc := range rows {
		if acc.WinningsLamports == 0 {
			continue
		}
		out = append(out, acc.LeaderRow)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].WinningsLamports != out[j].WinningsLamports {
			return out[i].WinningsLamports > out[j].WinningsLamports
		}
		return out[i].Wallet < out[j].Wallet
	})
	return out
}

func leaderSpan(span string, now time.Time) (start time.Time, round bool, ok bool) {
	switch span {
	case "round":
		return time.Time{}, true, true
	case "week":
		return now.AddDate(0, 0, -7), false, true
	case "month":
		return now.AddDate(0, -1, 0), false, true
	case "all":
		return time.Time{}, false, true
	default:
		return time.Time{}, false, false
	}
}

func eventInSpan(e *Event, start time.Time, round bool, latest bson.ObjectID) bool {
	if e.Settlement == nil && e.ResultsSetAt == nil {
		return false
	}
	if round {
		return e.ID == latest
	}
	if start.IsZero() {
		return true
	}
	when := e.EndTime
	if e.ResultsSetAt != nil {
		when = *e.ResultsSetAt
	}
	return !when.Before(start)
}

func (a *App) handleLeaderboard(w http.ResponseWriter, r *http.Request) {
	span := r.URL.Query().Get("span")
	if span == "" {
		span = "week"
	}
	start, round, ok := leaderSpan(span, time.Now())
	if !ok {
		writeError(w, http.StatusBadRequest, "span must be round, week, month or all")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	cur, err := a.events.Find(ctx, bson.M{"$or": []bson.M{
		{"settlement": bson.M{"$exists": true}},
		{"resultsSetAt": bson.M{"$exists": true}},
	}})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load events")
		return
	}
	var events []Event
	if err := cur.All(ctx, &events); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load events")
		return
	}

	var latest bson.ObjectID
	var latestAt time.Time
	for i := range events {
		if events[i].Settlement == nil && events[i].ResultsSetAt == nil {
			continue
		}
		when := events[i].EndTime
		if events[i].ResultsSetAt != nil {
			when = *events[i].ResultsSetAt
		}
		if latestAt.IsZero() || when.After(latestAt) {
			latestAt = when
			latest = events[i].ID
		}
	}

	ids := make([]bson.ObjectID, 0, len(events))
	byID := map[bson.ObjectID]*Event{}
	for i := range events {
		if !eventInSpan(&events[i], start, round, latest) {
			continue
		}
		ids = append(ids, events[i].ID)
		byID[events[i].ID] = &events[i]
	}

	acc := map[string]*leaderAcc{}
	for _, id := range ids {
		a.scoreLeaderEvent(ctx, byID[id], acc)
	}

	rows := leaderboardRows(acc)
	wallets := make([]string, 0, len(acc))
	for wallet := range acc {
		wallets = append(wallets, wallet)
	}
	if len(wallets) > 0 {
		userCur, err := a.users.Find(ctx, bson.M{"walletAddress": bson.M{"$in": wallets}})
		if err == nil {
			var users []User
			if userCur.All(ctx, &users) == nil {
				names := map[string]string{}
				for _, user := range users {
					names[user.WalletAddress] = user.DisplayName
				}
				for i := range rows {
					rows[i].Name = names[rows[i].Wallet]
				}
			}
		}
	}

	var me any
	if wallet := walletFrom(r); wallet != "" {
		rank := 0
		var mine LeaderRow
		if row := acc[wallet]; row != nil {
			mine = row.LeaderRow
		}
		for i, row := range rows {
			if row.Wallet == wallet {
				rank = i + 1
				mine = row
				break
			}
		}
		me = map[string]any{
			"rank":             rank,
			"winningsLamports": mine.WinningsLamports,
			"coupons":          mine.Coupons,
			"bestCorrect":      mine.BestCorrect,
			"gameCount":        mine.GameCount,
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"span": span, "rows": rows, "me": me})
}

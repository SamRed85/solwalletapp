package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const archiveCursorID = "cursor"

type slipRecord struct {
	ID           string `bson:"_id" json:"-"`
	Event        string `bson:"event" json:"event"`
	Kind         string `bson:"kind" json:"kind"`
	Index        uint32 `bson:"index" json:"index"`
	Owner        string `bson:"owner" json:"owner"`
	Combinations uint32 `bson:"combinations" json:"combinations"`
	Commitment   string `bson:"commitment" json:"commitment"`
	Picks        string `bson:"picks,omitempty" json:"picks,omitempty"`
	Signature    string `bson:"signature" json:"signature"`
}

type archiveCursor struct {
	ID             string `bson:"_id"`
	Tip            string `bson:"tip"`
	BackfillBefore string `bson:"backfillBefore"`
	BackfillDone   bool   `bson:"backfillDone"`
}

type chainSignature struct {
	Signature string `json:"signature"`
	Err       any    `json:"err"`
}

// parseSlipLog reads one program log line. The event is in the line so the archive
// does not depend on the transaction account list. A line that is not a slip is skipped.
func parseSlipLog(line string) (slipRecord, bool) {
	line = strings.TrimPrefix(line, "Program log: ")
	kind, rest, ok := strings.Cut(line, ":")
	if !ok {
		return slipRecord{}, false
	}
	parts := strings.Split(rest, ":")
	var record slipRecord
	switch kind {
	case "slip-v1":
		if len(parts) != 5 {
			return slipRecord{}, false
		}
		record.Kind = "entry"
	case "reveal-v1":
		if len(parts) != 6 {
			return slipRecord{}, false
		}
		record.Kind = "reveal"
		record.Picks = parts[5]
		if _, err := hex.DecodeString(record.Picks); err != nil || len(record.Picks)%2 != 0 {
			return slipRecord{}, false
		}
	case "score-v1", "claim-v1", "refund-v1":
		if len(parts) != 2 {
			return slipRecord{}, false
		}
		record.Kind = strings.TrimSuffix(kind, "-v1")
		index, err := strconv.ParseUint(parts[1], 10, 32)
		if err != nil || parts[0] == "" {
			return slipRecord{}, false
		}
		record.Event = parts[0]
		record.Index = uint32(index)
		record.ID = record.Event + ":" + record.Kind + ":" + parts[1]
		return record, true
	default:
		return slipRecord{}, false
	}
	index, err := strconv.ParseUint(parts[1], 10, 32)
	if err != nil {
		return slipRecord{}, false
	}
	combinations, err := strconv.ParseUint(parts[3], 10, 32)
	if err != nil || combinations == 0 {
		return slipRecord{}, false
	}
	if _, err = hex.DecodeString(parts[4]); err != nil || len(parts[4]) != 64 {
		return slipRecord{}, false
	}
	if parts[0] == "" || parts[2] == "" {
		return slipRecord{}, false
	}
	record.Event = parts[0]
	record.Index = uint32(index)
	record.Owner = parts[2]
	record.Combinations = uint32(combinations)
	record.Commitment = parts[4]
	record.ID = record.Event + ":" + record.Kind + ":" + parts[1]
	return record, true
}

// ingestSlipLogs stores each slip line once. A repeated signature does not create a second row.
func (a *App) ingestSlipLogs(ctx context.Context, signature string, logs []string) (int, error) {
	if a.slips == nil {
		return 0, nil
	}
	stored := 0
	for _, line := range logs {
		record, ok := parseSlipLog(line)
		if !ok {
			continue
		}
		record.Signature = signature
		_, err := a.slips.InsertOne(ctx, record)
		if err != nil {
			if mongo.IsDuplicateKeyError(err) {
				continue
			}
			return stored, err
		}
		stored++
	}
	return stored, nil
}

type slipArchiveView struct {
	Entries []slipRecord `json:"entries"`
	Reveals []slipRecord `json:"reveals"`
	Scores  []slipRecord `json:"scores"`
	Claims  []slipRecord `json:"claims"`
	Refunds []slipRecord `json:"refunds"`
}

func (a *App) slipArchive(ctx context.Context, event string) (slipArchiveView, error) {
	view := slipArchiveView{
		Entries: []slipRecord{}, Reveals: []slipRecord{}, Scores: []slipRecord{}, Claims: []slipRecord{}, Refunds: []slipRecord{},
	}
	if a.slips == nil {
		return view, nil
	}
	cursor, err := a.slips.Find(ctx, bson.M{"event": event}, options.Find().SetSort(bson.D{{Key: "index", Value: 1}}))
	if err != nil {
		return view, err
	}
	defer cursor.Close(ctx)
	for cursor.Next(ctx) {
		var record slipRecord
		if err = cursor.Decode(&record); err != nil {
			return view, err
		}
		switch record.Kind {
		case "reveal":
			view.Reveals = append(view.Reveals, record)
		case "score":
			view.Scores = append(view.Scores, record)
		case "claim":
			view.Claims = append(view.Claims, record)
		case "refund":
			view.Refunds = append(view.Refunds, record)
		default:
			view.Entries = append(view.Entries, record)
		}
	}
	return view, cursor.Err()
}

func (a *App) handleSlipArchive(w http.ResponseWriter, r *http.Request) {
	event := strings.TrimSpace(r.URL.Query().Get("event"))
	if event == "" {
		writeError(w, http.StatusBadRequest, "event is required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	view, err := a.slipArchive(ctx, event)
	if err != nil {
		writeError(w, http.StatusBadGateway, "could not read the slip archive")
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (a *App) handleIngestSlip(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Signature string `json:"signature"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Signature) == "" {
		writeError(w, http.StatusBadRequest, "signature is required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	logs, err := a.rpc.transactionLogs(ctx, strings.TrimSpace(body.Signature), "confirmed")
	if err != nil || len(logs) == 0 {
		writeError(w, http.StatusBadGateway, "the transaction could not be read yet")
		return
	}
	if _, err = a.ingestSlipLogs(ctx, body.Signature, logs); err != nil {
		writeError(w, http.StatusBadGateway, "could not store the slip")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stored"})
}

func (a *App) archiveLoop() {
	if a.slips == nil || a.rpc == nil || a.cfg.ProgramID == "" {
		return
	}
	a.archiveOnce()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		a.archiveOnce()
	}
}

func (a *App) archiveOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cursor, err := a.loadArchiveCursor(ctx)
	if err != nil {
		log.Printf("slip archive cursor: %v", err)
		return
	}
	fresh, oldest, err := a.signaturesSince(ctx, cursor.Tip)
	if err != nil {
		log.Printf("slip archive signatures: %v", err)
		return
	}
	for i := len(fresh) - 1; i >= 0; i-- {
		if fresh[i].Err != nil {
			continue
		}
		logs, err := a.rpc.transactionLogs(ctx, fresh[i].Signature, "finalized")
		if err != nil {
			log.Printf("slip archive transaction: %v", err)
			return
		}
		if _, err = a.ingestSlipLogs(ctx, fresh[i].Signature, logs); err != nil {
			log.Printf("slip archive save: %v", err)
			return
		}
	}
	if len(fresh) > 0 {
		cursor.Tip = fresh[0].Signature
	}
	if !cursor.BackfillDone && oldest != "" {
		if cursor.BackfillBefore == "" {
			cursor.BackfillBefore = oldest
		}
		older, err := a.rpc.signaturesForAddress(ctx, a.cfg.ProgramID, cursor.BackfillBefore)
		if err != nil {
			log.Printf("slip archive backfill: %v", err)
			return
		}
		for i := len(older) - 1; i >= 0; i-- {
			if older[i].Err != nil {
				continue
			}
			logs, err := a.rpc.transactionLogs(ctx, older[i].Signature, "finalized")
			if err != nil {
				log.Printf("slip archive transaction: %v", err)
				return
			}
			if _, err = a.ingestSlipLogs(ctx, older[i].Signature, logs); err != nil {
				log.Printf("slip archive save: %v", err)
				return
			}
		}
		if len(older) < 25 {
			cursor.BackfillDone = true
		} else {
			cursor.BackfillBefore = older[len(older)-1].Signature
		}
	}
	if err := a.saveArchiveCursor(ctx, cursor); err != nil {
		log.Printf("slip archive cursor: %v", err)
	}
}

func (a *App) loadArchiveCursor(ctx context.Context) (archiveCursor, error) {
	var cursor archiveCursor
	err := a.slips.FindOne(ctx, bson.M{"_id": archiveCursorID}).Decode(&cursor)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return archiveCursor{ID: archiveCursorID}, nil
	}
	return cursor, err
}

func (a *App) saveArchiveCursor(ctx context.Context, cursor archiveCursor) error {
	cursor.ID = archiveCursorID
	_, err := a.slips.ReplaceOne(ctx, bson.M{"_id": archiveCursorID}, cursor, options.Replace().SetUpsert(true))
	return err
}

// signaturesSince returns signatures newer than tip, newest first, and the oldest signature on the first page.
func (a *App) signaturesSince(ctx context.Context, tip string) (fresh []chainSignature, oldest string, err error) {
	before := ""
	for pages := 0; pages < 5; pages++ {
		page, err := a.rpc.signaturesForAddress(ctx, a.cfg.ProgramID, before)
		if err != nil {
			return nil, "", err
		}
		if len(page) == 0 {
			break
		}
		if oldest == "" {
			oldest = page[len(page)-1].Signature
		}
		for _, sig := range page {
			if tip != "" && sig.Signature == tip {
				return fresh, oldest, nil
			}
			fresh = append(fresh, sig)
		}
		if tip == "" {
			break
		}
		before = page[len(page)-1].Signature
	}
	return fresh, oldest, nil
}

func (s *solanaRPC) signaturesForAddress(ctx context.Context, address, before string) ([]chainSignature, error) {
	opts := map[string]any{"limit": 25, "commitment": "finalized"}
	if before != "" {
		opts["before"] = before
	}
	var sigs []chainSignature
	err := s.call(ctx, "getSignaturesForAddress", []any{address, opts}, &sigs)
	return sigs, err
}

func (s *solanaRPC) transactionLogs(ctx context.Context, signature, commitment string) ([]string, error) {
	if commitment == "" {
		commitment = "finalized"
	}
	var tx *struct {
		Meta *struct {
			Err         any      `json:"err"`
			LogMessages []string `json:"logMessages"`
		} `json:"meta"`
	}
	err := s.call(ctx, "getTransaction", []any{signature, map[string]any{
		"encoding":                       "json",
		"commitment":                     commitment,
		"maxSupportedTransactionVersion": 0,
	}}, &tx)
	if err != nil || tx == nil || tx.Meta == nil || tx.Meta.Err != nil {
		return nil, err
	}
	return tx.Meta.LogMessages, nil
}

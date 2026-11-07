package main

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	maxReductionPieces = 500
	sealedHexLen       = 42
	commitmentHexLen   = 64
)

// A reduction group is the browser's plan for one colored coupon. The picks are
// sealed before they arrive. This collection never stores them in the clear, and
// it is not the chain archive.
type reductionPiece struct {
	Commitment   string `json:"commitment" bson:"commitment"`
	Sealed       string `json:"sealed" bson:"sealed"`
	Combinations uint32 `json:"combinations" bson:"combinations"`
	Status       string `json:"status" bson:"status"`
	Signature    string `json:"signature,omitempty" bson:"signature,omitempty"`
	SlipIndex    uint32 `json:"slipIndex,omitempty" bson:"slipIndex,omitempty"`
}

type reductionGroup struct {
	ID        bson.ObjectID    `json:"id" bson:"_id"`
	Owner     string           `json:"owner" bson:"owner"`
	EventID   string           `json:"eventId" bson:"eventId"`
	Pieces    []reductionPiece `json:"pieces" bson:"pieces"`
	CreatedAt time.Time        `json:"createdAt" bson:"createdAt"`
	UpdatedAt time.Time        `json:"updatedAt" bson:"updatedAt"`
}

func validateReductionPieces(pieces []reductionPiece) error {
	if len(pieces) < 1 || len(pieces) > maxReductionPieces {
		return errors.New("a reduction must contain between 1 and 500 coupons")
	}
	seen := make(map[string]struct{}, len(pieces))
	for _, piece := range pieces {
		if piece.Combinations < 1 || piece.Combinations > 44000 {
			return errors.New("each coupon must be between 1 and 44,000 rows")
		}
		if len(piece.Commitment) != commitmentHexLen || len(piece.Sealed) != sealedHexLen {
			return errors.New("a coupon backup has the wrong size")
		}
		if _, err := hex.DecodeString(piece.Commitment); err != nil {
			return errors.New("a coupon commitment is not hex")
		}
		if _, err := hex.DecodeString(piece.Sealed); err != nil {
			return errors.New("a coupon backup is not hex")
		}
		if _, ok := seen[piece.Commitment]; ok {
			return errors.New("a reduction repeats a coupon")
		}
		seen[piece.Commitment] = struct{}{}
		if piece.Status != "" || piece.Signature != "" {
			return errors.New("a new reduction has no signatures yet")
		}
	}
	return nil
}

func validSlipSignature(signature string) bool {
	raw, err := base58Decode(signature)
	return err == nil && len(raw) == ed25519.SignatureSize
}

func (a *App) handleCreateReduction(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EventID string           `json:"eventId"`
		Pieces  []reductionPiece `json:"pieces"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := validateReductionPieces(req.Pieces); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	eventID, err := bson.ObjectIDFromHex(req.EventID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid event id")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	event, err := a.findEvent(ctx, eventID)
	if errors.Is(err, mongo.ErrNoDocuments) {
		writeError(w, http.StatusNotFound, "event not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load event")
		return
	}
	if event.Chain == nil || event.Status != "open" {
		writeError(w, http.StatusBadRequest, "color reduction is only for an open vault event")
		return
	}
	owner := walletFrom(r)
	count, err := a.reductions.CountDocuments(ctx, bson.M{"owner": owner, "eventId": req.EventID, "pieces.status": "pending"})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save the reduction")
		return
	}
	if count > 0 {
		writeError(w, http.StatusConflict, "this event already has a reduction waiting to be signed")
		return
	}
	now := time.Now().UTC()
	for i := range req.Pieces {
		req.Pieces[i].Status = "pending"
	}
	group := reductionGroup{
		ID:        bson.NewObjectID(),
		Owner:     owner,
		EventID:   req.EventID,
		Pieces:    req.Pieces,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if _, err = a.reductions.InsertOne(ctx, group); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save the reduction")
		return
	}
	writeJSON(w, http.StatusCreated, group)
}

func (a *App) handleListReductions(w http.ResponseWriter, r *http.Request) {
	eventID := r.URL.Query().Get("eventId")
	if _, err := bson.ObjectIDFromHex(eventID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid event id")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	cursor, err := a.reductions.Find(ctx, bson.M{"owner": walletFrom(r), "eventId": eventID}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load reductions")
		return
	}
	var groups []reductionGroup
	if err = cursor.All(ctx, &groups); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load reductions")
		return
	}
	if groups == nil {
		groups = []reductionGroup{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"reductions": groups})
}

func (a *App) handleMarkReduction(w http.ResponseWriter, r *http.Request) {
	id, err := bson.ObjectIDFromHex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid reduction id")
		return
	}
	var req struct {
		Piece     uint32 `json:"piece"`
		Signature string `json:"signature"`
		SlipIndex uint32 `json:"slipIndex"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if !validSlipSignature(req.Signature) {
		writeError(w, http.StatusBadRequest, "invalid signature")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	var group reductionGroup
	err = a.reductions.FindOne(ctx, bson.M{"_id": id, "owner": walletFrom(r)}).Decode(&group)
	if errors.Is(err, mongo.ErrNoDocuments) {
		writeError(w, http.StatusNotFound, "reduction not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load the reduction")
		return
	}
	if int(req.Piece) >= len(group.Pieces) {
		writeError(w, http.StatusBadRequest, "that coupon is not in the reduction")
		return
	}
	current := group.Pieces[req.Piece]
	if current.Status == "signed" {
		if current.Signature == req.Signature && current.SlipIndex == req.SlipIndex {
			writeJSON(w, http.StatusOK, group)
			return
		}
		writeError(w, http.StatusConflict, "that coupon is already signed")
		return
	}
	group.Pieces[req.Piece].Status = "signed"
	group.Pieces[req.Piece].Signature = req.Signature
	group.Pieces[req.Piece].SlipIndex = req.SlipIndex
	group.UpdatedAt = time.Now().UTC()
	if _, err = a.reductions.ReplaceOne(ctx, bson.M{"_id": id, "owner": group.Owner}, group); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update the reduction")
		return
	}
	writeJSON(w, http.StatusOK, group)
}

func (a *App) handleDeleteReduction(w http.ResponseWriter, r *http.Request) {
	id, err := bson.ObjectIDFromHex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid reduction id")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	result, err := a.reductions.DeleteOne(ctx, bson.M{"_id": id, "owner": walletFrom(r)})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete the reduction")
		return
	}
	if result.DeletedCount == 0 {
		writeError(w, http.StatusNotFound, "reduction not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

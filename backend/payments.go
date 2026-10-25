package main

import (
	"context"
	"errors"
	"net/http"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	paymentProcessing      = "processing"
	paymentAccepted        = "accepted"
	paymentRecoveryPending = "recovery_pending"
	paymentRefunded        = "refunded"
)

type PaymentRecord struct {
	Signature       string        `json:"signature" bson:"_id"`
	WalletAddress   string        `json:"walletAddress" bson:"walletAddress"`
	EventID         bson.ObjectID `json:"eventId" bson:"eventId"`
	System          []string      `json:"system" bson:"system"`
	Combinations    int           `json:"combinations" bson:"combinations"`
	Lamports        uint64        `json:"lamports" bson:"lamports"`
	Memo            string        `json:"memo" bson:"memo"`
	Status          string        `json:"status" bson:"status"`
	Reason          string        `json:"reason,omitempty" bson:"reason,omitempty"`
	CreatedAt       time.Time     `json:"createdAt" bson:"createdAt"`
	UpdatedAt       time.Time     `json:"updatedAt" bson:"updatedAt"`
	RefundSignature string        `json:"refundSignature,omitempty" bson:"refundSignature,omitempty"`
	RefundedAt      *time.Time    `json:"refundedAt,omitempty" bson:"refundedAt,omitempty"`
	Event           *Event        `json:"event,omitempty" bson:"-"`
}

func newPaymentRecord(signature, wallet string, eventID bson.ObjectID, system []string, combinations int, lamports uint64, memo, status string) PaymentRecord {
	now := time.Now()
	return PaymentRecord{
		Signature:     signature,
		WalletAddress: wallet,
		EventID:       eventID,
		System:        system,
		Combinations:  combinations,
		Lamports:      lamports,
		Memo:          memo,
		Status:        status,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

func (a *App) reservePayment(ctx context.Context, payment PaymentRecord) error {
	_, err := a.payments.InsertOne(ctx, payment)
	if mongo.IsDuplicateKeyError(err) {
		return errors.New("this payment is already accepted, recovering, or being processed")
	}
	return err
}

func (a *App) setPaymentStatus(ctx context.Context, signature, status, reason string) error {
	_, err := a.payments.UpdateByID(ctx, signature, bson.M{"$set": bson.M{
		"status": status, "reason": reason, "updatedAt": time.Now(),
	}})
	return err
}

// handleCreatePaymentRecovery records a finalized transfer that did not become an entry.
// Historical payments are allowed: recovery does not have the one-hour entry limit.
func (a *App) handleCreatePaymentRecovery(w http.ResponseWriter, r *http.Request) {
	e, ok := a.loadEvent(w, r)
	if !ok {
		return
	}
	var req struct {
		System    []string `json:"system"`
		Signature string   `json:"signature"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	system, combinations, msg := canonicalizeSystem(req.System, len(e.Games))
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if n, _ := a.entries.CountDocuments(ctx, bson.M{"signature": req.Signature}); n > 0 {
		writeError(w, http.StatusConflict, "this payment already has an accepted entry")
		return
	}

	wallet := walletFrom(r)
	lamports := e.CouponPriceLamports * uint64(combinations)
	memo := paymentMemo(e.ID.Hex(), system)
	if err := a.verifyRecoveryPayment(ctx, req.Signature, wallet, a.cfg.TreasuryWallet, lamports, memo); err != nil {
		writeError(w, http.StatusPaymentRequired, err.Error())
		return
	}

	// A recovery only ever leads to a refund, so it stays open for vault events: a treasury
	// transfer made for one is returned, never turned into an entry.
	payment := newPaymentRecord(req.Signature, wallet, e.ID, system, combinations, lamports, memo, paymentRecoveryPending)
	payment.Reason = "payment finalized but no entry was accepted"
	if e.hasVault {
		payment.Reason = "treasury transfer for an on-chain vault event; refund only"
	}
	if _, err := a.payments.InsertOne(ctx, payment); err != nil {
		if !mongo.IsDuplicateKeyError(err) {
			writeError(w, http.StatusInternalServerError, "failed to record payment recovery")
			return
		}
		if err := a.payments.FindOne(ctx, bson.M{"_id": req.Signature}).Decode(&payment); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load payment recovery")
			return
		}
		if payment.WalletAddress != wallet || payment.EventID != e.ID || payment.Memo != memo {
			writeError(w, http.StatusConflict, "this payment is already bound to different entry details")
			return
		}
		if payment.Status == paymentAccepted {
			writeError(w, http.StatusConflict, "this payment already has an accepted entry")
			return
		}
		if payment.Status == paymentProcessing {
			if time.Since(payment.UpdatedAt) < time.Minute {
				writeError(w, http.StatusConflict, "this payment is still being processed; retry shortly")
				return
			}
			if err := a.setPaymentStatus(ctx, payment.Signature, paymentRecoveryPending, "payment processing was interrupted before entry acceptance"); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to move payment into recovery")
				return
			}
			payment.Status = paymentRecoveryPending
			payment.Reason = "payment processing was interrupted before entry acceptance"
			payment.UpdatedAt = time.Now()
		}
	}
	writeJSON(w, http.StatusOK, payment)
}

func (a *App) handleListPaymentRecoveries(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	cur, err := a.payments.Find(ctx,
		bson.M{"status": paymentRecoveryPending},
		options.Find().SetSort(bson.M{"createdAt": 1}),
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load payment recoveries")
		return
	}
	recoveries := []PaymentRecord{}
	if err := cur.All(ctx, &recoveries); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load payment recoveries")
		return
	}
	writeJSON(w, http.StatusOK, recoveries)
}

func (a *App) handleRefundPaymentRecovery(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefundSignature string `json:"refundSignature"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	var payment PaymentRecord
	if err := a.payments.FindOne(ctx, bson.M{
		"_id": r.PathValue("signature"), "status": paymentRecoveryPending,
	}).Decode(&payment); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			writeError(w, http.StatusNotFound, "pending payment recovery not found")
		} else {
			writeError(w, http.StatusInternalServerError, "failed to load payment recovery")
		}
		return
	}
	if err := a.verifyRefund(ctx, req.RefundSignature, payment.Signature, a.cfg.TreasuryWallet, payment.WalletAddress, payment.Lamports); err != nil {
		writeError(w, http.StatusPaymentRequired, err.Error())
		return
	}
	now := time.Now()
	res, err := a.payments.UpdateOne(ctx,
		bson.M{"_id": payment.Signature, "status": paymentRecoveryPending},
		bson.M{"$set": bson.M{
			"status": paymentRefunded, "refundSignature": req.RefundSignature,
			"refundedAt": now, "updatedAt": now,
		}},
	)
	if mongo.IsDuplicateKeyError(err) {
		writeError(w, http.StatusConflict, "this refund transaction is already recorded")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to record refund")
		return
	}
	if res.MatchedCount == 0 {
		writeError(w, http.StatusConflict, "payment recovery is no longer pending")
		return
	}
	payment.Status = paymentRefunded
	payment.RefundSignature = req.RefundSignature
	payment.RefundedAt = &now
	payment.UpdatedAt = now
	writeJSON(w, http.StatusOK, payment)
}

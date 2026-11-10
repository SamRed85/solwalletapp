package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const maxCouponBatch = 50

// A wallet coupon is the row the browser used to keep in localStorage: the picks,
// the reveal nonce, and the chain address. It is stored for the signed-in wallet
// so the same wallet sees it on another device. The reveal signing key is not stored.
type storedColor struct {
	Signs string `json:"signs" bson:"signs"`
	Min   uint8  `json:"min" bson:"min"`
	Max   uint8  `json:"max" bson:"max"`
	ID    string `json:"id,omitempty" bson:"id,omitempty"`
}

type storedCoupon struct {
	Signature    string        `json:"signature,omitempty" bson:"signature,omitempty"`
	ProgramID    string        `json:"programId" bson:"programId"`
	EventID      string        `json:"eventId,omitempty" bson:"eventId,omitempty"`
	EventTitle   string        `json:"eventTitle,omitempty" bson:"eventTitle,omitempty"`
	Admin        string        `json:"admin,omitempty" bson:"admin,omitempty"`
	Nonce        uint64        `json:"nonce" bson:"nonce"`
	Index        uint32        `json:"index" bson:"index"`
	Slip         bool          `json:"slip" bson:"slip"`
	Address      string        `json:"address" bson:"address"`
	RevealNonce  string        `json:"revealNonce,omitempty" bson:"revealNonce,omitempty"`
	Commitment   string        `json:"commitment,omitempty" bson:"commitment,omitempty"`
	System       []string      `json:"system,omitempty" bson:"system,omitempty"`
	Combinations uint32        `json:"combinations" bson:"combinations"`
	Lamports     uint64        `json:"lamports" bson:"lamports"`
	PlacedAt     int64         `json:"placedAt" bson:"placedAt"`
	Revealed     bool          `json:"revealed" bson:"revealed"`
	Colors       []storedColor `json:"colors,omitempty" bson:"colors,omitempty"`
	ArmError     string        `json:"armError,omitempty" bson:"armError,omitempty"`
	Claimed      bool          `json:"claimed,omitempty" bson:"claimed,omitempty"`
	Refunded     bool          `json:"refunded,omitempty" bson:"refunded,omitempty"`
}

type walletCouponDoc struct {
	Wallet   string       `bson:"walletAddress"`
	CouponID string       `bson:"couponId"`
	PlacedAt int64        `bson:"placedAt"`
	Coupon   storedCoupon `bson:"coupon"`
	Updated  time.Time    `bson:"updatedAt"`
}

func validCouponID(id string) bool {
	if id == "" || len(id) > 160 {
		return false
	}
	for _, r := range id {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func validDecimal(value string, maxLen int) bool {
	if value == "" || len(value) > maxLen {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func validSystemGame(game string) bool {
	if game == "" || len(game) > 3 {
		return false
	}
	seen := 0
	for _, r := range game {
		bit := 0
		switch r {
		case '1':
			bit = 1
		case 'X':
			bit = 2
		case '2':
			bit = 4
		default:
			return false
		}
		if seen&bit != 0 {
			return false
		}
		seen |= bit
	}
	return true
}

func printable(value string, maxLen int) bool {
	if len(value) > maxLen {
		return false
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func validateStoredCoupon(coupon storedCoupon, programID string) error {
	if coupon.ProgramID == "" || (programID != "" && coupon.ProgramID != programID) {
		return errors.New("this coupon belongs to a different program")
	}
	if !validCouponID(coupon.Address) {
		return errors.New("a coupon needs an address")
	}
	if coupon.Signature != "" && !validSlipSignature(coupon.Signature) {
		return errors.New("a coupon signature is not a transaction signature")
	}
	if !printable(coupon.EventID, 64) || !printable(coupon.EventTitle, 160) || !printable(coupon.Admin, 64) || !printable(coupon.ArmError, 300) {
		return errors.New("a coupon field is too long")
	}
	if coupon.RevealNonce != "" && !validDecimal(coupon.RevealNonce, 20) {
		return errors.New("a reveal nonce must be a number")
	}
	if coupon.Commitment != "" && (len(coupon.Commitment) != 64 || !validHex(coupon.Commitment)) {
		return errors.New("a coupon commitment is not hex")
	}
	if len(coupon.System) > maxGames {
		return errors.New("a coupon has too many games")
	}
	for _, game := range coupon.System {
		if !validSystemGame(game) {
			return errors.New("a coupon pick must be 1, X, or 2")
		}
	}
	if coupon.Combinations > maxCombinations {
		return errors.New("a coupon has too many rows")
	}
	if len(coupon.Colors) > 4 {
		return errors.New("a coupon can have at most 4 colors")
	}
	for _, color := range coupon.Colors {
		if !validDecimal(color.Signs, 20) || color.Min > color.Max || !printable(color.ID, 40) {
			return errors.New("a color on this coupon is not valid")
		}
	}
	if coupon.PlacedAt < 0 {
		return errors.New("a coupon time is not valid")
	}
	return nil
}

func validHex(value string) bool {
	_, err := hex.DecodeString(value)
	return err == nil
}

func clampPlacedAt(at int64, now time.Time) int64 {
	if at < 0 {
		return 0
	}
	if at > now.UnixMilli()+60_000 {
		return now.UnixMilli()
	}
	return at
}

// The first saved picks win. Later writes can only move a coupon forward
// (revealed, claimed, refunded) or fill fields that were still empty.
func mergeStoredCoupon(prev, next storedCoupon) storedCoupon {
	if prev.Address == "" {
		return next
	}
	if len(prev.System) > 0 {
		next.System = prev.System
		if prev.RevealNonce != "" {
			next.RevealNonce = prev.RevealNonce
		}
		if len(prev.Colors) > 0 {
			next.Colors = prev.Colors
		}
	}
	if next.Signature == "" {
		next.Signature = prev.Signature
	}
	if next.EventID == "" {
		next.EventID = prev.EventID
	}
	if next.EventTitle == "" {
		next.EventTitle = prev.EventTitle
	}
	if next.Admin == "" {
		next.Admin = prev.Admin
	}
	if next.Commitment == "" {
		next.Commitment = prev.Commitment
	}
	if next.ProgramID == "" {
		next.ProgramID = prev.ProgramID
	}
	if next.Nonce == 0 {
		next.Nonce = prev.Nonce
	}
	if next.Index == 0 {
		next.Index = prev.Index
	}
	if next.Combinations == 0 {
		next.Combinations = prev.Combinations
	}
	if next.Lamports == 0 {
		next.Lamports = prev.Lamports
	}
	if prev.PlacedAt > 0 && (next.PlacedAt == 0 || next.PlacedAt > prev.PlacedAt) {
		next.PlacedAt = prev.PlacedAt
	}
	next.Address = prev.Address
	next.Revealed = next.Revealed || prev.Revealed
	next.Claimed = next.Claimed || prev.Claimed
	next.Refunded = next.Refunded || prev.Refunded
	next.Slip = next.Slip || prev.Slip
	return next
}

func decodeLoose(w http.ResponseWriter, r *http.Request, dst any, limit int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return false
		}
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

func (a *App) saveWalletCoupon(ctx context.Context, wallet string, coupon storedCoupon) (storedCoupon, error) {
	coupon.PlacedAt = clampPlacedAt(coupon.PlacedAt, time.Now())
	var existing walletCouponDoc
	err := a.coupons.FindOne(ctx, bson.M{"walletAddress": wallet, "couponId": coupon.Address}).Decode(&existing)
	if err == nil {
		coupon = mergeStoredCoupon(existing.Coupon, coupon)
	} else if !errors.Is(err, mongo.ErrNoDocuments) {
		return storedCoupon{}, err
	}
	_, err = a.coupons.UpdateOne(ctx,
		bson.M{"walletAddress": wallet, "couponId": coupon.Address},
		bson.M{"$set": walletCouponDoc{
			Wallet:   wallet,
			CouponID: coupon.Address,
			PlacedAt: coupon.PlacedAt,
			Coupon:   coupon,
			Updated:  time.Now().UTC(),
		}},
		options.UpdateOne().SetUpsert(true),
	)
	if err != nil {
		return storedCoupon{}, err
	}
	return coupon, nil
}

func (a *App) handleListCoupons(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	cur, err := a.coupons.Find(ctx, bson.M{"walletAddress": walletFrom(r)}, options.Find().SetLimit(2000))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load coupons")
		return
	}
	var docs []walletCouponDoc
	if err := cur.All(ctx, &docs); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load coupons")
		return
	}
	out := make([]storedCoupon, 0, len(docs))
	for _, doc := range docs {
		coupon := doc.Coupon
		if coupon.Address == "" {
			coupon.Address = doc.CouponID
		}
		if coupon.PlacedAt == 0 {
			coupon.PlacedAt = doc.PlacedAt
		}
		out = append(out, coupon)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].PlacedAt != out[j].PlacedAt {
			return out[i].PlacedAt > out[j].PlacedAt
		}
		return out[i].Index > out[j].Index
	})
	writeJSON(w, http.StatusOK, out)
}

func (a *App) handleSaveCoupon(w http.ResponseWriter, r *http.Request) {
	var coupon storedCoupon
	if !decodeLoose(w, r, &coupon, 64<<10) {
		return
	}
	if err := validateStoredCoupon(coupon, a.cfg.ProgramID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	saved, err := a.saveWalletCoupon(ctx, walletFrom(r), coupon)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save coupon")
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (a *App) handleSaveCoupons(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Coupons []storedCoupon `json:"coupons"`
	}
	if !decodeLoose(w, r, &req, 512<<10) {
		return
	}
	if len(req.Coupons) == 0 || len(req.Coupons) > maxCouponBatch {
		writeError(w, http.StatusBadRequest, "send between 1 and 50 coupons")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	wallet := walletFrom(r)
	seen := make(map[string]struct{}, len(req.Coupons))
	saved := 0
	for _, coupon := range req.Coupons {
		if err := validateStoredCoupon(coupon, a.cfg.ProgramID); err != nil {
			continue
		}
		if _, ok := seen[coupon.Address]; ok {
			continue
		}
		seen[coupon.Address] = struct{}{}
		if _, err := a.saveWalletCoupon(ctx, wallet, coupon); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to save coupons")
			return
		}
		saved++
	}
	if saved == 0 {
		writeError(w, http.StatusBadRequest, "no coupon could be saved")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"saved": saved})
}

func (a *App) handleDeleteCoupon(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if !validCouponID(id) {
		writeError(w, http.StatusBadRequest, "missing coupon")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if _, err := a.coupons.DeleteOne(ctx, bson.M{"walletAddress": walletFrom(r), "couponId": id}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete coupon")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

package main

import (
	"context"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	minDisplayNameLen = 2
	maxDisplayNameLen = 32
)

type User struct {
	ID            bson.ObjectID `json:"id" bson:"_id,omitempty"`
	WalletAddress string        `json:"walletAddress" bson:"walletAddress"`
	DisplayName   string        `json:"displayName" bson:"displayName"`
	CreatedAt     time.Time     `json:"createdAt" bson:"createdAt"`
	LastLoginAt   time.Time     `json:"lastLoginAt" bson:"lastLoginAt"`
	IsAdmin       bool          `json:"isAdmin" bson:"-"`
	ActiveCoupons int           `json:"activeCoupons" bson:"-"`
}

func normalizeDisplayName(raw string) (string, string) {
	name := strings.TrimSpace(raw)
	name = strings.Join(strings.Fields(name), " ")
	if n := utf8.RuneCountInString(name); n < minDisplayNameLen || n > maxDisplayNameLen {
		return "", "a profile name must be 2 to 32 characters"
	}
	for _, r := range name {
		if r < 0x20 {
			return "", "a profile name cannot contain control characters"
		}
	}
	return name, ""
}

func (a *App) handleListUsers(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	cur, err := a.users.Find(ctx, bson.M{}, options.Find().SetSort(bson.M{"lastLoginAt": -1}))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load users")
		return
	}
	users := []User{}
	if err := cur.All(ctx, &users); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load users")
		return
	}
	counts, err := a.activeCouponCounts(ctx, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load users")
		return
	}
	for i := range users {
		users[i].IsAdmin = a.cfg.AdminWallets[users[i].WalletAddress]
		users[i].ActiveCoupons = counts[users[i].WalletAddress]
	}
	writeJSON(w, http.StatusOK, users)
}

func (a *App) findUser(ctx context.Context, wallet string) (User, error) {
	var user User
	err := a.users.FindOne(ctx, bson.M{"walletAddress": wallet}).Decode(&user)
	return user, err
}

func (a *App) publicUser(user User) map[string]any {
	return map[string]any{
		"walletAddress": user.WalletAddress,
		"displayName":   user.DisplayName,
		"isAdmin":       a.cfg.AdminWallets[user.WalletAddress],
	}
}

// activeCouponCounts sums combination rows on events whose results are not set yet.
// A nil wallet filter counts every user.
func (a *App) activeCouponCounts(ctx context.Context, wallet *string) (map[string]int, error) {
	filter := bson.M{}
	if wallet != nil {
		filter["walletAddress"] = *wallet
	}
	cur, err := a.entries.Find(ctx, filter, options.Find().SetProjection(bson.M{
		"walletAddress": 1, "eventId": 1, "combinations": 1, "coupons": 1,
	}))
	if err != nil {
		return nil, err
	}
	var entries []Entry
	if err := cur.All(ctx, &entries); err != nil {
		return nil, err
	}
	ids := map[bson.ObjectID]bool{}
	for _, en := range entries {
		ids[en.EventID] = true
	}
	active := map[bson.ObjectID]bool{}
	if len(ids) > 0 {
		list := make([]bson.ObjectID, 0, len(ids))
		for id := range ids {
			list = append(list, id)
		}
		evCur, err := a.events.Find(ctx,
			bson.M{"_id": bson.M{"$in": list}, "resultsSetAt": bson.M{"$exists": false}},
			options.Find().SetProjection(bson.M{"_id": 1}),
		)
		if err != nil {
			return nil, err
		}
		var events []Event
		if err := evCur.All(ctx, &events); err != nil {
			return nil, err
		}
		for _, e := range events {
			active[e.ID] = true
		}
	}
	counts := map[string]int{}
	for _, en := range entries {
		if active[en.EventID] {
			counts[en.WalletAddress] += en.rowCount()
		}
	}
	return counts, nil
}

func (a *App) handleUpdateMe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DisplayName string `json:"displayName"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	name, msg := normalizeDisplayName(req.DisplayName)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	wallet := walletFrom(r)
	res, err := a.users.UpdateOne(ctx, bson.M{"walletAddress": wallet}, bson.M{"$set": bson.M{"displayName": name}})
	if err != nil || res.MatchedCount == 0 {
		writeError(w, http.StatusInternalServerError, "failed to save profile")
		return
	}
	user, err := a.findUser(ctx, wallet)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save profile")
		return
	}
	writeJSON(w, http.StatusOK, a.publicUser(user))
}

func (a *App) handleAdminUpdateUser(w http.ResponseWriter, r *http.Request) {
	wallet := r.PathValue("wallet")
	if !isValidWallet(wallet) {
		writeError(w, http.StatusBadRequest, "invalid wallet address")
		return
	}
	var req struct {
		DisplayName string `json:"displayName"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	name, msg := normalizeDisplayName(req.DisplayName)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if a.rejectIfActiveCoupons(ctx, w, wallet) {
		return
	}
	res, err := a.users.UpdateOne(ctx, bson.M{"walletAddress": wallet}, bson.M{"$set": bson.M{"displayName": name}})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update user")
		return
	}
	if res.MatchedCount == 0 {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	user, err := a.findUser(ctx, wallet)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update user")
		return
	}
	user.IsAdmin = a.cfg.AdminWallets[wallet]
	writeJSON(w, http.StatusOK, user)
}

func (a *App) handleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	wallet := r.PathValue("wallet")
	if !isValidWallet(wallet) {
		writeError(w, http.StatusBadRequest, "invalid wallet address")
		return
	}
	if wallet == walletFrom(r) {
		writeError(w, http.StatusConflict, "you cannot remove your own account")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if a.rejectIfActiveCoupons(ctx, w, wallet) {
		return
	}
	res, err := a.users.DeleteOne(ctx, bson.M{"walletAddress": wallet})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove user")
		return
	}
	if res.DeletedCount == 0 {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// rejectIfActiveCoupons writes the error response and reports whether the caller should stop.
func (a *App) rejectIfActiveCoupons(ctx context.Context, w http.ResponseWriter, wallet string) bool {
	counts, err := a.activeCouponCounts(ctx, &wallet)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check coupons")
		return true
	}
	if counts[wallet] > 0 {
		writeError(w, http.StatusConflict, "this user has coupons on an event that is still in play")
		return true
	}
	return false
}

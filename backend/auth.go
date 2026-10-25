package main

import (
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	nonceTTL      = 5 * time.Minute
	maxNonces     = 10000
	sessionTTL    = 12 * time.Hour
	bearerPrefix  = "Bearer "
	walletContext = contextKey("wallet")
)

type contextKey string

type pendingNonce struct {
	wallet  string
	message string
	expires time.Time
}

type nonceStore struct {
	mu      sync.Mutex
	pending map[string]pendingNonce
	coll    *mongo.Collection
}

func newNonceStore(coll *mongo.Collection) *nonceStore {
	return &nonceStore{pending: map[string]pendingNonce{}, coll: coll}
}

func (s *nonceStore) issue(ctx context.Context, wallet string, now time.Time) (nonce, message string, err error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	nonce = hex.EncodeToString(buf)
	message = loginMessage(wallet, nonce, now)
	expires := now.Add(nonceTTL)
	if s.coll != nil {
		_, err = s.coll.InsertOne(ctx, bson.M{"_id": nonce, "wallet": wallet, "message": message, "expiresAt": expires})
		return nonce, message, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for k, p := range s.pending {
		if now.After(p.expires) {
			delete(s.pending, k)
		}
	}
	if len(s.pending) >= maxNonces {
		return "", "", fmt.Errorf("too many pending sign-in requests")
	}
	s.pending[nonce] = pendingNonce{wallet: wallet, message: message, expires: expires}
	return nonce, message, nil
}

// consume removes the nonce whether or not it is valid, so each one can be tried once.
func (s *nonceStore) consume(ctx context.Context, nonce, wallet string, now time.Time) (string, bool) {
	if s.coll != nil {
		var doc struct {
			Message string `bson:"message"`
		}
		err := s.coll.FindOneAndDelete(ctx, bson.M{
			"_id": nonce, "wallet": wallet, "expiresAt": bson.M{"$gt": now},
		}).Decode(&doc)
		if err != nil {
			return "", false
		}
		return doc.Message, true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.pending[nonce]
	delete(s.pending, nonce)
	if !ok || p.wallet != wallet || now.After(p.expires) {
		return "", false
	}
	return p.message, true
}

func loginMessage(wallet, nonce string, now time.Time) string {
	return fmt.Sprintf("Sign in to Sol (devnet)\n\nWallet: %s\nNonce: %s\nIssued at: %s",
		wallet, nonce, now.UTC().Format(time.RFC3339))
}

func verifyWalletSignature(wallet, message string, signature []byte) bool {
	pub, err := base58Decode(wallet)
	if err != nil || len(pub) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(pub), []byte(message), signature)
}

func isValidWallet(wallet string) bool {
	pub, err := base58Decode(wallet)
	return err == nil && len(pub) == ed25519.PublicKeySize
}

func (a *App) signToken(wallet string, expires time.Time) string {
	payload := wallet + "." + strconv.FormatInt(expires.Unix(), 10)
	mac := hmac.New(sha256.New, a.cfg.SessionSecret)
	mac.Write([]byte(payload))
	enc := base64.RawURLEncoding
	return enc.EncodeToString([]byte(payload)) + "." + enc.EncodeToString(mac.Sum(nil))
}

func (a *App) parseToken(token string, now time.Time) (string, bool) {
	enc := base64.RawURLEncoding
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return "", false
	}
	payload, err1 := enc.DecodeString(parts[0])
	sig, err2 := enc.DecodeString(parts[1])
	if err1 != nil || err2 != nil {
		return "", false
	}
	mac := hmac.New(sha256.New, a.cfg.SessionSecret)
	mac.Write(payload)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return "", false
	}
	wallet, expStr, ok := strings.Cut(string(payload), ".")
	if !ok {
		return "", false
	}
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || now.Unix() >= exp {
		return "", false
	}
	return wallet, true
}

const sessionCookie = "sol_session"

func (a *App) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		wallet, ok := a.authenticatedWallet(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "session expired or invalid")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), walletContext, wallet)))
	}
}

func (a *App) authenticatedWallet(r *http.Request) (string, bool) {
	now := time.Now()
	if a.sessions != nil {
		if cookie, err := r.Cookie(sessionCookie); err == nil && cookie.Value != "" {
			if wallet, ok := a.loadSession(r.Context(), cookie.Value, now); ok {
				return wallet, true
			}
		}
		header := r.Header.Get("Authorization")
		if strings.HasPrefix(header, bearerPrefix) {
			return a.loadSession(r.Context(), strings.TrimPrefix(header, bearerPrefix), now)
		}
		return "", false
	}
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, bearerPrefix) {
		return "", false
	}
	return a.parseToken(strings.TrimPrefix(header, bearerPrefix), now)
}

func (a *App) loadSession(ctx context.Context, id string, now time.Time) (string, bool) {
	var doc struct {
		Wallet    string    `bson:"wallet"`
		ExpiresAt time.Time `bson:"expiresAt"`
	}
	err := a.sessions.FindOne(ctx, bson.M{"_id": id}).Decode(&doc)
	if err != nil || !now.Before(doc.ExpiresAt) {
		if err == nil {
			_, _ = a.sessions.DeleteOne(ctx, bson.M{"_id": id})
		}
		return "", false
	}
	return doc.Wallet, true
}

func (a *App) issueSession(w http.ResponseWriter, ctx context.Context, wallet string, now time.Time) (time.Time, string, error) {
	expires := now.Add(sessionTTL)
	if a.sessions == nil {
		return expires, a.signToken(wallet, expires), nil
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return time.Time{}, "", err
	}
	id := hex.EncodeToString(buf)
	if _, err := a.sessions.InsertOne(ctx, bson.M{"_id": id, "wallet": wallet, "expiresAt": expires}); err != nil {
		return time.Time{}, "", err
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: id, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, Expires: expires,
	})
	return expires, "", nil
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
}

func (a *App) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return a.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		if !a.cfg.AdminWallets[walletFrom(r)] {
			writeError(w, http.StatusForbidden, "admin access required")
			return
		}
		next(w, r)
	})
}

func walletFrom(r *http.Request) string {
	wallet, _ := r.Context().Value(walletContext).(string)
	return wallet
}

func (a *App) handleNonce(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PublicKey string `json:"publicKey"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if !isValidWallet(req.PublicKey) {
		writeError(w, http.StatusBadRequest, "invalid wallet address")
		return
	}
	nonce, message, err := a.nonces.issue(r.Context(), req.PublicKey, time.Now())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"nonce": nonce, "message": message})
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PublicKey string `json:"publicKey"`
		Nonce     string `json:"nonce"`
		Signature string `json:"signature"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	signature, err := base64.StdEncoding.DecodeString(req.Signature)
	if err != nil {
		writeError(w, http.StatusBadRequest, "signature must be base64")
		return
	}
	now := time.Now()
	message, ok := a.nonces.consume(r.Context(), req.Nonce, req.PublicKey, now)
	if !ok {
		writeError(w, http.StatusUnauthorized, "sign-in request expired, please try again")
		return
	}
	if !verifyWalletSignature(req.PublicKey, message, signature) {
		writeError(w, http.StatusUnauthorized, "signature does not match wallet")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	_, err = a.users.UpdateOne(ctx,
		bson.M{"walletAddress": req.PublicKey},
		bson.M{
			"$set":         bson.M{"lastLoginAt": now},
			"$setOnInsert": bson.M{"walletAddress": req.PublicKey, "createdAt": now},
		},
		options.UpdateOne().SetUpsert(true),
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save user")
		return
	}

	user, err := a.findUser(ctx, req.PublicKey)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save user")
		return
	}
	expires, token, err := a.issueSession(w, ctx, req.PublicKey, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start the session")
		return
	}
	body := map[string]any{"expiresAt": expires, "user": a.publicUser(user)}
	if token != "" {
		body["token"] = token
	}
	writeJSON(w, http.StatusOK, body)
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	if a.sessions != nil {
		if cookie, err := r.Cookie(sessionCookie); err == nil && cookie.Value != "" {
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			_, _ = a.sessions.DeleteOne(ctx, bson.M{"_id": cookie.Value})
			cancel()
		}
	}
	clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleMe(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	wallet := walletFrom(r)
	user, err := a.findUser(ctx, wallet)
	if errors.Is(err, mongo.ErrNoDocuments) {
		writeJSON(w, http.StatusOK, map[string]any{
			"walletAddress": wallet,
			"displayName":   "",
			"isAdmin":       a.cfg.AdminWallets[wallet],
		})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load profile")
		return
	}
	writeJSON(w, http.StatusOK, a.publicUser(user))
}

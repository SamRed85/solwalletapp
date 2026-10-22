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
}

func newNonceStore() *nonceStore {
	return &nonceStore{pending: map[string]pendingNonce{}}
}

func (s *nonceStore) issue(wallet string, now time.Time) (nonce, message string, err error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	nonce = hex.EncodeToString(buf)
	message = loginMessage(wallet, nonce, now)

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
	s.pending[nonce] = pendingNonce{wallet: wallet, message: message, expires: now.Add(nonceTTL)}
	return nonce, message, nil
}

// consume removes the nonce whether or not it is valid, so each one can be tried once.
func (s *nonceStore) consume(nonce, wallet string, now time.Time) (string, bool) {
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

func (a *App) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, bearerPrefix) {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		wallet, ok := a.parseToken(strings.TrimPrefix(header, bearerPrefix), time.Now())
		if !ok {
			writeError(w, http.StatusUnauthorized, "session expired or invalid")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), walletContext, wallet)))
	}
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
	nonce, message, err := a.nonces.issue(req.PublicKey, time.Now())
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
	message, ok := a.nonces.consume(req.Nonce, req.PublicKey, now)
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
	expires := now.Add(sessionTTL)
	writeJSON(w, http.StatusOK, map[string]any{
		"token":     a.signToken(req.PublicKey, expires),
		"expiresAt": expires,
		"user":      a.publicUser(user),
	})
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

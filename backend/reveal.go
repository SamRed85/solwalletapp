package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/drand/tlock"
	drandhttp "github.com/drand/tlock/networks/http"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// quicknet is the League of Entropy randomness chain. A round cannot be decrypted
// until the network publishes it, which happens every 3 seconds.
const (
	quicknetGenesis = 1692803367
	quicknetPeriod  = 3
	quicknetHash    = "52db9ba70e0cc0f6eaf7803dd07447a1f5477735fd3f661792ba94600c84e971"
	clockSysvar     = "SysvarC1ock11111111111111111111111111111111"
)

var tlockHeader = regexp.MustCompile(`-> tlock ([0-9]+) ` + quicknetHash)

type scheduledReveal struct {
	Coupon       string    `bson:"coupon"`
	Owner        string    `bson:"owner"`
	Event        string    `bson:"event"`
	NonceAccount string    `bson:"nonceAccount"`
	Deadline     int64     `bson:"deadline"`
	RevealBy     int64     `bson:"revealBy"`
	Ciphertext   string    `bson:"ciphertext,omitempty"`
	Status       string    `bson:"status"`
	Attempts     int       `bson:"attempts"`
	LastError    string    `bson:"lastError,omitempty"`
	NextTry      time.Time `bson:"nextTry"`
	UpdatedAt    time.Time `bson:"updatedAt"`
	Slip         bool      `bson:"slip,omitempty"`
	Index        uint32    `bson:"index,omitempty"`
	Combinations uint32    `bson:"combinations,omitempty"`
	Commitment   string    `bson:"commitment,omitempty"`
}

func defaultKeeperPath() string {
	for _, path := range []string{
		filepath.Join("..", "programs", "predictions", "keys", "reveal-keeper.json"),
		filepath.Join("programs", "predictions", "keys", "reveal-keeper.json"),
	} {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	return ""
}

func readKeeper(path string) (ed25519.PrivateKey, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	var nums []int
	if err := json.Unmarshal(raw, &nums); err != nil {
		return nil, "", err
	}
	if len(nums) != ed25519.PrivateKeySize {
		return nil, "", fmt.Errorf("%s is not a solana keypair", path)
	}
	secret := make([]byte, ed25519.PrivateKeySize)
	for i, n := range nums {
		if n < 0 || n > 255 {
			return nil, "", fmt.Errorf("%s has a byte out of range", path)
		}
		secret[i] = byte(n)
	}
	key := ed25519.PrivateKey(secret)
	pub := key.Public().(ed25519.PublicKey)
	if !bytes.Equal(pub, secret[ed25519.SeedSize:]) {
		return nil, "", fmt.Errorf("%s does not match its public key", path)
	}
	return key, base58Encode(pub), nil
}

func (a *App) loadKeeper() {
	path := os.Getenv("REVEAL_KEEPER_KEYPAIR")
	if path == "" {
		path = defaultKeeperPath()
	}
	if path == "" {
		log.Println("automatic reveal will be stored, but not broadcast: keys/reveal-keeper.json is missing")
		return
	}
	key, address, err := readKeeper(path)
	if err != nil {
		log.Fatalf("reveal keeper key: %v", err)
	}
	a.keeper = key
	a.keeperAddress = address
}

func roundReady(round uint64) int64 {
	return quicknetGenesis + int64(round-1)*quicknetPeriod
}

func decodeAgeArmor(armor string) ([]byte, error) {
	armor = strings.TrimSpace(armor)
	const header = "-----BEGIN AGE ENCRYPTED FILE-----"
	const footer = "-----END AGE ENCRYPTED FILE-----"
	if !strings.HasPrefix(armor, header) || !strings.HasSuffix(armor, footer) {
		return nil, errors.New("the reveal is not a time-locked payload")
	}
	body := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, armor[len(header):len(armor)-len(footer)])
	return base64.StdEncoding.DecodeString(body)
}

func tlockRound(armor string) (uint64, error) {
	raw, err := decodeAgeArmor(armor)
	if err != nil {
		return 0, err
	}
	if len(raw) > 220 {
		raw = raw[:220]
	}
	match := tlockHeader.FindSubmatch(raw)
	if match == nil {
		return 0, errors.New("the time lock is not for the quicknet chain")
	}
	return strconv.ParseUint(string(match[1]), 10, 64)
}

func ciphertextWindow(armor string, deadline, revealBy int64) error {
	round, err := tlockRound(armor)
	if err != nil {
		return err
	}
	ready := roundReady(round)
	if ready < deadline {
		return errors.New("the time lock opens before entries close")
	}
	if ready >= revealBy {
		return errors.New("the time lock opens after the reveal window")
	}
	return nil
}

func (a *App) handleArmReveal(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Coupon     string `json:"coupon"`
		Ciphertext string `json:"ciphertext"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	wallet := walletFrom(r)
	if strings.HasPrefix(req.Coupon, "slip:") {
		a.armSlipReveal(w, r, wallet, req.Coupon, req.Ciphertext)
		return
	}
	if !isValidWallet(req.Coupon) {
		writeError(w, http.StatusBadRequest, "invalid coupon")
		return
	}
	if len(req.Ciphertext) < 200 || len(req.Ciphertext) > 8000 {
		writeError(w, http.StatusBadRequest, "the time-locked reveal has an unexpected size")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	coupon, err := a.fetchAccount(ctx, req.Coupon)
	if err != nil || coupon == nil || coupon.Owner != a.cfg.ProgramID || len(coupon.Data) < 123 {
		writeError(w, http.StatusBadRequest, "the coupon was not found on chain")
		return
	}
	owner := base58Encode(coupon.Data[40:72])
	if owner != wallet {
		writeError(w, http.StatusForbidden, "this coupon belongs to another wallet")
		return
	}
	if coupon.Data[122] == 1 {
		writeError(w, http.StatusConflict, "this coupon is already revealed")
		return
	}
	eventKey := base58Encode(coupon.Data[8:40])
	event, err := a.fetchAccount(ctx, eventKey)
	if err != nil || event == nil || len(event.Data) < eventAccountMinimumSize {
		writeError(w, http.StatusBadGateway, "the vault event could not be read")
		return
	}
	if event.Data[eventStatusOffset] != 0 {
		writeError(w, http.StatusConflict, "this event is no longer open")
		return
	}
	deadline := int64(binary.LittleEndian.Uint64(event.Data[eventDeadlineOffset:]))
	revealBy := int64(binary.LittleEndian.Uint64(event.Data[eventRevealOffset:]))
	if time.Now().Unix() >= revealBy {
		writeError(w, http.StatusConflict, "the reveal window has closed")
		return
	}
	if err := ciphertextWindow(req.Ciphertext, deadline, revealBy); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var existing scheduledReveal
	findErr := a.reveals.FindOne(ctx, bson.M{"coupon": req.Coupon}).Decode(&existing)
	if findErr == nil && existing.Status == "revealed" {
		writeError(w, http.StatusConflict, "this coupon is already revealed")
		return
	}
	if findErr != nil && !errors.Is(findErr, mongo.ErrNoDocuments) {
		writeError(w, http.StatusInternalServerError, "could not store the scheduled reveal")
		return
	}
	_, err = a.reveals.UpdateOne(ctx, bson.M{"coupon": req.Coupon}, bson.M{"$set": scheduledReveal{
		Coupon: req.Coupon, Owner: wallet, Event: eventKey,
		Deadline: deadline, RevealBy: revealBy, Ciphertext: req.Ciphertext, Status: "pending",
		NextTry: time.Now(), UpdatedAt: time.Now(),
	}}, options.UpdateOne().SetUpsert(true))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not store the scheduled reveal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "armed"})
}

func (a *App) revealLoop() {
	if a.keeper == nil || a.rpc == nil {
		return
	}
	log.Printf("automatic reveal keeper %s", a.keeperAddress)
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	a.publishDue()
	for range ticker.C {
		a.publishDue()
	}
}

func (a *App) publishDue() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cursor, err := a.reveals.Find(ctx, bson.M{
		"status":   "pending",
		"deadline": bson.M{"$lte": time.Now().Unix() - 8},
		"$or": []bson.M{
			{"nextTry": bson.M{"$lte": time.Now()}},
			{"nextTry": bson.M{"$exists": false}},
		},
	}, options.Find().SetLimit(8))
	if err != nil {
		log.Printf("automatic reveal: %v", err)
		return
	}
	defer cursor.Close(ctx)
	for cursor.Next(ctx) {
		var item scheduledReveal
		if err := cursor.Decode(&item); err != nil {
			continue
		}
		a.publishOne(ctx, item)
	}
}

func (a *App) publishOne(ctx context.Context, item scheduledReveal) {
	if item.Slip {
		a.publishSlip(ctx, item)
		return
	}
	if time.Now().Unix() >= item.RevealBy {
		a.finishReveal(ctx, item.Coupon, "expired", "the reveal window closed", true)
		return
	}
	coupon, err := a.fetchAccount(ctx, item.Coupon)
	if err != nil {
		a.retryReveal(ctx, item, "could not read the coupon")
		return
	}
	if coupon == nil || len(coupon.Data) < 123 || coupon.Data[122] == 1 {
		a.finishReveal(ctx, item.Coupon, "revealed", "", true)
		return
	}
	clock, err := a.fetchAccount(ctx, clockSysvar)
	if err != nil || clock == nil || len(clock.Data) < 40 || int64(binary.LittleEndian.Uint64(clock.Data[32:])) < item.Deadline {
		return
	}
	plain, err := decryptLocked(item.Ciphertext)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "too early") {
			return
		}
		a.retryReveal(ctx, item, "could not open the time lock")
		return
	}
	defer func() {
		for i := range plain {
			plain[i] = 0
		}
	}()
	picks, revealNonce, err := lockedPicks(plain)
	if err != nil {
		a.finishReveal(ctx, item.Coupon, "rejected", err.Error(), true)
		return
	}
	owner := coupon.Data[40:72]
	eventKey := coupon.Data[8:40]
	games := int(coupon.Data[76])
	if base58Encode(owner) != item.Owner || base58Encode(eventKey) != item.Event || games < 1 || games > len(picks) {
		a.finishReveal(ctx, item.Coupon, "rejected", "the coupon no longer matches the scheduled reveal", true)
		return
	}
	for _, pick := range picks[games:] {
		if pick != 0 {
			a.finishReveal(ctx, item.Coupon, "rejected", "the stored picks do not match this coupon", true)
			return
		}
	}
	if !bytes.Equal(couponCommitment(owner, picks, revealNonce), coupon.Data[90:122]) {
		a.finishReveal(ctx, item.Coupon, "rejected", "the stored picks do not match this coupon", true)
		return
	}
	program, err := base58Decode(a.cfg.ProgramID)
	couponKey, err2 := base58Decode(item.Coupon)
	if err != nil || err2 != nil {
		a.retryReveal(ctx, item, "the coupon address is invalid")
		return
	}
	hash, err := a.rpc.latestBlockhash(ctx)
	if err != nil {
		a.retryReveal(ctx, item, "could not read a recent blockhash")
		return
	}
	raw, err := buildRevealTx(a.keeper, program, owner, couponKey, eventKey, hash, picks, revealNonce)
	if err != nil {
		a.retryReveal(ctx, item, err.Error())
		return
	}
	if err := a.rpc.simulate(ctx, raw); err != nil {
		a.retryReveal(ctx, item, err.Error())
		return
	}
	signature, err := a.rpc.send(ctx, raw)
	if err != nil {
		a.retryReveal(ctx, item, err.Error())
		return
	}
	if err := a.rpc.confirm(ctx, signature); err != nil {
		a.retryReveal(ctx, item, err.Error())
		return
	}
	a.finishReveal(ctx, item.Coupon, "revealed", "", true)
	log.Printf("revealed %s in %s", item.Coupon, signature)
}

func (a *App) finishReveal(ctx context.Context, coupon, status, message string, drop bool) {
	update := bson.M{"$set": bson.M{"status": status, "lastError": trimErr(message), "updatedAt": time.Now()}}
	if drop {
		update["$unset"] = bson.M{"ciphertext": ""}
	}
	if _, err := a.reveals.UpdateOne(ctx, bson.M{"coupon": coupon}, update); err != nil {
		log.Printf("automatic reveal %s: %v", coupon, err)
	}
}

func (a *App) retryReveal(ctx context.Context, item scheduledReveal, message string) {
	attempts := item.Attempts + 1
	if attempts >= 12 {
		a.finishReveal(ctx, item.Coupon, "rejected", trimErr(message), true)
		return
	}
	wait := time.Duration(attempts) * time.Minute
	_, err := a.reveals.UpdateOne(ctx, bson.M{"coupon": item.Coupon}, bson.M{"$set": bson.M{
		"attempts": attempts, "lastError": trimErr(message), "nextTry": time.Now().Add(wait), "updatedAt": time.Now(),
	}})
	if err != nil {
		log.Printf("automatic reveal %s: %v", item.Coupon, err)
	}
}

func trimErr(message string) string {
	message = strings.Join(strings.Fields(message), " ")
	if len(message) > 180 {
		return message[:180]
	}
	return message
}

func decryptLocked(armor string) ([]byte, error) {
	network, err := drandhttp.NewNetwork("https://api.drand.sh", quicknetHash)
	if err != nil {
		return nil, err
	}
	var plain bytes.Buffer
	if err := tlock.New(network).Decrypt(&plain, strings.NewReader(armor)); err != nil {
		return nil, err
	}
	return plain.Bytes(), nil
}

func lockedPicks(plain []byte) ([]byte, uint64, error) {
	var payload struct {
		RevealNonce string `json:"revealNonce"`
		Picks       []int  `json:"picks"`
	}
	if err := json.Unmarshal(plain, &payload); err != nil || len(payload.Picks) != 13 {
		return nil, 0, errors.New("the time lock does not contain picks")
	}
	picks := make([]byte, 13)
	for i, pick := range payload.Picks {
		if pick < 0 || pick > 255 {
			return nil, 0, errors.New("the time lock does not contain picks")
		}
		picks[i] = byte(pick)
	}
	nonce, err := strconv.ParseUint(payload.RevealNonce, 10, 64)
	if err != nil {
		return nil, 0, errors.New("the time lock does not contain picks")
	}
	return picks, nonce, nil
}

func couponCommitment(owner, picks []byte, nonce uint64) []byte {
	var encoded [8]byte
	binary.LittleEndian.PutUint64(encoded[:], nonce)
	sum := sha256.Sum256(append(append(append([]byte{}, owner...), picks...), encoded[:]...))
	return sum[:]
}

func buildRevealTx(keeper ed25519.PrivateKey, program, owner, coupon, event, blockhash, picks []byte, nonce uint64) ([]byte, error) {
	if len(picks) != 13 || len(program) != 32 || len(owner) != 32 || len(coupon) != 32 || len(event) != 32 {
		return nil, errors.New("the reveal accounts are the wrong size")
	}
	disc := sha256.Sum256([]byte("global:reveal_coupon"))
	data := make([]byte, 29)
	copy(data, disc[:8])
	copy(data[8:21], picks)
	binary.LittleEndian.PutUint64(data[21:], nonce)
	// 0 keeper, 1 coupon, 2 owner, 3 event, 4 program. The owner is not a signer.
	return signLegacy(keeper, [][]byte{coupon}, [][]byte{owner, event, program}, []ixSpec{{
		program: 4, accounts: []int{2, 1, 3}, data: data,
	}}, blockhash)
}

type ixSpec struct {
	program  int
	accounts []int
	data     []byte
}

func shortBytes(n int) []byte {
	if n < 0x80 {
		return []byte{byte(n)}
	}
	return []byte{byte(n) | 0x80, byte(n >> 7)}
}

func signLegacy(payer ed25519.PrivateKey, writable, readonly [][]byte, ixs []ixSpec, blockhash []byte) ([]byte, error) {
	if len(blockhash) != 32 {
		return nil, errors.New("blockhash must be 32 bytes")
	}
	pub := payer.Public().(ed25519.PublicKey)
	keys := append([][]byte{pub}, append(writable, readonly...)...)
	var msg []byte
	msg = append(msg, 1, 0, byte(len(readonly)))
	msg = append(msg, shortBytes(len(keys))...)
	for _, key := range keys {
		msg = append(msg, key...)
	}
	msg = append(msg, blockhash...)
	msg = append(msg, shortBytes(len(ixs))...)
	for _, ix := range ixs {
		msg = append(msg, byte(ix.program))
		msg = append(msg, shortBytes(len(ix.accounts))...)
		for _, account := range ix.accounts {
			msg = append(msg, byte(account))
		}
		msg = append(msg, shortBytes(len(ix.data))...)
		msg = append(msg, ix.data...)
	}
	sig := ed25519.Sign(payer, msg)
	out := append([]byte{1}, sig...)
	return append(out, msg...), nil
}

func (s *solanaRPC) latestBlockhash(ctx context.Context) ([]byte, error) {
	var out struct {
		Value struct {
			Blockhash string `json:"blockhash"`
		} `json:"value"`
	}
	if err := s.call(ctx, "getLatestBlockhash", []any{map[string]string{"commitment": "confirmed"}}, &out); err != nil {
		return nil, err
	}
	return base58Decode(out.Value.Blockhash)
}

func (s *solanaRPC) send(ctx context.Context, tx []byte) (string, error) {
	var signature string
	err := s.call(ctx, "sendTransaction", []any{
		base64.StdEncoding.EncodeToString(tx),
		map[string]any{"encoding": "base64", "preflightCommitment": "confirmed"},
	}, &signature)
	return signature, err
}

func (s *solanaRPC) simulate(ctx context.Context, tx []byte) error {
	var out struct {
		Value struct {
			Err any `json:"err"`
		} `json:"value"`
	}
	if err := s.call(ctx, "simulateTransaction", []any{
		base64.StdEncoding.EncodeToString(tx),
		map[string]any{"encoding": "base64", "sigVerify": true, "replaceRecentBlockhash": false, "commitment": "confirmed"},
	}, &out); err != nil {
		return err
	}
	if out.Value.Err != nil {
		return fmt.Errorf("simulation failed: %v", out.Value.Err)
	}
	return nil
}

func (s *solanaRPC) confirm(ctx context.Context, signature string) error {
	for attempt := 0; attempt < 30; attempt++ {
		var out struct {
			Value []*struct {
				Err                any    `json:"err"`
				ConfirmationStatus string `json:"confirmationStatus"`
			} `json:"value"`
		}
		if err := s.call(ctx, "getSignatureStatuses", []any{[]string{signature}, map[string]any{"searchTransactionHistory": true}}, &out); err != nil {
			return err
		}
		if len(out.Value) > 0 && out.Value[0] != nil {
			if out.Value[0].Err != nil {
				return fmt.Errorf("reveal transaction failed: %v", out.Value[0].Err)
			}
			if out.Value[0].ConfirmationStatus == "confirmed" || out.Value[0].ConfirmationStatus == "finalized" {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return errors.New("the reveal was not confirmed")
}

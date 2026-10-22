package main

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func base58Encode(b []byte) string {
	n := new(big.Int).SetBytes(b)
	radix, mod := big.NewInt(58), new(big.Int)
	var out []byte
	for n.Sign() > 0 {
		n.DivMod(n, radix, mod)
		out = append([]byte{base58Alphabet[mod.Int64()]}, out...)
	}
	for _, c := range b {
		if c != 0 {
			break
		}
		out = append([]byte{'1'}, out...)
	}
	return string(out)
}

func testWallet(t *testing.T) (string, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	return base58Encode(pub), priv
}

func TestNormalizeDisplayName(t *testing.T) {
	name, msg := normalizeDisplayName("  Sam   Rend  ")
	if msg != "" || name != "Sam Rend" {
		t.Fatalf("got %q %q", name, msg)
	}
	for _, bad := range []string{"", " ", "A", strings.Repeat("a", 33), "ok\x01name"} {
		if _, msg := normalizeDisplayName(bad); msg == "" {
			t.Fatalf("accepted %q", bad)
		}
	}
}

func TestBase58RoundTrip(t *testing.T) {
	for _, in := range [][]byte{{0, 0, 1, 2}, {255, 254}, make([]byte, 32)} {
		out, err := base58Decode(base58Encode(in))
		if err != nil || string(out) != string(in) {
			t.Fatalf("round trip failed for %v: got %v, %v", in, out, err)
		}
	}
	if _, err := base58Decode("0OIl"); err == nil {
		t.Fatal("expected error for characters outside the base58 alphabet")
	}
	if !isValidWallet(defaultWallet) {
		t.Fatal("default wallet should be a valid 32-byte address")
	}
}

func TestWalletSignature(t *testing.T) {
	wallet, priv := testWallet(t)
	other, _ := testWallet(t)
	msg := loginMessage(wallet, "abc", time.Now())
	sig := ed25519.Sign(priv, []byte(msg))

	if !verifyWalletSignature(wallet, msg, sig) {
		t.Fatal("valid signature rejected")
	}
	if verifyWalletSignature(other, msg, sig) {
		t.Fatal("signature accepted for a different wallet")
	}
	if verifyWalletSignature(wallet, msg+"x", sig) {
		t.Fatal("signature accepted for a different message")
	}
	if verifyWalletSignature(wallet, msg, make([]byte, 64)) {
		t.Fatal("random signature accepted")
	}
}

func TestNonceIsSingleUseAndBoundToWallet(t *testing.T) {
	s := newNonceStore()
	wallet, _ := testWallet(t)
	other, _ := testWallet(t)
	now := time.Now()

	nonce, msg, err := s.issue(wallet, now)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, wallet) || !strings.Contains(msg, nonce) {
		t.Fatal("message must name the wallet and nonce")
	}
	if _, ok := s.consume(nonce, other, now); ok {
		t.Fatal("nonce accepted for another wallet")
	}
	if _, ok := s.consume(nonce, wallet, now); ok {
		t.Fatal("nonce usable after a failed attempt")
	}

	nonce, _, _ = s.issue(wallet, now)
	if _, ok := s.consume(nonce, wallet, now.Add(nonceTTL+time.Second)); ok {
		t.Fatal("expired nonce accepted")
	}

	nonce, _, _ = s.issue(wallet, now)
	if _, ok := s.consume(nonce, wallet, now); !ok {
		t.Fatal("valid nonce rejected")
	}
	if _, ok := s.consume(nonce, wallet, now); ok {
		t.Fatal("nonce reused")
	}
}

func TestSessionToken(t *testing.T) {
	a := &App{cfg: Config{SessionSecret: []byte("secret-1")}}
	now := time.Now()
	token := a.signToken("WalletA", now.Add(time.Hour))

	if w, ok := a.parseToken(token, now); !ok || w != "WalletA" {
		t.Fatalf("valid token rejected: %q %v", w, ok)
	}
	if _, ok := a.parseToken(token, now.Add(2*time.Hour)); ok {
		t.Fatal("expired token accepted")
	}
	if _, ok := (&App{cfg: Config{SessionSecret: []byte("secret-2")}}).parseToken(token, now); ok {
		t.Fatal("token accepted with a different secret")
	}
	forged := a.signToken("WalletB", now.Add(time.Hour))
	if _, ok := a.parseToken(strings.Split(forged, ".")[0]+"."+strings.Split(token, ".")[1], now); ok {
		t.Fatal("payload swap accepted")
	}
}

func payment(source, dest string, lamports uint64, age time.Duration, failed bool) *rpcTransaction {
	tx := &rpcTransaction{}
	bt := time.Now().Add(-age).Unix()
	tx.BlockTime = &bt
	tx.Meta = &struct {
		Err any `json:"err"`
	}{}
	if failed {
		tx.Meta.Err = map[string]any{"InstructionError": 1}
	}
	tx.Transaction.Message.Instructions = append(tx.Transaction.Message.Instructions, struct {
		Program   string          `json:"program"`
		ProgramID string          `json:"programId"`
		Parsed    json.RawMessage `json:"parsed"`
	}{Program: "system"})
	ix := &tx.Transaction.Message.Instructions[0]
	ix.Parsed, _ = json.Marshal(map[string]any{
		"type": "transfer",
		"info": map[string]any{"source": source, "destination": dest, "lamports": lamports},
	})
	memo, _ := json.Marshal("expected-memo")
	tx.Transaction.Message.Instructions = append(tx.Transaction.Message.Instructions, struct {
		Program   string          `json:"program"`
		ProgramID string          `json:"programId"`
		Parsed    json.RawMessage `json:"parsed"`
	}{Program: "spl-memo", Parsed: memo})
	return tx
}

func TestCheckPayment(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name string
		tx   *rpcTransaction
		ok   bool
	}{
		{"exact payment", payment("payer", "treasury", 3000, time.Minute, false), true},
		{"missing transaction", nil, false},
		{"failed transaction", payment("payer", "treasury", 3000, time.Minute, true), false},
		{"underpaid", payment("payer", "treasury", 2999, time.Minute, false), false},
		{"overpaid", payment("payer", "treasury", 3001, time.Minute, false), false},
		{"wrong payer", payment("someone", "treasury", 3000, time.Minute, false), false},
		{"wrong recipient", payment("payer", "elsewhere", 3000, time.Minute, false), false},
		{"too old", payment("payer", "treasury", 3000, 2*time.Hour, false), false},
	}
	for _, c := range cases {
		err := checkPayment(c.tx, "payer", "treasury", 3000, "expected-memo", now)
		if (err == nil) != c.ok {
			t.Errorf("%s: got err=%v, want ok=%v", c.name, err, c.ok)
		}
	}
	if err := checkPayment(payment("payer", "treasury", 3000, time.Minute, false), "payer", "treasury", 3000, "other-memo", now); err == nil {
		t.Fatal("payment with a mismatched coupon memo was accepted")
	}
}

func TestPaymentMemo(t *testing.T) {
	got := paymentMemo("0123456789abcdef01234567", []string{"1X", "2", "12"})
	want := "sol-entry-v1:589e18e1039533ed869223442c572df7e13ecd6c1cfd05238f6b14f8222edfd5"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRecoveryCanVerifyHistoricalPayment(t *testing.T) {
	tx := payment("payer", "treasury", 3000, 2*time.Hour, false)
	if err := checkPayment(tx, "payer", "treasury", 3000, "expected-memo", time.Now()); err == nil {
		t.Fatal("old payment was accepted as a new entry")
	}
	if err := checkPaymentWithAge(tx, "payer", "treasury", 3000, "expected-memo", time.Now(), 0); err != nil {
		t.Fatalf("historical payment was rejected for recovery: %v", err)
	}
	if err := checkPaymentWithAge(tx, "payer", "treasury", 3000, "", time.Now(), 0); err != nil {
		t.Fatalf("historical treasury transfer was rejected as a refund: %v", err)
	}
}

func TestGetTransactionRequiresFinality(t *testing.T) {
	var commitment string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Params []json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		var options map[string]any
		if len(request.Params) != 2 {
			t.Fatalf("got %d params, want 2", len(request.Params))
		}
		if err := json.Unmarshal(request.Params[1], &options); err != nil {
			t.Fatal(err)
		}
		commitment, _ = options["commitment"].(string)
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": nil})
	}))
	defer server.Close()

	if _, err := newSolanaRPC(server.URL).getTransaction(context.Background(), "signature"); err != nil {
		t.Fatal(err)
	}
	if commitment != "finalized" {
		t.Fatalf("got commitment %q, want finalized", commitment)
	}
}

func TestRateLimiterWindow(t *testing.T) {
	var limiter rateLimiter
	now := time.Now()
	if !limiter.allow("wallet", 2, time.Minute, now) || !limiter.allow("wallet", 2, time.Minute, now) {
		t.Fatal("requests inside the limit were rejected")
	}
	if limiter.allow("wallet", 2, time.Minute, now) {
		t.Fatal("request over the limit was accepted")
	}
	if !limiter.allow("wallet", 2, time.Minute, now.Add(time.Minute)) {
		t.Fatal("request was rejected after the window reset")
	}
	if !limiter.allow("other-wallet", 2, time.Minute, now) {
		t.Fatal("one key limited a different key")
	}
}

func TestRateLimitResponse(t *testing.T) {
	app := &App{}
	calls := 0
	handler := app.limitRequests("login", 1, time.Minute, ipRateKey, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	})
	request := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
		r.RemoteAddr = "192.0.2.1:1234"
		w := httptest.NewRecorder()
		handler(w, r)
		return w
	}
	if got := request().Code; got != http.StatusNoContent {
		t.Fatalf("first status = %d", got)
	}
	blocked := request()
	if blocked.Code != http.StatusTooManyRequests || blocked.Header().Get("Retry-After") != "60" {
		t.Fatalf("blocked response = %d, Retry-After %q", blocked.Code, blocked.Header().Get("Retry-After"))
	}
	if calls != 1 {
		t.Fatalf("handler called %d times, want 1", calls)
	}
}

func TestValidateCoupons(t *testing.T) {
	if msg := validateCoupons([]string{"1X21X2X", "2222222"}, 7); msg != "" {
		t.Fatalf("valid coupons rejected: %s", msg)
	}
	bad := [][]string{
		nil,
		{"1X2"},
		{"1X21X2Y"},
		{"1x21x2x"},
		make([]string, maxCombinations+1),
	}
	for _, c := range bad {
		if validateCoupons(c, 7) == "" {
			t.Errorf("invalid coupons accepted: %v", c)
		}
	}
}

func TestPrepareHidesOtherProgram(t *testing.T) {
	app := &App{cfg: Config{ProgramID: "35eSgMRzq1xNM4qDDLezingfXmSDM2ZAjRMTWcJ4QNca", CouponPriceLamports: 1_000_000}}
	event := Event{
		GameCount: 7,
		StartTime: time.Now().Add(time.Hour),
		Chain:     &ChainVault{ProgramID: "11111111111111111111111111111111", Admin: "955GUT5WguA7BJr1J9JVzRnSkqr1uszy2yHGBCz6Adun", Cosigner: "4gGayWwFTHeuwK2o1chizBDPaMwqUBUvmBdpaYXzFEHH", Nonce: 1},
	}
	app.prepare(&event, time.Now())
	if event.Chain != nil {
		t.Fatal("a vault from another program was returned")
	}
	event.Chain = &ChainVault{ProgramID: app.cfg.ProgramID, Admin: "955GUT5WguA7BJr1J9JVzRnSkqr1uszy2yHGBCz6Adun", Cosigner: "4gGayWwFTHeuwK2o1chizBDPaMwqUBUvmBdpaYXzFEHH", Nonce: 1}
	app.prepare(&event, time.Now())
	if event.Chain == nil {
		t.Fatal("the current program vault was hidden")
	}
}

func TestValidateChainVault(t *testing.T) {
	admin := "955GUT5WguA7BJr1J9JVzRnSkqr1uszy2yHGBCz6Adun"
	other := "4gGayWwFTHeuwK2o1chizBDPaMwqUBUvmBdpaYXzFEHH"
	start := time.Now().Add(time.Hour)
	vault := ChainVault{Admin: admin, Cosigner: other, Nonce: 1}
	if msg := validateChainVault(vault, 7, start, time.Now()); msg != "" {
		t.Fatal(msg)
	}
	if validateChainVault(vault, 0, start, time.Now()) == "" {
		t.Fatal("games are required")
	}
	if validateChainVault(vault, 7, time.Now().Add(-time.Minute), time.Now()) == "" {
		t.Fatal("a started event cannot open a vault")
	}
	if validateChainVault(ChainVault{Admin: admin, Cosigner: admin, Nonce: 1}, 7, start, time.Now()) == "" {
		t.Fatal("signers must differ")
	}
}

func TestEventStatus(t *testing.T) {
	now := time.Now()
	e := Event{StartTime: now.Add(time.Hour)}
	if s := e.statusAt(now); s != "draft" {
		t.Fatalf("want draft, got %s", s)
	}
	e.Games = make([]Game, 7)
	if s := e.statusAt(now); s != "open" {
		t.Fatalf("want open, got %s", s)
	}
	if s := e.statusAt(now.Add(2 * time.Hour)); s != "closed" {
		t.Fatalf("want closed, got %s", s)
	}
	e.ResultsSetAt = &now
	if s := e.statusAt(now); s != "completed" {
		t.Fatalf("want completed, got %s", s)
	}
}

package main

import (
	"crypto/ed25519"
	"math/big"
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
		Program string `json:"program"`
		Parsed  *struct {
			Type string `json:"type"`
			Info struct {
				Source      string `json:"source"`
				Destination string `json:"destination"`
				Lamports    uint64 `json:"lamports"`
			} `json:"info"`
		} `json:"parsed"`
	}{Program: "system"})
	ix := &tx.Transaction.Message.Instructions[0]
	ix.Parsed = &struct {
		Type string `json:"type"`
		Info struct {
			Source      string `json:"source"`
			Destination string `json:"destination"`
			Lamports    uint64 `json:"lamports"`
		} `json:"info"`
	}{Type: "transfer"}
	ix.Parsed.Info.Source, ix.Parsed.Info.Destination, ix.Parsed.Info.Lamports = source, dest, lamports
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
		err := checkPayment(c.tx, "payer", "treasury", 3000, now)
		if (err == nil) != c.ok {
			t.Errorf("%s: got err=%v, want ok=%v", c.name, err, c.ok)
		}
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

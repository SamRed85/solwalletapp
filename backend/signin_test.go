package main

import (
	"context"
	"crypto/ed25519"
	"encoding/binary"
	"testing"
	"time"
)

func signInText(host, origin, wallet, nonce, issued, chain string) string {
	return host + " wants you to sign in with your Solana account:\n" +
		wallet + "\n\n" +
		"Sign in to Sol (devnet). This is free and does not send a transaction.\n\n" +
		"URI: " + origin + "\n" +
		"Version: 1\n" +
		"Chain ID: " + chain + "\n" +
		"Nonce: " + nonce + "\n" +
		"Issued At: " + issued
}

func offchainV1(signer []byte, content string) []byte {
	out := append([]byte(nil), offchainPrefix...)
	out = append(out, 1, 1)
	out = append(out, signer...)
	return append(out, content...)
}

func offchainV0(signer []byte, format byte, content string) []byte {
	body := []byte(content)
	out := append([]byte(nil), offchainPrefix...)
	out = append(out, 0)
	out = append(out, make([]byte, 32)...)
	out = append(out, format, 1)
	out = append(out, signer...)
	var lenb [2]byte
	binary.LittleEndian.PutUint16(lenb[:], uint16(len(body)))
	out = append(out, lenb[:]...)
	return append(out, body...)
}

func loginAfter(store *nonceStore, origin, wallet, nonce string, priv ed25519.PrivateKey, signed []byte, now time.Time) bool {
	stored, ok := store.consume(context.Background(), nonce, wallet, now)
	if !ok {
		return false
	}
	payload := signed
	if payload == nil {
		payload = []byte(stored)
	}
	signature := ed25519.Sign(priv, payload)
	return loginSignatureOK(origin, wallet, stored, nonce, signature, signed, now)
}

func TestStoredMessageAndSignInShareOneNonce(t *testing.T) {
	store := newNonceStore(nil)
	wallet, priv := testWallet(t)
	pub, err := base58Decode(wallet)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 12, 20, 0, 0, 0, time.UTC)
	origin := "http://localhost:3000"
	issued := now.Format(time.RFC3339)

	nonce, _, err := store.issue(context.Background(), wallet, now)
	if err != nil {
		t.Fatal(err)
	}
	if !loginAfter(store, origin, wallet, nonce, priv, nil, now) {
		t.Fatal("stored sign-in message rejected")
	}
	if loginAfter(store, origin, wallet, nonce, priv, nil, now) {
		t.Fatal("stored sign-in nonce reused")
	}

	nonce, _, _ = store.issue(context.Background(), wallet, now)
	text := signInText("localhost:3000", origin, wallet, nonce, issued, solanaDevnet)
	if !loginAfter(store, origin, wallet, nonce, priv, []byte(text), now) {
		t.Fatal("sign in with solana rejected")
	}
	if loginAfter(store, origin, wallet, nonce, priv, []byte(text), now) {
		t.Fatal("sign in with solana nonce reused")
	}

	nonce, _, _ = store.issue(context.Background(), wallet, now)
	wrapped := offchainV1(pub, signInText("localhost:3000", origin, wallet, nonce, issued, solanaDevnet))
	if !loginAfter(store, origin, wallet, nonce, priv, wrapped, now) {
		t.Fatal("off-chain sign in rejected")
	}

	nonce, stored, _ := store.issue(context.Background(), wallet, now)
	if !loginAfter(store, origin, wallet, nonce, priv, offchainV0(pub, 1, stored), now) {
		t.Fatal("off-chain copy of the stored message rejected")
	}
}

func TestSignInRejectsTheWrongDomainChainAndAddress(t *testing.T) {
	store := newNonceStore(nil)
	wallet, priv := testWallet(t)
	other, otherPriv := testWallet(t)
	pub, _ := base58Decode(wallet)
	otherPub, _ := base58Decode(other)
	now := time.Date(2026, 11, 12, 20, 0, 0, 0, time.UTC)
	origin := "http://localhost:3000"
	issued := now.Format(time.RFC3339Nano)

	reject := func(signed []byte) {
		t.Helper()
		nonce, _, err := store.issue(context.Background(), wallet, now)
		if err != nil {
			t.Fatal(err)
		}
		if loginAfter(store, origin, wallet, nonce, priv, signed, now) {
			t.Fatal("sign-in accepted")
		}
	}
	withNonce := func(edit func(nonce string) []byte) {
		t.Helper()
		nonce, _, err := store.issue(context.Background(), wallet, now)
		if err != nil {
			t.Fatal(err)
		}
		if loginAfter(store, origin, wallet, nonce, priv, edit(nonce), now) {
			t.Fatal("sign-in accepted")
		}
	}

	withNonce(func(nonce string) []byte {
		return []byte(signInText("evil.example", origin, wallet, nonce, issued, solanaDevnet))
	})
	withNonce(func(nonce string) []byte {
		return []byte(signInText("localhost:3000", "https://localhost:3000", wallet, nonce, issued, solanaDevnet))
	})
	withNonce(func(nonce string) []byte {
		return []byte(signInText("localhost:3000", origin, other, nonce, issued, solanaDevnet))
	})
	withNonce(func(nonce string) []byte {
		return []byte(signInText("localhost:3000", origin, wallet, nonce, issued, "solana:mainnet"))
	})
	withNonce(func(nonce string) []byte {
		return []byte(signInText("localhost:3000", origin, wallet, nonce, now.Add(-nonceTTL-time.Second).Format(time.RFC3339), solanaDevnet))
	})
	withNonce(func(nonce string) []byte {
		return []byte("The nonce is " + nonce)
	})
	withNonce(func(nonce string) []byte {
		return offchainV1(otherPub, signInText("localhost:3000", origin, wallet, nonce, issued, solanaDevnet))
	})
	reject(offchainV0(pub, 0, "Sign in to Sol (devnet)\nnot format 0"))

	nonce, stored, err := store.issue(context.Background(), wallet, now)
	if err != nil {
		t.Fatal(err)
	}
	storedSig := ed25519.Sign(priv, []byte(stored))
	consumed, ok := store.consume(context.Background(), nonce, wallet, now)
	if !ok {
		t.Fatal("nonce missing")
	}
	if loginSignatureOK(origin, wallet, consumed, nonce, storedSig, []byte("different bytes"), now) {
		t.Fatal("signature of the stored message accepted for different bytes")
	}

	nonce, _, err = store.issue(context.Background(), wallet, now)
	if err != nil {
		t.Fatal(err)
	}
	text := []byte(signInText("localhost:3000", origin, wallet, nonce, issued, solanaDevnet))
	consumed, ok = store.consume(context.Background(), nonce, wallet, now)
	if !ok || loginSignatureOK(origin, wallet, consumed, nonce, ed25519.Sign(otherPriv, text), text, now) {
		t.Fatal("another wallet's signature accepted")
	}
}

func TestLoginAcceptsAnyConfiguredOrigin(t *testing.T) {
	wallet, priv := testWallet(t)
	now := time.Date(2026, 11, 12, 20, 0, 0, 0, time.UTC)
	issued := now.Format(time.RFC3339)
	store := newNonceStore(nil)
	nonce, _, err := store.issue(context.Background(), wallet, now)
	if err != nil {
		t.Fatal(err)
	}
	stored, ok := store.consume(context.Background(), nonce, wallet, now)
	if !ok {
		t.Fatal("nonce missing")
	}
	text := []byte(signInText("samwebb.duckdns.org", "https://samwebb.duckdns.org", wallet, nonce, issued, solanaDevnet))
	app := &App{cfg: Config{AllowedOrigins: []string{"http://localhost:3000", "https://samwebb.duckdns.org"}}}
	if !app.loginSignatureAccepted(wallet, stored, nonce, ed25519.Sign(priv, text), text, now) {
		t.Fatal("listed public origin rejected")
	}
	app.cfg.AllowedOrigins = []string{"http://localhost:3000"}
	if app.loginSignatureAccepted(wallet, stored, nonce, ed25519.Sign(priv, text), text, now) {
		t.Fatal("unlisted origin accepted")
	}
}

func TestParseAllowedOrigins(t *testing.T) {
	got := parseAllowedOrigins(" http://localhost:3000/, https://samwebb.duckdns.org, http://localhost:3000 ")
	want := []string{"http://localhost:3000", "https://samwebb.duckdns.org"}
	if len(got) != len(want) {
		t.Fatalf("origins %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("origins %v", got)
		}
	}
}

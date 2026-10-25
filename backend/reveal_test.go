package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"strings"
	"testing"
)

const pingArmor = `-----BEGIN AGE ENCRYPTED FILE-----
YWdlLWVuY3J5cHRpb24ub3JnL3YxCi0+IHRsb2NrIDIwMDAwMDAgNTJkYjliYTcw
ZTBjYzBmNmVhZjc4MDNkZDA3NDQ3YTFmNTQ3NzczNWZkM2Y2NjE3OTJiYTk0NjAw
Yzg0ZTk3MQpoNFZMeWJWemJxSEZIdDArcklFaUhjSnRaZzBSTHh1Y241dWwzN1JT
MjJKK1QwK2pjUzkzTEF0ejZoSmdVb3BHCkFJT1RlY3VTYmJFcVIrY09HSzhsT1k5
MGpzaG1udm5UNmpVMFBQbE01N1JWZTlTTUMyWno2b3UyYjRJVHRPbkkKMXRmWGVX
UjVPNDlrcFlFMjgyaU9Xb3FRNS9jcDI0Uk5FbklrbFdqMGp3WQotLS0gUi9hY1lw
YjZyQ0JCWCtFWHBDM3RwTWVYL0p6cWdrUmppYWxvbEdNK0d5YwqNEgnEEKaLKqkL
YittkXqAX/1S/uqfKOG/KMF3jWb4Zy2dkls=
-----END AGE ENCRYPTED FILE-----`

func TestKeeperRevealIsSignedOnlyByTheKeeper(t *testing.T) {
	_, keeper, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	owner := bytes.Repeat([]byte{1}, 32)
	coupon := bytes.Repeat([]byte{2}, 32)
	event := bytes.Repeat([]byte{3}, 32)
	program := bytes.Repeat([]byte{4}, 32)
	hash := bytes.Repeat([]byte{5}, 32)
	picks, nonce, err := lockedPicks([]byte(`{"revealNonce":"99","picks":[1,0,0,0,0,0,0,0,0,0,0,0,0]}`))
	if err != nil || nonce != 99 || picks[0] != 1 {
		t.Fatalf("picks %v nonce %d: %v", picks, nonce, err)
	}
	if _, _, err := lockedPicks([]byte(`not-json`)); err == nil {
		t.Fatal("a signed transaction was accepted as picks")
	}
	raw, err := buildRevealTx(keeper, program, owner, coupon, event, hash, picks, nonce)
	if err != nil {
		t.Fatal(err)
	}
	if raw[0] != 1 || !ed25519.Verify(keeper.Public().(ed25519.PublicKey), raw[65:], raw[1:65]) {
		t.Fatal("the keeper signature does not cover the reveal")
	}
	if ed25519.Verify(owner, raw[65:], raw[1:65]) {
		t.Fatal("the owner must not sign the reveal")
	}
	disc := sha256.Sum256([]byte("global:reveal_coupon"))
	if !bytes.Contains(raw, append(disc[:8], picks...)) {
		t.Fatal("the reveal instruction does not carry the picks")
	}
	if raw[65] != 1 || raw[67] != 3 {
		t.Fatalf("reveal header %v", raw[65:68])
	}
	if len(couponCommitment(owner, picks, nonce)) != 32 {
		t.Fatal("commitment must be 32 bytes")
	}
	if bytes.Equal(couponCommitment(bytes.Repeat([]byte{9}, 32), picks, nonce), couponCommitment(owner, picks, nonce)) {
		t.Fatal("a different owner produced the same commitment")
	}
}

func TestTimeLockRound(t *testing.T) {
	round, err := tlockRound(pingArmor)
	if err != nil || round != 2000000 {
		t.Fatalf("round %d: %v", round, err)
	}
	ready := roundReady(round)
	if err := ciphertextWindow(pingArmor, ready, ready+10); err != nil {
		t.Fatal(err)
	}
	if err := ciphertextWindow(pingArmor, ready+1, ready+10); err == nil {
		t.Fatal("a time lock that opens before the deadline was accepted")
	}
	if strings.Contains(pingArmor, "tlock 2000000") {
		t.Fatal("the round must stay inside the armor, not in the clear")
	}
}

func TestTimeLockDecryptsAfterItsRound(t *testing.T) {
	plain, err := decryptLocked(pingArmor)
	if err != nil {
		t.Fatal(err)
	}
	if string(plain) != "ping" {
		t.Fatalf("decrypted %q", plain)
	}
}

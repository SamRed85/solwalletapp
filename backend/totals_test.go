package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"testing"
)

func TestTotalsWaitForTheWalletSigner(t *testing.T) {
	admin := "wallet-admin"
	cosigner := "council"
	deployer := "deployer"
	held := map[string]ed25519.PrivateKey{cosigner: {}}
	waiting, pending := totalsSignPlan([]string{admin, cosigner, deployer}, held, map[string]bool{})
	if waiting != admin || pending != nil {
		t.Fatalf("the wallet signer was skipped: wait %q pending %v", waiting, pending)
	}
	waiting, pending = totalsSignPlan([]string{admin, cosigner}, held, map[string]bool{admin: true})
	if waiting != "" || len(pending) != 1 || pending[0] != cosigner {
		t.Fatalf("the server signer did not lock last: wait %q pending %v", waiting, pending)
	}
	waiting, pending = totalsSignPlan([]string{admin, cosigner}, map[string]ed25519.PrivateKey{admin: {}, cosigner: {}}, map[string]bool{})
	if waiting != "" || len(pending) != 2 || pending[0] != admin || pending[1] != cosigner {
		t.Fatalf("two server keys should both sign, cosigner last: wait %q pending %v", waiting, pending)
	}
}

func TestTotalsFallbackOpensTwentyFourHoursAfterResults(t *testing.T) {
	approved := int64(1_700_000_000)
	waiting := &chainEvent{Status: "tallying", SettlementPath: pathSigned, ApprovedAt: approved}
	if totalsFallbackDue(waiting, approved+totalsWaitSeconds-1) {
		t.Fatal("the on-chain count opened before 24 hours")
	}
	if !totalsFallbackDue(waiting, approved+totalsWaitSeconds) {
		t.Fatal("the on-chain count stayed closed at 24 hours")
	}
	started := *waiting
	started.CountStarted = 1
	if totalsFallbackDue(&started, approved+totalsWaitSeconds) {
		t.Fatal("a count that already started was treated as still waiting")
	}
	if totalsFallbackDue(&chainEvent{Status: "tallying", SettlementPath: pathSigned}, approved+totalsWaitSeconds) {
		t.Fatal("a result with no approval time opened the count")
	}
	if totalsFallbackDue(&chainEvent{Status: "window", SettlementPath: pathSigned, ApprovedAt: approved}, approved+totalsWaitSeconds) {
		t.Fatal("a posted list opened the on-chain count")
	}
}

func TestDisputeBondClamp(t *testing.T) {
	sol := uint64(1_000_000_000)
	got, err := disputeBondLamports(100*sol, 0, 100, 10, 50)
	if err != nil || got != 10*sol {
		t.Fatalf("a 100 SOL pot locks 10 SOL, got %d %v", got, err)
	}
	got, err = disputeBondLamports(250*sol, 0, 100, 10, 50)
	if err != nil || got != 25*sol {
		t.Fatalf("a 250 SOL pot locks 25 SOL, got %d %v", got, err)
	}
	got, err = disputeBondLamports(500*sol, 0, 100, 10, 50)
	if err != nil || got != 50*sol {
		t.Fatalf("a 500 SOL pot locks 50 SOL, got %d %v", got, err)
	}
	got, err = disputeBondLamports(1_000*sol, 0, 100, 0, 0)
	if err != nil || got != 50*sol {
		t.Fatalf("zero limits mean 10 and 50, got %d %v", got, err)
	}
	// Two coupons at 0.2 SOL with a 90% tier: the player pot is 0.36 SOL, so the bond is the minimum.
	got, err = disputeBondLamports(400_000_000, 0, 90, 10, 50)
	if err != nil || got != 10*sol {
		t.Fatalf("a small pot locks the minimum, got %d %v", got, err)
	}
	if _, err = disputeBondLamports(sol, 0, 90, 40, 10); err == nil {
		t.Fatal("a minimum above the maximum was accepted")
	}
}

func TestArchiveTotalsUsesKeptRows(t *testing.T) {
	picks := make([]byte, 13)
	for i := 0; i < 7; i++ {
		picks[i] = 0b001
	}
	hexPicks := hex.EncodeToString(picks)
	view := slipArchiveView{Entries: []slipRecord{
		{Index: 0, Combinations: 1},
		{Index: 1, Combinations: 1},
	}, Reveals: []slipRecord{
		{Index: 0, Picks: hexPicks},
		{Index: 1, Picks: hexPicks},
	}}
	totals, err := archiveTotals(view, 2, "1111111")
	if err != nil {
		t.Fatal(err)
	}
	if totals[7] != 2 {
		t.Fatalf("two all-home coupons are 2 rows at 7 correct, got %d", totals[7])
	}
	if _, err = archiveTotals(view, 3, "1111111"); err == nil {
		t.Fatal("a missing coupon was counted")
	}
	view.Reveals[1].Picks = "zz"
	if _, err = archiveTotals(view, 2, "1111111"); err == nil {
		t.Fatal("invalid picks were counted")
	}
}

func TestSignTotalsLockRequiresThePoster(t *testing.T) {
	poster := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{1}, 32))
	signer := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{2}, 32))
	keys := totalsKeys{
		event: bytes.Repeat([]byte{3}, 32), vault: bytes.Repeat([]byte{4}, 32),
		signers: bytes.Repeat([]byte{5}, 32), canopy: bytes.Repeat([]byte{6}, 32),
		totals: bytes.Repeat([]byte{7}, 32), program: bytes.Repeat([]byte{8}, 32),
	}
	hash := bytes.Repeat([]byte{9}, 32)
	var rows [14]uint64
	rows[7] = 2
	open, err := buildSignTotals(signer, poster, false, keys, hash, rows)
	if err != nil {
		t.Fatal(err)
	}
	if open[0] != 1 {
		t.Fatalf("a proposal has one signature, got %d", open[0])
	}
	msg := open[1+64:]
	if msg[0] != 1 || msg[1] != 0 {
		t.Fatalf("proposal header %d %d", msg[0], msg[1])
	}
	if !bytes.Equal(msg[4:36], signer.Public().(ed25519.PublicKey)) {
		t.Fatal("the proposal fee payer is not the result signer")
	}
	if !bytes.Equal(msg[36:68], poster.Public().(ed25519.PublicKey)) {
		t.Fatal("the poster is not the second account")
	}
	locked, err := buildSignTotals(signer, poster, true, keys, hash, rows)
	if err != nil {
		t.Fatal(err)
	}
	if locked[0] != 2 {
		t.Fatalf("the locking transaction has two signatures, got %d", locked[0])
	}
	lockedMsg := locked[1+128:]
	if lockedMsg[0] != 2 || lockedMsg[1] != 0 || lockedMsg[2] != 4 {
		t.Fatalf("locking header %d %d %d", lockedMsg[0], lockedMsg[1], lockedMsg[2])
	}
	if !bytes.Equal(lockedMsg[4:36], poster.Public().(ed25519.PublicKey)) {
		t.Fatal("the poster does not sign the transaction that locks the bond")
	}
	if !bytes.Contains(lockedMsg, totalsInstruction(rows)[8:]) {
		t.Fatal("the locking transaction does not carry the totals")
	}
}

func TestAttachKeeperSignatureLocksTheSameMessage(t *testing.T) {
	wallet := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{2}, 32))
	poster := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{1}, 32))
	walletPub := wallet.Public().(ed25519.PublicKey)
	posterPub := poster.Public().(ed25519.PublicKey)
	keys := totalsKeys{
		event: bytes.Repeat([]byte{3}, 32), vault: bytes.Repeat([]byte{4}, 32),
		signers: bytes.Repeat([]byte{5}, 32), canopy: bytes.Repeat([]byte{6}, 32),
		totals: bytes.Repeat([]byte{7}, 32), program: bytes.Repeat([]byte{8}, 32),
	}
	hash := bytes.Repeat([]byte{9}, 32)
	var rows [14]uint64
	rows[2] = 896
	budget := mustDecode(computeBudgetProgram)
	raw, err := signAccounts([]ed25519.PrivateKey{wallet, poster}, []txAccount{
		{key: walletPub, signer: true, writable: true},
		{key: posterPub, signer: true, writable: true},
		{key: keys.event, writable: true},
		{key: keys.vault, writable: true},
		{key: keys.totals, writable: true},
		{key: keys.signers},
		{key: keys.canopy},
		{key: systemProgram},
		{key: budget},
		{key: keys.program},
	}, []ixSpec{
		{program: 8, data: unitLimit},
		{program: 9, accounts: []int{0, 1, 2, 3, 5, 6, 4, 7}, data: totalsInstruction(rows)},
	}, hash)
	if err != nil {
		t.Fatal(err)
	}
	partial := append([]byte(nil), raw...)
	for i := 1 + 64; i < 1+128; i++ {
		partial[i] = 0
	}
	lock := totalsLock{
		wallet: walletPub, poster: posterPub, event: keys.event, vault: keys.vault,
		signers: keys.signers, canopy: keys.canopy, totals: keys.totals, program: keys.program, rows: rows,
	}
	got, err := attachKeeperSignature(partial, poster, lock)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, raw) {
		t.Fatal("the keeper signature did not match the message the wallet signed")
	}
	partial[1] ^= 0xff
	if _, err = attachKeeperSignature(partial, poster, lock); err == nil {
		t.Fatal("a wallet signature that does not match was accepted")
	}
	moved := append([]byte(nil), raw...)
	for i := 1 + 64; i < 1+128; i++ {
		moved[i] = 0
	}
	other := rows
	other[2] = 1
	bad := totalsLock{
		wallet: walletPub, poster: posterPub, event: keys.event, vault: keys.vault,
		signers: keys.signers, canopy: keys.canopy, totals: keys.totals, program: keys.program, rows: other,
	}
	if _, err = attachKeeperSignature(moved, poster, bad); err == nil {
		t.Fatal("a different totals list was co-signed")
	}
}

func TestNormalizeDispute(t *testing.T) {
	min, max, msg := normalizeDispute(0, 0)
	if msg != "" || min != 10 || max != 50 {
		t.Fatalf("defaults %d %d %q", min, max, msg)
	}
	if _, _, msg = normalizeDispute(20, 40); msg != "" {
		t.Fatal(msg)
	}
	if _, _, msg = normalizeDispute(9, 50); msg != "" {
		t.Fatal("a minimum under 10 is a warning, not a rejection")
	}
	if _, _, msg = normalizeDispute(40, 10); msg == "" {
		t.Fatal("a minimum above the maximum was accepted")
	}
}

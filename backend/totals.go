package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"log"
	"math/big"
	"math/bits"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const (
	pathLegacy = 0
	pathSigned = 1

	challengeSlots = 54_000
	posterBond     = 3_000_000_000
	solLamports    = 1_000_000_000
	// totalsWaitSeconds is the unix-time wait after results lock. With no locked totals,
	// the on-chain count may start. It matches TOTALS_WAIT in the program.
	totalsWaitSeconds int64 = 24 * 60 * 60
)

// totalsFallbackDue is true once a signed-totals event has waited 24 hours with nothing locked.
// The first score transaction is what sets the on-chain count flag.
func totalsFallbackDue(chain *chainEvent, now int64) bool {
	if chain == nil || chain.Status != "tallying" || chain.SettlementPath != pathSigned || chain.CountStarted != 0 {
		return false
	}
	if chain.ApprovedAt == 0 {
		return false
	}
	return now >= chain.ApprovedAt+totalsWaitSeconds
}

// systemProgram is the all-zero system program id.
var systemProgram = make([]byte, 32)

var totalsAlerted sync.Map

type recountPay struct {
	poster     []byte
	challenger []byte
}

type chainTotals struct {
	Poster        string
	PosterKey     []byte
	Totals        [14]uint64
	Approvals     byte
	Locked        byte
	WindowSlot    uint64
	Challenger    string
	ChallengerKey []byte
	Rejected      byte
}

func alertTotals(address, kind string) {
	if _, loaded := totalsAlerted.LoadOrStore(address+":"+kind, true); loaded {
		return
	}
	log.Printf("totals alert: %s %s", address, kind)
}

// disputeBondLamports is 10% of the player pot, clamped to whole-SOL limits.
// Zero limits mean 10 and 50. The player pot is the sales the tiers take, plus rollover.
func disputeBondLamports(sales, rollover, percent, minSol, maxSol uint64) (uint64, error) {
	if minSol == 0 {
		minSol = 10
	}
	if maxSol == 0 {
		maxSol = 50
	}
	if minSol < 1 || maxSol > 100 || minSol > maxSol {
		return 0, errors.New("the challenge bond limits are whole SOL from 1 to 100")
	}
	share := new(big.Int).Mul(new(big.Int).SetUint64(sales), new(big.Int).SetUint64(percent))
	share.Quo(share, big.NewInt(100))
	pot := new(big.Int).Add(share, new(big.Int).SetUint64(rollover))
	raw := new(big.Int).Quo(pot, big.NewInt(10))
	min := new(big.Int).Mul(new(big.Int).SetUint64(minSol), big.NewInt(solLamports))
	max := new(big.Int).Mul(new(big.Int).SetUint64(maxSol), big.NewInt(solLamports))
	if raw.Cmp(min) < 0 {
		raw = min
	}
	if raw.Cmp(max) > 0 {
		raw = max
	}
	if !raw.IsUint64() {
		return 0, errors.New("the challenge bond overflows")
	}
	return raw.Uint64(), nil
}

// archiveTotals counts kept rows by score, the same count the program uses.
// A missing coupon or a count that does not match the paid rows is an error:
// those totals must not be posted.
func archiveTotals(view slipArchiveView, couponCount uint32, results string) ([14]uint64, error) {
	var totals [14]uint64
	entries := make(map[uint32]slipRecord, len(view.Entries))
	for _, entry := range view.Entries {
		if _, ok := entries[entry.Index]; ok {
			return totals, errors.New("the archive has two entries for one coupon")
		}
		entries[entry.Index] = entry
	}
	reveals := make(map[uint32]slipRecord, len(view.Reveals))
	for _, reveal := range view.Reveals {
		reveals[reveal.Index] = reveal
	}
	if uint32(len(entries)) != couponCount {
		return totals, errors.New("the archive does not have every coupon, so the totals were not posted")
	}
	for index := uint32(0); index < couponCount; index++ {
		entry, ok := entries[index]
		if !ok {
			return totals, errors.New("the archive does not have every coupon, so the totals were not posted")
		}
		reveal, revealed := reveals[index]
		if !revealed {
			continue
		}
		picks, err := hex.DecodeString(reveal.Picks)
		if err != nil || len(picks) < len(results) {
			return totals, errors.New("the stored picks are invalid, so the totals were not posted")
		}
		colors, err := parseColorHex(reveal.Colors)
		if err != nil {
			return totals, err
		}
		counts := keptCounts(picks, colors, results)
		if counts == nil {
			return totals, errors.New("the row count failed, so the totals were not posted")
		}
		var rows uint64
		for score, n := range counts {
			if score >= len(totals) {
				return totals, errors.New("the row count failed, so the totals were not posted")
			}
			totals[score] += n
			rows += n
		}
		if rows != uint64(entry.Combinations) {
			return totals, errors.New("the row count does not match the paid rows, so the totals were not posted")
		}
	}
	return totals, nil
}

func (a *App) readTotals(ctx context.Context, eventAddress string) (*chainTotals, error) {
	program, err := decodeKey(a.cfg.ProgramID)
	if err != nil {
		return nil, err
	}
	eventKey, err := decodeKey(eventAddress)
	if err != nil {
		return nil, err
	}
	address, err := findPDA([][]byte{[]byte("totals"), eventKey}, program)
	if err != nil {
		return nil, err
	}
	account, err := a.fetchAccount(ctx, base58Encode(address))
	if err != nil || account == nil {
		return nil, err
	}
	return decodeTotals(account, a.cfg.ProgramID)
}

func decodeTotals(account *chainAccount, programID string) (*chainTotals, error) {
	if account == nil {
		return nil, nil
	}
	if account.Owner != programID {
		return nil, errors.New("the totals account is not owned by the vault program")
	}
	d := account.Data
	if len(d) < 8+476 || string(d[:8]) != string(anchorDiscriminator("SignedTotals")) {
		return nil, errors.New("the account is not signed totals")
	}
	out := &chainTotals{
		PosterKey:     append([]byte(nil), d[40:72]...),
		Poster:        base58Encode(d[40:72]),
		Approvals:     d[184],
		Locked:        d[185],
		WindowSlot:    binary.LittleEndian.Uint64(d[186:]),
		ChallengerKey: append([]byte(nil), d[194:226]...),
		Challenger:    base58Encode(d[194:226]),
		Rejected:      d[370],
	}
	for i := 0; i < 14; i++ {
		out.Totals[i] = binary.LittleEndian.Uint64(d[72+8*i:])
	}
	return out, nil
}

func (a *App) handleTotalsPreview(w http.ResponseWriter, r *http.Request) {
	event, ok := a.loadEvent(w, r)
	if !ok {
		return
	}
	if event.Chain == nil || event.Chain.Address == "" {
		writeError(w, http.StatusConflict, "this event has no on-chain vault")
		return
	}
	if a.keeper == nil {
		writeError(w, http.StatusConflict, "the reveal keeper is not configured")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	account, err := a.fetchAccount(ctx, event.Chain.Address)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	chain, err := decodeChainEvent(account, a.cfg.ProgramID)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if chain.Status != "tallying" || chain.SettlementPath != pathSigned || chain.CountStarted != 0 {
		writeError(w, http.StatusConflict, "this event is not waiting for signed totals")
		return
	}
	first, err := a.archiveViewTotals(ctx, event, chain)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	second, err := a.archiveViewTotals(ctx, event, chain)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if first != second {
		writeError(w, http.StatusConflict, "the count changed while it was being made")
		return
	}
	counts := make([]string, len(first))
	for i, n := range first {
		counts[i] = strconv.FormatUint(n, 10)
	}
	writeJSON(w, http.StatusOK, map[string]any{"totals": counts, "poster": a.keeperAddress})
}

func (a *App) archiveViewTotals(ctx context.Context, event *Event, chain *chainEvent) ([14]uint64, error) {
	var none [14]uint64
	if len(chain.Results) != chain.GameCount || chain.GameCount == 0 {
		return none, errors.New("the locked results are missing, so the totals were not posted")
	}
	view, err := a.slipArchive(ctx, event.Chain.Address)
	if err != nil {
		return none, err
	}
	return archiveTotals(view, chain.CouponCount, chain.Results)
}

func (a *App) postTotals(ctx context.Context, event *Event, chain *chainEvent) error {
	if a.keeper == nil {
		log.Printf("totals: %s is waiting for the reveal keeper", event.Chain.Address)
		return nil
	}
	first, err := a.archiveViewTotals(ctx, event, chain)
	if err != nil {
		return err
	}
	second, err := a.archiveViewTotals(ctx, event, chain)
	if err != nil {
		return err
	}
	if first != second {
		log.Printf("totals: the two counts of %s differ; waiting for the 24 hour fallback", event.Chain.Address)
		return nil
	}
	signed, err := a.readTotals(ctx, event.Chain.Address)
	if err != nil {
		return err
	}
	members, err := a.resultMembers(ctx, event.Chain.Address)
	if err != nil {
		return err
	}
	poster := base58Encode(a.keeper.Public().(ed25519.PublicKey))
	held := map[string]ed25519.PrivateKey{}
	for _, key := range []ed25519.PrivateKey{a.adminKey, a.cosignerKey} {
		if key == nil {
			continue
		}
		address := base58Encode(key.Public().(ed25519.PublicKey))
		for _, member := range members {
			if member == address {
				held[address] = key
			}
		}
	}
	if len(held) == 0 {
		log.Printf("totals: %s has no result signer key on this server", event.Chain.Address)
		return nil
	}
	approved := map[string]bool{}
	for _, member := range members {
		approved[member] = totalsApproved(signed, members, member, first, poster)
	}
	waiting, pending := totalsSignPlan(members, held, approved)
	if waiting != "" {
		log.Printf("totals: %s is waiting for %s to sign the counted totals", event.Chain.Address, waiting)
		return nil
	}
	for i, address := range pending {
		raw, err := a.signTotalsTx(ctx, held[address], event.Chain.Address, first, i == len(pending)-1)
		if err != nil {
			return err
		}
		if err = a.submit(ctx, raw); err != nil {
			return err
		}
		if i == len(pending)-1 {
			log.Printf("totals locked on %s", event.Chain.Address)
		} else {
			log.Printf("totals proposed on %s by %s", event.Chain.Address, address)
		}
	}
	return nil
}

// totalsSignPlan decides who still has to sign. A wallet the server cannot sign for must
// approve first. The last server signature locks the bond, so that transaction can include the poster.
func totalsSignPlan(members []string, held map[string]ed25519.PrivateKey, approved map[string]bool) (waiting string, pending []string) {
	for _, member := range members {
		if _, ok := held[member]; ok || approved[member] {
			continue
		}
		return member, nil
	}
	for _, member := range members {
		if _, ok := held[member]; ok && !approved[member] {
			pending = append(pending, member)
		}
	}
	return "", pending
}

func totalsApproved(signed *chainTotals, members []string, who string, rows [14]uint64, poster string) bool {
	if signed == nil || signed.Locked != 0 || signed.Poster != poster || signed.Totals != rows {
		return false
	}
	return signerApproved(signed.Approvals, members, who)
}

func (a *App) resultMembers(ctx context.Context, eventAddress string) ([]string, error) {
	signers, err := a.resultSigners(ctx, eventAddress)
	if err != nil {
		return nil, err
	}
	return signers.Members, nil
}

func (a *App) resultSigners(ctx context.Context, eventAddress string) (*chainSigners, error) {
	program, err := decodeKey(a.cfg.ProgramID)
	if err != nil {
		return nil, err
	}
	eventKey, err := decodeKey(eventAddress)
	if err != nil {
		return nil, err
	}
	address, err := findPDA([][]byte{[]byte("signers"), eventKey}, program)
	if err != nil {
		return nil, err
	}
	account, err := a.fetchAccount(ctx, base58Encode(address))
	if err != nil {
		return nil, err
	}
	return decodeChainSigners(account, a.cfg.ProgramID)
}

func (a *App) watchTotals(ctx context.Context, event *Event, chain *chainEvent) error {
	rows, err := a.archiveViewTotals(ctx, event, chain)
	if err != nil {
		log.Printf("totals alert: %s could not be counted: %v", event.Chain.Address, err)
		return nil
	}
	signed, err := a.readTotals(ctx, event.Chain.Address)
	if err != nil || signed == nil || signed.Locked == 0 {
		return err
	}
	if rows != signed.Totals {
		alertTotals(event.Chain.Address, "differs from this archive")
		if !bytes.Equal(signed.ChallengerKey, systemProgram) {
			return nil
		}
		raw, err := a.challengeTx(ctx, event.Chain.Address)
		if err != nil {
			return err
		}
		return a.submit(ctx, raw)
	}
	alertTotals(event.Chain.Address, "matches this archive")
	return nil
}

func (a *App) finalizeTotals(ctx context.Context, event *Event, chain *chainEvent) error {
	signed, err := a.readTotals(ctx, event.Chain.Address)
	if err != nil || signed == nil || signed.Locked == 0 || !bytes.Equal(signed.ChallengerKey, systemProgram) {
		return err
	}
	slot, err := a.rpc.slot(ctx)
	if err != nil {
		return err
	}
	if slot < signed.WindowSlot+challengeSlots {
		return nil
	}
	raw, err := a.finalizeTx(ctx, event.Chain.Admin, event.Chain.Address, signed.PosterKey)
	if err != nil {
		return err
	}
	if err = a.submit(ctx, raw); err != nil {
		return err
	}
	log.Printf("totals finalized on %s", event.Chain.Address)
	_ = chain
	return nil
}

func (a *App) submit(ctx context.Context, raw []byte) error {
	if err := a.rpc.simulate(ctx, raw); err != nil {
		return err
	}
	signature, err := a.rpc.send(ctx, raw)
	if err != nil {
		return err
	}
	if err = a.rpc.confirm(ctx, signature); err != nil {
		return err
	}
	if logs, err := a.rpc.transactionLogs(ctx, signature, "confirmed"); err == nil {
		_, _ = a.ingestSlipLogs(ctx, signature, logs)
	}
	return nil
}

func (s *solanaRPC) slot(ctx context.Context) (uint64, error) {
	var slot uint64
	err := s.call(ctx, "getSlot", []any{map[string]string{"commitment": "confirmed"}}, &slot)
	return slot, err
}

func totalsInstruction(rows [14]uint64) []byte {
	data := ixDisc("sign_totals")
	for _, n := range rows {
		data = append(data, putU64(n)...)
	}
	return data
}

type totalsKeys struct {
	event, vault, signers, canopy, totals, program []byte
}

func (a *App) signTotalsTx(ctx context.Context, signer ed25519.PrivateKey, eventAddress string, rows [14]uint64, lock bool) ([]byte, error) {
	keys, hash, err := a.totalsKeys(ctx, eventAddress)
	if err != nil {
		return nil, err
	}
	return buildSignTotals(signer, a.keeper, lock, keys, hash, rows)
}

func (a *App) totalsKeys(ctx context.Context, eventAddress string) (totalsKeys, []byte, error) {
	var keys totalsKeys
	var err error
	keys.program, err = decodeKey(a.cfg.ProgramID)
	if err != nil {
		return keys, nil, err
	}
	keys.event, err = decodeKey(eventAddress)
	if err != nil {
		return keys, nil, err
	}
	keys.vault, err = findPDA([][]byte{[]byte("vault"), keys.event}, keys.program)
	if err != nil {
		return keys, nil, err
	}
	keys.signers, err = findPDA([][]byte{[]byte("signers"), keys.event}, keys.program)
	if err != nil {
		return keys, nil, err
	}
	keys.canopy, err = findPDA([][]byte{[]byte("canopy"), keys.event}, keys.program)
	if err != nil {
		return keys, nil, err
	}
	keys.totals, err = findPDA([][]byte{[]byte("totals"), keys.event}, keys.program)
	if err != nil {
		return keys, nil, err
	}
	hash, err := a.rpc.latestBlockhash(ctx)
	return keys, hash, err
}

func buildSignTotals(signer, poster ed25519.PrivateKey, lock bool, keys totalsKeys, hash []byte, rows [14]uint64) ([]byte, error) {
	signerPub := signer.Public().(ed25519.PublicKey)
	posterPub := poster.Public().(ed25519.PublicKey)
	data := totalsInstruction(rows)
	if !lock {
		// The result signer pays. The poster is writable and does not sign, so no bond moves.
		return signAccounts([]ed25519.PrivateKey{signer}, []txAccount{
			{key: signerPub, signer: true, writable: true},
			{key: posterPub, writable: true},
			{key: keys.event, writable: true},
			{key: keys.vault, writable: true},
			{key: keys.totals, writable: true},
			{key: keys.signers},
			{key: keys.canopy},
			{key: systemProgram},
			{key: keys.program},
		}, []ixSpec{{program: 8, accounts: []int{0, 1, 2, 3, 5, 6, 4, 7}, data: data}}, hash)
	}
	// The poster pays the 3 SOL bond on the transaction that reaches the threshold.
	return signAccounts([]ed25519.PrivateKey{poster, signer}, []txAccount{
		{key: posterPub, signer: true, writable: true},
		{key: signerPub, signer: true, writable: true},
		{key: keys.event, writable: true},
		{key: keys.vault, writable: true},
		{key: keys.totals, writable: true},
		{key: keys.signers},
		{key: keys.canopy},
		{key: systemProgram},
		{key: keys.program},
	}, []ixSpec{{program: 8, accounts: []int{1, 0, 2, 3, 5, 6, 4, 7}, data: data}}, hash)
}

// handleLockTotals co-signs the browser transaction that reaches the signer threshold.
// A wallet cannot hold the reveal keeper, and the bond moves only when the keeper signs
// the same transaction as the signature that completes the count.
func (a *App) handleLockTotals(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Transaction string `json:"transaction"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	wallet := walletFrom(r)
	if wallet == "" || wallet == a.cfg.ResultCosigner || wallet == a.cosignerKeyAddress {
		writeError(w, http.StatusConflict, "the result cosigner signs on the server")
		return
	}
	if a.keeper == nil {
		writeError(w, http.StatusConflict, "the reveal keeper is not configured")
		return
	}
	event, ok := a.loadEvent(w, r)
	if !ok {
		return
	}
	if event.Chain == nil || event.Chain.Address == "" {
		writeError(w, http.StatusConflict, "this event has no on-chain vault")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	account, err := a.fetchAccount(ctx, event.Chain.Address)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	chain, err := decodeChainEvent(account, a.cfg.ProgramID)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if chain.Status != "tallying" || chain.SettlementPath != pathSigned || chain.CountStarted != 0 {
		writeError(w, http.StatusConflict, "this event is not waiting for signed totals")
		return
	}
	rows, err := a.archiveViewTotals(ctx, event, chain)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	again, err := a.archiveViewTotals(ctx, event, chain)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if rows != again {
		writeError(w, http.StatusConflict, "the count changed while it was being made")
		return
	}
	signers, err := a.resultSigners(ctx, event.Chain.Address)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	position := -1
	for i, member := range signers.Members {
		if member == wallet {
			position = i
		}
	}
	if position < 0 || position > 7 {
		writeError(w, http.StatusForbidden, "this wallet is not a result signer")
		return
	}
	signed, err := a.readTotals(ctx, event.Chain.Address)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	poster := a.keeper.Public().(ed25519.PublicKey)
	mask := byte(1 << position)
	if signed != nil && signed.Locked == 0 && signed.Poster == base58Encode(poster) && signed.Totals == rows {
		mask |= signed.Approvals
	}
	if bits.OnesCount8(mask) < signers.Threshold {
		writeError(w, http.StatusConflict, "this signature does not lock the totals")
		return
	}
	raw, err := base64.StdEncoding.DecodeString(body.Transaction)
	if err != nil {
		writeError(w, http.StatusBadRequest, "the transaction is not valid")
		return
	}
	keys, _, err := a.totalsKeys(ctx, event.Chain.Address)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	walletKey, err := decodeKey(wallet)
	if err != nil {
		writeError(w, http.StatusBadRequest, "the wallet address is invalid")
		return
	}
	locked, err := attachKeeperSignature(raw, a.keeper, totalsLock{
		wallet: walletKey, poster: poster, event: keys.event, vault: keys.vault,
		signers: keys.signers, canopy: keys.canopy, totals: keys.totals, program: keys.program, rows: rows,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err = a.submit(ctx, locked); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	log.Printf("totals locked on %s by %s", event.Chain.Address, wallet)
	writeJSON(w, http.StatusOK, map[string]string{"signature": base58Encode(locked[1:65])})
}

// totalsLock is the only sign_totals transaction the reveal keeper will co-sign.
type totalsLock struct {
	wallet, poster, event, vault, signers, canopy, totals, program []byte
	rows                                                           [14]uint64
}

// attachKeeperSignature checks that the wallet signed a totals lock, then signs it as the poster.
// The signature covers the message only, so filling the poster's slot does not change the wallet signature.
func attachKeeperSignature(tx []byte, keeper ed25519.PrivateKey, lock totalsLock) ([]byte, error) {
	if !bytes.Equal(keeper.Public().(ed25519.PublicKey), lock.poster) {
		return nil, errors.New("the reveal keeper key does not match")
	}
	count, rest, err := readShortVec(tx)
	if err != nil {
		return nil, err
	}
	if count < 2 || count > 8 || len(rest) < count*64+3 {
		return nil, errors.New("the transaction does not have both signatures")
	}
	sigs := rest[:count*64]
	msg := rest[count*64:]
	if int(msg[0]) != count {
		return nil, errors.New("the transaction signatures do not match the message")
	}
	body := msg[3:]
	nKeys, body, err := readShortVec(body)
	if err != nil {
		return nil, err
	}
	if nKeys < count || nKeys > 32 || len(body) < nKeys*32+32 {
		return nil, errors.New("the transaction accounts are truncated")
	}
	keys := make([][]byte, nKeys)
	for i := range keys {
		keys[i] = body[:32]
		body = body[32:]
	}
	body = body[32:]
	nIx, body, err := readShortVec(body)
	if err != nil {
		return nil, err
	}
	walletAt, posterAt := -1, -1
	for i := 0; i < count && i < len(keys); i++ {
		if bytes.Equal(keys[i], lock.wallet) {
			walletAt = i
		}
		if bytes.Equal(keys[i], lock.poster) {
			posterAt = i
		}
	}
	if walletAt < 0 {
		return nil, errors.New("the wallet does not sign this transaction")
	}
	if posterAt < 0 {
		return nil, errors.New("the reveal keeper does not sign this transaction")
	}
	if !ed25519.Verify(ed25519.PublicKey(lock.wallet), msg, sigs[walletAt*64:(walletAt+1)*64]) {
		return nil, errors.New("the wallet signature does not match this transaction")
	}
	budget := mustDecode(computeBudgetProgram)
	found := false
	for i := 0; i < nIx; i++ {
		if len(body) < 1 {
			return nil, errors.New("the transaction is truncated")
		}
		program := int(body[0])
		body = body[1:]
		var nAcc int
		nAcc, body, err = readShortVec(body)
		if err != nil || program >= len(keys) || len(body) < nAcc {
			return nil, errors.New("the transaction is truncated")
		}
		accs := make([]int, nAcc)
		for j := range accs {
			accs[j] = int(body[0])
			body = body[1:]
		}
		var nData int
		nData, body, err = readShortVec(body)
		if err != nil || len(body) < nData {
			return nil, errors.New("the transaction is truncated")
		}
		data := body[:nData]
		body = body[nData:]
		switch {
		case bytes.Equal(keys[program], budget):
			continue
		case bytes.Equal(keys[program], lock.program) && !found && bytes.Equal(data, totalsInstruction(lock.rows)):
			want := [][]byte{lock.wallet, lock.poster, lock.event, lock.vault, lock.signers, lock.canopy, lock.totals, systemProgram}
			if len(accs) != len(want) {
				return nil, errors.New("the transaction is not the totals signature")
			}
			for j, key := range want {
				if accs[j] >= len(keys) || !bytes.Equal(keys[accs[j]], key) {
					return nil, errors.New("the transaction is not the totals signature")
				}
			}
			found = true
		default:
			return nil, errors.New("the transaction is not the totals signature")
		}
	}
	if !found || len(body) != 0 {
		return nil, errors.New("the transaction is not the totals signature")
	}
	out := append([]byte(nil), tx...)
	off := len(tx) - len(rest) + posterAt*64
	copy(out[off:off+64], ed25519.Sign(keeper, msg))
	return out, nil
}

func readShortVec(in []byte) (int, []byte, error) {
	if len(in) == 0 {
		return 0, nil, errors.New("the transaction is truncated")
	}
	if in[0] < 128 {
		return int(in[0]), in[1:], nil
	}
	if len(in) < 2 {
		return 0, nil, errors.New("the transaction is truncated")
	}
	return int(in[0]&0x7f) | int(in[1])<<7, in[2:], nil
}

func (a *App) challengeTx(ctx context.Context, eventAddress string) ([]byte, error) {
	keys, hash, err := a.totalsKeys(ctx, eventAddress)
	if err != nil {
		return nil, err
	}
	return buildChallenge(a.keeper, keys, hash)
}

func buildChallenge(challenger ed25519.PrivateKey, keys totalsKeys, hash []byte) ([]byte, error) {
	pub := challenger.Public().(ed25519.PublicKey)
	return signAccounts([]ed25519.PrivateKey{challenger}, []txAccount{
		{key: pub, signer: true, writable: true},
		{key: keys.event, writable: true},
		{key: keys.vault, writable: true},
		{key: keys.totals, writable: true},
		{key: systemProgram},
		{key: keys.program},
	}, []ixSpec{{program: 5, accounts: []int{0, 1, 2, 3, 4}, data: ixDisc("challenge")}}, hash)
}

func (a *App) finalizeTx(ctx context.Context, admin, eventAddress string, poster []byte) ([]byte, error) {
	program, err := decodeKey(a.cfg.ProgramID)
	if err != nil {
		return nil, err
	}
	eventKey, err := decodeKey(eventAddress)
	if err != nil {
		return nil, err
	}
	adminKey, err := decodeKey(admin)
	if err != nil {
		return nil, err
	}
	vault, err := findPDA([][]byte{[]byte("vault"), eventKey}, program)
	if err != nil {
		return nil, err
	}
	totals, err := findPDA([][]byte{[]byte("totals"), eventKey}, program)
	if err != nil {
		return nil, err
	}
	schedule, err := findPDA([][]byte{[]byte("schedule"), adminKey}, program)
	if err != nil {
		return nil, err
	}
	kinds, err := findPDA([][]byte{[]byte("schedule-kinds"), adminKey}, program)
	if err != nil {
		return nil, err
	}
	league, err := findPDA([][]byte{[]byte("league")}, program)
	if err != nil {
		return nil, err
	}
	hash, err := a.rpc.latestBlockhash(ctx)
	if err != nil {
		return nil, err
	}
	caller := a.keeper.Public().(ed25519.PublicKey)
	return signAccounts([]ed25519.PrivateKey{a.keeper}, []txAccount{
		{key: caller, signer: true, writable: true},
		{key: eventKey, writable: true},
		{key: vault, writable: true},
		{key: totals, writable: true},
		{key: poster, writable: true},
		{key: schedule, writable: true},
		{key: kinds, writable: true},
		{key: league, writable: true},
		{key: program},
	}, []ixSpec{{program: 8, accounts: []int{0, 1, 2, 3, 4, 5, 6, 7}, data: ixDisc("finalize_totals")}}, hash)
}

func scoreRecountCanopyTx(keeper ed25519.PrivateKey, event, vault, schedule, kinds, league, canopy, program, data, blockhash []byte, pay *recountPay) ([]byte, error) {
	fee, err := findPDA([][]byte{[]byte("recount-fee"), event, keeper.Public().(ed25519.PublicKey)}, program)
	if err != nil {
		return nil, err
	}
	totals, err := findPDA([][]byte{[]byte("totals"), event}, program)
	if err != nil {
		return nil, err
	}
	budget := mustDecode(computeBudgetProgram)
	caller := keeper.Public().(ed25519.PublicKey)
	return signAccounts([]ed25519.PrivateKey{keeper}, []txAccount{
		{key: caller, signer: true, writable: true},
		{key: event, writable: true},
		{key: vault, writable: true},
		{key: schedule, writable: true},
		{key: kinds, writable: true},
		{key: league, writable: true},
		{key: totals, writable: true},
		{key: fee, writable: true},
		{key: pay.poster, writable: true},
		{key: pay.challenger, writable: true},
		{key: canopy},
		{key: budget},
		{key: systemProgram},
		{key: program},
	}, []ixSpec{
		{program: 11, data: unitLimit},
		{program: 13, accounts: []int{0, 1, 2, 10, 3, 4, 5, 6, 7, 12, 8, 9}, data: data},
	}, blockhash)
}

func scoreRecountSlipTx(keeper ed25519.PrivateKey, event, vault, slips, schedule, kinds, league, program, data, blockhash []byte, pay *recountPay) ([]byte, error) {
	fee, err := findPDA([][]byte{[]byte("recount-fee"), event, keeper.Public().(ed25519.PublicKey)}, program)
	if err != nil {
		return nil, err
	}
	totals, err := findPDA([][]byte{[]byte("totals"), event}, program)
	if err != nil {
		return nil, err
	}
	caller := keeper.Public().(ed25519.PublicKey)
	return signAccounts([]ed25519.PrivateKey{keeper}, []txAccount{
		{key: caller, signer: true, writable: true},
		{key: event, writable: true},
		{key: vault, writable: true},
		{key: slips, writable: true},
		{key: schedule, writable: true},
		{key: kinds, writable: true},
		{key: league, writable: true},
		{key: totals, writable: true},
		{key: fee, writable: true},
		{key: pay.poster, writable: true},
		{key: pay.challenger, writable: true},
		{key: systemProgram},
		{key: program},
	}, []ixSpec{{program: 12, accounts: []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 11, 9, 10}, data: data}}, blockhash)
}

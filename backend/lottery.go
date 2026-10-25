package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"log"
	"net/http"
	"time"

	"filippo.io/edwards25519"
)

const (
	slotHashesSysvar  = "SysvarS1otHashes111111111111111111111111111"
	couponAccountSize = 157
	lotteryBodySize   = 8312
	ownersOffset      = 126
)

func (a *App) handleLottery(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	view, err := a.lotteryView(ctx, walletFrom(r))
	if err != nil {
		writeError(w, http.StatusBadGateway, "could not read the monthly draw")
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (a *App) lotteryView(ctx context.Context, wallet string) (map[string]any, error) {
	admin, err := base58Decode(a.cfg.TreasuryWallet)
	if err != nil {
		return nil, err
	}
	program, err := base58Decode(a.cfg.ProgramID)
	if err != nil {
		return nil, err
	}
	lotteryKey, err := findPDA([][]byte{[]byte("lottery"), admin}, program)
	if err != nil {
		return nil, err
	}
	account, err := a.fetchAccount(ctx, base58Encode(lotteryKey))
	if err != nil {
		return nil, err
	}
	if account == nil || len(account.Data) < 8+lotteryBodySize {
		return map[string]any{"ready": false, "potLamports": 0, "tickets": 0, "myTickets": 0, "monthEndsAt": monthEndOfID(monthID(time.Now().Unix())), "drawSlot": 0}, nil
	}
	state, err := decodeLottery(account.Data)
	if err != nil {
		return nil, err
	}
	vaultKey, err := findPDA([][]byte{[]byte("lottery-vault"), lotteryKey}, program)
	if err != nil {
		return nil, err
	}
	vault, err := a.fetchAccount(ctx, base58Encode(vaultKey))
	if err != nil {
		return nil, err
	}
	var pot uint64
	if vault != nil && vault.Lamports > state.RentReserve {
		pot = vault.Lamports - state.RentReserve
	}
	walletBytes, _ := base58Decode(wallet)
	mine := 0
	for i := 0; i < int(state.Tickets) && i < len(state.Owners); i++ {
		if len(walletBytes) == 32 && equal32(state.Owners[i], walletBytes) {
			mine++
		}
	}
	month := state.Month
	if state.Tickets == 0 {
		month = monthID(time.Now().Unix())
	}
	view := map[string]any{
		"ready":       true,
		"potLamports": pot,
		"tickets":     state.Tickets,
		"myTickets":   mine,
		"monthEndsAt": monthEndOfID(month),
		"drawSlot":    state.DrawSlot,
	}
	if state.Paid > 0 {
		view["last"] = map[string]any{
			"month":        state.DrawnMonth,
			"winner":       base58Encode(state.Winner),
			"paidLamports": state.Paid,
			"index":        state.WinningIndex,
		}
	}
	return view, nil
}

func (a *App) lotteryLoop() {
	if a.keeper == nil || a.rpc == nil {
		return
	}
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	a.tendLottery()
	for range ticker.C {
		a.tendLottery()
	}
}

func (a *App) tendLottery() {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := a.tendLotteryOnce(ctx); err != nil {
		log.Printf("monthly draw: %v", err)
	}
}

func (a *App) tendLotteryOnce(ctx context.Context) error {
	program, err := base58Decode(a.cfg.ProgramID)
	if err != nil {
		return err
	}
	clock, err := a.fetchAccount(ctx, clockSysvar)
	if err != nil || clock == nil || len(clock.Data) < 40 {
		return err
	}
	now := int64(binary.LittleEndian.Uint64(clock.Data[32:40]))
	a.closeLosers(ctx, program, now)
	return nil
}

func (a *App) scheduleDraw(ctx context.Context, program, lotteryKey, vaultKey []byte) error {
	blockhash, err := a.rpc.latestBlockhash(ctx)
	if err != nil {
		return err
	}
	tx, err := signLegacy(a.keeper, [][]byte{lotteryKey, vaultKey}, [][]byte{program}, []ixSpec{{
		program: 3, accounts: []int{0, 1, 2}, data: ixDisc("schedule_lottery_draw"),
	}}, blockhash)
	if err != nil {
		return err
	}
	if err := a.rpc.simulate(ctx, tx); err != nil {
		return err
	}
	sig, err := a.rpc.send(ctx, tx)
	if err != nil {
		return err
	}
	log.Printf("monthly draw scheduled: %s", sig)
	return a.rpc.confirm(ctx, sig)
}

func (a *App) takeDraw(ctx context.Context, program, lotteryKey, vaultKey []byte, state *lotteryState, slot uint64) error {
	hashes, err := a.fetchAccount(ctx, slotHashesSysvar)
	if err != nil || hashes == nil {
		return err
	}
	hash, ok := slotHash(hashes.Data, state.DrawSlot)
	winner := a.keeper.Public().(ed25519.PublicKey)
	if ok && state.Tickets > 0 {
		index := binary.LittleEndian.Uint64(hash[:8]) % uint64(state.Tickets)
		if int(index) < len(state.Owners) {
			winner = state.Owners[index]
		}
	} else if slot <= state.DrawSlot+512 {
		return nil
	}
	blockhash, err := a.rpc.latestBlockhash(ctx)
	if err != nil {
		return err
	}
	// 0 keeper, 1 lottery, 2 vault, 3 winner, 4 slot hashes, 5 compute budget, 6 program.
	keys := [][]byte{lotteryKey, vaultKey}
	if !equal32(winner, a.keeper.Public().(ed25519.PublicKey)) {
		keys = append(keys, winner)
	}
	winnerIndex := 0
	if !equal32(winner, a.keeper.Public().(ed25519.PublicKey)) {
		winnerIndex = 3
	}
	budget := []byte{2, 0x80, 0x1a, 0x06, 0x00}
	readonly := [][]byte{mustDecode(slotHashesSysvar), mustDecode("ComputeBudget111111111111111111111111111111"), program}
	tx, err := signLegacy(a.keeper, keys, readonly, []ixSpec{
		{program: len(keys) + 2, accounts: nil, data: budget},
		{program: len(keys) + 3, accounts: []int{0, 1, 2, winnerIndex, len(keys) + 1}, data: ixDisc("take_lottery_draw")},
	}, blockhash)
	if err != nil {
		return err
	}
	if err := a.rpc.simulate(ctx, tx); err != nil {
		return err
	}
	sig, err := a.rpc.send(ctx, tx)
	if err != nil {
		return err
	}
	log.Printf("monthly draw paid: %s", sig)
	return a.rpc.confirm(ctx, sig)
}

func (a *App) closeLosers(ctx context.Context, program []byte, now int64) {
	coupons, err := a.rpc.programAccounts(ctx, a.cfg.ProgramID, couponAccountSize)
	if err != nil {
		log.Printf("monthly draw coupons: %v", err)
		return
	}
	events := map[string]*chainAccount{}
	closed := 0
	tried := 0
	for _, coupon := range coupons {
		if closed >= 2 || tried >= 8 {
			return
		}
		if len(coupon.Data) < 123 {
			continue
		}
		eventKey := coupon.Data[8:40]
		eventID := base58Encode(eventKey)
		event := events[eventID]
		if event == nil {
			loaded, loadErr := a.fetchAccount(ctx, eventID)
			if loadErr != nil || loaded == nil {
				continue
			}
			events[eventID] = loaded
			event = loaded
		}
		if len(event.Data) < 124 || event.Data[116] != 1 {
			continue
		}
		revealed := coupon.Data[122] == 1
		revealBy := int64(binary.LittleEndian.Uint64(event.Data[92:100]))
		if !revealed && now < revealBy {
			continue
		}
		tried++
		if err := a.closeLost(ctx, program, mustDecode(coupon.Pubkey), eventKey, coupon.Data[40:72]); err != nil {
			continue
		}
		closed++
	}
}

func (a *App) closeLost(ctx context.Context, program, coupon, event, owner []byte) error {
	blockhash, err := a.rpc.latestBlockhash(ctx)
	if err != nil {
		return err
	}
	// 0 keeper, 1 coupon, 2 owner, 3 event, 4 program.
	tx, err := signLegacy(a.keeper, [][]byte{coupon, owner}, [][]byte{event, program}, []ixSpec{{
		program: 4, accounts: []int{1, 3, 2, 0}, data: ixDisc("close_lost"),
	}}, blockhash)
	if err != nil {
		return err
	}
	if err := a.rpc.simulate(ctx, tx); err != nil {
		return err
	}
	sig, err := a.rpc.send(ctx, tx)
	if err != nil {
		return err
	}
	log.Printf("losing coupon closed, rent returned: %s", sig)
	return a.rpc.confirm(ctx, sig)
}

type lotteryState struct {
	Month        uint32
	Tickets      uint32
	PotSnapshot  uint64
	DrawSlot     uint64
	DrawnMonth   uint32
	WinningIndex uint32
	Winner       []byte
	Paid         uint64
	RentReserve  uint64
	Owners       [][]byte
}

func decodeLottery(data []byte) (*lotteryState, error) {
	state := &lotteryState{
		Winner:       append([]byte(nil), data[40:72]...),
		PotSnapshot:  binary.LittleEndian.Uint64(data[72:80]),
		DrawSlot:     binary.LittleEndian.Uint64(data[80:88]),
		Paid:         binary.LittleEndian.Uint64(data[88:96]),
		RentReserve:  binary.LittleEndian.Uint64(data[96:104]),
		Month:        binary.LittleEndian.Uint32(data[104:108]),
		Tickets:      binary.LittleEndian.Uint32(data[108:112]),
		DrawnMonth:   binary.LittleEndian.Uint32(data[116:120]),
		WinningIndex: binary.LittleEndian.Uint32(data[120:124]),
	}
	count := int(state.Tickets)
	if count > 256 {
		count = 256
	}
	for i := 0; i < count; i++ {
		at := ownersOffset + i*32
		state.Owners = append(state.Owners, append([]byte(nil), data[at:at+32]...))
	}
	return state, nil
}

func (s *solanaRPC) programAccounts(ctx context.Context, program string, dataSize int) ([]struct {
	Pubkey string
	Data   []byte
}, error) {
	var out []struct {
		Pubkey  string `json:"pubkey"`
		Account struct {
			Data [2]string `json:"data"`
		} `json:"account"`
	}
	err := s.call(ctx, "getProgramAccounts", []any{program, map[string]any{
		"encoding":   "base64",
		"commitment": "confirmed",
		"filters":    []any{map[string]any{"dataSize": dataSize}},
	}}, &out)
	if err != nil {
		return nil, err
	}
	rows := make([]struct {
		Pubkey string
		Data   []byte
	}, 0, len(out))
	for _, row := range out {
		decoded, err := base64.StdEncoding.DecodeString(row.Account.Data[0])
		if err != nil {
			continue
		}
		rows = append(rows, struct {
			Pubkey string
			Data   []byte
		}{Pubkey: row.Pubkey, Data: decoded})
	}
	return rows, nil
}

func findPDA(seeds [][]byte, program []byte) ([]byte, error) {
	for bump := 255; bump >= 0; bump-- {
		hash := sha256.New()
		for _, seed := range seeds {
			hash.Write(seed)
		}
		hash.Write([]byte{byte(bump)})
		hash.Write(program)
		hash.Write([]byte("ProgramDerivedAddress"))
		out := hash.Sum(nil)
		if _, err := new(edwards25519.Point).SetBytes(out); err != nil {
			return out, nil
		}
	}
	return nil, errNoPDA
}

var errNoPDA = errString("no program address for those seeds")

type errString string

func (e errString) Error() string { return string(e) }

func ixDisc(name string) []byte {
	sum := sha256.Sum256([]byte("global:" + name))
	return sum[:8]
}

func slotHash(data []byte, slot uint64) ([]byte, bool) {
	if len(data) < 8 {
		return nil, false
	}
	count := binary.LittleEndian.Uint64(data[:8])
	rest := data[8:]
	for i := uint64(0); i < count; i++ {
		if len(rest) < 40 {
			return nil, false
		}
		if binary.LittleEndian.Uint64(rest[:8]) == slot {
			return append([]byte(nil), rest[8:40]...), true
		}
		rest = rest[40:]
	}
	return nil, false
}

func monthID(unix int64) uint32 {
	year, month, _ := civilYMD(unix)
	return uint32(year)*12 + uint32(month) - 1
}

func monthEndOfID(id uint32) int64 {
	year := int32(id / 12)
	monthIndex := id % 12
	nextYear, nextMonth := year, monthIndex+2
	if monthIndex == 11 {
		nextYear = year + 1
		nextMonth = 1
	}
	return daysFromCivil(nextYear, nextMonth, 1) * 86400
}

func civilYMD(unix int64) (int32, uint32, uint32) {
	z := unix/86400 + 719468
	era := z / 146097
	doe := uint64(z - era*146097)
	yoe := (doe - doe/1460 + doe/36524 - doe/146096) / 365
	y := int32(yoe) + int32(era)*400
	doy := doe - (365*yoe + yoe/4 - yoe/100)
	mp := (5*doy + 2) / 153
	day := doy - (153*mp+2)/5 + 1
	var month uint32
	if mp < 10 {
		month = uint32(mp + 3)
	} else {
		month = uint32(mp - 9)
	}
	year := y
	if month <= 2 {
		year = y + 1
	}
	return year, month, uint32(day)
}

func daysFromCivil(year int32, month, day uint32) int64 {
	if month <= 2 {
		year--
	}
	era := int64(year) / 400
	if year < 0 && int64(year)%400 != 0 {
		era--
	}
	yoe := uint64(int64(year) - era*400)
	monthPrime := month - 3
	if month <= 2 {
		monthPrime = month + 9
	}
	doy := (153*monthPrime+2)/5 + day - 1
	doe := yoe*365 + yoe/4 - yoe/100 + uint64(doy)
	return era*146097 + int64(doe) - 719468
}

func equal32(a, b []byte) bool {
	if len(a) != 32 || len(b) != 32 {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func mustDecode(value string) []byte {
	out, err := base58Decode(value)
	if err != nil {
		return make([]byte, 32)
	}
	return out
}

package main

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	eventProposalKindOffset    = 290
	eventProposalResultsOffset = 291
	eventApprovalsOffset       = 304
	proposalResolve            = 1
	proposalCancel             = 2
)

type chainProposal struct {
	Kind      byte
	Results   string
	Approvals byte
}

type autoDecision struct {
	Results string
	Block   string
	Wait    string
}

func autoResolveBlock(games []Game) string {
	for i, game := range games {
		if game.FixtureID <= 0 {
			return fmt.Sprintf("Game %d is not linked to a match, so auto resolve is waiting.", i+1)
		}
	}
	return ""
}

func readProposal(data []byte, gameCount int) (chainProposal, bool) {
	if len(data) < eventApprovalsOffset+1 || gameCount < 0 || gameCount > maxGames {
		return chainProposal{}, false
	}
	return chainProposal{
		Kind:      data[eventProposalKindOffset],
		Results:   string(data[eventProposalResultsOffset : eventProposalResultsOffset+gameCount]),
		Approvals: data[eventApprovalsOffset],
	}, true
}

func signerApproved(approvals byte, members []string, who string) bool {
	for i, member := range members {
		if member == who && i < 8 {
			return approvals&(1<<uint(i)) != 0
		}
	}
	return false
}

func sameGoals(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func sameReading(a, b fixtureState) bool {
	return a.Status == b.Status && sameGoals(a.Home, b.Home) && sameGoals(a.Away, b.Away) && sameGoals(a.FullHome, b.FullHome) && sameGoals(a.FullAway, b.FullAway)
}

// terminalOutcome maps one finished reading. A live or suspended match waits.
// Cancelled and postponed become C. Anything else that will not finish is a block.
func terminalOutcome(state fixtureState) (byte, string) {
	switch state.Status {
	case "CANC", "PST":
		return 'C', ""
	case "FT", "AET", "PEN":
		outcome, _, _, final := projectFixture(state)
		if !final || len(outcome) != 1 {
			return 0, "wait"
		}
		return outcome[0], ""
	case "ABD", "AWD", "WO":
		return 0, "block"
	default:
		return 0, "wait"
	}
}

// decideAutoResolve accepts a result only when both readings of every game agree
// and every game is finished, cancelled, or postponed.
func decideAutoResolve(games []Game, first, second map[int]fixtureState) autoDecision {
	if len(games) == 0 {
		return autoDecision{Wait: "the event has no games yet"}
	}
	if block := autoResolveBlock(games); block != "" {
		return autoDecision{Block: block}
	}
	out := make([]byte, len(games))
	for i, game := range games {
		left, ok := first[game.FixtureID]
		right, ok2 := second[game.FixtureID]
		if !ok || !ok2 {
			return autoDecision{Wait: fmt.Sprintf("the score feed has no reading for game %d", i+1)}
		}
		if !sameReading(left, right) {
			return autoDecision{Wait: fmt.Sprintf("the two readings of game %d do not agree", i+1)}
		}
		mark, kind := terminalOutcome(left)
		switch kind {
		case "block":
			return autoDecision{Block: fmt.Sprintf("Game %d finished in a way auto resolve does not score. Set the result by hand.", i+1)}
		case "wait":
			return autoDecision{Wait: fmt.Sprintf("game %d is still in play", i+1)}
		}
		out[i] = mark
	}
	return autoDecision{Results: string(out)}
}

func (a *App) loadResolutionKeys() {
	if path := keypairPath("deployer-keypair.json"); path == "" {
		log.Print("auto resolve cannot sign as the admin: keys/deployer-keypair.json is missing")
	} else if key, address, err := readKeeper(path); err != nil {
		log.Printf("auto resolve admin key: %v", err)
	} else {
		a.adminKey = key
		a.adminKeyAddress = address
		log.Printf("auto resolve admin signer %s", address)
	}
	if path := keypairPath("council-signer.json"); path == "" {
		log.Print("auto resolve cannot sign as the cosigner: keys/council-signer.json is missing")
	} else if key, address, err := readKeeper(path); err != nil {
		log.Printf("auto resolve cosigner key: %v", err)
	} else if address != a.cfg.ResultCosigner {
		log.Printf("auto resolve cosigner key is %s, expected %s", address, a.cfg.ResultCosigner)
	} else {
		a.cosignerKey = key
		a.cosignerKeyAddress = address
		log.Printf("auto resolve cosigner %s", address)
	}
}

func wantAutoResolve(flag *bool) bool {
	return flag == nil || *flag
}

func keypairPath(name string) string {
	for _, path := range []string{
		filepath.Join("..", "programs", "predictions", "keys", name),
		filepath.Join("programs", "predictions", "keys", name),
	} {
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() {
			return path
		}
	}
	return ""
}

func (a *App) resolveLoop() {
	if a.rpc == nil || a.fetchAccount == nil || a.feed == nil || a.feed.key == "" {
		return
	}
	log.Print("automatic resolve is on")
	ticker := time.NewTicker(45 * time.Second)
	defer ticker.Stop()
	a.resolveDue()
	for range ticker.C {
		a.resolveDue()
	}
}

func (a *App) resolveDue() {
	if a.events == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cursor, err := a.events.Find(ctx, bson.M{
		"autoResolve":   true,
		"chain.address": bson.M{"$exists": true, "$ne": ""},
		"chain.status":  bson.M{"$nin": []string{"resolved", "cancelled", "tallying"}},
	})
	if err != nil {
		log.Printf("auto resolve: %v", err)
		return
	}
	defer cursor.Close(ctx)
	for cursor.Next(ctx) {
		var event Event
		if err = cursor.Decode(&event); err != nil || event.Chain == nil || event.Chain.Address == "" || !event.AutoResolve {
			continue
		}
		if err = a.resolveEvent(ctx, &event); err != nil {
			a.noteResolve(event.Chain.Address, err.Error())
		}
	}
}

func (a *App) resolveEvent(ctx context.Context, event *Event) error {
	if block := autoResolveBlock(event.Games); block != "" {
		a.noteResolve(event.Chain.Address, block)
		return nil
	}
	account, err := a.fetchAccount(ctx, event.Chain.Address)
	if err != nil {
		return err
	}
	if account == nil || account.Owner != a.cfg.ProgramID {
		return nil
	}
	chain, err := decodeChainEvent(account, a.cfg.ProgramID)
	if err != nil {
		return err
	}
	if chain.Status != "open" || chain.GameCount != len(event.Games) {
		return nil
	}
	now := time.Now().Unix()
	if now < chain.EndTime {
		return nil
	}
	if now >= chain.ResolveTimeout {
		a.noteResolve(event.Chain.Address, "the resolution window has closed")
		return nil
	}
	first, err := a.feed.readFixtures(ctx, event.Games)
	if err != nil {
		return err
	}
	_ = a.storeFixtures(ctx, first)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(3 * time.Second):
	}
	second, err := a.feed.readFixtures(ctx, event.Games)
	if err != nil {
		return err
	}
	_ = a.storeFixtures(ctx, second)
	decision := decideAutoResolve(event.Games, first, second)
	switch {
	case decision.Block != "":
		a.noteResolve(event.Chain.Address, decision.Block)
		return nil
	case decision.Results == "":
		a.noteResolve(event.Chain.Address, decision.Wait)
		return nil
	}
	if err = a.submitResolution(ctx, event, account.Data, decision.Results); err != nil {
		return err
	}
	a.noteResolve(event.Chain.Address, "submitted "+decision.Results)
	return nil
}

func (a *App) noteResolve(event, message string) {
	if a.resolveNotes == nil {
		a.resolveNotes = map[string]string{}
	}
	if a.resolveNotes[event] == message {
		return
	}
	a.resolveNotes[event] = message
	if message != "" {
		log.Printf("auto resolve %s: %s", event, message)
	}
}

func (a *App) submitResolution(ctx context.Context, event *Event, data []byte, results string) error {
	proposal, ok := readProposal(data, len(results))
	if !ok {
		return errors.New("the event account has no proposal")
	}
	if proposal.Kind == proposalCancel && proposal.Approvals != 0 {
		return errors.New("a cancellation is already being approved")
	}
	if proposal.Kind == proposalResolve && proposal.Results != results && proposal.Approvals != 0 {
		return errors.New("different results are already being approved")
	}
	eventKey, err := decodeKey(event.Chain.Address)
	if err != nil {
		return err
	}
	program, err := decodeKey(a.cfg.ProgramID)
	if err != nil {
		return err
	}
	signersKey, err := findPDA([][]byte{[]byte("signers"), eventKey}, program)
	if err != nil {
		return err
	}
	signersAccount, err := a.fetchAccount(ctx, base58Encode(signersKey))
	if err != nil {
		return err
	}
	signers, err := decodeChainSigners(signersAccount, a.cfg.ProgramID)
	if err != nil {
		return err
	}
	agreed := proposal.Kind == proposalResolve && proposal.Results == results
	sent := 0
	for _, key := range []ed25519.PrivateKey{a.adminKey, a.cosignerKey} {
		if key == nil {
			continue
		}
		address := base58Encode(key.Public().(ed25519.PublicKey))
		if address != event.Chain.Admin && address != event.Chain.Cosigner {
			continue
		}
		if agreed && signerApproved(proposal.Approvals, signers.Members, address) {
			continue
		}
		raw, err := a.resolutionTransaction(ctx, key, event.Chain.Admin, event.Chain.Address, results)
		if err != nil {
			return err
		}
		if err = a.rpc.simulate(ctx, raw); err != nil {
			return err
		}
		signature, err := a.rpc.send(ctx, raw)
		if err != nil {
			return err
		}
		if err = a.rpc.confirm(ctx, signature); err != nil {
			return err
		}
		sent++
		log.Printf("auto resolve approval %s on %s", signature, event.Chain.Address)
	}
	if sent == 0 {
		return errors.New("auto resolve cannot sign this event")
	}
	return nil
}

func (a *App) resolutionTransaction(ctx context.Context, signer ed25519.PrivateKey, admin, eventAddress, results string) ([]byte, error) {
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
	signers, err := findPDA([][]byte{[]byte("signers"), eventKey}, program)
	if err != nil {
		return nil, err
	}
	schedule, err := findPDA([][]byte{[]byte("schedule"), adminKey}, program)
	if err != nil {
		return nil, err
	}
	hash, err := a.rpc.latestBlockhash(ctx)
	if err != nil {
		return nil, err
	}
	data := append(ixDisc("approve_resolution"), putU32(uint32(len(results)))...)
	data = append(data, results...)
	// 0 signer, 1 event, 2 schedule, 3 signers, 4 program.
	return signLegacy(signer, [][]byte{eventKey, schedule}, [][]byte{signers, program}, []ixSpec{{
		program: 4, accounts: []int{0, 1, 3, 2}, data: data,
	}}, hash)
}

func decodeKey(value string) ([]byte, error) {
	raw, err := base58Decode(value)
	if err != nil || len(raw) != 32 {
		return nil, errors.New("the event address is invalid")
	}
	return raw, nil
}

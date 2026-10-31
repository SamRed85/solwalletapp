package main

import (
	"testing"
	"time"
)

func finished(status string, home, away int) fixtureState {
	return fixtureState{Status: status, FullHome: intptr(home), FullAway: intptr(away), Home: intptr(home), Away: intptr(away)}
}

func TestAutoResolveAcceptsTwoMatchingFinishedReadings(t *testing.T) {
	games := []Game{{FixtureID: 1}, {FixtureID: 2}, {FixtureID: 3}}
	first := map[int]fixtureState{
		1: finished("FT", 2, 0),
		2: finished("FT", 1, 1),
		3: finished("PST", 0, 0),
	}
	first[2] = finished("AET", 1, 1)
	first[3] = fixtureState{Status: "PST"}
	second := map[int]fixtureState{
		1: finished("FT", 2, 0),
		2: finished("AET", 1, 1),
		3: {Status: "PST"},
	}
	got := decideAutoResolve(games, first, second)
	if got.Results != "1XC" || got.Block != "" || got.Wait != "" {
		t.Fatalf("decision %+v", got)
	}
}

func TestAutoResolveWaitsForALiveScore(t *testing.T) {
	games := []Game{{FixtureID: 7}}
	live := fixtureState{Status: "1H", Home: intptr(1), Away: intptr(0)}
	got := decideAutoResolve(games, map[int]fixtureState{7: live}, map[int]fixtureState{7: live})
	if got.Results != "" || got.Block != "" || got.Wait == "" {
		t.Fatalf("a live score settled: %+v", got)
	}
}

func TestAutoResolveWaitsWhenTheReadingsDiffer(t *testing.T) {
	games := []Game{{FixtureID: 4}}
	got := decideAutoResolve(games, map[int]fixtureState{4: finished("FT", 1, 0)}, map[int]fixtureState{4: finished("FT", 0, 1)})
	if got.Results != "" || got.Wait == "" {
		t.Fatalf("different scores settled: %+v", got)
	}
}

func TestAutoResolveBlocksAnUnlinkedGame(t *testing.T) {
	got := decideAutoResolve([]Game{{HomeTeam: "Home", AwayTeam: "Away"}}, nil, nil)
	if got.Results != "" || got.Block == "" {
		t.Fatalf("an unlinked game was scored: %+v", got)
	}
}

func TestAutoResolveUsesSavedScoresUntilTheyAreFinal(t *testing.T) {
	games := []Game{{FixtureID: 7}}
	live := map[int]fixtureState{7: {Status: "2H", Home: intptr(1), Away: intptr(0)}}
	settled, blocked := savedScoreState(games, live)
	if settled || blocked {
		t.Fatal("a live saved score looked finished")
	}
	now := time.Now()
	if !allowScoreFetch(0, time.Time{}, now) {
		t.Fatal("the first look at an unfinished saved score was skipped")
	}
	if allowScoreFetch(1, now, now.Add(time.Minute)) {
		t.Fatal("a second feed call was allowed inside the gap")
	}
	if !allowScoreFetch(1, now, now.Add(resolveScoreGap)) {
		t.Fatal("a later confirm of an unfinished score was skipped")
	}
	if allowScoreFetch(resolveScoreFetches, time.Time{}, now) {
		t.Fatal("the feed was called again after the saved scores stayed unfinished")
	}
	if resolveScoreFetches != 5 || resolveScoreGap != 5*time.Minute {
		t.Fatalf("retries = %d gap = %s", resolveScoreFetches, resolveScoreGap)
	}
	notice := scoreGiveUpMessage("game 1 is still in play")
	if notice != "Auto resolve stopped after 5 tries, 5 minutes apart: game 1 is still in play. Set the results by hand." {
		t.Fatal(notice)
	}
	final := map[int]fixtureState{7: finished("FT", 1, 0)}
	settled, blocked = savedScoreState(games, final)
	if !settled || blocked {
		t.Fatal("a finished saved score was not used")
	}
	got := decideAutoResolve(games, final, final)
	if got.Results != "1" {
		t.Fatalf("saved score decision %+v", got)
	}
}

func TestNewEventsAutoResolveUnlessTurnedOff(t *testing.T) {
	on := true
	off := false
	if !wantAutoResolve(nil) || !wantAutoResolve(&on) || wantAutoResolve(&off) {
		t.Fatal("auto resolve default is wrong")
	}
}

func TestProposalBitsSkipASignerWhoAlreadyAgreed(t *testing.T) {
	data := make([]byte, eventApprovalsOffset+1)
	data[eventProposalKindOffset] = proposalResolve
	copy(data[eventProposalResultsOffset:], []byte("1X2"))
	data[eventApprovalsOffset] = 1
	got, ok := readProposal(data, 3)
	if !ok || got.Kind != proposalResolve || got.Results != "1X2" || got.Approvals != 1 {
		t.Fatalf("proposal %+v ok %v", got, ok)
	}
	members := []string{"admin", "cosigner"}
	if !signerApproved(got.Approvals, members, "admin") || signerApproved(got.Approvals, members, "cosigner") {
		t.Fatal("the approval bit pointed at the wrong signer")
	}
}

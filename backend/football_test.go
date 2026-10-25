package main

import (
	"errors"
	"testing"
	"time"
)

func intptr(n int) *int { return &n }

func TestProjectFixtureUsesRegularTime(t *testing.T) {
	one, home, away, final := projectFixture(fixtureState{Status: "2H", Home: intptr(2), Away: intptr(1)})
	if one != "1" || final || *home != 2 || *away != 1 {
		t.Fatalf("live score = %s %v final %v", one, []int{*home, *away}, final)
	}
	level, _, _, final := projectFixture(fixtureState{Status: "FT", FullHome: intptr(1), FullAway: intptr(1), Home: intptr(1), Away: intptr(1)})
	if level != "X" || !final {
		t.Fatalf("full time = %s final %v", level, final)
	}
	draw, _, _, _ := projectFixture(fixtureState{Status: "PEN", FullHome: intptr(1), FullAway: intptr(1), Home: intptr(1), Away: intptr(2)})
	if draw != "X" {
		t.Fatalf("penalties changed the result to %s", draw)
	}
	if outcome, _, _, final := projectFixture(fixtureState{Status: "NS"}); outcome != "X" || final {
		t.Fatalf("a match that has not started projected %s final %v", outcome, final)
	}
	if outcome, _, _, _ := projectFixture(fixtureState{Status: "CANC", Home: intptr(1), Away: intptr(0)}); outcome != "" {
		t.Fatalf("a cancelled match projected %s", outcome)
	}
}

func TestProjectionIsAMaximumUntilTheRevealWindowCloses(t *testing.T) {
	now := time.Now()
	games := []Game{{FixtureID: 7}, {FixtureID: 8}}
	scores := map[int]fixtureState{
		7: {Status: "FT", FullHome: intptr(1), FullAway: intptr(0)},
		8: {Status: "1H", Home: intptr(0), Away: intptr(0)},
	}
	tiers := []Tier{{Correct: 2, Percent: 70}, {Correct: 1, Percent: 25}}
	open := buildProjection(games, scores, now, now.Add(time.Hour), 1_000_000, 0, tiers, map[int]int{2: 9})
	if !open.Complete || !open.Maximum {
		t.Fatalf("open projection complete %v maximum %v", open.Complete, open.Maximum)
	}
	if open.Tiers[0].PerWinnerLamports != 700_000 {
		t.Fatalf("maximum payout = %d", open.Tiers[0].PerWinnerLamports)
	}
	shared := buildProjection(games, scores, now, now.Add(-time.Minute), 1_000_000, 0, tiers, map[int]int{2: 2})
	if shared.Maximum || shared.Tiers[0].PerWinnerLamports != 350_000 {
		t.Fatalf("shared payout maximum %v each %d", shared.Maximum, shared.Tiers[0].PerWinnerLamports)
	}
	waiting := buildProjection([]Game{{FixtureID: 7}, {FixtureID: 9}}, scores, now, now.Add(time.Hour), 1_000_000, 0, tiers, nil)
	if waiting.Complete || len(waiting.Tiers) != 0 {
		t.Fatal("a match without a score produced a payout")
	}
}

func TestScoreRefreshIsOnlyAtFortyFiveAndNinety(t *testing.T) {
	start := time.Date(2026, 10, 25, 15, 0, 0, 0, time.UTC)
	if marks := dueScoreMarks(start, start.Add(44*time.Minute), nil, "e"); len(marks) != 0 {
		t.Fatalf("early marks %v", marks)
	}
	if marks := dueScoreMarks(start, start.Add(45*time.Minute), nil, "e"); len(marks) != 1 || marks[0] != "45" {
		t.Fatalf("45-minute marks %v", marks)
	}
	done := map[string]bool{"e:45": true}
	if marks := dueScoreMarks(start, start.Add(46*time.Minute), done, "e"); len(marks) != 0 {
		t.Fatalf("repeat marks %v", marks)
	}
	if marks := dueScoreMarks(start, start.Add(90*time.Minute), done, "e"); len(marks) != 1 || marks[0] != "90" {
		t.Fatalf("90-minute marks %v", marks)
	}
	if marks := dueScoreMarks(start, start.Add(3*time.Hour), nil, "e"); len(marks) != 0 {
		t.Fatalf("late marks %v", marks)
	}
}

func TestMaskCountsMatchASystemCoupon(t *testing.T) {
	winners := map[int]int{}
	addMaskCounts(winners, []byte{0b011, 0b100}, "12")
	if winners[2] != 1 || winners[1] != 1 {
		t.Fatalf("counts = %v", winners)
	}
}

func TestFixtureSearchRanksBothNamesFirst(t *testing.T) {
	rows := []footballHit{
		{ID: 1, Home: "Arsenal", Away: "Chelsea", League: "Premier League"},
		{ID: 2, Home: "Arsenal", Away: "FC Dnepr Mogilev", League: "Friendly"},
		{ID: 3, Home: "Liverpool", Away: "Everton"},
	}
	hits := rankFixtures(rows, "Arsenal", "Mogilev")
	if len(hits) != 2 || hits[0].ID != 2 {
		t.Fatalf("ranked = %+v", hits)
	}
	if msg := searchMessage("Arsenal", "Mogilev", "2026-10-25", hits); msg != "2 matches on 2026-10-25. Pick the right one." {
		t.Fatalf("message = %s", msg)
	}
	none := rankFixtures(rows, "Arsenal", "Barcelona")
	if msg := searchMessage("Arsenal", "Barcelona", "2026-10-25", none); msg != "No match on 2026-10-25 includes both names. These matches include Arsenal." {
		t.Fatalf("partial = %s", msg)
	}
	if msg := searchMessage("Arsenal", "", "2026-10-25", nil); msg != "No match on 2026-10-25 includes Arsenal. Check the spelling and the kickoff date." {
		t.Fatalf("empty = %s", msg)
	}
}

func TestFootballErrorIsShownToTheAdmin(t *testing.T) {
	if got := footballError([]byte(`{"season":"The Season field is required."}`)); got != "The Season field is required." {
		t.Fatalf("season = %s", got)
	}
	if got := footballError([]byte("[]")); got != "" {
		t.Fatalf("empty list = %s", got)
	}
	if got := feedMessage(errors.New("football feed quota")); got != "The score feed has reached its request limit. Try again later." {
		t.Fatalf("quota = %s", got)
	}
	if cleanZone("Europe/Stockholm") != "Europe/Stockholm" || cleanZone("bad zone") != "" {
		t.Fatal("timezone was not cleaned")
	}
}

package main

import (
	"bytes"
	"encoding/hex"
	"math/rand"
	"testing"
)

func bruteKept(picks []byte, colors []slipColor, results string) []uint64 {
	games := len(picks)
	counts := make([]uint64, games+1)
	var walk func(game int, chosen uint64, score int)
	walk = func(game int, chosen uint64, score int) {
		if game == games {
			for _, color := range colors {
				hits := 0
				for bits := color.Signs & chosen; bits != 0; bits &= bits - 1 {
					hits++
				}
				if hits < int(color.Min) || hits > int(color.Max) {
					return
				}
			}
			counts[score]++
			return
		}
		for outcome := 0; outcome < 3; outcome++ {
			if picks[game]&(1<<outcome) == 0 {
				continue
			}
			correct := 0
			if results != "" && (results[game] == "1X2"[outcome] || results[game] == 'C') {
				correct = 1
			}
			walk(game+1, chosen|uint64(1)<<(3*game+outcome), score+correct)
		}
	}
	walk(0, 0, 0)
	return counts
}

func TestKeptCountsMatchEveryRow(t *testing.T) {
	random := rand.New(rand.NewSource(7))
	for trial := 0; trial < 400; trial++ {
		games := 1 + random.Intn(8)
		picks := make([]byte, games)
		var frame uint64
		for game := range picks {
			picks[game] = byte(1 + random.Intn(7))
			frame |= uint64(picks[game]) << (3 * game)
		}
		colors := make([]slipColor, random.Intn(5))
		for c := range colors {
			signs := frame & random.Uint64()
			if signs == 0 {
				signs = frame & -frame
			}
			low := uint8(random.Intn(games + 1))
			high := low + uint8(random.Intn(games+1-int(low)))
			colors[c] = slipColor{Signs: signs, Min: low, Max: high}
		}
		results := ""
		if random.Intn(2) == 0 {
			outcomes := []byte("1X2C")
			raw := make([]byte, games)
			for game := range raw {
				raw[game] = outcomes[random.Intn(4)]
			}
			results = string(raw)
		}
		got := keptCounts(picks, colors, results)
		want := bruteKept(picks, colors, results)
		if results == "" {
			got, want = got[:1], want[:1]
		}
		for score := range want {
			if got[score] != want[score] {
				t.Fatalf("trial %d picks %v colors %+v results %q: got %v want %v", trial, picks, colors, results, got, want)
			}
		}
	}
}

func TestColoredPickWeightsCountKeptRows(t *testing.T) {
	picks := []byte{7, 7, 7}
	colors := []slipColor{{Signs: 0b001001001, Min: 1, Max: 2}}
	weights := make([][3]uint64, 3)
	addColoredWeights(weights, picks, colors)
	total := keptCounts(picks, colors, "")[0]
	if total != 18 {
		t.Fatalf("kept %d rows, want 18", total)
	}
	for game, weight := range weights {
		if weight[0]+weight[1]+weight[2] != total {
			t.Fatalf("game %d weights %v do not add up to the kept rows", game, weight)
		}
	}
	if weights[0][0] != 8 || weights[0][1] != 5 || weights[0][2] != 5 {
		t.Fatalf("game 0 weights %v", weights[0])
	}
}

// The same vector is pinned in the program and browser tests, so all three hash a colored slip alike.
func TestColoredHashesMatchTheProgram(t *testing.T) {
	owner := bytes.Repeat([]byte{1}, 32)
	picks := make([]byte, 13)
	picks[0], picks[1], picks[2] = 7, 7, 7
	colors := colorsBytes([]slipColor{{Signs: 0b001001001, Min: 1, Max: 2}})
	if hex.EncodeToString(colors) != "49000000000000000102" {
		t.Fatalf("color bytes %x", colors)
	}
	commitment := slipCommitment(owner, picks, 7, colors)
	if !bytes.Equal(slipCommitment(owner, picks, 7, nil), couponCommitment(owner, picks, 7)) {
		t.Fatal("a slip with no colors must keep the old commitment")
	}
	leaf := slipRevealLeaf(3, owner, 18, commitment, picks, colors)
	if hex.EncodeToString(commitment) != coloredCommitmentVector || hex.EncodeToString(leaf) != coloredLeafVector {
		t.Fatalf("commitment %x leaf %x", commitment, leaf)
	}
}

const (
	coloredCommitmentVector = "a202294cf5f800ffa813f4fa136a55f66013ad1427c533b33be77e3effdb6553"
	coloredLeafVector       = "c27ac67dfad8d1e2c95fb654740478450fc5196fe39b1165be61ff3f853dc74c"
)

func TestColoredRevealParsesAndScores(t *testing.T) {
	commitment := hex.EncodeToString(bytes.Repeat([]byte{4}, 32))
	record, ok := parseSlipLog("Program log: reveal-v1:Event333:0:Owner333:18:" + commitment + ":07070700000000000000000000:49000000000000000102")
	if !ok || record.Colors != "49000000000000000102" {
		t.Fatalf("colored reveal %+v ok %v", record, ok)
	}
	if _, ok := parseSlipLog("Program log: reveal-v1:Event333:0:Owner333:18:" + commitment + ":0707:4900"); ok {
		t.Fatal("a broken color list was accepted")
	}
	picks, _, colors, err := lockedPicks([]byte(`{"revealNonce":"5","picks":[7,7,7,0,0,0,0,0,0,0,0,0,0],"colors":[{"signs":"73","min":1,"max":2}]}`))
	if err != nil || picks[0] != 7 || hex.EncodeToString(colors) != "49000000000000000102" {
		t.Fatalf("locked colors %x: %v", colors, err)
	}
	if _, _, _, err := lockedPicks([]byte(`{"revealNonce":"5","picks":[7,7,7,0,0,0,0,0,0,0,0,0,0],"colors":[{"signs":"73","min":3,"max":2}]}`)); err == nil {
		t.Fatal("a color with min above max was accepted")
	}

	owner := base58Encode(bytes.Repeat([]byte{1}, 32))
	entry := slipRecord{Index: 0, Owner: owner, Combinations: 18, Commitment: commitment}
	reveal := entry
	reveal.Picks = "07070700000000000000000000"
	reveal.Colors = "49000000000000000102"
	data, err := scoreSlipData(0, slipArchiveView{Entries: []slipRecord{entry}, Reveals: []slipRecord{reveal}})
	if err != nil {
		t.Fatal(err)
	}
	at := 8 + 4 + 32 + 4 + 13 + 8 + 32
	if hex.EncodeToString(data[at:at+4+10]) != "01000000"+"49000000000000000102" || data[at+14] != 1 {
		t.Fatalf("score data does not carry the colors: %x", data[at:])
	}
}

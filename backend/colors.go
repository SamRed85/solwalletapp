package main

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
)

const (
	maxSlipColors  = 4
	slipColorBytes = 10
)

// slipColor is one reduction color as the program stores it. Game g outcome k is
// bit 3*g+k of Signs, in pick-mask order 1, X, 2.
type slipColor struct {
	Signs uint64
	Min   uint8
	Max   uint8
}

func colorsBytes(colors []slipColor) []byte {
	out := make([]byte, 0, len(colors)*slipColorBytes)
	for _, color := range colors {
		out = binary.LittleEndian.AppendUint64(out, color.Signs)
		out = append(out, color.Min, color.Max)
	}
	return out
}

func parseColorBytes(raw []byte) ([]slipColor, error) {
	if len(raw)%slipColorBytes != 0 || len(raw)/slipColorBytes > maxSlipColors {
		return nil, errors.New("the colors are not a list of up to four colors")
	}
	colors := make([]slipColor, 0, len(raw)/slipColorBytes)
	for at := 0; at < len(raw); at += slipColorBytes {
		colors = append(colors, slipColor{
			Signs: binary.LittleEndian.Uint64(raw[at:]),
			Min:   raw[at+8],
			Max:   raw[at+9],
		})
	}
	return colors, nil
}

func parseColorHex(text string) ([]slipColor, error) {
	if text == "" {
		return nil, nil
	}
	raw, err := hex.DecodeString(text)
	if err != nil || len(raw) == 0 {
		return nil, errors.New("the stored colors are invalid")
	}
	return parseColorBytes(raw)
}

// slipCommitment matches the program: no colors is the plain coupon commitment.
func slipCommitment(owner, picks []byte, nonce uint64, colors []byte) []byte {
	if len(colors) == 0 {
		return couponCommitment(owner, picks, nonce)
	}
	return slipHash(owner, picks, putU64(nonce), []byte("colors-v1"), colors)
}

// colorsArg is the Borsh vector the program reads after the commitment.
func colorsArg(colors []byte) []byte {
	return append(putU32(uint32(len(colors)/slipColorBytes)), colors...)
}

type colorState struct {
	hits  [maxSlipColors]uint8
	score uint8
}

// keptCounts returns rows by number of correct picks, counting only rows inside every
// color's range. An empty results string puts every row at zero correct. Sign bits
// outside the frame are ignored, so a caller may narrow one game to a single outcome.
func keptCounts(picks []byte, colors []slipColor, results string) []uint64 {
	if len(colors) > maxSlipColors {
		return nil
	}
	if results != "" && len(results) > len(picks) {
		return nil
	}
	games := len(picks)
	if results != "" {
		games = len(results)
	}
	reach := make([]uint8, len(colors))
	for c, color := range colors {
		for game := 0; game < games; game++ {
			if color.Signs&(uint64(picks[game]&0b111)<<(3*game)) != 0 {
				reach[c]++
			}
		}
	}
	rows := map[colorState]uint64{{}: 1}
	for game := 0; game < games; game++ {
		var resultBits byte
		if results != "" {
			switch results[game] {
			case '1':
				resultBits = 0b001
			case 'X':
				resultBits = 0b010
			case '2':
				resultBits = 0b100
			case 'C':
				resultBits = 0b111
			default:
				return nil
			}
		}
		next := map[colorState]uint64{}
		for outcome := 0; outcome < 3; outcome++ {
			if picks[game]&(1<<outcome) == 0 {
				continue
			}
			bit := uint64(1) << (3*game + outcome)
			for state, count := range rows {
				moved := state
				allowed := true
				for c, color := range colors {
					if color.Signs&bit == 0 {
						continue
					}
					moved.hits[c]++
					if moved.hits[c] > color.Max {
						allowed = false
						break
					}
					// Once max cannot be passed, only reaching min still matters.
					if color.Max >= reach[c] && moved.hits[c] > color.Min {
						moved.hits[c] = color.Min
					}
				}
				if !allowed {
					continue
				}
				if resultBits&(1<<outcome) != 0 {
					moved.score++
				}
				next[moved] += count
			}
		}
		rows = next
	}
	counts := make([]uint64, games+1)
	for state, count := range rows {
		kept := true
		for c, color := range colors {
			if state.hits[c] < color.Min {
				kept = false
				break
			}
		}
		if kept {
			counts[state.score] += count
		}
	}
	return counts
}

// addColoredWeights adds, for each game, the kept rows that chose each outcome.
func addColoredWeights(weights [][3]uint64, picks []byte, colors []slipColor) {
	for game := 0; game < len(weights) && game < len(picks); game++ {
		for outcome := 0; outcome < 3; outcome++ {
			if picks[game]&(1<<outcome) == 0 {
				continue
			}
			narrowed := append([]byte{}, picks[:len(weights)]...)
			narrowed[game] = 1 << outcome
			for _, rows := range keptCounts(narrowed, colors, "") {
				weights[game][outcome] += rows
			}
		}
	}
}
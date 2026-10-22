package main

import (
	"errors"
	"math/big"
)

const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

var base58Index = func() [256]int {
	var idx [256]int
	for i := range idx {
		idx[i] = -1
	}
	for i := 0; i < len(base58Alphabet); i++ {
		idx[base58Alphabet[i]] = i
	}
	return idx
}()

func base58Decode(s string) ([]byte, error) {
	if s == "" || len(s) > 128 {
		return nil, errors.New("invalid base58 length")
	}
	n := new(big.Int)
	radix := big.NewInt(58)
	for i := 0; i < len(s); i++ {
		v := base58Index[s[i]]
		if v < 0 {
			return nil, errors.New("invalid base58 character")
		}
		n.Mul(n, radix)
		n.Add(n, big.NewInt(int64(v)))
	}
	zeros := 0
	for zeros < len(s) && s[zeros] == '1' {
		zeros++
	}
	return append(make([]byte, zeros), n.Bytes()...), nil
}

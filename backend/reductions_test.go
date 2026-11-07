package main

import (
	"encoding/hex"
	"testing"
)

func TestValidateReductionPieces(t *testing.T) {
	good := reductionPiece{Commitment: hex64(), Sealed: hex42(), Combinations: 12}
	if err := validateReductionPieces([]reductionPiece{good}); err != nil {
		t.Fatal(err)
	}
	if err := validateReductionPieces(nil); err == nil {
		t.Fatal("empty reduction was accepted")
	}
	over := good
	over.Combinations = 44001
	if err := validateReductionPieces([]reductionPiece{over}); err == nil {
		t.Fatal("a coupon over 44,000 rows was accepted")
	}
	open := good
	open.Sealed = open.Sealed[:len(open.Sealed)-2] + "zz"
	if err := validateReductionPieces([]reductionPiece{open}); err == nil {
		t.Fatal("a non-hex backup was accepted")
	}
	again := good
	if err := validateReductionPieces([]reductionPiece{good, again}); err == nil {
		t.Fatal("a repeated commitment was accepted")
	}
}

func hex64() string {
	return hex.EncodeToString(make([]byte, 32))
}

func hex42() string {
	return hex.EncodeToString(make([]byte, 21))
}

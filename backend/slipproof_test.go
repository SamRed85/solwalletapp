package main

import (
	"bytes"
	"crypto/sha256"
	"testing"
)

func TestSlipProofMatchesTheEmptyTree(t *testing.T) {
	empty := sha256.Sum256([]byte("slip-empty"))
	if !bytes.Equal(slipZero(0), empty[:]) {
		t.Fatal("empty leaf hash drifted")
	}
	proof := sparseProof(nil, 0)
	if !bytes.Equal(proof[0], slipZero(0)) {
		t.Fatal("the first coupon proof must start with the empty sibling")
	}
	owner := bytes.Repeat([]byte{1}, 32)
	commitment := bytes.Repeat([]byte{2}, 32)
	first := slipLeaf(0, owner, 1, commitment)
	second := slipLeaf(1, owner, 1, commitment)
	next := sparseProof([][]byte{first}, 1)
	if !bytes.Equal(next[0], first) {
		t.Fatal("the second coupon's first sibling is the first leaf")
	}
	if bytes.Equal(next[0], second) {
		t.Fatal("a leaf must not be its own sibling")
	}
}

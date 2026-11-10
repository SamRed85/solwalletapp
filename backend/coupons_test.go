package main

import "testing"

func sampleCoupon() storedCoupon {
	return storedCoupon{
		ProgramID:    "Prog111",
		Address:      "slip:vault:3",
		EventID:      "abc",
		EventTitle:   "Saturday",
		Admin:        "admin",
		Nonce:        4,
		Index:        3,
		Slip:         true,
		RevealNonce:  "99",
		Commitment:   hex64(),
		System:       []string{"1", "1X", "2"},
		Combinations: 2,
		Lamports:     1000,
		PlacedAt:     50,
	}
}

func TestValidateStoredCoupon(t *testing.T) {
	if err := validateStoredCoupon(sampleCoupon(), "Prog111"); err != nil {
		t.Fatal(err)
	}
	other := sampleCoupon()
	other.ProgramID = "other"
	if err := validateStoredCoupon(other, "Prog111"); err == nil {
		t.Fatal("a coupon for another program was accepted")
	}
	badPick := sampleCoupon()
	badPick.System = []string{"11"}
	if err := validateStoredCoupon(badPick, "Prog111"); err == nil {
		t.Fatal("a repeated pick was accepted")
	}
	missing := sampleCoupon()
	missing.Address = ""
	if err := validateStoredCoupon(missing, "Prog111"); err == nil {
		t.Fatal("a coupon without an address was accepted")
	}
}

func TestMergeStoredCouponKeepsPicks(t *testing.T) {
	prev := sampleCoupon()
	next := sampleCoupon()
	next.System = []string{"2"}
	next.RevealNonce = "1"
	next.Revealed = true
	next.PlacedAt = 80
	next.EventTitle = ""
	merged := mergeStoredCoupon(prev, next)
	if merged.System[0] != "1" || merged.RevealNonce != "99" {
		t.Fatalf("picks were replaced: %+v", merged)
	}
	if !merged.Revealed {
		t.Fatal("revealed did not move forward")
	}
	if merged.PlacedAt != 50 {
		t.Fatalf("placedAt = %d, want the earlier time", merged.PlacedAt)
	}
	if merged.EventTitle != "Saturday" {
		t.Fatal("an empty title cleared the saved title")
	}
	filling := sampleCoupon()
	filling.System = nil
	filling.RevealNonce = ""
	later := sampleCoupon()
	if got := mergeStoredCoupon(filling, later); len(got.System) != 3 {
		t.Fatal("the first copy with picks was dropped")
	}
}

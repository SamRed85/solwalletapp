package main

import (
	"encoding/binary"
	"testing"
	"time"
)

func TestMonthEndMatchesCalendar(t *testing.T) {
	cases := []struct{ at, end time.Time }{
		{time.Date(2026, 10, 24, 15, 0, 0, 0, time.UTC), time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)},
		{time.Date(2026, 12, 31, 23, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)},
		{time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC), time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, c := range cases {
		year, month, day := civilYMD(c.at.Unix())
		if year != int32(c.at.Year()) || month != uint32(c.at.Month()) || day != uint32(c.at.Day()) {
			t.Fatalf("%s decoded as %d-%d-%d", c.at, year, month, day)
		}
		if got := monthEndOfID(monthID(c.at.Unix())); got != c.end.Unix() {
			t.Fatalf("%s ends at %d, want %d", c.at, got, c.end.Unix())
		}
	}
}

func TestSlotHashSelectsTicket(t *testing.T) {
	var data []byte
	count := make([]byte, 8)
	binary.LittleEndian.PutUint64(count, 1)
	data = append(data, count...)
	slot := make([]byte, 8)
	binary.LittleEndian.PutUint64(slot, 150)
	data = append(data, slot...)
	hash := make([]byte, 32)
	hash[0] = 5
	data = append(data, hash...)
	found, ok := slotHash(data, 150)
	if !ok || found[0] != 5 {
		t.Fatal("slot hash was not found")
	}
	if binary.LittleEndian.Uint64(found[:8])%4 != 1 {
		t.Fatal("ticket index")
	}
}

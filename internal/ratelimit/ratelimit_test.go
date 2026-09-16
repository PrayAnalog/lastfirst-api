package ratelimit

import (
	"testing"
	"time"
)

func TestReserveRefusesPastTheLimit(t *testing.T) {
	b := NewDailyBudget(10)

	if _, ok := b.Reserve(10); !ok {
		t.Fatal("Reserve(10) against an untouched limit of 10 was refused")
	}
	if _, ok := b.Reserve(1); ok {
		t.Fatal("Reserve(1) succeeded with the whole budget already reserved")
	}
}

func TestSettleReturnsTheUnusedUnits(t *testing.T) {
	b := NewDailyBudget(10)

	r, ok := b.Reserve(8)
	if !ok {
		t.Fatal("Reserve(8) against an untouched limit of 10 was refused")
	}
	r.Settle(3)

	if _, ok := b.Reserve(7); !ok {
		t.Fatal("the 5 units the work did not spend were not returned to the budget")
	}
}

func TestSettleChargesWorkThatRanPastItsReservation(t *testing.T) {
	b := NewDailyBudget(10)

	r, ok := b.Reserve(2)
	if !ok {
		t.Fatal("Reserve(2) against an untouched limit of 10 was refused")
	}
	r.Settle(10)

	if _, ok := b.Reserve(1); ok {
		t.Fatal("budget admitted more work after all 10 units had really been spent")
	}
}

func TestSettleFromAFinishedDayIsDropped(t *testing.T) {
	b := NewDailyBudget(10)

	r, ok := b.Reserve(9)
	if !ok {
		t.Fatal("Reserve(9) against an untouched limit of 10 was refused")
	}

	// Bring on the rollover the next call would have done at midnight.
	b.mu.Lock()
	b.resetAt = time.Now().Add(-time.Second)
	b.mu.Unlock()
	if _, ok := b.Reserve(9); !ok {
		t.Fatal("the new day did not start from a cleared budget")
	}

	// Yesterday's work finally finishes, having spent 1 of its 9 units.
	r.Settle(1)

	if _, ok := b.Reserve(2); ok {
		t.Fatal("a settlement from the previous day freed units on the new day")
	}
}

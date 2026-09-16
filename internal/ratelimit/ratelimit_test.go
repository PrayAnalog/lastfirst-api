package ratelimit

import (
	"testing"
	"time"
)

func TestReservationCommitsActualCostAndReleasesRemainder(t *testing.T) {
	budget := NewDailyBudget(10)
	reservation, ok := budget.Reserve(8)
	if !ok {
		t.Fatal("Reserve() rejected available capacity")
	}

	reservation.Commit(3)
	reservation.Release() // finalization must be idempotent

	budget.mu.Lock()
	defer budget.mu.Unlock()
	if budget.used != 3 {
		t.Fatalf("used = %d, want 3", budget.used)
	}
}

func TestReservationReleaseReturnsCapacity(t *testing.T) {
	budget := NewDailyBudget(2)
	reservation, ok := budget.Reserve(2)
	if !ok {
		t.Fatal("Reserve() rejected available capacity")
	}
	reservation.Release()

	if _, ok := budget.Reserve(2); !ok {
		t.Fatal("released capacity was not made available")
	}
}

func TestIPLimiterEvictsLeastRecentlyUsedAddressAtBound(t *testing.T) {
	limiter := NewIPLimiter(1, time.Hour, 2)
	if !limiter.Allow("192.0.2.1") || !limiter.Allow("192.0.2.2") {
		t.Fatal("limiter rejected an address before reaching its bound")
	}
	if limiter.Allow("192.0.2.1") {
		t.Fatal("limiter refilled a recently used address too early")
	}
	if !limiter.Allow("192.0.2.3") {
		t.Fatal("limiter rejected a new address at its bound")
	}

	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	if _, ok := limiter.buckets["192.0.2.2"]; ok {
		t.Fatal("least recently used address was not evicted")
	}
	if len(limiter.buckets) != 2 {
		t.Fatalf("tracked addresses = %d, want 2", len(limiter.buckets))
	}
}

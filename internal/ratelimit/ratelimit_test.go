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

func TestDailyBudgetRetryAfterUsesResetTime(t *testing.T) {
	budget := NewDailyBudget(1)
	budget.mu.Lock()
	budget.resetAt = time.Now().Add(time.Hour)
	budget.mu.Unlock()

	seconds := budget.RetryAfterSeconds()
	if seconds < 3599 || seconds > 3600 {
		t.Fatalf("RetryAfterSeconds() = %d, want about 3600", seconds)
	}
}

func TestReservationDoesNotReleaseIntoNewQuotaPeriod(t *testing.T) {
	budget := NewDailyBudget(2)
	oldReservation, ok := budget.Reserve(1)
	if !ok {
		t.Fatal("Reserve() rejected available capacity")
	}

	budget.mu.Lock()
	budget.resetAt = time.Now().Add(-time.Second)
	budget.mu.Unlock()
	newReservation, ok := budget.Reserve(1)
	if !ok {
		t.Fatal("Reserve() rejected capacity after reset")
	}
	oldReservation.Release()

	budget.mu.Lock()
	used := budget.used
	budget.mu.Unlock()
	if used != 1 {
		t.Fatalf("new-period used = %d, want 1", used)
	}
	newReservation.Release()
}

func TestReservationCannotBeUsedAfterQuotaPeriodReset(t *testing.T) {
	budget := NewDailyBudget(2)
	oldReservation, ok := budget.Reserve(2)
	if !ok {
		t.Fatal("Reserve() rejected available capacity")
	}

	budget.mu.Lock()
	budget.resetAt = time.Now().Add(-time.Second)
	budget.mu.Unlock()
	if oldReservation.Use(1) {
		t.Fatal("old-period reservation authorized a new-period call")
	}
	newReservation, ok := budget.Reserve(1)
	if !ok {
		t.Fatal("Reserve() rejected new-period capacity")
	}
	newReservation.Commit(1)
	oldReservation.Release()

	budget.mu.Lock()
	used := budget.used
	budget.mu.Unlock()
	if used != 1 {
		t.Fatalf("new-period used = %d, want 1", used)
	}
}

func TestReservationReleaseKeepsUnitsMarkedUsed(t *testing.T) {
	budget := NewDailyBudget(2)
	reservation, ok := budget.Reserve(2)
	if !ok {
		t.Fatal("Reserve() rejected available capacity")
	}
	if !reservation.Use(1) {
		t.Fatal("Use() rejected a current-period unit")
	}
	reservation.Release()

	budget.mu.Lock()
	used := budget.used
	budget.mu.Unlock()
	if used != 1 {
		t.Fatalf("used = %d, want 1", used)
	}
}

func TestReserveForUseChargesCurrentPeriodEvenWhenReleased(t *testing.T) {
	budget := NewDailyBudget(2)
	budget.mu.Lock()
	budget.resetAt = time.Now().Add(-time.Second)
	oldPeriod := budget.period
	budget.mu.Unlock()

	reservation, ok := budget.ReserveForUse(1)
	if !ok {
		t.Fatal("ReserveForUse() rejected available capacity")
	}
	reservation.Release()

	budget.mu.Lock()
	used := budget.used
	period := budget.period
	budget.mu.Unlock()
	if period != oldPeriod+1 {
		t.Fatalf("period = %d, want %d", period, oldPeriod+1)
	}
	if used != 1 {
		t.Fatalf("used = %d, want 1", used)
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

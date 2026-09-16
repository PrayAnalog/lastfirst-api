// Package ratelimit protects the YouTube Data API quota shared by all users
// of this service: a per-IP burst limiter stops a single caller from
// hammering the API, and a daily budget stops the service as a whole from
// spending more quota units than it has for the day.
package ratelimit

import (
	"sync"
	"time"
)

// IPLimiter is a per-IP token bucket. Each IP starts with burst tokens and
// regains one every refill; a request is allowed only while tokens remain.
type IPLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	burst   int
	refill  time.Duration
}

type bucket struct {
	tokens   int
	lastSeen time.Time
}

func NewIPLimiter(burst int, refill time.Duration) *IPLimiter {
	l := &IPLimiter{buckets: make(map[string]*bucket), burst: burst, refill: refill}
	go l.cleanupLoop()
	return l
}

func (l *IPLimiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, ok := l.buckets[ip]
	if !ok {
		l.buckets[ip] = &bucket{tokens: l.burst - 1, lastSeen: now}
		return true
	}

	if refilled := int(now.Sub(b.lastSeen) / l.refill); refilled > 0 {
		b.tokens = min(b.tokens+refilled, l.burst)
		b.lastSeen = now
	}
	if b.tokens <= 0 {
		return false
	}
	b.tokens--
	return true
}

// cleanupLoop drops buckets that have been idle long enough to be full
// again, so memory doesn't grow with every distinct IP ever seen.
func (l *IPLimiter) cleanupLoop() {
	idleAfter := l.refill * time.Duration(l.burst)
	for range time.Tick(10 * time.Minute) {
		l.mu.Lock()
		now := time.Now()
		for ip, b := range l.buckets {
			if now.Sub(b.lastSeen) > idleAfter {
				delete(l.buckets, ip)
			}
		}
		l.mu.Unlock()
	}
}

// DailyBudget caps total YouTube API units spent across all users in a
// calendar day, resetting at the same time YouTube's own quota does
// (midnight Pacific Time).
type DailyBudget struct {
	mu      sync.Mutex
	limit   int
	used    int
	resetAt time.Time
	// day counts rollovers rather than naming one, so a reservation can tell
	// the day it was taken in from the day it is being settled in even when
	// both fall on the same wall-clock date.
	day uint64
}

// Reservation is a claim on one accounting day's budget, held until the
// caller knows how many units the work really cost.
type Reservation struct {
	budget *DailyBudget
	day    uint64
	units  int
}

func NewDailyBudget(limit int) *DailyBudget {
	return &DailyBudget{limit: limit, resetAt: nextPacificMidnight(time.Now())}
}

// Reserve claims cost units against today's budget and reports whether there
// was room for them. The claim is charged up front, so the caller can make
// the calls it covers before knowing how many it will need.
func (b *DailyBudget) Reserve(cost int) (*Reservation, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.rollOverLocked(time.Now())
	if b.used+cost > b.limit {
		return nil, false
	}
	b.used += cost
	return &Reservation{budget: b, day: b.day, units: cost}, true
}

// Settle replaces a reservation with the units actually spent: unused units
// go back to the budget, and work that ran past its reservation is charged
// for what it really used, even if that takes the day over its limit. A
// reservation taken before the budget rolled over is dropped rather than
// applied to the new day, whose used total it says nothing about. Settling
// again reconciles against the figure last settled.
func (r *Reservation) Settle(spent int) {
	b := r.budget
	b.mu.Lock()
	defer b.mu.Unlock()

	b.rollOverLocked(time.Now())
	if b.day != r.day {
		return
	}
	b.used += spent - r.units
	if b.used < 0 {
		b.used = 0
	}
	r.units = spent
}

func (b *DailyBudget) rollOverLocked(now time.Time) {
	if !now.Before(b.resetAt) {
		b.used = 0
		b.resetAt = nextPacificMidnight(now)
		b.day++
	}
}

func nextPacificMidnight(t time.Time) time.Time {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		loc = time.UTC
	}
	pt := t.In(loc)
	return time.Date(pt.Year(), pt.Month(), pt.Day()+1, 0, 0, 0, 0, loc)
}

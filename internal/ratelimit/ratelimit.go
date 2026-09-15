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
}

func NewDailyBudget(limit int) *DailyBudget {
	return &DailyBudget{limit: limit, resetAt: nextPacificMidnight(time.Now())}
}

// Reserve commits cost units against today's budget and reports whether
// there was room for them.
func (b *DailyBudget) Reserve(cost int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	if !now.Before(b.resetAt) {
		b.used = 0
		b.resetAt = nextPacificMidnight(now)
	}
	if b.used+cost > b.limit {
		return false
	}
	b.used += cost
	return true
}

// Release returns units to today's budget, for a reservation that turned
// out larger than the work actually done.
func (b *DailyBudget) Release(units int) {
	if units <= 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	if !now.Before(b.resetAt) {
		b.used = 0
		b.resetAt = nextPacificMidnight(now)
		return
	}
	b.used -= units
	if b.used < 0 {
		b.used = 0
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

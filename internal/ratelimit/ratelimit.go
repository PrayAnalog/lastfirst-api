// Package ratelimit protects the YouTube Data API quota shared by all users
// of this service: a per-IP burst limiter stops a single caller from
// hammering the API, and a daily budget stops the service as a whole from
// spending more quota units than it has for the day.
package ratelimit

import (
	"container/list"
	"sync"
	"time"
)

// IPLimiter is a per-IP token bucket. Each IP starts with burst tokens and
// regains one every refill; a request is allowed only while tokens remain.
type IPLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	recent  *list.List
	burst   int
	refill  time.Duration
	maxIPs  int
}

type bucket struct {
	tokens     int
	lastRefill time.Time
	lastSeen   time.Time
	recent     *list.Element
}

func NewIPLimiter(burst int, refill time.Duration, maxIPs int) *IPLimiter {
	l := &IPLimiter{
		buckets: make(map[string]*bucket),
		recent:  list.New(),
		burst:   burst,
		refill:  refill,
		maxIPs:  maxIPs,
	}
	go l.cleanupLoop()
	return l
}

func (l *IPLimiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, ok := l.buckets[ip]
	if !ok {
		// Keep memory bounded without allowing a full table to lock every new
		// client out. Address churn can still evade a process-local IP limiter,
		// so ingress/edge controls remain necessary for distributed attacks.
		if len(l.buckets) >= l.maxIPs {
			oldest := l.recent.Back()
			delete(l.buckets, oldest.Value.(string))
			l.recent.Remove(oldest)
		}
		element := l.recent.PushFront(ip)
		l.buckets[ip] = &bucket{
			tokens:     l.burst - 1,
			lastRefill: now,
			lastSeen:   now,
			recent:     element,
		}
		return true
	}

	l.recent.MoveToFront(b.recent)
	b.lastSeen = now
	if refilled := int(now.Sub(b.lastRefill) / l.refill); refilled > 0 {
		b.tokens = min(b.tokens+refilled, l.burst)
		b.lastRefill = b.lastRefill.Add(time.Duration(refilled) * l.refill)
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
				l.recent.Remove(b.recent)
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
	period  uint64
}

func NewDailyBudget(limit int) *DailyBudget {
	return &DailyBudget{limit: limit, resetAt: nextPacificMidnight(time.Now())}
}

// Reservation holds a worst-case number of quota units until the caller
// records the number actually attempted or releases it without making a call.
type Reservation struct {
	budget *DailyBudget
	cost   int
	period uint64
	once   sync.Once
}

// Reserve holds cost units against today's budget and reports whether there
// was room for them. Call Commit or Release on every successful reservation.
func (b *DailyBudget) Reserve(cost int) (*Reservation, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.resetIfNeeded(time.Now())
	if cost <= 0 || b.used > b.limit-cost {
		return nil, false
	}
	b.used += cost
	return &Reservation{budget: b, cost: cost, period: b.period}, true
}

// RetryAfterSeconds returns the number of seconds until the current quota
// period resets. It is suitable for an HTTP Retry-After response header.
func (b *DailyBudget) RetryAfterSeconds() int {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	b.resetIfNeeded(now)
	delay := b.resetAt.Sub(now)
	return max(1, int((delay+time.Second-1)/time.Second))
}

func (b *DailyBudget) resetIfNeeded(now time.Time) {
	if !now.Before(b.resetAt) {
		b.used = 0
		b.resetAt = nextPacificMidnight(now)
		b.period++
	}
}

// Commit records actualCost units as spent and releases the unused portion of
// the reservation. Values outside the reserved range are clamped defensively.
func (r *Reservation) Commit(actualCost int) {
	r.finish(actualCost)
}

// Release returns the entire reservation when no billable call was attempted.
func (r *Reservation) Release() {
	r.finish(0)
}

func (r *Reservation) finish(actualCost int) {
	r.once.Do(func() {
		actualCost = max(0, min(actualCost, r.cost))
		r.budget.mu.Lock()
		defer r.budget.mu.Unlock()
		if r.period == r.budget.period {
			r.budget.used -= r.cost - actualCost
		}
	})
}

func nextPacificMidnight(t time.Time) time.Time {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		loc = time.UTC
	}
	pt := t.In(loc)
	return time.Date(pt.Year(), pt.Month(), pt.Day()+1, 0, 0, 0, 0, loc)
}

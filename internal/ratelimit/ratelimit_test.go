package ratelimit

import (
	"strconv"
	"testing"
	"time"
)

func TestIPLimiterBoundsDistinctIPs(t *testing.T) {
	l := NewIPLimiter(5, time.Minute)
	for i := range 20000 {
		if !l.Allow("ip-" + strconv.Itoa(i)) {
			t.Fatalf("first request from new IP %d was denied", i)
		}
	}
	l.mu.Lock()
	n := len(l.buckets)
	l.mu.Unlock()
	if n > 10000 {
		t.Fatalf("limiter holds %d buckets after 20000 distinct IPs, want at most 10000", n)
	}
}

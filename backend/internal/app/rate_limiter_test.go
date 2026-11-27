package app

import (
	"testing"
	"time"
)

func TestRateLimiterRemovesInactiveDevicesWithoutResettingActiveWindows(t *testing.T) {
	limiter := newMemoryRateLimiter()
	limiter.buckets["analytics:old"] = rateLimitBucket{
		hits: []time.Time{time.Now().Add(-2 * time.Minute)}, expiresAt: time.Now().Add(-time.Minute),
	}
	if !limiter.allow("intent", "buyer", 1, 24*time.Hour) {
		t.Fatal("first request rejected")
	}
	if _, ok := limiter.buckets["analytics:old"]; ok {
		t.Fatal("inactive device retained")
	}
	limiter.nextSweep = time.Time{}
	if limiter.allow("intent", "buyer", 1, 24*time.Hour) {
		t.Fatal("sweep bypassed active daily limit")
	}
}

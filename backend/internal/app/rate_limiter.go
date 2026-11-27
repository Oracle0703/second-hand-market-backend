package app

import (
	"fmt"
	"sync"
	"time"
)

type memoryRateLimiter struct {
	mu        sync.Mutex
	buckets   map[string]rateLimitBucket
	nextSweep time.Time
}

type rateLimitBucket struct {
	hits      []time.Time
	expiresAt time.Time
}

func newMemoryRateLimiter() *memoryRateLimiter {
	return &memoryRateLimiter{buckets: make(map[string]rateLimitBucket)}
}

func (l *memoryRateLimiter) allow(scope string, key string, limit int, window time.Duration) bool {
	if limit <= 0 {
		return true
	}
	now := time.Now()
	bucket := fmt.Sprintf("%s:%s", scope, key)
	cutoff := now.Add(-window)

	l.mu.Lock()
	defer l.mu.Unlock()

	// Page exposure traffic creates a key for each installation. Expire idle
	// buckets, including those from devices that never return.
	if !now.Before(l.nextSweep) {
		for key, entry := range l.buckets {
			if !now.Before(entry.expiresAt) {
				delete(l.buckets, key)
			}
		}
		l.nextSweep = now.Add(time.Minute)
	}
	entry := l.buckets[bucket]
	history := entry.hits
	pruned := history[:0]
	for _, ts := range history {
		if ts.After(cutoff) {
			pruned = append(pruned, ts)
		}
	}
	if len(pruned) >= limit {
		entry.hits = pruned
		l.buckets[bucket] = entry
		return false
	}
	pruned = append(pruned, now)
	l.buckets[bucket] = rateLimitBucket{hits: pruned, expiresAt: now.Add(window)}
	return true
}

package app

import (
	"fmt"
	"sync"
	"time"
)

type memoryRateLimiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

func newMemoryRateLimiter() *memoryRateLimiter {
	return &memoryRateLimiter{hits: map[string][]time.Time{}}
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

	history := l.hits[bucket]
	pruned := history[:0]
	for _, ts := range history {
		if ts.After(cutoff) {
			pruned = append(pruned, ts)
		}
	}
	if len(pruned) >= limit {
		l.hits[bucket] = pruned
		return false
	}
	pruned = append(pruned, now)
	l.hits[bucket] = pruned
	return true
}

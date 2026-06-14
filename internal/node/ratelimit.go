package node

import (
	"sync"
	"time"
)

type tokenBucket struct {
	capacity int
	tokens   int
	interval time.Duration
	lastFill time.Time
	mu       sync.Mutex
}

func newTokenBucket(capacity int, interval time.Duration) *tokenBucket {
	return &tokenBucket{
		capacity: capacity,
		tokens:   capacity,
		interval: interval,
		lastFill: time.Now(),
	}
}

func (tb *tokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastFill)
	tb.lastFill = now

	tb.tokens += int(elapsed / tb.interval)
	if tb.tokens > tb.capacity {
		tb.tokens = tb.capacity
	}

	if tb.tokens > 0 {
		tb.tokens--
		return true
	}
	return false
}

type peerRateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*tokenBucket
	capacity int
	interval time.Duration
}

func newPeerRateLimiter(capacity int, interval time.Duration) *peerRateLimiter {
	return &peerRateLimiter{
		buckets:  make(map[string]*tokenBucket),
		capacity: capacity,
		interval: interval,
	}
}

func (prl *peerRateLimiter) Allow(peerID string) bool {
	prl.mu.Lock()
	bucket, exists := prl.buckets[peerID]
	if !exists {
		bucket = newTokenBucket(prl.capacity, prl.interval)
		prl.buckets[peerID] = bucket
	}
	prl.mu.Unlock()
	return bucket.Allow()
}

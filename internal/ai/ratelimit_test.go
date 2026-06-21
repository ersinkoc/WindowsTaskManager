package ai

import (
	"testing"
	"time"
)

func TestNewTokenBucketZeroNormalized(t *testing.T) {
	tb := NewTokenBucket(0)
	if tb.max != 1 {
		t.Errorf("expected max=1 for perMinute=0, got %d", tb.max)
	}
	if tb.tokens != 1 {
		t.Errorf("expected tokens=1 for perMinute=0, got %d", tb.tokens)
	}
}

func TestNewTokenBucketNegativeNormalized(t *testing.T) {
	tb := NewTokenBucket(-5)
	if tb.max != 1 {
		t.Errorf("expected max=1 for perMinute=-5, got %d", tb.max)
	}
}

func TestTokenBucketTakeAllTokensThenFalse(t *testing.T) {
	tb := NewTokenBucket(3)
	if !tb.Take() {
		t.Fatal("first Take should succeed")
	}
	if !tb.Take() {
		t.Fatal("second Take should succeed")
	}
	if !tb.Take() {
		t.Fatal("third Take should succeed")
	}
	if tb.Take() {
		t.Error("fourth Take should fail — bucket exhausted")
	}
}

func TestTokenBucketRefillRestoresTokens(t *testing.T) {
	tb := NewTokenBucket(60)
	// Drain
	for i := 0; i < 60; i++ {
		if !tb.Take() {
			t.Fatalf("drain Take #%d failed", i)
		}
	}
	if tb.Take() {
		t.Fatal("bucket should be empty")
	}

	// Manually push lastFill into the past so refill adds tokens.
	tb.mu.Lock()
	tb.lastFill = time.Now().Add(-1 * time.Minute)
	tb.mu.Unlock()

	if !tb.Take() {
		t.Error("Take after refill should succeed")
	}
	if got := tb.Available(); got > 60 {
		t.Errorf("Available = %d, expected <= max=60", got)
	}
}

func TestTokenBucketRefillCappedAtMax(t *testing.T) {
	tb := NewTokenBucket(2)
	// Push lastFill way into the past — would refill to many more than 2 tokens.
	tb.mu.Lock()
	tb.lastFill = time.Now().Add(-1 * time.Hour)
	tb.tokens = 2
	tb.mu.Unlock()

	if got := tb.Available(); got != 2 {
		t.Errorf("Available = %d, expected 2 (capped at max)", got)
	}
}

func TestTokenBucketRefillZeroElapsed(t *testing.T) {
	tb := NewTokenBucket(5)
	tb.mu.Lock()
	tb.lastFill = time.Now()
	tb.tokens = 2
	tb.mu.Unlock()

	if got := tb.Available(); got != 2 {
		t.Errorf("Available = %d, expected 2 (no refill when elapsed=0)", got)
	}
}

func TestTokenBucketRefillEmptyLastFillBranch(t *testing.T) {
	// Construct a bucket where lastFill is in the past and tokens are at 0,
	// so the refill branch that adds tokens runs.
	tb := NewTokenBucket(60)
	tb.mu.Lock()
	tb.tokens = 0
	tb.lastFill = time.Now().Add(-1 * time.Minute)
	tb.mu.Unlock()

	if got := tb.Available(); got <= 0 {
		t.Errorf("Available = %d, expected > 0 after refill", got)
	}
}

func TestTokenBucketAvailable(t *testing.T) {
	tb := NewTokenBucket(5)
	if got := tb.Available(); got != 5 {
		t.Errorf("initial Available = %d, want 5", got)
	}
	tb.Take()
	if got := tb.Available(); got != 4 {
		t.Errorf("after 1 Take, Available = %d, want 4", got)
	}
}

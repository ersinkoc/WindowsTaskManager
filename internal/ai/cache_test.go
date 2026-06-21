package ai

import (
	"testing"
	"time"
)

func TestNewCacheBelowMinSizeRoundsUp(t *testing.T) {
	c := NewCache(time.Minute, 4)
	if c.maxSize != 16 {
		t.Errorf("maxSize = %d, want 16 (min)", c.maxSize)
	}
	if c.ttl != time.Minute {
		t.Errorf("ttl = %v, want 1m", c.ttl)
	}
}

func TestNewCacheLargerSizeRespected(t *testing.T) {
	c := NewCache(time.Minute, 100)
	if c.maxSize != 100 {
		t.Errorf("maxSize = %d, want 100", c.maxSize)
	}
}

func TestCacheGetMissingReturnsFalse(t *testing.T) {
	c := NewCache(time.Minute, 16)
	if v, ok := c.Get("nope"); ok || v != "" {
		t.Errorf("expected ('', false), got (%q, %v)", v, ok)
	}
}

func TestCacheGetExpiredReturnsFalse(t *testing.T) {
	c := NewCache(10*time.Millisecond, 16)
	c.Set("k", "v")
	// Sleep past TTL
	time.Sleep(50 * time.Millisecond)
	if v, ok := c.Get("k"); ok || v != "" {
		t.Errorf("expected expired entry to return false, got (%q, %v)", v, ok)
	}
}

func TestCacheSetThenGet(t *testing.T) {
	c := NewCache(time.Minute, 16)
	c.Set("hello", "world")
	if v, ok := c.Get("hello"); !ok || v != "world" {
		t.Errorf("Get = (%q, %v), want (world, true)", v, ok)
	}
	if c.Size() != 1 {
		t.Errorf("Size = %d, want 1", c.Size())
	}
}

func TestCacheSetEvictsOldest(t *testing.T) {
	c := NewCache(time.Minute, 16)
	// Force a smaller cap by direct manipulation; NewCache clamps below 16 to 16.
	c.maxSize = 2
	c.Set("a", "1")
	time.Sleep(5 * time.Millisecond)
	c.Set("b", "2")
	time.Sleep(5 * time.Millisecond)
	c.Set("c", "3") // should evict "a"
	if c.Size() != 2 {
		t.Errorf("Size after eviction = %d, want 2", c.Size())
	}
	if _, ok := c.Get("a"); ok {
		t.Error("expected 'a' to be evicted")
	}
	if v, ok := c.Get("b"); !ok || v != "2" {
		t.Errorf("Get(b) = (%q, %v), want (2, true)", v, ok)
	}
	if v, ok := c.Get("c"); !ok || v != "3" {
		t.Errorf("Get(c) = (%q, %v), want (3, true)", v, ok)
	}
}

func TestCacheKeyOfDeterministic(t *testing.T) {
	k1 := keyOf("hello world")
	k2 := keyOf("hello world")
	if k1 != k2 {
		t.Errorf("keyOf not deterministic: %q vs %q", k1, k2)
	}
	k3 := keyOf("hello WORLD")
	if k1 == k3 {
		t.Error("keyOf should differ for different inputs")
	}
	if len(k1) == 0 {
		t.Error("keyOf returned empty string")
	}
}

func TestCacheSizeEmpty(t *testing.T) {
	c := NewCache(time.Minute, 16)
	if c.Size() != 0 {
		t.Errorf("empty cache Size = %d, want 0", c.Size())
	}
}

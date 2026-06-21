package anomaly

import (
	"sync"
	"testing"
	"time"
)

func TestNewAlertStoreDefaultsBelowThreshold(t *testing.T) {
	s := NewAlertStore(0)
	if s.maxHistory != 32 {
		t.Fatalf("maxHistory=%d want 32", s.maxHistory)
	}
	if s.maxActive != 200 {
		t.Fatalf("maxActive=%d want 200", s.maxActive)
	}
}

func TestNewAlertStoreKeepsLargerHistory(t *testing.T) {
	s := NewAlertStore(100)
	if s.maxHistory != 100 {
		t.Fatalf("maxHistory=%d want 100", s.maxHistory)
	}
}

func TestAlertKeyWithAndWithoutPID(t *testing.T) {
	if got := alertKey("foo", 0); got != "foo" {
		t.Fatalf("alertKey foo/0 = %q want %q", got, "foo")
	}
	if got := alertKey("foo", 42); got != "foo/42" {
		t.Fatalf("alertKey foo/42 = %q want %q", got, "foo/42")
	}
}

func TestUint32ToAAllBranches(t *testing.T) {
	if got := uint32ToA(0); got != "0" {
		t.Fatalf("uint32ToA(0) = %q want 0", got)
	}
	if got := uint32ToA(1); got != "1" {
		t.Fatalf("uint32ToA(1) = %q want 1", got)
	}
	if got := uint32ToA(123456789); got != "123456789" {
		t.Fatalf("uint32ToA(123456789) = %q want 123456789", got)
	}
}

func TestSetMaxActiveUpdates(t *testing.T) {
	s := NewAlertStore(64)
	s.SetMaxActive(50)
	s.mu.RLock()
	got := s.maxActive
	s.mu.RUnlock()
	if got != 50 {
		t.Fatalf("maxActive=%d want 50", got)
	}
	// Setting <= 0 disables the cap.
	s.SetMaxActive(0)
	s.mu.RLock()
	got = s.maxActive
	s.mu.RUnlock()
	if got != 0 {
		t.Fatalf("maxActive=%d want 0", got)
	}
}

func TestRaiseNewAlert(t *testing.T) {
	s := NewAlertStore(64)
	got, isNew := s.Raise(Alert{Type: "test", Severity: SeverityInfo, Title: "x"})
	if !isNew {
		t.Fatal("expected new")
	}
	if got.ID == "" || got.Detected.IsZero() {
		t.Fatalf("expected populated ID/Detected, got %+v", got)
	}
	if findActiveByKey(s, got.ID) == nil {
		t.Fatal("expected active alert")
	}
}

func TestRaiseDuplicateRefreshes(t *testing.T) {
	s := NewAlertStore(64)
	first, _ := s.Raise(Alert{Type: "dup", Severity: SeverityInfo, Title: "first", PID: 7})
	second, isNew := s.Raise(Alert{Type: "dup", Severity: SeverityWarning, Title: "second", PID: 7, Description: "desc"})
	if isNew {
		t.Fatal("duplicate should not be new")
	}
	if second.ID != first.ID {
		t.Fatalf("ID mismatch: %q vs %q", first.ID, second.ID)
	}
	if second.Description != "desc" {
		t.Fatalf("description=%q want desc", second.Description)
	}
	if second.Severity != SeverityWarning {
		t.Fatalf("severity=%s want warning", second.Severity)
	}
}

func TestRaiseRespectsSnooze(t *testing.T) {
	s := NewAlertStore(64)
	until := time.Now().Add(time.Minute)
	if !s.Snooze("snz", 99, until) {
		t.Fatal("snooze should return true")
	}
	_, isNew := s.Raise(Alert{Type: "snz", Severity: SeverityInfo, PID: 99})
	if isNew {
		t.Fatal("snoozed raise should not be new")
	}
}

func TestRaiseRespectsExpiredSnooze(t *testing.T) {
	s := NewAlertStore(64)
	until := time.Now().Add(-time.Second) // already expired
	s.Snooze("exp", 1, until)
	_, isNew := s.Raise(Alert{Type: "exp", Severity: SeverityInfo, PID: 1})
	if !isNew {
		t.Fatal("expired snooze should allow new raise")
	}
}

func TestRaiseDropsWhenActiveCapReached(t *testing.T) {
	s := NewAlertStore(64)
	s.SetMaxActive(2)
	// Fill two entries.
	_, n1 := s.Raise(Alert{Type: "a", PID: 1})
	_, n2 := s.Raise(Alert{Type: "a", PID: 2})
	if !n1 || !n2 {
		t.Fatal("first two should be new")
	}
	// Third should be dropped from active but logged in history.
	_, n3 := s.Raise(Alert{Type: "a", PID: 3})
	if n3 {
		t.Fatal("third raise should be dropped (cap)")
	}
	// History should still contain it.
	hist := s.History()
	found := false
	for _, h := range hist {
		if h.PID == 3 {
			found = true
		}
	}
	if !found {
		t.Fatal("capped alert should appear in history")
	}
}

func TestRaiseDisabledCap(t *testing.T) {
	s := NewAlertStore(64)
	s.SetMaxActive(0) // disabled
	for i := uint32(1); i <= 5; i++ {
		_, isNew := s.Raise(Alert{Type: "nc", PID: i})
		if !isNew {
			t.Fatalf("raise %d should be new (cap disabled)", i)
		}
	}
}

func TestClearByType(t *testing.T) {
	s := NewAlertStore(64)
	s.Raise(Alert{Type: "x", PID: 1})
	s.Raise(Alert{Type: "x", PID: 2})
	s.Raise(Alert{Type: "y", PID: 3})
	if removed := s.ClearByType("x"); removed != 2 {
		t.Fatalf("removed=%d want 2", removed)
	}
	if removed := s.ClearByType("x"); removed != 0 {
		t.Fatalf("second clear removed=%d want 0", removed)
	}
	if len(s.Active()) != 1 {
		t.Fatalf("remaining active=%d want 1", len(s.Active()))
	}
}

func TestClearAll(t *testing.T) {
	s := NewAlertStore(64)
	s.Raise(Alert{Type: "a", PID: 1})
	s.Raise(Alert{Type: "b", PID: 2})
	if removed := s.ClearAll(); removed != 2 {
		t.Fatalf("removed=%d want 2", removed)
	}
	if len(s.Active()) != 0 {
		t.Fatalf("active=%d want 0", len(s.Active()))
	}
	if removed := s.ClearAll(); removed != 0 {
		t.Fatalf("empty clear removed=%d want 0", removed)
	}
}

func TestClearByKeyMissing(t *testing.T) {
	s := NewAlertStore(64)
	s.ClearByKey("nope") // should not panic
	if len(s.Active()) != 0 {
		t.Fatalf("active=%d want 0", len(s.Active()))
	}
}

func TestClearByKeyHit(t *testing.T) {
	s := NewAlertStore(64)
	s.Raise(Alert{Type: "hit", PID: 9})
	s.ClearByKey("hit/9")
	if findActiveByKey(s, "hit/9") != nil {
		t.Fatal("expected cleared")
	}
}

func TestSnoozeClearsActive(t *testing.T) {
	s := NewAlertStore(64)
	s.Raise(Alert{Type: "snz", PID: 1})
	s.Snooze("snz", 1, time.Now().Add(time.Minute))
	if findActiveByKey(s, "snz/1") != nil {
		t.Fatal("expected active cleared by snooze")
	}
	// Re-raise while snoozed should be ignored.
	if _, isNew := s.Raise(Alert{Type: "snz", PID: 1}); isNew {
		t.Fatal("raise during snooze should not be new")
	}
}

func TestActiveAndHistoryReturnCopies(t *testing.T) {
	s := NewAlertStore(64)
	s.Raise(Alert{Type: "c", Severity: SeverityInfo})
	active := s.Active()
	if len(active) != 1 {
		t.Fatalf("active=%d want 1", len(active))
	}
	hist := s.History()
	if len(hist) != 1 {
		t.Fatalf("history=%d want 1", len(hist))
	}
}

func TestHistoryRingRolls(t *testing.T) {
	s := NewAlertStore(64)
	s.maxHistory = 2 // override the 32-floor to exercise ring eviction
	for i := uint32(1); i <= 5; i++ {
		s.Raise(Alert{Type: "ring", PID: i}) // unique PIDs => unique keys
	}
	if got := len(s.History()); got != 2 {
		t.Fatalf("history length=%d want 2", got)
	}
}

func TestAppendHistoryEvicts(t *testing.T) {
	// Directly exercise appendHistory by raising enough alerts to overflow.
	s := NewAlertStore(64)
	s.maxHistory = 3 // override the 32-floor
	for i := uint32(1); i <= 6; i++ {
		s.Raise(Alert{Type: "evict", PID: i}) // unique PIDs => unique keys
	}
	if got := len(s.History()); got != 3 {
		t.Fatalf("history=%d want 3", got)
	}
}

func findActiveByKey(s *AlertStore, key string) *Alert {
	for _, a := range s.Active() {
		if a.ID == key {
			return &a
		}
	}
	return nil
}

// TestAlertStoreConcurrentSafe just exercises the lock paths without checking
// strict ordering — guards against accidental unsynchronized writes.
func TestAlertStoreConcurrentSafe(t *testing.T) {
	s := NewAlertStore(128)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				s.Raise(Alert{Type: "c", PID: uint32(id*1000 + j)})
			}
		}(i)
	}
	wg.Wait()
	_ = s.Active()
	_ = s.History()
	s.ClearAll()
	s.SetMaxActive(10)
}

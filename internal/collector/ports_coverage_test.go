//go:build windows

package collector

import (
	"testing"

	"github.com/ersinkoc/WindowsTaskManager/internal/metrics"
	"github.com/ersinkoc/WindowsTaskManager/internal/winapi"
)

func TestNewPortCollectorInitializesState(t *testing.T) {
	pc := NewPortCollector(map[uint16]string{80: "HTTP"})
	if pc == nil {
		t.Fatal("expected non-nil port collector")
	}
	if pc.since == nil {
		t.Fatal("expected since map initialized")
	}
	if pc.wellKnownPorts == nil {
		t.Fatal("expected wellKnownPorts initialized")
	}
	if pc.wellKnownPorts[80] != "HTTP" {
		t.Fatalf("wellKnownPorts[80]=%q", pc.wellKnownPorts[80])
	}
}

func TestNewPortCollectorNilWellKnown(t *testing.T) {
	pc := NewPortCollector(nil)
	if pc == nil {
		t.Fatal("expected non-nil")
	}
	if pc.wellKnownPorts != nil {
		t.Fatal("expected wellKnownPorts nil when passed nil")
	}
}

func TestSetWellKnownReplacesMap(t *testing.T) {
	pc := NewPortCollector(map[uint16]string{80: "HTTP"})
	pc.SetWellKnown(map[uint16]string{443: "HTTPS", 22: "SSH"})
	if _, ok := pc.wellKnownPorts[80]; ok {
		t.Fatal("expected old port removed")
	}
	if pc.wellKnownPorts[443] != "HTTPS" {
		t.Fatalf("wellKnownPorts[443]=%q", pc.wellKnownPorts[443])
	}
}

func TestSetWellKnownNilReplacesWithNil(t *testing.T) {
	pc := NewPortCollector(map[uint16]string{80: "HTTP"})
	pc.SetWellKnown(nil)
	if pc.wellKnownPorts != nil {
		t.Fatalf("expected nil wellKnownPorts, got %v", pc.wellKnownPorts)
	}
}

func TestPortCollectorCollectSmoke(t *testing.T) {
	pc := NewPortCollector(map[uint16]string{80: "HTTP", 8080: "Alt"})
	out := pc.Collect(nil)
	// May be empty if no listeners exist, but should be a valid (possibly empty) slice.
	if out == nil {
		t.Fatal("expected non-nil slice")
	}
	for _, pb := range out {
		if pb.Protocol == "" {
			t.Fatal("expected protocol set")
		}
		if pb.Label != "" {
			// Verify a known port has its label populated.
			if pb.LocalPort != 80 && pb.LocalPort != 8080 {
				t.Fatalf("unexpected labeled port %d", pb.LocalPort)
			}
		}
	}
}

func TestPortCollectorCollectWithResolverPopulatesProcess(t *testing.T) {
	pc := NewPortCollector(nil)
	out := pc.Collect(func(pid uint32) string {
		if pid == 0xDEAD {
			return "needle"
		}
		return ""
	})
	for _, pb := range out {
		if pb.PID == 0xDEAD && pb.Process != "needle" {
			t.Fatalf("expected process name 'needle' for pid 0xDEAD, got %q", pb.Process)
		}
	}
}

func TestPortCollectorCollectSecondRunPrunesStaleSinceMap(t *testing.T) {
	pc := NewPortCollector(nil)
	// Seed a stale entry directly via the finalize path or by adding to since map.
	pc.since["stale|key|0|0|0|0|listen"] = 1

	// Manually invoke finalize to register a key, ensuring it goes into seen.
	pb := metrics.PortBinding{
		Protocol:   "tcp",
		LocalAddr:  "127.0.0.1",
		LocalPort:  9999,
		RemoteAddr: "0.0.0.0",
		RemotePort: 0,
		State:      "listen",
		StateCode:  2,
		PID:        1,
	}
	seen := map[string]struct{}{}
	pc.finalize(&pb, nil, 100, seen)

	out := pc.Collect(nil)
	_ = out
	// After Collect, the stale key should be removed (not in seen).
	if _, exists := pc.since["stale|key|0|0|0|0|listen"]; exists {
		t.Fatal("expected stale since key to be pruned")
	}
}

func TestFinalizeCarriesForwardExistingSince(t *testing.T) {
	pc := NewPortCollector(nil)
	pb1 := metrics.PortBinding{
		Protocol:   "tcp",
		LocalAddr:  "127.0.0.1",
		LocalPort:  9000,
		RemoteAddr: "0.0.0.0",
		RemotePort: 0,
		State:      "listen",
		StateCode:  2,
		PID:        42,
	}
	seen := map[string]struct{}{}
	pc.finalize(&pb1, nil, 50, seen)
	if pb1.Since != 50 {
		t.Fatalf("expected first-seen=50, got %d", pb1.Since)
	}

	// Now do it again — since should be preserved.
	pb2 := metrics.PortBinding{
		Protocol:   "tcp",
		LocalAddr:  "127.0.0.1",
		LocalPort:  9000,
		RemoteAddr: "0.0.0.0",
		RemotePort: 0,
		State:      "listen",
		StateCode:  2,
		PID:        42,
	}
	pc.finalize(&pb2, nil, 9999, map[string]struct{}{})
	if pb2.Since != 50 {
		t.Fatalf("expected preserved first-seen=50, got %d", pb2.Since)
	}
}

func TestFinalizeAppliesWellKnownLabel(t *testing.T) {
	pc := NewPortCollector(map[uint16]string{9001: "Test"})
	pb := metrics.PortBinding{
		Protocol:  "tcp",
		LocalPort: 9001,
		State:     "listen",
	}
	pc.finalize(&pb, nil, 10, map[string]struct{}{})
	if pb.Label != "Test" {
		t.Fatalf("expected label 'Test', got %q", pb.Label)
	}
}

func TestIPv6StringFormatsArray(t *testing.T) {
	addr := [16]byte{
		0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0x01,
	}
	got := ipv6String(addr)
	if got != "2001:db8::1" {
		t.Fatalf("ipv6String=%q", got)
	}
}

func TestTCPStateNameAllKnownStates(t *testing.T) {
	cases := map[uint32]string{
		winapi.MIB_TCP_STATE_CLOSED:     "closed",
		winapi.MIB_TCP_STATE_LISTEN:     "listen",
		winapi.MIB_TCP_STATE_SYN_SENT:   "syn-sent",
		winapi.MIB_TCP_STATE_SYN_RCVD:   "syn-rcvd",
		winapi.MIB_TCP_STATE_ESTAB:      "established",
		winapi.MIB_TCP_STATE_FIN_WAIT1:  "fin-wait-1",
		winapi.MIB_TCP_STATE_FIN_WAIT2:  "fin-wait-2",
		winapi.MIB_TCP_STATE_CLOSE_WAIT: "close-wait",
		winapi.MIB_TCP_STATE_CLOSING:    "closing",
		winapi.MIB_TCP_STATE_LAST_ACK:   "last-ack",
		winapi.MIB_TCP_STATE_TIME_WAIT:  "time-wait",
		winapi.MIB_TCP_STATE_DELETE_TCB: "delete-tcb",
	}
	for in, want := range cases {
		if got := tcpStateName(in); got != want {
			t.Fatalf("tcpStateName(%d)=%q want %q", in, got, want)
		}
	}
}

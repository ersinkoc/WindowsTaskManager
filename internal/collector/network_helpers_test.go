//go:build windows

package collector

import (
	"errors"
	"testing"

	"github.com/ersinkoc/WindowsTaskManager/internal/winapi"
)

func TestNewNetworkCollectorInitializesPrev(t *testing.T) {
	n := NewNetworkCollector()
	if n == nil {
		t.Fatal("expected non-nil network collector")
	}
	if n.prev == nil {
		t.Fatal("expected prev map to be initialized")
	}
	if len(n.prev) != 0 {
		t.Fatalf("expected empty prev map, got %d", len(n.prev))
	}
}

func TestNetworkCollectorCollectReturnsMetrics(t *testing.T) {
	n := NewNetworkCollector()
	out := n.Collect()
	// We can't assume there are interfaces in the test environment, but the
	// returned struct must be valid either way.
	_ = out
	// A second sample should populate deltas where applicable.
	out2 := n.Collect()
	_ = out2
}

func TestNetworkCollectorCollectErrorReturnsEmpty(t *testing.T) {
	saved := getIfTable2
	t.Cleanup(func() { getIfTable2 = saved })
	getIfTable2 = func() ([]winapi.IfRow2, error) {
		return nil, errors.New("iftable2 failed")
	}
	n := NewNetworkCollector()
	out := n.Collect()
	if out.Interfaces != nil {
		t.Fatalf("expected nil Interfaces on error, got %v", out.Interfaces)
	}
}

func TestNetworkCollectorCollectEmptyRowsReturnsEmpty(t *testing.T) {
	saved := getIfTable2
	t.Cleanup(func() { getIfTable2 = saved })
	getIfTable2 = func() ([]winapi.IfRow2, error) {
		return nil, nil
	}
	n := NewNetworkCollector()
	out := n.Collect()
	if out.Interfaces != nil {
		t.Fatalf("expected nil Interfaces on empty rows, got %v", out.Interfaces)
	}
}

func TestShouldSkipInterfaceSkipsLoopbackAndTunnelTypes(t *testing.T) {
	loopback := winapi.IfRow2{Type: 24, Alias: "Loopback", Description: "Loopback"}
	if !shouldSkipInterface(loopback, map[string]struct{}{}) {
		t.Fatal("expected loopback type to be skipped")
	}
	tunnel := winapi.IfRow2{Type: 131, Alias: "Tunnel", Description: "Tunnel"}
	if !shouldSkipInterface(tunnel, map[string]struct{}{}) {
		t.Fatal("expected tunnel type to be skipped")
	}
}

func TestShouldSkipInterfaceSkipsDownDisconnectedWithNoSpeed(t *testing.T) {
	row := winapi.IfRow2{
		Alias:      "eth",
		Type:       6,
		OperStatus: 2,
	}
	if !shouldSkipInterface(row, map[string]struct{}{}) {
		t.Fatal("expected disconnected interface with no traffic to be skipped")
	}
}

func TestShouldSkipInterfaceSkipsLocalAreaConnectionWildcardWithZeroSpeed(t *testing.T) {
	row := winapi.IfRow2{
		Alias:      "Local Area Connection* 12",
		Type:       6,
		OperStatus: 1,
		SpeedRx:    0,
		SpeedTx:    0,
	}
	if !shouldSkipInterface(row, map[string]struct{}{}) {
		t.Fatal("expected Local Area Connection* zero-speed wildcard to be skipped")
	}
}

func TestShouldSkipInterfaceKeepsRealEthernet(t *testing.T) {
	row := winapi.IfRow2{
		Alias:      "Ethernet",
		Type:       6,
		OperStatus: 1,
		SpeedRx:    1_000_000_000,
		SpeedTx:    1_000_000_000,
	}
	if shouldSkipInterface(row, map[string]struct{}{}) {
		t.Fatal("expected real ethernet with speed to be kept")
	}
}

func TestDisplayInterfaceNameFallsBackToDescription(t *testing.T) {
	// alias empty, desc empty → both branches hit the desc fallback path.
	got := displayInterfaceName("", "")
	if got != "" {
		t.Fatalf("displayInterfaceName=%q want empty", got)
	}
	// alias non-empty wins
	got = displayInterfaceName("Alias1", "Desc1")
	if got != "Alias1" {
		t.Fatalf("displayInterfaceName=%q want Alias1", got)
	}
	// alias whitespace-only falls back to desc
	got = displayInterfaceName("   ", "DescOnly")
	if got != "DescOnly" {
		t.Fatalf("displayInterfaceName=%q want DescOnly", got)
	}
}

func TestDisplayInterfaceNameTrimsAndStripsTokens(t *testing.T) {
	// Verify a token is stripped when alias has one of the suffixes.
	got := displayInterfaceName("MyAdapter-Npcap", "")
	if got != "MyAdapter" {
		t.Fatalf("displayInterfaceName=%q want MyAdapter", got)
	}
}

func TestNormalizeIfaceNameEmpty(t *testing.T) {
	if got := normalizeIfaceName(""); got != "" {
		t.Fatalf("normalizeIfaceName(empty)=%q", got)
	}
	if got := normalizeIfaceName("   "); got != "" {
		t.Fatalf("normalizeIfaceName(spaces)=%q", got)
	}
}

func TestNormalizeIfaceNameCollapsesPunctuation(t *testing.T) {
	got := normalizeIfaceName("Ethernet 2 (Foo)")
	if got != "ethernet 2 foo" {
		t.Fatalf("normalizeIfaceName=%q", got)
	}
}

func TestPreferAliasPicksAliasWhenPresent(t *testing.T) {
	if got := preferAlias("Alias", "Description"); got != "Alias" {
		t.Fatalf("preferAlias=%q", got)
	}
}

func TestPreferAliasFallsBackToDescription(t *testing.T) {
	if got := preferAlias("", "Description"); got != "Description" {
		t.Fatalf("preferAlias=%q", got)
	}
}

func TestBpsDeltaComputesRate(t *testing.T) {
	got := bpsDelta(2_000_000, 0, 1.0)
	if got != 2_000_000 {
		t.Fatalf("bpsDelta=%d want 2000000", got)
	}
}

func TestBpsDeltaZeroWhenCurrNotGreater(t *testing.T) {
	if got := bpsDelta(100, 100, 1.0); got != 0 {
		t.Fatalf("bpsDelta=%d want 0", got)
	}
	if got := bpsDelta(50, 100, 1.0); got != 0 {
		t.Fatalf("bpsDelta=%d want 0", got)
	}
}

func TestSaturatingSubBasic(t *testing.T) {
	if got := saturatingSub(100, 30); got != 70 {
		t.Fatalf("saturatingSub=%d", got)
	}
}

func TestSaturatingSubClampsToZero(t *testing.T) {
	if got := saturatingSub(10, 30); got != 0 {
		t.Fatalf("saturatingSub=%d want 0", got)
	}
	if got := saturatingSub(10, 10); got != 0 {
		t.Fatalf("saturatingSub=%d want 0", got)
	}
}

func TestPickSpeedMbpsPicksHigher(t *testing.T) {
	if got := pickSpeedMbps(1_000_000_000, 500_000_000); got != 1000 {
		t.Fatalf("pickSpeedMbps=%d want 1000", got)
	}
	if got := pickSpeedMbps(500_000_000, 1_000_000_000); got != 1000 {
		t.Fatalf("pickSpeedMbps=%d want 1000", got)
	}
}

func TestPickSpeedMbpsZero(t *testing.T) {
	if got := pickSpeedMbps(0, 0); got != 0 {
		t.Fatalf("pickSpeedMbps=%d want 0", got)
	}
}

func TestIfTypeNameMapsKnown(t *testing.T) {
	cases := map[uint32]string{
		6:    "ethernet",
		9:    "token-ring",
		23:   "ppp",
		24:   "loopback",
		71:   "wifi",
		131:  "tunnel",
		144:  "ieee1394",
		9999: "other",
	}
	for in, want := range cases {
		if got := ifTypeName(in); got != want {
			t.Fatalf("ifTypeName(%d)=%q want %q", in, got, want)
		}
	}
}

func TestOperStatusNameMapsKnown(t *testing.T) {
	cases := map[uint32]string{
		1: "up",
		2: "down",
		3: "testing",
		4: "unknown",
		5: "dormant",
		6: "not-present",
		7: "lower-layer-down",
		9: "unknown",
	}
	for in, want := range cases {
		if got := operStatusName(in); got != want {
			t.Fatalf("operStatusName(%d)=%q want %q", in, got, want)
		}
	}
}

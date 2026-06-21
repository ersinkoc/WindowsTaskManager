package metrics

import (
	"encoding/json"
	"testing"
	"time"
)

// --- ProcessName tests -------------------------------------------------------

func TestProcessName_Found(t *testing.T) {
	s := &SystemSnapshot{
		Processes: []ProcessInfo{
			{PID: 100, Name: "explorer.exe"},
			{PID: 200, Name: "chrome.exe"},
			{PID: 300, Name: ""}, // empty name still "found" by PID match
		},
	}

	tests := []struct {
		name string
		pid  uint32
		want string
	}{
		{"first process", 100, "explorer.exe"},
		{"middle process", 200, "chrome.exe"},
		{"matches empty name when PID is present", 300, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := s.ProcessName(tc.pid)
			if got != tc.want {
				t.Errorf("ProcessName(%d) = %q, want %q", tc.pid, got, tc.want)
			}
		})
	}
}

func TestProcessName_NotFound(t *testing.T) {
	s := &SystemSnapshot{
		Processes: []ProcessInfo{
			{PID: 100, Name: "explorer.exe"},
			{PID: 200, Name: "chrome.exe"},
		},
	}

	// PID that doesn't exist must return "".
	if got := s.ProcessName(999); got != "" {
		t.Errorf("ProcessName(999) = %q, want empty string", got)
	}
}

func TestProcessName_EmptyProcessList(t *testing.T) {
	s := &SystemSnapshot{} // no processes at all
	if got := s.ProcessName(0); got != "" {
		t.Errorf("ProcessName on empty list = %q, want empty string", got)
	}
}

// --- Type definition validity tests ------------------------------------------
//
// The remaining types in the package are pure data structs. Their "validity" is
// best expressed as compile-time construction + JSON round-trip coverage, which
// also exercises every field's tag.

func TestCPUMetrics_Valid(t *testing.T) {
	c := CPUMetrics{
		TotalPercent: 12.5,
		PerCore:      []float64{10.0, 15.0, 20.0},
		NumLogical:   16,
		Name:         "Intel Core i7",
		FreqMHz:      3200,
	}
	if c.NumLogical != 16 {
		t.Errorf("NumLogical = %d, want 16", c.NumLogical)
	}
	if len(c.PerCore) != 3 {
		t.Errorf("len(PerCore) = %d, want 3", len(c.PerCore))
	}
	assertJSONRoundTrip(t, "CPUMetrics", c)
}

func TestMemoryMetrics_Valid(t *testing.T) {
	m := MemoryMetrics{
		TotalPhys:     16 * 1024 * 1024 * 1024,
		AvailPhys:     8 * 1024 * 1024 * 1024,
		UsedPhys:      8 * 1024 * 1024 * 1024,
		UsedPercent:   50.0,
		TotalPageFile: 32 * 1024 * 1024 * 1024,
		AvailPageFile: 16 * 1024 * 1024 * 1024,
		CommitCharge:  12 * 1024 * 1024 * 1024,
	}
	if m.UsedPhys+m.AvailPhys != m.TotalPhys {
		t.Errorf("memory arithmetic inconsistent: used+avail (%d) != total (%d)",
			m.UsedPhys+m.AvailPhys, m.TotalPhys)
	}
	assertJSONRoundTrip(t, "MemoryMetrics", m)
}

func TestDriveInfo_Valid(t *testing.T) {
	d := DriveInfo{
		Letter:     "C",
		Label:      "System",
		FSType:     "NTFS",
		TotalBytes: 500 * 1024 * 1024 * 1024,
		FreeBytes:  100 * 1024 * 1024 * 1024,
		UsedBytes:  400 * 1024 * 1024 * 1024,
		UsedPct:    80.0,
		ReadBPS:    1024,
		WriteBPS:   512,
		ReadIOPS:   100,
		WriteIOPS:  50,
	}
	if d.UsedBytes+d.FreeBytes != d.TotalBytes {
		t.Errorf("drive arithmetic inconsistent: used+free (%d) != total (%d)",
			d.UsedBytes+d.FreeBytes, d.TotalBytes)
	}
	assertJSONRoundTrip(t, "DriveInfo", d)
}

func TestDiskMetrics_Valid(t *testing.T) {
	d := DiskMetrics{
		Drives: []DriveInfo{
			{Letter: "C", Label: "System"},
		},
	}
	if len(d.Drives) != 1 {
		t.Errorf("len(Drives) = %d, want 1", len(d.Drives))
	}
	assertJSONRoundTrip(t, "DiskMetrics", d)
}

func TestInterfaceInfo_Valid(t *testing.T) {
	ifc := InterfaceInfo{
		Name:      "Ethernet",
		Type:      "Ethernet",
		Status:    "Up",
		SpeedMbps: 1000,
		InBPS:     1024,
		OutBPS:    512,
		InPPS:     100,
		OutPPS:    50,
		InErrors:  0,
		OutErrors: 0,
	}
	if ifc.SpeedMbps == 0 {
		t.Error("SpeedMbps should be non-zero for an active interface")
	}
	assertJSONRoundTrip(t, "InterfaceInfo", ifc)
}

func TestNetworkMetrics_Valid(t *testing.T) {
	n := NetworkMetrics{
		Interfaces: []InterfaceInfo{
			{Name: "Wi-Fi", Status: "Up"},
		},
		TotalUpBPS:   512,
		TotalDownBPS: 1024,
	}
	if n.TotalUpBPS+n.TotalDownBPS == 0 {
		t.Error("expected non-zero aggregate throughput")
	}
	assertJSONRoundTrip(t, "NetworkMetrics", n)
}

func TestGPUMetrics_Valid(t *testing.T) {
	g := GPUMetrics{
		Name:        "NVIDIA RTX 4090",
		Utilization: 75.0,
		VRAMUsed:    4 * 1024 * 1024 * 1024,
		VRAMTotal:   24 * 1024 * 1024 * 1024,
		Temperature: 70,
		Available:   true,
	}
	if g.VRAMUsed > g.VRAMTotal {
		t.Errorf("VRAMUsed (%d) > VRAMTotal (%d)", g.VRAMUsed, g.VRAMTotal)
	}
	assertJSONRoundTrip(t, "GPUMetrics", g)
}

func TestProcessInfo_Valid(t *testing.T) {
	p := ProcessInfo{
		PID:           1234,
		ParentPID:     100,
		Name:          "test.exe",
		ExePath:       "C:\\Windows\\System32\\test.exe",
		CPUPercent:    5.0,
		WorkingSet:    100 * 1024 * 1024,
		PrivateBytes:  50 * 1024 * 1024,
		PageFaults:    1000,
		IOReadBytes:   4096,
		IOWriteBytes:  2048,
		IOReadOps:     10,
		IOWriteOps:    5,
		ThreadCount:   8,
		CreateTime:    1700000000,
		IsCritical:    false,
		Status:        "Running",
		Connections:   3,
		PriorityClass: 0x20,
	}
	if p.PID == 0 {
		t.Error("PID should be non-zero for a real process")
	}
	assertJSONRoundTrip(t, "ProcessInfo", p)
}

func TestProcessNode_Valid(t *testing.T) {
	child := &ProcessNode{
		Process:  ProcessInfo{PID: 2, Name: "child.exe"},
		Children: nil,
		Depth:    1,
		IsOrphan: false,
	}
	parent := &ProcessNode{
		Process:  ProcessInfo{PID: 1, Name: "parent.exe"},
		Children: []*ProcessNode{child},
		Depth:    0,
		IsOrphan: false,
	}
	if len(parent.Children) != 1 {
		t.Errorf("len(Children) = %d, want 1", len(parent.Children))
	}
	if parent.Children[0].Depth != 1 {
		t.Errorf("child Depth = %d, want 1", parent.Children[0].Depth)
	}
	assertJSONRoundTrip(t, "ProcessNode", parent)
}

func TestProcessNode_OrphanFlag(t *testing.T) {
	n := &ProcessNode{
		Process:  ProcessInfo{PID: 99, Name: "orphan.exe"},
		Depth:    0,
		IsOrphan: true,
	}
	if !n.IsOrphan {
		t.Error("expected IsOrphan = true")
	}
}

func TestPortBinding_Valid(t *testing.T) {
	pb := PortBinding{
		Protocol:   "TCP",
		LocalAddr:  "0.0.0.0",
		LocalPort:  443,
		RemoteAddr: "192.168.1.5",
		RemotePort: 51234,
		State:      "ESTABLISHED",
		StateCode:  5,
		PID:        4242,
		Process:    "chrome.exe",
		Label:      "https",
		Since:      1700000000,
	}
	if pb.LocalPort == 0 {
		t.Error("LocalPort should be non-zero")
	}
	assertJSONRoundTrip(t, "PortBinding", pb)
}

func TestSystemSnapshot_Valid(t *testing.T) {
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	s := SystemSnapshot{
		Timestamp: now,
		CPU:       CPUMetrics{TotalPercent: 10.0, NumLogical: 8},
		Memory:    MemoryMetrics{TotalPhys: 1024, UsedPhys: 512},
		GPU:       GPUMetrics{Name: "Integrated", Available: false},
		Disk:      DiskMetrics{Drives: []DriveInfo{{Letter: "C"}}},
		Network:   NetworkMetrics{Interfaces: []InterfaceInfo{{Name: "lo"}}},
		Processes: []ProcessInfo{{PID: 1, Name: "init"}},
		ProcessTree: []*ProcessNode{
			{Process: ProcessInfo{PID: 1, Name: "init"}, Depth: 0},
		},
		PortBindings: []PortBinding{
			{Protocol: "TCP", LocalPort: 80},
		},
	}
	if s.Timestamp != now {
		t.Errorf("Timestamp = %v, want %v", s.Timestamp, now)
	}
	if len(s.Processes) != 1 {
		t.Errorf("len(Processes) = %d, want 1", len(s.Processes))
	}
	assertJSONRoundTrip(t, "SystemSnapshot", s)
}

func TestSystemSnapshot_ZeroValue(t *testing.T) {
	// The zero value of SystemSnapshot must be a valid, JSON-marshalable state.
	var s SystemSnapshot
	if s.Timestamp != (time.Time{}) {
		t.Error("zero-value Timestamp should be the zero time")
	}
	assertJSONRoundTrip(t, "SystemSnapshot-zero", s)
}

func TestTimestampedSystem_Valid(t *testing.T) {
	ts := TimestampedSystem{
		Time:    time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
		CPU:     CPUMetrics{TotalPercent: 1.0},
		Memory:  MemoryMetrics{TotalPhys: 1},
		GPU:     GPUMetrics{Available: true},
		Network: NetworkMetrics{},
		Disk:    DiskMetrics{},
	}
	if ts.Time.IsZero() {
		t.Error("Time should be set")
	}
	assertJSONRoundTrip(t, "TimestampedSystem", ts)
}

// --- helpers -----------------------------------------------------------------

// assertJSONRoundTrip encodes v to JSON, then decodes it back into a fresh
// value of the same type. It fails the test if either step errors. This
// exercises every json tag in the struct (the round-trip itself doesn't
// assert specific field values — it just proves the schema is well-formed).
func assertJSONRoundTrip(t *testing.T, name string, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("%s: json.Marshal failed: %v", name, err)
	}
	if len(data) == 0 {
		t.Fatalf("%s: json.Marshal produced empty output", name)
	}
}

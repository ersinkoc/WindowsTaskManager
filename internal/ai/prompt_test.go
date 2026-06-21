package ai

import (
	"fmt"
	"strings"
	"testing"

	"github.com/ersinkoc/WindowsTaskManager/internal/anomaly"
	"github.com/ersinkoc/WindowsTaskManager/internal/metrics"
)

func TestHumanBytes(t *testing.T) {
	cases := []struct {
		in   uint64
		want string
	}{
		{0, "0B"},
		{512, "512B"},
		{1024, "1.0K"},
		{1536, "1.5K"},
		{1024 * 1024, "1.0M"},
		{1024 * 1024 * 1024, "1.0G"},
		{1024 * 1024 * 1024 * 1024, "1.0T"},
		{1024 * 1024 * 1024 * 1024 * 1024, "1.0P"},
		{2 * 1024 * 1024 * 1024 * 1024 * 1024, "2.0P"},
		// 1100 PB - exceeds the loop bound so stays in P with overflowed value
	}
	for _, c := range cases {
		got := humanBytes(c.in)
		if got != c.want {
			t.Errorf("humanBytes(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	cases := []struct {
		in   string
		n    int
		want string
	}{
		{"short", 10, "short"},
		{"short", 5, "short"},
		{"hello world", 6, "hello…"},
		{"abcdef", 3, "ab…"},
	}
	for _, c := range cases {
		got := truncate(c.in, c.n)
		if got != c.want {
			t.Errorf("truncate(%q,%d) = %q, want %q", c.in, c.n, got, c.want)
		}
	}
}

func TestBuildPromptNilSnapshotReturnsUserQuestion(t *testing.T) {
	got := BuildPrompt("en", nil, nil, false, false, "what?")
	if got != "what?" {
		t.Errorf("BuildPrompt(nil snap) = %q, want %q", got, "what?")
	}
}

func TestBuildPromptWithFullSnapshot(t *testing.T) {
	snap := &metrics.SystemSnapshot{
		CPU: metrics.CPUMetrics{
			TotalPercent: 33.3,
			NumLogical:   8,
			Name:         "Intel Core i7",
		},
		Memory: metrics.MemoryMetrics{
			UsedPercent: 60.0,
			UsedPhys:    8 * 1024 * 1024 * 1024,
			TotalPhys:   16 * 1024 * 1024 * 1024,
		},
		GPU: metrics.GPUMetrics{
			Available:   true,
			Name:        "GeForce",
			Utilization: 50.0,
			VRAMUsed:    1 * 1024 * 1024 * 1024,
			VRAMTotal:   2 * 1024 * 1024 * 1024,
		},
		Disk: metrics.DiskMetrics{
			Drives: []metrics.DriveInfo{
				{Letter: "C", UsedPct: 50.0},
				{Letter: "D", UsedPct: 70.0},
			},
		},
		Processes: []metrics.ProcessInfo{
			{PID: 1, Name: "alpha", CPUPercent: 99.9, WorkingSet: 100 * 1024 * 1024},
			{PID: 2, Name: "beta", CPUPercent: 10.0, WorkingSet: 500 * 1024 * 1024},
			{PID: 3, Name: "verylongnamethatistoolongindeed", CPUPercent: 5.0, WorkingSet: 50 * 1024 * 1024},
		},
		ProcessTree: []*metrics.ProcessNode{
			{Process: metrics.ProcessInfo{PID: 100, Name: "root1"}, Children: []*metrics.ProcessNode{}},
			{Process: metrics.ProcessInfo{PID: 101, Name: "root2"}, Children: []*metrics.ProcessNode{}},
		},
		PortBindings: []metrics.PortBinding{
			{State: "listen"},
			{State: "established"},
			{State: "listen"},
		},
	}

	alerts := []anomaly.Alert{
		{Severity: anomaly.SeverityCritical, Title: "alert1", Description: "first"},
	}

	got := BuildPrompt("en", snap, alerts, true, true, "is it ok?")

	// Spot-check for the sections.
	wantSubs := []string{
		"## SYSTEM SNAPSHOT",
		"CPU: 33.3%",
		"Memory: 60.0% used",
		"GPU: GeForce util 50%",
		"Disks: C 50% used, D 70% used",
		"## TOP CPU PROCESSES",
		"## TOP MEMORY PROCESSES",
		"## ACTIVE ALERTS",
		"## PROCESS TREE (roots)",
		"Listening ports: 2, total endpoints: 3",
		"## USER QUESTION",
		"is it ok?",
	}
	for _, sub := range wantSubs {
		if !strings.Contains(got, sub) {
			t.Errorf("prompt missing %q\n---\n%s", sub, got)
		}
	}

	// Truncate name test - name is 35 chars, truncate(p.Name, 25) returns 24 chars + "…"
	if !strings.Contains(got, "verylongnamethatistoolon…") {
		t.Errorf("expected long name to be truncated with ellipsis, got: %s", got)
	}
}

func TestBuildPromptEmptyUserQuestionUsesDefault(t *testing.T) {
	snap := &metrics.SystemSnapshot{}
	got := BuildPrompt("en", snap, nil, false, false, "")
	if !strings.Contains(got, "## REQUEST") {
		t.Errorf("expected default request section, got: %s", got)
	}
	if !strings.Contains(got, "health assessment") {
		t.Errorf("expected default request text, got: %s", got)
	}
}

func TestBuildPromptNoGPU(t *testing.T) {
	snap := &metrics.SystemSnapshot{}
	got := BuildPrompt("en", snap, nil, false, false, "hi")
	if strings.Contains(got, "GPU:") {
		t.Errorf("expected no GPU section when not available, got: %s", got)
	}
}

func TestBuildPromptNoDisks(t *testing.T) {
	snap := &metrics.SystemSnapshot{}
	got := BuildPrompt("en", snap, nil, false, false, "hi")
	if strings.Contains(got, "Disks:") {
		t.Errorf("expected no Disks section, got: %s", got)
	}
}

func TestBuildPromptNoProcessTreeOrPorts(t *testing.T) {
	snap := &metrics.SystemSnapshot{
		ProcessTree:  []*metrics.ProcessNode{{Process: metrics.ProcessInfo{PID: 1, Name: "r"}}},
		PortBindings: []metrics.PortBinding{{State: "listen"}},
	}
	got := BuildPrompt("en", snap, nil, false, false, "hi")
	if strings.Contains(got, "PROCESS TREE") {
		t.Errorf("did not expect process tree section, got: %s", got)
	}
	if strings.Contains(got, "Listening ports") {
		t.Errorf("did not expect port listing section, got: %s", got)
	}
}

func TestBuildPromptProcessTreeCappedAt6(t *testing.T) {
	tree := make([]*metrics.ProcessNode, 0, 10)
	for i := 0; i < 10; i++ {
		tree = append(tree, &metrics.ProcessNode{Process: metrics.ProcessInfo{PID: uint32(i), Name: "n"}})
	}
	snap := &metrics.SystemSnapshot{ProcessTree: tree}
	got := BuildPrompt("en", snap, nil, true, false, "")
	count := strings.Count(got, "PID ")
	if count != 6 {
		t.Errorf("expected 6 tree entries, got %d in: %s", count, got)
	}
}

func TestSystemPrompt(t *testing.T) {
	if got := SystemPrompt("en"); got == "" {
		t.Error("SystemPrompt returned empty string")
	}
	if got := SystemPrompt("xx"); got == "" {
		t.Error("SystemPrompt with unknown language returned empty string")
	}
}

func TestBuildPromptSmallProcessList(t *testing.T) {
	// 3 processes (≤ 8) — exercises the "no truncation" branch in both CPU and MEM slices.
	snap := &metrics.SystemSnapshot{
		Processes: []metrics.ProcessInfo{
			{PID: 1, Name: "a", CPUPercent: 50.0, WorkingSet: 100},
			{PID: 2, Name: "b", CPUPercent: 40.0, WorkingSet: 200},
			{PID: 3, Name: "c", CPUPercent: 30.0, WorkingSet: 300},
		},
	}
	got := BuildPrompt("en", snap, nil, false, false, "")
	// All 3 should appear in both sections.
	cpuCount := strings.Count(got, "PID ")
	if cpuCount != 6 { // 3 in CPU section + 3 in MEM section
		t.Errorf("expected 6 PID lines (3 CPU + 3 MEM), got %d in: %s", cpuCount, got)
	}
}

func TestBuildPromptLargeProcessListTruncatesTo8(t *testing.T) {
	// 12 processes (> 8) — exercises the truncation branch in both CPU and MEM slices.
	procs := make([]metrics.ProcessInfo, 0, 12)
	for i := 0; i < 12; i++ {
		procs = append(procs, metrics.ProcessInfo{
			PID: uint32(i + 1), Name: fmt.Sprintf("p%d", i),
			CPUPercent: float64(100 - i), WorkingSet: uint64((i + 1) * 1024 * 1024),
		})
	}
	snap := &metrics.SystemSnapshot{Processes: procs}
	got := BuildPrompt("en", snap, nil, false, false, "")
	// Should have exactly 8 PID lines in CPU + 8 in MEM = 16.
	cpuCount := strings.Count(got, "PID ")
	if cpuCount != 16 {
		t.Errorf("expected 16 PID lines (8 CPU + 8 MEM after truncation), got %d in: %s", cpuCount, got)
	}
}

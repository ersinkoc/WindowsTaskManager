//go:build windows

package collector

import (
	"testing"
	"time"
)

func TestNewCPUCollectorSetsDefaults(t *testing.T) {
	c := NewCPUCollector("Test CPU", 2400)
	if c.cpuName != "Test CPU" {
		t.Fatalf("cpuName=%q", c.cpuName)
	}
	if c.freqMHz != 2400 {
		t.Fatalf("freqMHz=%d", c.freqMHz)
	}
	if c.numLogical <= 0 {
		t.Fatalf("numLogical=%d", c.numLogical)
	}
	if c.hasPrev {
		t.Fatal("hasPrev should default to false")
	}
}

func TestNewCPUCollectorWithEmptyArgs(t *testing.T) {
	c := NewCPUCollector("", 0)
	if c.cpuName != "" {
		t.Fatalf("cpuName=%q", c.cpuName)
	}
	if c.freqMHz != 0 {
		t.Fatalf("freqMHz=%d", c.freqMHz)
	}
}

func TestCPUCollectorCollectPopulatesStatic(t *testing.T) {
	c := NewCPUCollector("Test CPU", 2400)
	out := c.Collect()
	if out.Name != "Test CPU" {
		t.Fatalf("Name=%q", out.Name)
	}
	if out.FreqMHz != 2400 {
		t.Fatalf("FreqMHz=%d", out.FreqMHz)
	}
	if out.NumLogical <= 0 {
		t.Fatalf("NumLogical=%d", out.NumLogical)
	}
	if len(out.PerCore) != out.NumLogical {
		t.Fatalf("PerCore len=%d NumLogical=%d", len(out.PerCore), out.NumLogical)
	}
}

func TestCPUCollectorCollectSecondSampleComputesDeltas(t *testing.T) {
	c := NewCPUCollector("Test", 2400)
	_ = c.Collect()
	// Sleep enough that kernel/user activity accumulates to a non-zero delta.
	time.Sleep(50 * time.Millisecond)
	out := c.Collect()
	if out.TotalPercent < 0 {
		t.Fatalf("TotalPercent=%v", out.TotalPercent)
	}
	if out.TotalPercent > 100 {
		t.Fatalf("TotalPercent=%v want <= 100", out.TotalPercent)
	}
}

func TestClamp01ClampsBounds(t *testing.T) {
	if got := clamp01(-0.5); got != 0 {
		t.Fatalf("clamp01(-0.5)=%v want 0", got)
	}
	if got := clamp01(0.5); got != 0.5 {
		t.Fatalf("clamp01(0.5)=%v want 0.5", got)
	}
	if got := clamp01(1.5); got != 1 {
		t.Fatalf("clamp01(1.5)=%v want 1", got)
	}
	if got := clamp01(0); got != 0 {
		t.Fatalf("clamp01(0)=%v", got)
	}
	if got := clamp01(1); got != 1 {
		t.Fatalf("clamp01(1)=%v", got)
	}
}

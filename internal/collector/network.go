//go:build windows

package collector

import (
	"strings"
	"sync"
	"time"

	"github.com/ersinkoc/WindowsTaskManager/internal/metrics"
	"github.com/ersinkoc/WindowsTaskManager/internal/winapi"
)

// netPrev caches an interface's prior counter snapshot for delta calc.
type netPrev struct {
	inOctets   uint64
	outOctets  uint64
	inPackets  uint64
	outPackets uint64
	inErrors   uint64
	outErrors  uint64
	sampleTime time.Time
}

// NetworkCollector samples per-interface throughput.
type NetworkCollector struct {
	mu   sync.Mutex
	prev map[uint32]netPrev
}

// Function variable wrapping the Win32 call so tests can stub the failure path.
var getIfTable2 = winapi.GetIfTable2

func NewNetworkCollector() *NetworkCollector {
	return &NetworkCollector{prev: make(map[uint32]netPrev)}
}

func (n *NetworkCollector) Collect() metrics.NetworkMetrics {
	rows, err := getIfTable2()
	if err != nil || len(rows) == 0 {
		return metrics.NetworkMetrics{}
	}

	now := time.Now()
	out := metrics.NetworkMetrics{
		Interfaces: make([]metrics.InterfaceInfo, 0, len(rows)),
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	live := make(map[uint32]netPrev, len(rows))
	seenNames := make(map[string]struct{}, len(rows))

	for _, r := range rows {
		if shouldSkipInterface(r, seenNames) {
			live[r.Index] = netPrev{
				inOctets: r.InOctets, outOctets: r.OutOctets,
				inPackets: r.InUcastPkts, outPackets: r.OutUcastPkts,
				inErrors: r.InErrors, outErrors: r.OutErrors, sampleTime: now,
			}
			continue
		}

		ii := metrics.InterfaceInfo{
			Name:      displayInterfaceName(r.Alias, r.Description),
			Type:      ifTypeName(r.Type),
			Status:    operStatusName(r.OperStatus),
			SpeedMbps: pickSpeedMbps(r.SpeedRx, r.SpeedTx),
		}

		prev, hasPrev := n.prev[r.Index]
		if hasPrev {
			elapsed := now.Sub(prev.sampleTime).Seconds()
			if elapsed > 0 {
				ii.InBPS = bpsDelta(r.InOctets, prev.inOctets, elapsed)
				ii.OutBPS = bpsDelta(r.OutOctets, prev.outOctets, elapsed)
				ii.InPPS = bpsDelta(r.InUcastPkts, prev.inPackets, elapsed)
				ii.OutPPS = bpsDelta(r.OutUcastPkts, prev.outPackets, elapsed)
				ii.InErrors = saturatingSub(r.InErrors, prev.inErrors)
				ii.OutErrors = saturatingSub(r.OutErrors, prev.outErrors)
			}
		}
		live[r.Index] = netPrev{
			inOctets: r.InOctets, outOctets: r.OutOctets,
			inPackets: r.InUcastPkts, outPackets: r.OutUcastPkts,
			inErrors: r.InErrors, outErrors: r.OutErrors,
			sampleTime: now,
		}

		out.Interfaces = append(out.Interfaces, ii)
		out.TotalDownBPS += ii.InBPS
		out.TotalUpBPS += ii.OutBPS
	}

	n.prev = live
	return out
}

func shouldSkipInterface(r winapi.IfRow2, seen map[string]struct{}) bool {
	if r.Type == 24 || r.Type == 131 {
		return true
	}
	rawName := strings.TrimSpace(preferAlias(r.Alias, r.Description))
	low := strings.ToLower(rawName + " " + r.Description)
	for _, token := range []string{
		"npcap",
		"teredo",
		"isatap",
		"loopback",
		"wfp",
		"filter",
		"lightweight",
		"packet scheduler",
		"virtual wifi filter driver",
		"native wifi filter driver",
		"kernel debugger",
		"hyper-v virtual switch extension",
	} {
		if strings.Contains(low, token) {
			return true
		}
	}
	name := displayInterfaceName(r.Alias, r.Description)
	key := normalizeIfaceName(name)
	if key != "" {
		if _, ok := seen[key]; ok {
			return true
		}
		seen[key] = struct{}{}
	}
	if r.OperStatus != 1 && r.SpeedRx == 0 && r.SpeedTx == 0 && r.InOctets == 0 && r.OutOctets == 0 {
		return true
	}
	if strings.HasPrefix(strings.ToLower(name), "local area connection*") && r.SpeedRx == 0 && r.SpeedTx == 0 {
		return true
	}
	return false
}

func displayInterfaceName(alias, desc string) string {
	name := strings.TrimSpace(preferAlias(alias, desc))
	if name == "" {
		name = strings.TrimSpace(desc)
	}
	low := strings.ToLower(name)
	for _, token := range []string{
		"-npcap",
		"-wfp",
		"-qos packet scheduler",
		"-lightweight filter",
		"-filter driver",
		"-virtual filtering platform",
		"-native wifi filter driver",
		"-virtual wifi filter driver",
	} {
		if idx := strings.Index(low, token); idx > 0 {
			name = strings.TrimSpace(name[:idx])
			break
		}
	}
	return name
}

func normalizeIfaceName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	lastSpace := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastSpace = false
			continue
		}
		if !lastSpace {
			b.WriteByte(' ')
			lastSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

func preferAlias(alias, desc string) string {
	if alias != "" {
		return alias
	}
	return desc
}

func bpsDelta(curr, prev uint64, elapsed float64) uint64 {
	if curr <= prev {
		return 0
	}
	return uint64(float64(curr-prev) / elapsed)
}

func saturatingSub(curr, prev uint64) uint64 {
	if curr <= prev {
		return 0
	}
	return curr - prev
}

func pickSpeedMbps(rx, tx uint64) uint64 {
	speed := rx
	if tx > speed {
		speed = tx
	}
	return speed / 1_000_000
}

// ifTypeName maps IANAifType numbers to readable strings.
func ifTypeName(t uint32) string {
	switch t {
	case 6:
		return "ethernet"
	case 9:
		return "token-ring"
	case 23:
		return "ppp"
	case 24:
		return "loopback"
	case 71:
		return "wifi"
	case 131:
		return "tunnel"
	case 144:
		return "ieee1394"
	}
	return "other"
}

func operStatusName(s uint32) string {
	switch s {
	case 1:
		return "up"
	case 2:
		return "down"
	case 3:
		return "testing"
	case 4:
		return "unknown"
	case 5:
		return "dormant"
	case 6:
		return "not-present"
	case 7:
		return "lower-layer-down"
	}
	return "unknown"
}

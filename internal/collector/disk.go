//go:build windows

package collector

import (
	"math"
	"strings"

	"github.com/ersinkoc/WindowsTaskManager/internal/metrics"
	"github.com/ersinkoc/WindowsTaskManager/internal/winapi"
)

type diskPerfCounters struct {
	query     winapi.PdhQuery
	readBPS   winapi.PdhCounter
	writeBPS  winapi.PdhCounter
	readIOPS  winapi.PdhCounter
	writeIOPS winapi.PdhCounter
}

// DiskCollector returns the list of visible drives plus live LogicalDisk I/O counters.
type DiskCollector struct {
	perf *diskPerfCounters
}

// Function variables wrapping the Win32/PDH calls so tests can stub the
// failure paths without invoking real OS APIs.
var (
	diskGetLogicalDriveStrings = winapi.GetLogicalDriveStrings
	diskGetDriveType           = winapi.GetDriveType
	diskGetDiskFreeSpaceEx     = winapi.GetDiskFreeSpaceEx
	diskGetVolumeInformation   = winapi.GetVolumeInformation
	diskOpenPdhQuery           = winapi.OpenPdhQuery
	diskAddEnglishCounter      = winapi.AddEnglishCounter
	diskCollectQueryData       = winapi.CollectQueryData
	diskGetFormattedCounter    = winapi.GetFormattedCounterArrayDouble
)

func NewDiskCollector() *DiskCollector { return &DiskCollector{} }

func (d *DiskCollector) Collect() metrics.DiskMetrics {
	roots, err := diskGetLogicalDriveStrings()
	if err != nil {
		return metrics.DiskMetrics{}
	}

	readBPS, writeBPS, readIOPS, writeIOPS := d.samplePerfCounters()
	out := metrics.DiskMetrics{Drives: make([]metrics.DriveInfo, 0, len(roots))}
	for _, root := range roots {
		t := diskGetDriveType(root)
		if t != winapi.DRIVE_FIXED && t != winapi.DRIVE_REMOVABLE && t != winapi.DRIVE_REMOTE {
			continue
		}
		_, total, free, err := diskGetDiskFreeSpaceEx(root)
		if err != nil || total == 0 {
			continue
		}
		used := total - free
		label, fs := diskGetVolumeInformation(root)
		instance := strings.ToLower(strings.TrimRight(root, "\\"))
		out.Drives = append(out.Drives, metrics.DriveInfo{
			Letter:     strings.TrimRight(root, "\\"),
			Label:      label,
			FSType:     fs,
			TotalBytes: total,
			FreeBytes:  free,
			UsedBytes:  used,
			UsedPct:    float64(used) / float64(total) * 100.0,
			ReadBPS:    counterUint64(readBPS[instance]),
			WriteBPS:   counterUint64(writeBPS[instance]),
			ReadIOPS:   counterUint64(readIOPS[instance]),
			WriteIOPS:  counterUint64(writeIOPS[instance]),
		})
	}
	return out
}

func (d *DiskCollector) samplePerfCounters() (map[string]float64, map[string]float64, map[string]float64, map[string]float64) {
	if d.perf == nil {
		perf, err := newDiskPerfCounters()
		if err == nil {
			d.perf = perf
		}
	}
	if d.perf == nil {
		return map[string]float64{}, map[string]float64{}, map[string]float64{}, map[string]float64{}
	}
	if err := diskCollectQueryData(d.perf.query); err != nil {
		return map[string]float64{}, map[string]float64{}, map[string]float64{}, map[string]float64{}
	}
	readBPS, err1 := diskGetFormattedCounter(d.perf.readBPS)
	writeBPS, err2 := diskGetFormattedCounter(d.perf.writeBPS)
	readIOPS, err3 := diskGetFormattedCounter(d.perf.readIOPS)
	writeIOPS, err4 := diskGetFormattedCounter(d.perf.writeIOPS)
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
		return map[string]float64{}, map[string]float64{}, map[string]float64{}, map[string]float64{}
	}
	return normalizeCounterMap(readBPS), normalizeCounterMap(writeBPS), normalizeCounterMap(readIOPS), normalizeCounterMap(writeIOPS)
}

func newDiskPerfCounters() (*diskPerfCounters, error) {
	query, err := diskOpenPdhQuery()
	if err != nil {
		return nil, err
	}
	perf := &diskPerfCounters{query: query}
	if perf.readBPS, err = diskAddEnglishCounter(query, `\LogicalDisk(*)\Disk Read Bytes/sec`); err != nil {
		query.Close()
		return nil, err
	}
	if perf.writeBPS, err = diskAddEnglishCounter(query, `\LogicalDisk(*)\Disk Write Bytes/sec`); err != nil {
		query.Close()
		return nil, err
	}
	if perf.readIOPS, err = diskAddEnglishCounter(query, `\LogicalDisk(*)\Disk Reads/sec`); err != nil {
		query.Close()
		return nil, err
	}
	if perf.writeIOPS, err = diskAddEnglishCounter(query, `\LogicalDisk(*)\Disk Writes/sec`); err != nil {
		query.Close()
		return nil, err
	}
	if err := diskCollectQueryData(query); err != nil {
		query.Close()
		return nil, err
	}
	return perf, nil
}

func normalizeCounterMap(values map[string]float64) map[string]float64 {
	normalized := make(map[string]float64, len(values))
	for name, value := range values {
		key := strings.ToLower(strings.TrimSpace(name))
		if key == "" || key == "_total" {
			continue
		}
		normalized[key] = value
	}
	return normalized
}

func counterUint64(value float64) uint64 {
	if value <= 0 {
		return 0
	}
	return uint64(math.Round(value))
}

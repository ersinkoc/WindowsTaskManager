//go:build windows

package collector

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/ersinkoc/WindowsTaskManager/internal/metrics"
	"github.com/ersinkoc/WindowsTaskManager/internal/winapi"
)

var (
	gpuEnginePattern = regexp.MustCompile(`(?i)^pid_\d+_(luid_0x[0-9a-f]+_0x[0-9a-f]+_phys_\d+)_eng_(\d+)_`)
	gpuMemoryPattern = regexp.MustCompile(`(?i)^(luid_0x[0-9a-f]+_0x[0-9a-f]+_phys_\d+)$`)
)

type gpuAdapterInfo struct {
	name       string
	totalBytes uint64
}

type gpuAdapterSample struct {
	utilization float64
	dedicated   uint64
	shared      uint64
}

type gpuPerfCounters struct {
	query     winapi.PdhQuery
	util      winapi.PdhCounter
	dedicated winapi.PdhCounter
	shared    winapi.PdhCounter
}

// GPUCollector returns live engine utilization and adapter memory usage via
// built-in Windows perf counters, with adapter metadata sourced from registry.
type GPUCollector struct {
	perf            *gpuPerfCounters
	adapters        []gpuAdapterInfo
	cachedName      string
	cachedTotalVRAM uint64
}

// Function variables wrapping the Win32/PDH/registry calls so tests can
// stub the failure paths without invoking real OS APIs.
var (
	gpuOpenPdhQuery      = winapi.OpenPdhQuery
	gpuAddEnglishCounter = winapi.AddEnglishCounter
	gpuCollectQueryData  = winapi.CollectQueryData
	gpuGetFormatted      = winapi.GetFormattedCounterArrayDouble
	gpuRegReadString     = winapi.RegReadString
	gpuRegReadQWORD      = winapi.RegReadQWORD
)

func NewGPUCollector() *GPUCollector { return &GPUCollector{} }

func (g *GPUCollector) Collect() metrics.GPUMetrics {
	g.initInventory()
	if g.perf == nil {
		perf, err := newGPUPerfCounters()
		if err == nil {
			g.perf = perf
		}
	}
	if g.perf == nil {
		return metrics.GPUMetrics{
			Name:        g.cachedName,
			Temperature: -1,
			Available:   false,
		}
	}
	if err := gpuCollectQueryData(g.perf.query); err != nil {
		return metrics.GPUMetrics{
			Name:        g.cachedName,
			VRAMTotal:   g.cachedTotalVRAM,
			Temperature: -1,
			Available:   false,
		}
	}

	utilValues, err1 := formattedCounterArrayDouble(g.perf.util)
	dedicatedValues, err2 := formattedCounterArrayDouble(g.perf.dedicated)
	sharedValues, err3 := formattedCounterArrayDouble(g.perf.shared)
	if err1 != nil && err2 != nil && err3 != nil {
		return metrics.GPUMetrics{
			Name:        g.cachedName,
			VRAMTotal:   g.cachedTotalVRAM,
			Temperature: -1,
			Available:   false,
		}
	}

	samples := aggregateGPUSamples(utilValues, dedicatedValues, sharedValues)
	key, sample, ok := pickPrimaryGPU(samples)
	available := ok || len(utilValues) > 0 || len(dedicatedValues) > 0 || len(sharedValues) > 0

	name := g.cachedName
	total := g.cachedTotalVRAM
	if key != "" {
		if idx, err := parseGPUAdapterIndex(key); err == nil && idx >= 0 && idx < len(g.adapters) {
			if g.adapters[idx].name != "" {
				name = g.adapters[idx].name
			}
			if g.adapters[idx].totalBytes > 0 {
				total = g.adapters[idx].totalBytes
			}
		}
	}
	if name == "" {
		name = "Unknown GPU"
	}

	used := sample.dedicated
	if used == 0 {
		used = sample.dedicated + sample.shared
	}
	if total < used {
		total = used
	}
	return metrics.GPUMetrics{
		Name:        name,
		Utilization: sample.utilization,
		VRAMUsed:    used,
		VRAMTotal:   total,
		Temperature: -1,
		Available:   available,
	}
}

func (g *GPUCollector) initInventory() {
	if len(g.adapters) != 0 {
		return
	}
	g.adapters = readGPUAdapters()
	for _, adapter := range g.adapters {
		if g.cachedName == "" && adapter.name != "" {
			g.cachedName = adapter.name
		}
		if adapter.totalBytes > g.cachedTotalVRAM {
			g.cachedTotalVRAM = adapter.totalBytes
			if adapter.name != "" {
				g.cachedName = adapter.name
			}
		}
	}
	if g.cachedName == "" {
		g.cachedName = "Unknown GPU"
	}
}

func newGPUPerfCounters() (*gpuPerfCounters, error) {
	query, err := gpuOpenPdhQuery()
	if err != nil {
		return nil, err
	}
	perf := &gpuPerfCounters{query: query}
	if perf.util, err = gpuAddEnglishCounter(query, `\GPU Engine(*)\Utilization Percentage`); err != nil {
		query.Close()
		return nil, err
	}
	// Adapter-memory counters are optional on some Windows installs. Keep the
	// GPU path alive if utilization is available and just omit VRAM numbers.
	perf.dedicated, _ = gpuAddEnglishCounter(query, `\GPU Adapter Memory(*)\Dedicated Usage`)
	perf.shared, _ = gpuAddEnglishCounter(query, `\GPU Adapter Memory(*)\Shared Usage`)
	if err := gpuCollectQueryData(query); err != nil {
		query.Close()
		return nil, err
	}
	return perf, nil
}

func formattedCounterArrayDouble(counter winapi.PdhCounter) (map[string]float64, error) {
	if counter == 0 {
		return map[string]float64{}, nil
	}
	return gpuGetFormatted(counter)
}

func aggregateGPUSamples(utilValues, dedicatedValues, sharedValues map[string]float64) map[string]gpuAdapterSample {
	samples := make(map[string]gpuAdapterSample)
	engineTotals := map[string]map[string]float64{}

	for instance, value := range utilValues {
		adapterKey, engineKey, ok := parseGPUEngineInstance(instance)
		if !ok {
			continue
		}
		if engineTotals[adapterKey] == nil {
			engineTotals[adapterKey] = map[string]float64{}
		}
		engineTotals[adapterKey][engineKey] += value
	}
	for adapterKey, engines := range engineTotals {
		sample := samples[adapterKey]
		for _, value := range engines {
			if value > 100 {
				value = 100
			}
			if value > sample.utilization {
				sample.utilization = value
			}
		}
		samples[adapterKey] = sample
	}

	for instance, value := range dedicatedValues {
		adapterKey, ok := parseGPUAdapterInstance(instance)
		if !ok {
			continue
		}
		sample := samples[adapterKey]
		sample.dedicated = counterUint64(value)
		samples[adapterKey] = sample
	}
	for instance, value := range sharedValues {
		adapterKey, ok := parseGPUAdapterInstance(instance)
		if !ok {
			continue
		}
		sample := samples[adapterKey]
		sample.shared = counterUint64(value)
		samples[adapterKey] = sample
	}
	return samples
}

func pickPrimaryGPU(samples map[string]gpuAdapterSample) (string, gpuAdapterSample, bool) {
	var (
		bestKey    string
		bestSample gpuAdapterSample
		found      bool
		bestScore  float64
	)
	for key, sample := range samples {
		score := sample.utilization*1_000_000 + float64(sample.dedicated+sample.shared)
		if !found || score > bestScore {
			bestKey = key
			bestSample = sample
			bestScore = score
			found = true
		}
	}
	if found {
		return bestKey, bestSample, true
	}
	return "", gpuAdapterSample{}, false
}

func parseGPUEngineInstance(instance string) (string, string, bool) {
	m := gpuEnginePattern.FindStringSubmatch(strings.ToLower(strings.TrimSpace(instance)))
	if len(m) != 3 {
		return "", "", false
	}
	return m[1], m[2], true
}

func parseGPUAdapterInstance(instance string) (string, bool) {
	m := gpuMemoryPattern.FindStringSubmatch(strings.ToLower(strings.TrimSpace(instance)))
	if len(m) != 2 {
		return "", false
	}
	return m[1], true
}

func parseGPUAdapterIndex(adapterKey string) (int, error) {
	idx := strings.LastIndex(strings.ToLower(adapterKey), "_phys_")
	if idx < 0 {
		return 0, fmt.Errorf("adapter key %q missing phys marker", adapterKey)
	}
	value := adapterKey[idx+len("_phys_"):]
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}
	return n, nil
}

func readGPUAdapters() []gpuAdapterInfo {
	const classRoot = `SYSTEM\CurrentControlSet\Control\Class\{4d36e968-e325-11ce-bfc1-08002be10318}`
	adapters := make([]gpuAdapterInfo, 0, 4)
	for idx := 0; idx < 32; idx++ {
		subKey := fmt.Sprintf(`%s\%04d`, classRoot, idx)
		name, _ := gpuRegReadString(winapi.HKEY_LOCAL_MACHINE, subKey, "DriverDesc")
		if name == "" {
			name, _ = gpuRegReadString(winapi.HKEY_LOCAL_MACHINE, subKey, "HardwareInformation.AdapterString")
		}
		totalBytes, _ := gpuRegReadQWORD(winapi.HKEY_LOCAL_MACHINE, subKey, "HardwareInformation.qwMemorySize")
		if name == "" && totalBytes == 0 {
			continue
		}
		adapters = append(adapters, gpuAdapterInfo{
			name:       strings.TrimSpace(name),
			totalBytes: totalBytes,
		})
	}
	if len(adapters) == 0 {
		if name := readGPUName(); name != "" {
			adapters = append(adapters, gpuAdapterInfo{name: name})
		}
	}
	return adapters
}

// readGPUName reads a best-effort display adapter name from the registry.
func readGPUName() string {
	const path = `SYSTEM\CurrentControlSet\Control\Video\{00000000-0000-0000-0000-000000000000}\0000`
	if name, err := gpuRegReadString(winapi.HKEY_LOCAL_MACHINE, path, "DriverDesc"); err == nil && name != "" {
		return name
	}
	return "Unknown GPU"
}

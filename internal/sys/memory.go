// Package sys provides lightweight system-level utilities such as
// memory monitoring, inspired by Python Crawl4AI's memory checks.
package sys

import "runtime"

// MemStats holds a snapshot of memory statistics.
type MemStats struct {
	TotalMB      uint64  // total memory available to the Go runtime (HeapSys)
	UsedMB       uint64  // memory currently in use (HeapAlloc)
	AvailableMB  uint64  // estimated available memory (TotalMB - UsedMB)
	HeapMB       uint64  // heap in-use bytes (HeapInuse) in MB
	StackMB      uint64  // stack in-use bytes (StackInuse) in MB
	UsagePercent float64 // HeapInuse / HeapSys * 100
}

// GetMemStats returns a snapshot of the current memory statistics
// using runtime.MemStats.
func GetMemStats() MemStats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	const mb = 1024 * 1024

	totalMB := m.HeapSys / mb
	usedMB := m.HeapAlloc / mb
	heapMB := m.HeapInuse / mb
	stackMB := m.StackInuse / mb

	var availMB uint64
	if totalMB > usedMB {
		availMB = totalMB - usedMB
	}

	var pct float64
	if m.HeapSys > 0 {
		pct = float64(m.HeapInuse) / float64(m.HeapSys) * 100
	}

	return MemStats{
		TotalMB:      totalMB,
		UsedMB:       usedMB,
		AvailableMB:  availMB,
		HeapMB:       heapMB,
		StackMB:      stackMB,
		UsagePercent: pct,
	}
}

// IsMemoryHigh returns true when heap usage (HeapInuse / HeapSys * 100)
// exceeds thresholdPercent.
func IsMemoryHigh(thresholdPercent float64) bool {
	s := GetMemStats()
	return s.UsagePercent > thresholdPercent
}

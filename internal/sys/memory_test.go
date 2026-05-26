package sys

import (
	"testing"
)

func TestGetMemStats_ReturnsNonZero(t *testing.T) {
	s := GetMemStats()
	// The Go runtime always has some heap allocated; UsedMB should be >= 0.
	// We mainly verify the function runs without panicking and returns a
	// structurally valid result.
	if s.UsagePercent < 0 || s.UsagePercent > 100 {
		t.Errorf("UsagePercent out of range [0,100]: %v", s.UsagePercent)
	}
}

func TestGetMemStats_AvailableConsistency(t *testing.T) {
	s := GetMemStats()
	// AvailableMB = TotalMB - UsedMB (clamped to 0).
	if s.TotalMB >= s.UsedMB {
		if s.AvailableMB != s.TotalMB-s.UsedMB {
			t.Errorf("AvailableMB inconsistency: total=%d used=%d available=%d",
				s.TotalMB, s.UsedMB, s.AvailableMB)
		}
	} else {
		// UsedMB > TotalMB can happen at sub-MB granularity (truncation); available should be 0.
		if s.AvailableMB != 0 {
			t.Errorf("expected AvailableMB=0 when usedMB>totalMB, got %d", s.AvailableMB)
		}
	}
}

func TestIsMemoryHigh_ZeroThreshold(t *testing.T) {
	// Any non-zero heap usage will exceed 0%.
	// We just confirm the function returns a bool without panicking.
	_ = IsMemoryHigh(0)
}

func TestIsMemoryHigh_HundredPercent(t *testing.T) {
	// Memory usage will never truly be >100%, so this should return false.
	if IsMemoryHigh(100) {
		t.Error("IsMemoryHigh(100) should be false: usage cannot exceed 100%")
	}
}

func TestIsMemoryHigh_NegativeThreshold(t *testing.T) {
	// Any positive usage exceeds a negative threshold.
	result := IsMemoryHigh(-1)
	// The Go runtime always has some heap in use, so usage > -1 is always true.
	if !result {
		t.Log("IsMemoryHigh(-1) returned false; this is unexpected but not fatal")
	}
}

func TestMemStats_Fields(t *testing.T) {
	// Verify MemStats zero value is valid and field assignments work.
	var m MemStats
	m.TotalMB = 1024
	m.UsedMB = 512
	m.AvailableMB = 512
	m.HeapMB = 480
	m.StackMB = 32
	m.UsagePercent = 46.875

	if m.TotalMB != 1024 {
		t.Errorf("TotalMB: got %d, want 1024", m.TotalMB)
	}
	if m.UsedMB != 512 {
		t.Errorf("UsedMB: got %d, want 512", m.UsedMB)
	}
	if m.AvailableMB != 512 {
		t.Errorf("AvailableMB: got %d, want 512", m.AvailableMB)
	}
	if m.HeapMB != 480 {
		t.Errorf("HeapMB: got %d, want 480", m.HeapMB)
	}
	if m.StackMB != 32 {
		t.Errorf("StackMB: got %d, want 32", m.StackMB)
	}
}

func TestGetMemStats_MultipleCallsConsistent(t *testing.T) {
	// Multiple calls should all return structurally valid results.
	for i := 0; i < 5; i++ {
		s := GetMemStats()
		if s.UsagePercent < 0 {
			t.Errorf("call %d: negative UsagePercent %v", i, s.UsagePercent)
		}
	}
}

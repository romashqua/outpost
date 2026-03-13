package s2s

import "testing"

func TestHealthTracker_Report(t *testing.T) {
	ht := NewHealthTracker(3)

	ht.Report("tunnel-1", "gw-a", "gw-b", true, 10)

	if !ht.IsHealthy("tunnel-1", "gw-a", "gw-b") {
		t.Error("expected healthy after successful report")
	}

	health := ht.GetHealth("tunnel-1")
	if len(health) != 1 {
		t.Fatalf("expected 1 health entry, got %d", len(health))
	}
	if health[0].LatencyMs != 10 {
		t.Errorf("expected latency 10, got %d", health[0].LatencyMs)
	}
}

func TestHealthTracker_UnhealthyAfterThreshold(t *testing.T) {
	ht := NewHealthTracker(3)

	// Start healthy.
	ht.Report("t1", "a", "b", true, 5)
	if !ht.IsHealthy("t1", "a", "b") {
		t.Error("expected healthy")
	}

	// Fail 1 and 2 — still healthy (threshold is 3).
	ht.Report("t1", "a", "b", false, 0)
	ht.Report("t1", "a", "b", false, 0)
	if !ht.IsHealthy("t1", "a", "b") {
		t.Error("expected still healthy after 2 failures")
	}

	// Fail 3 — now unhealthy.
	ht.Report("t1", "a", "b", false, 0)
	if ht.IsHealthy("t1", "a", "b") {
		t.Error("expected unhealthy after 3 failures")
	}
}

func TestHealthTracker_RecoverAfterSuccess(t *testing.T) {
	ht := NewHealthTracker(2)

	ht.Report("t1", "a", "b", false, 0)
	ht.Report("t1", "a", "b", false, 0)
	if ht.IsHealthy("t1", "a", "b") {
		t.Error("expected unhealthy")
	}

	ht.Report("t1", "a", "b", true, 5)
	if !ht.IsHealthy("t1", "a", "b") {
		t.Error("expected healthy after recovery")
	}
}

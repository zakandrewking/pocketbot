package session

import (
	"testing"
	"time"
)

func TestActivityMonitorInitialState(t *testing.T) {
	monitor := NewActivityMonitor(2 * time.Second)

	// Should start as active (just created)
	if monitor.GetState() != StateActive {
		t.Error("New monitor should start in active state")
	}
}

func TestActivityMonitorIdleTransition(t *testing.T) {
	monitor := NewActivityMonitor(100 * time.Millisecond)

	// Should be active initially
	if monitor.GetState() != StateActive {
		t.Error("Should be active initially")
	}

	// Wait for idle timeout
	time.Sleep(150 * time.Millisecond)

	// Should now be idle
	if monitor.GetState() != StateIdle {
		t.Error("Should be idle after timeout")
	}
}

func TestActivityMonitorRecordActivity(t *testing.T) {
	monitor := NewActivityMonitor(100 * time.Millisecond)

	// Wait almost to timeout
	time.Sleep(80 * time.Millisecond)

	// Record activity
	monitor.RecordActivity()

	// Should still be active
	if monitor.GetState() != StateActive {
		t.Error("Should be active after recording activity")
	}

	// Wait a bit more
	time.Sleep(80 * time.Millisecond)

	// Should still be active (reset the timer)
	if monitor.GetState() != StateActive {
		t.Error("Should be active - timer was reset")
	}

	// Wait for full timeout
	time.Sleep(120 * time.Millisecond)

	// Now should be idle
	if monitor.GetState() != StateIdle {
		t.Error("Should be idle after full timeout")
	}
}

func TestSessionActivityState(t *testing.T) {
	m := New()

	// Not running = idle
	if m.GetActivityState() != StateIdle {
		t.Error("Non-running session should be idle")
	}

	// Start session
	if err := m.Start(); err != nil {
		t.Skipf("Skipping: claude not available: %v", err)
	}
	defer m.Stop()

	// Should be active (just started)
	state := m.GetActivityState()
	if state != StateActive && state != StateIdle {
		t.Error("Running session should have valid activity state")
	}
}

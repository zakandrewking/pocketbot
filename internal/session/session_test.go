package session

import (
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	m := New()
	if m == nil {
		t.Fatal("New() should return a non-nil manager")
	}
	if m.IsRunning() {
		t.Error("New manager should not be running")
	}
}

func TestStartStop(t *testing.T) {
	m := New()

	// Note: This test requires 'claude' to be in PATH
	// In a real scenario, we might want to mock the command
	err := m.Start()
	if err != nil {
		t.Skipf("Skipping test: claude command not available: %v", err)
	}

	// Give it a moment to start
	time.Sleep(100 * time.Millisecond)

	if !m.IsRunning() {
		t.Error("Session should be running after Start()")
	}

	// Stop the session
	err = m.Stop()
	if err != nil {
		t.Errorf("Stop() failed: %v", err)
	}

	// Give it a moment to stop
	time.Sleep(100 * time.Millisecond)

	if m.IsRunning() {
		t.Error("Session should not be running after Stop()")
	}
}

func TestDoubleStart(t *testing.T) {
	m := New()

	err := m.Start()
	if err != nil {
		t.Skipf("Skipping test: claude command not available: %v", err)
	}
	defer m.Stop()

	// Try to start again
	err = m.Start()
	if err == nil {
		t.Error("Starting an already running session should return error")
	}
}

func TestStopNonRunning(t *testing.T) {
	m := New()

	// Stop should not error on a non-running session
	err := m.Stop()
	if err != nil {
		t.Errorf("Stop() on non-running session should not error: %v", err)
	}
}

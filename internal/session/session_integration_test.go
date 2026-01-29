package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestAttachDetachFlow tests the full attach/detach cycle with a mock command
func TestAttachDetachFlow(t *testing.T) {
	// Create a mock script that simulates Claude
	tmpDir := t.TempDir()
	mockScript := filepath.Join(tmpDir, "mock-claude")

	// Mock script that just echoes input and waits
	scriptContent := `#!/bin/bash
echo "Mock Claude started"
# Read from stdin and echo back
while IFS= read -r line; do
    echo "You said: $line"
done
`
	if err := os.WriteFile(mockScript, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("Failed to create mock script: %v", err)
	}

	// Create a manager with mock command
	m := NewWithCommand(mockScript)

	// Start the mock session
	if err := m.Start(); err != nil {
		t.Fatalf("Failed to start session: %v", err)
	}
	defer m.Stop()

	// Give it time to start
	time.Sleep(100 * time.Millisecond)

	if !m.IsRunning() {
		t.Error("Session should be running after Start()")
	}

	// Note: We can't easily test the full Attach() flow in an automated test
	// because it requires terminal interaction and raw mode.
	// This test at least verifies the session lifecycle works.

	// Stop the session
	if err := m.Stop(); err != nil {
		t.Errorf("Failed to stop session: %v", err)
	}

	// Give it time to stop
	time.Sleep(100 * time.Millisecond)

	if m.IsRunning() {
		t.Error("Session should not be running after Stop()")
	}
}

// TestSessionSurvivesDetach verifies session keeps running after detach
func TestSessionSurvivesDetach(t *testing.T) {
	tmpDir := t.TempDir()
	mockScript := filepath.Join(tmpDir, "mock-claude-long")

	// Mock script that runs for a while
	scriptContent := `#!/bin/bash
echo "Mock Claude started"
sleep 10
`
	if err := os.WriteFile(mockScript, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("Failed to create mock script: %v", err)
	}

	m := NewWithCommand(mockScript)

	if err := m.Start(); err != nil {
		t.Fatalf("Failed to start session: %v", err)
	}
	defer m.Stop()

	time.Sleep(100 * time.Millisecond)

	if !m.IsRunning() {
		t.Error("Session should be running")
	}

	// Simulate detach by just checking session is still running
	// (we can't actually test Attach() with Ctrl+D in automated tests)
	time.Sleep(200 * time.Millisecond)

	if !m.IsRunning() {
		t.Error("Session should still be running after simulated detach")
	}
}

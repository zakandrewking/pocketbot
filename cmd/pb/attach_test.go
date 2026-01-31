package main

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/zakandrewking/pocketbot/internal/config"
	"github.com/zakandrewking/pocketbot/internal/tmux"
)

// TestClaudeCommandFlag verifies the claude command uses valid flags.
// This is a regression test for the bug where --accept-edits (invalid)
// was used instead of --permission-mode acceptEdits (valid).
func TestClaudeCommandFlag(t *testing.T) {
	cfg := config.DefaultConfig()

	// The command should use --permission-mode, NOT --accept-edits
	if strings.Contains(cfg.Claude.Command, "--accept-edits") {
		t.Errorf("Claude command uses invalid --accept-edits flag: %s", cfg.Claude.Command)
		t.Log("This causes claude to exit immediately with 'unknown option' error")
	}

	if !strings.Contains(cfg.Claude.Command, "--permission-mode") {
		t.Errorf("Claude command should use --permission-mode flag: %s", cfg.Claude.Command)
	}

	t.Logf("✓ Claude command is valid: %s", cfg.Claude.Command)
}

// TestInvalidClaudeFlagCausesExit demonstrates what happens with the bug.
// This test proves the bug by showing claude exits with invalid flag.
func TestInvalidClaudeFlagCausesExit(t *testing.T) {
	if !tmux.Available() {
		t.Skip("tmux not available")
	}

	// Check if claude is available
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude not available")
	}

	sessionName := "test-invalid-flag"
	tmux.KillSession(sessionName)
	defer tmux.KillSession(sessionName)
	defer tmux.KillServer()

	// Test with INVALID flag (the bug)
	// We use a wrapper that captures the error and keeps session alive
	invalidCmd := "claude --accept-edits 2>&1 | head -1; sleep 2"
	if err := tmux.CreateSession(sessionName, invalidCmd); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	time.Sleep(500 * time.Millisecond)
	out, err := tmux.CapturePane(sessionName)
	if err != nil {
		t.Fatalf("Failed to capture pane: %v", err)
	}

	// Should show the error message
	if !strings.Contains(out, "unknown option") && !strings.Contains(out, "error") {
		t.Errorf("Expected error message for invalid flag, got: %s", out)
	} else {
		t.Logf("✓ Confirmed: Invalid flag causes error: %s", strings.TrimSpace(out))
	}
}

// TestValidClaudeFlagWorks proves the fix works.
func TestValidClaudeFlagWorks(t *testing.T) {
	if !tmux.Available() {
		t.Skip("tmux not available")
	}

	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude not available")
	}

	sessionName := "test-valid-flag"
	tmux.KillSession(sessionName)
	defer tmux.KillSession(sessionName)
	defer tmux.KillServer()

	// Test with VALID flag (the fix) - just check it starts without error
	validCmd := "claude --permission-mode acceptEdits --help 2>&1 | head -3; sleep 2"
	if err := tmux.CreateSession(sessionName, validCmd); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	time.Sleep(500 * time.Millisecond)
	out, err := tmux.CapturePane(sessionName)
	if err != nil {
		t.Fatalf("Failed to capture pane: %v", err)
	}

	// Should NOT show "unknown option" error
	if strings.Contains(out, "unknown option") {
		t.Errorf("Valid flag should not cause error, got: %s", out)
	} else {
		t.Logf("✓ Valid flag works without error")
	}
}

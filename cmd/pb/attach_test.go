package main

import (
	"testing"
	"time"

	"github.com/zakandrewking/pocketbot/internal/tmux"
)

func TestAttachAfterCreate(t *testing.T) {
	if !tmux.Available() {
		t.Skip("tmux not available")
	}

	sessionName := "test-attach-race"

	// Clean up any existing session
	tmux.KillSession(sessionName)
	defer tmux.KillSession(sessionName)

	// Create a session with a simple command that stays alive
	command := "echo 'Test session started'; sleep 30"
	if err := tmux.CreateSession(sessionName, command); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Verify session exists
	if !tmux.SessionExists(sessionName) {
		t.Fatal("Session should exist after creation")
	}

	// Wait for session to initialize (mimics what main.go does)
	time.Sleep(500 * time.Millisecond)

	// Verify we can check if it's running
	// (In the real bug, this would pass but attach would fail)
	if !tmux.SessionExists(sessionName) {
		t.Fatal("Session should still exist after delay")
	}

	// Test that we can capture pane content (proves session is ready)
	// This simulates what would happen during attach
	out, err := tmux.CapturePane(sessionName)
	if err != nil {
		t.Fatalf("Failed to capture pane (session not ready): %v", err)
	}

	// Verify the command output is visible
	if out == "" {
		t.Error("Pane content should not be empty")
	}

	t.Logf("Session initialized successfully. Pane content length: %d bytes", len(out))
}

func TestAttachWithoutDelay(t *testing.T) {
	if !tmux.Available() {
		t.Skip("tmux not available")
	}

	sessionName := "test-attach-nodelay"

	// Clean up
	tmux.KillSession(sessionName)
	defer tmux.KillSession(sessionName)

	// Create session
	if err := tmux.CreateSession(sessionName, "echo 'Starting...'; sleep 30"); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Try to capture immediately (this might fail due to race condition)
	// We don't fail the test if this errors - we just log it
	_, err := tmux.CapturePane(sessionName)
	if err != nil {
		t.Logf("Immediate capture failed (race condition): %v", err)

		// Wait and try again
		time.Sleep(500 * time.Millisecond)
		_, err := tmux.CapturePane(sessionName)
		if err != nil {
			t.Fatalf("Capture still failed after delay: %v", err)
		}
		t.Log("✓ Session became ready after delay")
	} else {
		t.Log("✓ Session was immediately ready (no race condition)")
	}
}

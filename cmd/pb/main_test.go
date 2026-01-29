package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestCtrlCQuits(t *testing.T) {
	m := initialModel()

	// Simulate ctrl-c key press
	msg := tea.KeyMsg{
		Type: tea.KeyCtrlC,
	}

	updatedModel, cmd := m.Update(msg)

	// Verify the model is still the right type
	if _, ok := updatedModel.(model); !ok {
		t.Fatal("Update should return a model")
	}

	// Verify that tea.Quit command is returned
	if cmd == nil {
		t.Fatal("Expected tea.Quit command, got nil")
	}

	// The tea.Quit command should be returned
	// We can't directly compare commands, but we can verify it's not nil
	// which indicates the quit behavior was triggered
}

func TestOtherKeysDoNotQuit(t *testing.T) {
	m := initialModel()

	// Test that other keys don't quit
	testKeys := []tea.KeyMsg{
		{Type: tea.KeyEnter},
		{Type: tea.KeySpace},
		{Type: tea.KeyUp},
		{Type: tea.KeyDown},
	}

	for _, msg := range testKeys {
		_, cmd := m.Update(msg)
		if cmd != nil {
			t.Errorf("Key %v should not trigger quit command", msg.Type)
		}
	}
}

func TestViewRendersWelcomeMessage(t *testing.T) {
	m := initialModel()
	view := m.View()

	if view == "" {
		t.Fatal("View should not be empty")
	}

	// Check that the view contains expected text
	expectedTexts := []string{"Welcome", "PocketBot", "Ctrl+C"}
	for _, expected := range expectedTexts {
		if !contains(view, expected) {
			t.Errorf("View should contain %q, got: %s", expected, view)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || contains(s[1:], substr)))
}

func TestStateTransitionToAttached(t *testing.T) {
	m := initialModel()

	// Verify we start in home view
	if m.viewState != viewHome {
		t.Errorf("Expected viewHome, got %v", m.viewState)
	}

	// Press 'c' to start/attach to Claude
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}
	updatedModel, _ := m.Update(msg)

	m, ok := updatedModel.(model)
	if !ok {
		t.Fatal("Update should return a model")
	}

	// Verify we're now in attached view
	if m.viewState != viewAttached {
		t.Errorf("Expected viewAttached after pressing 'c', got %v", m.viewState)
	}

	// Verify session was started
	if !m.session.IsRunning() {
		t.Error("Session should be running after pressing 'c'")
	}

	// Cleanup
	m.session.Stop()
}

func TestStateTransitionToDetached(t *testing.T) {
	m := initialModel()

	// Start session and attach
	m.session.Start()
	m.viewState = viewAttached

	// Press Ctrl+P to detach
	msg := tea.KeyMsg{Type: tea.KeyCtrlP}
	updatedModel, _ := m.Update(msg)

	m, ok := updatedModel.(model)
	if !ok {
		t.Fatal("Update should return a model")
	}

	// Verify we're back in home view
	if m.viewState != viewHome {
		t.Errorf("Expected viewHome after pressing Ctrl+P, got %v", m.viewState)
	}

	// Session should still be running
	if !m.session.IsRunning() {
		t.Error("Session should still be running after detach")
	}

	// Cleanup
	m.session.Stop()
}

func TestHomeViewShowsSessionStatus(t *testing.T) {
	m := initialModel()

	// View without running session
	view := m.View()
	if contains(view, "Claude is running") {
		t.Error("Should not show 'Claude is running' when session is not running")
	}

	// Start session
	m.session.Start()
	defer m.session.Stop()

	// View with running session
	view = m.View()
	if !contains(view, "Claude is running") {
		t.Error("Should show 'Claude is running' when session is running")
	}
}

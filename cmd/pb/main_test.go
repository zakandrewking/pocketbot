package main

import (
	"fmt"
	"os"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/zakandrewking/pocketbot/internal/config"
	"github.com/zakandrewking/pocketbot/internal/tmux"
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

func TestCtrlXEntersKillPrefixMode(t *testing.T) {
	m := model{
		config:      config.DefaultConfig(),
		sessions:    map[string]*tmux.Session{},
		bindings:    map[string]commandBinding{},
		windowWidth: 80,
		viewState:   viewHome,
	}

	updatedModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	m, ok := updatedModel.(model)
	if !ok {
		t.Fatal("Update should return a model")
	}
	if cmd != nil {
		t.Fatal("Ctrl+X should not quit")
	}
	if !m.killPrefixMode {
		t.Fatal("Ctrl+X should enable kill prefix mode")
	}
	if m.homeNotice != "" {
		t.Fatalf("expected no notice on kill prefix entry, got %q", m.homeNotice)
	}
	if !contains(m.View(), "Kill session: c=claude, x=codex") {
		t.Fatal("expected kill prompt in instructions after Ctrl+X")
	}
}

func TestKillPrefixEscCancels(t *testing.T) {
	m := model{
		config:         config.DefaultConfig(),
		sessions:       map[string]*tmux.Session{},
		bindings:       map[string]commandBinding{},
		windowWidth:    80,
		viewState:      viewHome,
		killPrefixMode: true,
		homeNotice:     "Kill session: c=claude, x=codex (Esc to cancel)",
	}

	updatedModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m, ok := updatedModel.(model)
	if !ok {
		t.Fatal("Update should return a model")
	}
	if cmd != nil {
		t.Fatal("Esc in kill prefix should not quit")
	}
	if m.killPrefixMode {
		t.Fatal("Esc should cancel kill prefix mode")
	}
	if m.homeNotice != "" {
		t.Fatalf("expected notice to clear, got %q", m.homeNotice)
	}
}

func TestKillPrefixStopsConfiguredTargetWithoutAttach(t *testing.T) {
	m := model{
		config:         config.DefaultConfig(),
		sessions:       map[string]*tmux.Session{},
		bindings:       map[string]commandBinding{},
		windowWidth:    80,
		viewState:      viewHome,
		killPrefixMode: true,
	}

	updatedModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m, ok := updatedModel.(model)
	if !ok {
		t.Fatal("Update should return a model")
	}
	if cmd != nil {
		t.Fatal("Kill-prefix subcommand should not quit")
	}
	if m.killPrefixMode {
		t.Fatal("Kill-prefix mode should be consumed after subcommand")
	}
	if m.shouldAttach {
		t.Fatal("Kill-prefix subcommand should not trigger attach")
	}
	if !contains(m.homeNotice, "not configured") {
		t.Fatalf("expected missing-session notice, got %q", m.homeNotice)
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

func TestDefaultInstructionsHideKillTargetsUntilCtrlX(t *testing.T) {
	m := model{
		config:      config.DefaultConfig(),
		sessions:    map[string]*tmux.Session{},
		bindings:    map[string]commandBinding{},
		windowWidth: 80,
		viewState:   viewHome,
	}

	view := m.View()
	if !contains(view, "Ctrl+X to kill one") {
		t.Fatal("expected base instructions to mention Ctrl+X kill shortcut")
	}
	if contains(view, "c=claude") || contains(view, "x=codex") {
		t.Fatal("did not expect c/x kill targets before Ctrl+X is pressed")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || contains(s[1:], substr)))
}

func TestPressCSetsAttachFlag(t *testing.T) {
	m := initialModel()

	// Verify we start with shouldAttach=false
	if m.shouldAttach {
		t.Error("shouldAttach should be false initially")
	}

	claudeSessCfg := m.config.Claude
	if !claudeSessCfg.Enabled {
		t.Skip("claude session is disabled in config")
	}
	if claudeSessCfg.Key == "" {
		t.Skip("claude key is not configured")
	}

	// Ensure tmux session creation works in this environment
	claudeSess, exists := m.sessions["claude"]
	if !exists {
		t.Fatal("Expected 'claude' session in model")
	}
	if err := claudeSess.Start(); err != nil {
		t.Skipf("tmux sessions cannot be started in this environment: %v", err)
	}
	defer func() {
		for _, sess := range m.sessions {
			sess.Stop()
		}
	}()
	claudeSess.Stop()

	// Press configured key to request attach
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(claudeSessCfg.Key)}
	updatedModel, cmd := m.Update(msg)

	m, ok := updatedModel.(model)
	if !ok {
		t.Fatal("Update should return a model")
	}

	// Verify shouldAttach flag is set
	if !m.shouldAttach {
		t.Error("shouldAttach should be true after pressing 'c'")
	}

	// Verify sessionToAttach is set
	if m.sessionToAttach == "" {
		t.Error("sessionToAttach should be set after pressing configured key")
	}

	// Verify that quit command is returned (to exit Bubble Tea)
	if cmd == nil {
		t.Error("Expected quit command after pressing 'c'")
	}

}

func TestHomeViewShowsSessionStatus(t *testing.T) {
	m := initialModel()

	// View without running session
	view := m.View()
	if !contains(view, "not running") {
		t.Error("Should show 'not running' when session is stopped")
	}
	if contains(view, "● running") {
		t.Error("Should not show '● running' when session is not running")
	}

	// Start claude session (default config has 'claude' session)
	claudeSess, exists := m.sessions["claude"]
	if !exists {
		t.Fatal("Expected 'claude' session in default config")
	}
	if err := claudeSess.Start(); err != nil {
		t.Skipf("tmux sessions cannot be started in this environment: %v", err)
	}
	defer func() {
		for _, sess := range m.sessions {
			sess.Stop()
		}
	}()

	// View with running session
	view = m.View()
	// Should show either "● active" or "● idle" when running
	hasStatus := contains(view, "● active") || contains(view, "● idle")
	if !hasStatus {
		t.Error("Should show '● active' or '● idle' when session is running")
	}
	if !contains(view, "claude:") {
		t.Error("Should show 'claude:' label")
	}
}

func TestDirectoryBindingAllowsAttachInDifferentDirectory(t *testing.T) {
	cwd1 := t.TempDir()
	cwd2 := t.TempDir()
	sessionName := fmt.Sprintf("test-bind-%d", time.Now().UnixNano())
	originalCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	defer os.Chdir(originalCwd)

	cfg := config.DefaultConfig()
	cfg.Claude.Enabled = false
	cfg.Codex.Enabled = false
	cfg.Sessions = []config.SessionConfig{
		{Name: sessionName, Command: "sleep 60", Key: "t"},
	}

	m := model{
		config:    cfg,
		sessions:  map[string]*tmux.Session{sessionName: tmux.NewSession(sessionName, "sleep 60")},
		bindings:  make(map[string]commandBinding),
		viewState: viewHome,
	}

	// Launch from cwd1.
	if err := os.Chdir(cwd1); err != nil {
		t.Fatalf("failed to chdir to cwd1: %v", err)
	}
	updatedModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	m = updatedModel.(model)
	defer m.sessions[sessionName].Stop()

	if cmd == nil || !m.shouldAttach {
		t.Fatal("expected attach request when command starts in initial directory")
	}

	// Reset loop state as main() does between UI iterations.
	m.shouldAttach = false
	m.sessionToAttach = ""

	// Try from cwd2: should still allow attach even when bound elsewhere.
	if err := os.Chdir(cwd2); err != nil {
		t.Fatalf("failed to chdir to cwd2: %v", err)
	}
	updatedModel, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	m = updatedModel.(model)

	if cmd == nil {
		t.Fatal("expected attach quit command when directory differs")
	}
	if !m.shouldAttach {
		t.Fatal("shouldAttach should remain true on directory mismatch")
	}
	if m.homeNotice != "" {
		t.Fatalf("expected no mismatch block notice, got %q", m.homeNotice)
	}
}

func TestDirectoryBindingClearsWhenSessionStops(t *testing.T) {
	launchDir := t.TempDir()
	sessionName := fmt.Sprintf("test-bind-clear-%d", time.Now().UnixNano())
	originalCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	defer os.Chdir(originalCwd)

	cfg := config.DefaultConfig()
	cfg.Claude.Enabled = false
	cfg.Codex.Enabled = false
	cfg.Sessions = []config.SessionConfig{
		{Name: sessionName, Command: "sleep 60", Key: "t"},
	}

	m := model{
		config:    cfg,
		sessions:  map[string]*tmux.Session{sessionName: tmux.NewSession(sessionName, "sleep 60")},
		bindings:  make(map[string]commandBinding),
		viewState: viewHome,
	}
	if err := os.Chdir(launchDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}

	// Start and bind.
	updatedModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	m = updatedModel.(model)
	defer m.sessions[sessionName].Stop()

	m.refreshBindings()
	if _, ok := m.bindings[sessionName]; !ok {
		t.Fatal("expected binding to exist while session is running")
	}

	// Stop session and verify binding is cleared.
	if err := m.sessions[sessionName].Stop(); err != nil {
		t.Fatalf("failed to stop session: %v", err)
	}
	m.refreshBindings()
	if _, ok := m.bindings[sessionName]; ok {
		t.Fatal("expected binding to be cleared after session stops")
	}
}

func TestCreateSessionStoresCommandBindingOption(t *testing.T) {
	sessionName := fmt.Sprintf("test-command-opt-%d", time.Now().UnixNano())
	originalCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(originalCwd)
	defer tmux.KillSession(sessionName)

	if err := tmux.CreateSession(sessionName, "sleep 5"); err != nil {
		t.Skipf("tmux sessions cannot be started in this environment: %v", err)
	}

	commandName := tmux.GetSessionCommand(sessionName)
	if commandName != sessionName {
		t.Fatalf("expected @pb_command to be %q, got %q", sessionName, commandName)
	}
}

package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/zakandrewking/pocketbot/internal/config"
	"github.com/zakandrewking/pocketbot/internal/tmux"
)

func requireTmuxSessionCreation(t *testing.T) {
	t.Helper()
	name := fmt.Sprintf("test-probe-%d", time.Now().UnixNano())
	if err := tmux.CreateSession(name, "sleep 1"); err != nil {
		t.Skipf("tmux sessions cannot be started in this environment: %v", err)
	}
	_ = tmux.KillSession(name)
}

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

func TestNEntersNewMode(t *testing.T) {
	m := model{
		config:      config.DefaultConfig(),
		sessions:    map[string]*tmux.Session{},
		bindings:    map[string]commandBinding{},
		windowWidth: 80,
		viewState:   viewHome,
		mode:        modeHome,
	}

	updatedModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m, ok := updatedModel.(model)
	if !ok {
		t.Fatal("Update should return a model")
	}
	if cmd != nil {
		t.Fatal("n should not quit")
	}
	if m.mode != modeNewTool {
		t.Fatal("n should enter new-tool mode")
	}
	if m.homeNotice != "" {
		t.Fatalf("expected no notice on mode entry, got %q", m.homeNotice)
	}
	if !contains(m.View(), "c new claude") {
		t.Fatal("expected new-tool picker in view")
	}
	if !contains(m.View(), "u new cursor") {
		t.Fatal("expected cursor option in new-tool picker")
	}
}

func TestNModeHidesDisabledTool(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Cursor.Enabled = false
	m := model{
		config:      cfg,
		sessions:    map[string]*tmux.Session{},
		bindings:    map[string]commandBinding{},
		windowWidth: 80,
		viewState:   viewHome,
		mode:        modeHome,
	}

	updatedModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = updatedModel.(model)
	view := m.View()
	if contains(view, "u new cursor") {
		t.Fatalf("expected cursor option hidden when disabled, got: %s", view)
	}
}

func TestDisabledToolHotkeyIgnoredInHome(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Cursor.Enabled = false
	m := model{
		config:      cfg,
		sessions:    map[string]*tmux.Session{},
		bindings:    map[string]commandBinding{},
		windowWidth: 80,
		viewState:   viewHome,
		mode:        modeHome,
	}

	updatedModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
	m = updatedModel.(model)
	if m.shouldAttach {
		t.Fatal("disabled cursor key should not attach")
	}
	if m.homeNotice != "" {
		t.Fatalf("expected no notice for disabled key noop, got %q", m.homeNotice)
	}
}

func TestDisabledToolHotkeyIgnoredInNewMode(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Cursor.Enabled = false
	m := model{
		config:      cfg,
		sessions:    map[string]*tmux.Session{},
		bindings:    map[string]commandBinding{},
		windowWidth: 80,
		viewState:   viewHome,
		mode:        modeNewTool,
	}

	updatedModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
	m = updatedModel.(model)
	if m.shouldAttach {
		t.Fatal("disabled cursor key in new mode should not attach")
	}
	if m.homeNotice != "" {
		t.Fatalf("expected no notice for disabled key noop, got %q", m.homeNotice)
	}
}

func TestRemappedCursorKeyShownInNewMode(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Cursor.Key = "r"
	m := model{
		config:      cfg,
		sessions:    map[string]*tmux.Session{},
		bindings:    map[string]commandBinding{},
		windowWidth: 80,
		viewState:   viewHome,
		mode:        modeNewTool,
	}

	view := m.View()
	if !contains(view, "r new cursor") {
		t.Fatalf("expected remapped cursor key in new mode, got: %s", view)
	}
	if contains(view, "u new cursor") {
		t.Fatalf("did not expect default cursor key in new mode, got: %s", view)
	}
}

func TestRemappedCursorKeyHandledInNewMode(t *testing.T) {
	requireTmuxSessionCreation(t)

	originalCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	defer os.Chdir(originalCwd)

	launchDir := t.TempDir()
	if err := os.Chdir(launchDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}

	sessionName := fmt.Sprintf("cursor-testremap-%d", time.Now().UnixNano())
	if err := tmux.CreateSession(sessionName, "sleep 60"); err != nil {
		t.Skipf("tmux session unavailable in this environment: %v", err)
	}
	defer tmux.KillSession(sessionName)

	cfg := config.DefaultConfig()
	cfg.Cursor.Key = "r"
	m := model{
		config: cfg,
		sessions: map[string]*tmux.Session{
			sessionName: tmux.NewSession(sessionName, "sleep 60"),
		},
		bindings:  map[string]commandBinding{},
		viewState: viewHome,
		mode:      modeNewTool,
	}

	updatedModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	m = updatedModel.(model)
	if !contains(m.homeNotice, "cursor already running in this directory") {
		t.Fatalf("expected remapped key to resolve cursor action, got notice %q", m.homeNotice)
	}

	updatedModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
	m = updatedModel.(model)
	if !contains(m.homeNotice, "Unknown new target") {
		t.Fatalf("expected old key to be unknown in new mode, got %q", m.homeNotice)
	}
}

func TestNewModeDisablesClaudeWhenAlreadyRunningInCurrentDirectory(t *testing.T) {
	requireTmuxSessionCreation(t)

	originalCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	defer os.Chdir(originalCwd)

	launchDir := t.TempDir()
	if err := os.Chdir(launchDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}

	sessionName := fmt.Sprintf("claude-testdisable-%d", time.Now().UnixNano())
	if err := tmux.CreateSession(sessionName, "sleep 60"); err != nil {
		t.Skipf("tmux session unavailable in this environment: %v", err)
	}
	defer tmux.KillSession(sessionName)

	cfg := config.DefaultConfig()
	m := model{
		config: cfg,
		sessions: map[string]*tmux.Session{
			sessionName: tmux.NewSession(sessionName, "sleep 60"),
		},
		bindings:  map[string]commandBinding{},
		viewState: viewHome,
		mode:      modeHome,
	}

	updatedModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = updatedModel.(model)
	if cmd != nil {
		t.Fatal("n should not quit")
	}
	if !contains(m.View(), "claude already running") {
		t.Fatalf("expected disabled claude indicator in new mode, got: %s", m.View())
	}

	updatedModel, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = updatedModel.(model)
	if cmd != nil {
		t.Fatal("c should be disabled in new mode when already running in dir")
	}
	if !contains(m.homeNotice, "claude already running in this directory") {
		t.Fatalf("expected already-running notice, got %q", m.homeNotice)
	}
	if m.shouldAttach {
		t.Fatal("disabled c should not trigger attach")
	}
}

func TestKEntersKillMode(t *testing.T) {
	cfg := config.DefaultConfig()
	m := model{
		config:      cfg,
		sessions:    map[string]*tmux.Session{"codex": tmux.NewSession("codex", cfg.Codex.Command)},
		bindings:    map[string]commandBinding{},
		windowWidth: 80,
		viewState:   viewHome,
		mode:        modeHome,
	}
	if err := m.sessions["codex"].Start(); err != nil {
		t.Skipf("tmux sessions cannot be started in this environment: %v", err)
	}
	defer m.sessions["codex"].Stop()

	updatedModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m, ok := updatedModel.(model)
	if !ok {
		t.Fatal("Update should return a model")
	}
	if cmd != nil {
		t.Fatal("k should not quit")
	}
	if m.mode != modeKillTool {
		t.Fatal("k should enter kill-tool mode")
	}
	if !contains(m.View(), "kill codex") {
		t.Fatal("expected kill-tool picker to include running target")
	}
}

func TestREntersRenameMode(t *testing.T) {
	cfg := config.DefaultConfig()
	m := model{
		config:      cfg,
		sessions:    map[string]*tmux.Session{"codex": tmux.NewSession("codex", cfg.Codex.Command)},
		bindings:    map[string]commandBinding{},
		windowWidth: 80,
		viewState:   viewHome,
		mode:        modeHome,
	}
	if err := m.sessions["codex"].Start(); err != nil {
		t.Skipf("tmux sessions cannot be started in this environment: %v", err)
	}
	defer m.sessions["codex"].Stop()

	updatedModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	m, ok := updatedModel.(model)
	if !ok {
		t.Fatal("Update should return a model")
	}
	if cmd != nil {
		t.Fatal("r should not quit")
	}
	if m.mode != modeRenameTool {
		t.Fatal("r should enter rename-tool mode")
	}
}

func TestEscCancelsPickerMode(t *testing.T) {
	m := model{
		config:      config.DefaultConfig(),
		sessions:    map[string]*tmux.Session{},
		bindings:    map[string]commandBinding{},
		windowWidth: 80,
		viewState:   viewHome,
		mode:        modeNewTool,
	}

	updatedModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m, ok := updatedModel.(model)
	if !ok {
		t.Fatal("Update should return a model")
	}
	if cmd != nil {
		t.Fatal("esc in picker mode should not quit")
	}
	if m.mode != modeHome {
		t.Fatal("esc should return to home mode")
	}
	if m.homeNotice != "" {
		t.Fatalf("expected notice to clear, got %q", m.homeNotice)
	}
}

func TestEscCancelsAttachPickerMode(t *testing.T) {
	m := model{
		config:      config.DefaultConfig(),
		sessions:    map[string]*tmux.Session{},
		bindings:    map[string]commandBinding{},
		windowWidth: 80,
		viewState:   viewHome,
		mode:        modePickAttach,
		pickerTool:  "claude",
		pickerTargets: map[string]string{
			"a": "claude-1",
		},
	}

	updatedModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m, ok := updatedModel.(model)
	if !ok {
		t.Fatal("Update should return a model")
	}
	if cmd != nil {
		t.Fatal("esc in attach picker should not quit")
	}
	if m.mode != modeHome {
		t.Fatal("esc should cancel attach picker and return home")
	}
	if m.shouldAttach {
		t.Fatal("esc cancel should not trigger attach")
	}
}

func TestViewRendersWelcomeMessage(t *testing.T) {
	m := initialModel()
	view := m.View()

	if view == "" {
		t.Fatal("View should not be empty")
	}

	// Check that the view contains expected text
	expectedTexts := []string{"Welcome to PocketBot", "dir:", "kill-all"}
	for _, expected := range expectedTexts {
		if !contains(view, expected) {
			t.Errorf("View should contain %q, got: %s", expected, view)
		}
	}
}

func TestZEntersDirJumpMode(t *testing.T) {
	m := model{
		config:      config.DefaultConfig(),
		sessions:    map[string]*tmux.Session{},
		bindings:    map[string]commandBinding{},
		windowWidth: 80,
		viewState:   viewHome,
		mode:        modeHome,
		hasFasder:   true,
	}

	updatedModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")})
	m, ok := updatedModel.(model)
	if !ok {
		t.Fatal("Update should return a model")
	}
	if cmd != nil {
		t.Fatal("z should not quit")
	}
	if m.mode != modeDirJump {
		t.Fatal("z should enter dir-jump mode")
	}
	if !contains(m.View(), "search:") {
		t.Fatal("dir-jump view should render search line")
	}
}

func TestZLoadsSuggestionsWithoutSearchText(t *testing.T) {
	m := model{
		config:      config.DefaultConfig(),
		sessions:    map[string]*tmux.Session{},
		bindings:    map[string]commandBinding{},
		windowWidth: 80,
		viewState:   viewHome,
		mode:        modeHome,
		hasFasder:   true,
		lookupDirs: func(query string) ([]string, error) {
			if query != "" {
				t.Fatalf("expected empty search text on z open, got %q", query)
			}
			return []string{"/tmp/a", "/tmp/b"}, nil
		},
	}

	updatedModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")})
	m = updatedModel.(model)
	if len(m.dirSuggestions) == 0 {
		t.Fatal("expected initial suggestions to be loaded on z open")
	}
}

func TestZShowsHelpfulNoticeWhenFasderMissing(t *testing.T) {
	m := model{
		config:      config.DefaultConfig(),
		sessions:    map[string]*tmux.Session{},
		bindings:    map[string]commandBinding{},
		windowWidth: 80,
		viewState:   viewHome,
		mode:        modeHome,
		hasFasder:   false,
	}

	updatedModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")})
	m, ok := updatedModel.(model)
	if !ok {
		t.Fatal("Update should return a model")
	}
	if cmd != nil {
		t.Fatal("z without fasder should not quit")
	}
	if m.mode != modeHome {
		t.Fatal("z without fasder should stay on home mode")
	}
	if !contains(m.homeNotice, "fasder not found") {
		t.Fatalf("expected missing-fasder notice, got %q", m.homeNotice)
	}
}

func TestDirJumpEnterChangesDirectory(t *testing.T) {
	var changedTo string
	m := model{
		config:       config.DefaultConfig(),
		sessions:     map[string]*tmux.Session{},
		bindings:     map[string]commandBinding{},
		windowWidth:  80,
		viewState:    viewHome,
		mode:         modeDirJump,
		dirQuery:     "proj",
		dirSelection: 0,
		lookupDirs: func(query string) ([]string, error) {
			if query != "proj" {
				t.Fatalf("expected query proj, got %q", query)
			}
			return []string{"/tmp/project"}, nil
		},
		chdir: func(path string) error {
			changedTo = path
			return nil
		},
	}

	updatedModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m, ok := updatedModel.(model)
	if !ok {
		t.Fatal("Update should return a model")
	}
	if cmd != nil {
		t.Fatal("dir jump enter should not quit")
	}
	if changedTo != "/tmp/project" {
		t.Fatalf("expected chdir to /tmp/project, got %q", changedTo)
	}
	if m.mode != modeHome {
		t.Fatal("dir jump enter should return to home mode")
	}
	if m.homeNotice != "" {
		t.Fatalf("expected no notice after directory change, got %q", m.homeNotice)
	}
}

func TestDirJumpTypingDoesNotSelectSuggestion(t *testing.T) {
	m := model{
		config:         config.DefaultConfig(),
		sessions:       map[string]*tmux.Session{},
		bindings:       map[string]commandBinding{},
		windowWidth:    80,
		viewState:      viewHome,
		mode:           modeDirJump,
		dirQuery:       "pro",
		dirSuggestions: []string{"/tmp/one", "/tmp/two"},
		lookupDirs: func(query string) ([]string, error) {
			if query != "prob" {
				t.Fatalf("expected query to append typed rune, got %q", query)
			}
			return []string{"/tmp/three"}, nil
		},
	}

	updatedModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	m, ok := updatedModel.(model)
	if !ok {
		t.Fatal("Update should return a model")
	}
	if cmd != nil {
		t.Fatal("typing in dir jump should not quit")
	}
	if m.mode != modeDirJump {
		t.Fatal("typing should stay in dir jump mode")
	}
	if m.dirQuery != "prob" {
		t.Fatalf("expected updated search text, got %q", m.dirQuery)
	}
}

func TestReverseStrings(t *testing.T) {
	original := []string{"a", "b", "c"}
	reverseStrings(original)
	if original[0] != "c" || original[1] != "b" || original[2] != "a" {
		t.Fatalf("expected reversed order, got %#v", original)
	}
}

func TestDirJumpArrowSelectChangesDirectory(t *testing.T) {
	var changedTo string
	m := model{
		config:         config.DefaultConfig(),
		sessions:       map[string]*tmux.Session{},
		bindings:       map[string]commandBinding{},
		windowWidth:    80,
		viewState:      viewHome,
		mode:           modeDirJump,
		dirQuery:       "proj",
		dirSuggestions: []string{"/tmp/one", "/tmp/two"},
		dirSelection:   0,
		chdir:          func(path string) error { changedTo = path; return nil },
	}

	updatedModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, ok := updatedModel.(model)
	if !ok {
		t.Fatal("Update should return a model")
	}
	if cmd != nil {
		t.Fatal("dir jump arrow navigation should not quit")
	}
	if m.dirSelection != 1 {
		t.Fatalf("expected selection index 1, got %d", m.dirSelection)
	}

	updatedModel, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m, ok = updatedModel.(model)
	if !ok {
		t.Fatal("Update should return a model")
	}
	if cmd != nil {
		t.Fatal("dir jump enter should not quit")
	}
	if changedTo != "/tmp/two" {
		t.Fatalf("expected chdir to /tmp/two, got %q", changedTo)
	}
	if m.mode != modeHome {
		t.Fatal("arrow+enter select should return to home mode")
	}
}

func TestDefaultInstructionsShowMobileShortcuts(t *testing.T) {
	m := model{
		config:      config.DefaultConfig(),
		sessions:    map[string]*tmux.Session{},
		bindings:    map[string]commandBinding{},
		windowWidth: 80,
		viewState:   viewHome,
		mode:        modeHome,
	}

	view := m.View()
	if !contains(view, "jump-dir") || !contains(view, "new") || !contains(view, "kill-all") {
		t.Fatal("expected base instructions to mention mobile shortcuts")
	}
	if contains(view, "kill    ") {
		t.Fatal("did not expect kill hotkey hint when no sessions are running")
	}
	if contains(view, "Ctrl+X") {
		t.Fatal("did not expect Ctrl+X hints in mobile keymap")
	}
}

func TestKillModeShowsOnlyRunningTargets(t *testing.T) {
	cfg := config.DefaultConfig()
	m := model{
		config:      cfg,
		sessions:    map[string]*tmux.Session{"codex": tmux.NewSession("codex", cfg.Codex.Command)},
		bindings:    map[string]commandBinding{},
		windowWidth: 80,
		viewState:   viewHome,
		mode:        modeKillTool,
	}
	if err := m.sessions["codex"].Start(); err != nil {
		t.Skipf("tmux sessions cannot be started in this environment: %v", err)
	}
	defer m.sessions["codex"].Stop()

	view := m.View()
	if !contains(view, "kill codex") {
		t.Fatal("expected running codex kill option")
	}
	if contains(view, "kill claude") || contains(view, "kill cursor") {
		t.Fatal("did not expect non-running kill options")
	}
}

func TestKillModeShowsSecondKeyHintsForMultipleToolSessions(t *testing.T) {
	cfg := config.DefaultConfig()
	m := model{
		config: cfg,
		sessions: map[string]*tmux.Session{
			"codex":   tmux.NewSession("codex", cfg.Codex.Command),
			"codex-2": tmux.NewSession("codex-2", cfg.Codex.Command),
		},
		bindings:    map[string]commandBinding{},
		windowWidth: 80,
		viewState:   viewHome,
		mode:        modeKillTool,
	}
	if err := m.sessions["codex"].Start(); err != nil {
		t.Skipf("tmux sessions cannot be started in this environment: %v", err)
	}
	if err := m.sessions["codex-2"].Start(); err != nil {
		_ = m.sessions["codex"].Stop()
		t.Skipf("tmux sessions cannot be started in this environment: %v", err)
	}
	defer m.sessions["codex"].Stop()
	defer m.sessions["codex-2"].Stop()

	view := m.View()
	if !contains(view, "(x a) codex repo:") || !contains(view, "(x b) codex-2 repo:") {
		t.Fatalf("expected session names in second-key hints for multiple codex sessions, got: %s", view)
	}
}

func TestKillModeXStillOpensPickerWhenMultipleCodexSessions(t *testing.T) {
	cfg := config.DefaultConfig()
	m := model{
		config: cfg,
		sessions: map[string]*tmux.Session{
			"codex":   tmux.NewSession("codex", cfg.Codex.Command),
			"codex-2": tmux.NewSession("codex-2", cfg.Codex.Command),
		},
		bindings:    map[string]commandBinding{},
		windowWidth: 80,
		viewState:   viewHome,
		mode:        modeKillTool,
	}
	if err := m.sessions["codex"].Start(); err != nil {
		t.Skipf("tmux sessions cannot be started in this environment: %v", err)
	}
	if err := m.sessions["codex-2"].Start(); err != nil {
		_ = m.sessions["codex"].Stop()
		t.Skipf("tmux sessions cannot be started in this environment: %v", err)
	}
	defer m.sessions["codex"].Stop()
	defer m.sessions["codex-2"].Stop()

	updatedModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m, ok := updatedModel.(model)
	if !ok {
		t.Fatal("Update should return a model")
	}
	if cmd != nil {
		t.Fatal("x in kill mode should not quit")
	}
	if m.mode != modePickKill {
		t.Fatalf("expected modePickKill, got %v", m.mode)
	}
	if m.shouldAttach {
		t.Fatal("x in kill mode should never trigger attach")
	}
	if len(m.pickerTargets) != 2 {
		t.Fatalf("expected 2 picker targets, got %d", len(m.pickerTargets))
	}
}

func TestRenameModeXStillOpensPickerWhenMultipleCodexSessions(t *testing.T) {
	cfg := config.DefaultConfig()
	m := model{
		config: cfg,
		sessions: map[string]*tmux.Session{
			"codex":   tmux.NewSession("codex", cfg.Codex.Command),
			"codex-2": tmux.NewSession("codex-2", cfg.Codex.Command),
		},
		bindings:    map[string]commandBinding{},
		windowWidth: 80,
		viewState:   viewHome,
		mode:        modeRenameTool,
	}
	if err := m.sessions["codex"].Start(); err != nil {
		t.Skipf("tmux sessions cannot be started in this environment: %v", err)
	}
	if err := m.sessions["codex-2"].Start(); err != nil {
		_ = m.sessions["codex"].Stop()
		t.Skipf("tmux sessions cannot be started in this environment: %v", err)
	}
	defer m.sessions["codex"].Stop()
	defer m.sessions["codex-2"].Stop()

	updatedModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m, ok := updatedModel.(model)
	if !ok {
		t.Fatal("Update should return a model")
	}
	if cmd != nil {
		t.Fatal("x in rename mode should not quit")
	}
	if m.mode != modePickRename {
		t.Fatalf("expected modePickRename, got %v", m.mode)
	}
	if len(m.pickerTargets) != 2 {
		t.Fatalf("expected 2 picker targets, got %d", len(m.pickerTargets))
	}
}

func TestRenameModeShowsSessionNamesForMultipleToolSessions(t *testing.T) {
	cfg := config.DefaultConfig()
	m := model{
		config: cfg,
		sessions: map[string]*tmux.Session{
			"codex":   tmux.NewSession("codex", cfg.Codex.Command),
			"codex-2": tmux.NewSession("codex-2", cfg.Codex.Command),
		},
		bindings:    map[string]commandBinding{},
		windowWidth: 80,
		viewState:   viewHome,
		mode:        modeRenameTool,
	}
	if err := m.sessions["codex"].Start(); err != nil {
		t.Skipf("tmux sessions cannot be started in this environment: %v", err)
	}
	if err := m.sessions["codex-2"].Start(); err != nil {
		_ = m.sessions["codex"].Stop()
		t.Skipf("tmux sessions cannot be started in this environment: %v", err)
	}
	defer m.sessions["codex"].Stop()
	defer m.sessions["codex-2"].Stop()

	view := m.View()
	if !contains(view, "(x a) codex repo:") || !contains(view, "(x b) codex-2 repo:") {
		t.Fatalf("expected session names in rename mode hints, got: %s", view)
	}
}

func TestApplyRenameTargetRenamesSessionInModel(t *testing.T) {
	cfg := config.DefaultConfig()
	m := model{
		config:       cfg,
		sessions:     map[string]*tmux.Session{"codex": tmux.NewSession("codex", cfg.Codex.Command)},
		sessionTools: map[string]string{"codex": "codex"},
		bindings:     map[string]commandBinding{},
		mode:         modeRenameInput,
		renameTarget: "codex",
		renameInput:  "focus",
	}

	originalRename := renameSessionFn
	originalSetTool := setSessionToolFn
	originalListSessions := listSessionsFn
	defer func() { renameSessionFn = originalRename }()
	defer func() { setSessionToolFn = originalSetTool }()
	defer func() { listSessionsFn = originalListSessions }()
	renameSessionFn = func(oldName, newName string) error { return nil }
	setSessionToolFn = func(sessionName, tool string) error { return nil }
	listCalls := 0
	listSessionsFn = func() []string {
		listCalls++
		if listCalls == 1 {
			return []string{"codex"}
		}
		return []string{"focus"}
	}

	updatedModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedModel.(model)
	if m.mode != modeHome {
		t.Fatalf("expected modeHome after rename, got %v", m.mode)
	}
	if _, ok := m.sessions["focus"]; !ok {
		t.Fatal("expected renamed session key to exist")
	}
	if got := m.sessionTools["focus"]; got != "codex" {
		t.Fatalf("expected renamed session tool mapping to persist, got %q", got)
	}
	if !contains(m.homeNotice, "renamed codex to focus") {
		t.Fatalf("expected rename notice, got %q", m.homeNotice)
	}
}

func TestRunningToolSessionsIncludesRenamedSessionViaStoredToolMapping(t *testing.T) {
	m := model{
		config: config.DefaultConfig(),
		sessions: map[string]*tmux.Session{
			"focus run": tmux.NewSession("focus run", ""),
		},
		sessionTools: map[string]string{
			"focus run": "claude",
		},
		bindings: map[string]commandBinding{
			"focus run": {SessionName: "focus run", Running: true},
		},
	}

	got := m.runningToolSessions("claude")
	if len(got) != 1 || got[0] != "focus run" {
		t.Fatalf("expected renamed claude session to be listed, got %v", got)
	}
}

func TestSyncSessionsWithTmuxKeepsConfiguredAndPrunesStale(t *testing.T) {
	cfg := config.DefaultConfig()
	m := model{
		config: cfg,
		sessions: map[string]*tmux.Session{
			"ghost": tmux.NewSession("ghost", ""),
		},
		sessionTools: map[string]string{
			"ghost": "claude",
		},
	}

	originalList := listSessionsFn
	originalGetTool := getSessionToolFn
	defer func() {
		listSessionsFn = originalList
		getSessionToolFn = originalGetTool
	}()
	listSessionsFn = func() []string { return []string{"focus run"} }
	getSessionToolFn = func(sessionName string) string {
		if sessionName == "focus run" {
			return "codex"
		}
		return ""
	}

	m.syncSessionsWithTmux()
	if _, ok := m.sessions["ghost"]; ok {
		t.Fatal("expected stale non-configured session to be pruned")
	}
	if _, ok := m.sessions["focus run"]; !ok {
		t.Fatal("expected live tmux session to be added")
	}
	if got := m.sessionTools["focus run"]; got != "codex" {
		t.Fatalf("expected tool mapping for live session, got %q", got)
	}
	if _, ok := m.sessions["claude"]; !ok {
		t.Fatal("expected configured base session wrapper to be retained")
	}
}

func TestValidSessionNameAllowsSpaces(t *testing.T) {
	if !validSessionName("my focus run") {
		t.Fatal("expected spaces to be allowed in session names")
	}
}

func TestRenameInputShowsCursorIndicator(t *testing.T) {
	m := model{
		config:       config.DefaultConfig(),
		sessions:     map[string]*tmux.Session{},
		bindings:     map[string]commandBinding{},
		viewState:    viewHome,
		mode:         modeRenameInput,
		renameTarget: "codex",
		renameInput:  "my session",
	}
	view := m.View()
	if !contains(view, "▌") {
		t.Fatalf("expected cursor indicator in rename input, got: %s", view)
	}
}

func TestRenameUpdatesHomeRowWithNewName(t *testing.T) {
	requireTmuxSessionCreation(t)

	sessionName := fmt.Sprintf("codex-rename-%d", time.Now().UnixNano())
	newName := "focus run"
	if err := tmux.CreateSession(sessionName, "sleep 60"); err != nil {
		t.Skipf("tmux session unavailable in this environment: %v", err)
	}
	defer tmux.KillSession(newName)
	defer tmux.KillSession(sessionName)

	cfg := config.DefaultConfig()
	m := model{
		config:   cfg,
		sessions: map[string]*tmux.Session{sessionName: tmux.NewSession(sessionName, cfg.Codex.Command)},
		bindings: map[string]commandBinding{},
		mode:     modeRenameInput,
		viewState: viewHome,
		renameTarget: sessionName,
		renameInput:  newName,
	}

	m = m.applyRenameTarget()
	view := m.View()
	if !contains(view, newName) {
		t.Fatalf("expected renamed session in home view, got: %s", view)
	}
}

func TestHomeAttachKeyOpensPickerWhenMultipleToolSessionsExist(t *testing.T) {
	requireTmuxSessionCreation(t)

	cfg := config.DefaultConfig()
	m := model{
		config: cfg,
		sessions: map[string]*tmux.Session{
			"codex":   tmux.NewSession("codex", cfg.Codex.Command),
			"codex-2": tmux.NewSession("codex-2", cfg.Codex.Command),
		},
		bindings:    map[string]commandBinding{},
		windowWidth: 80,
		viewState:   viewHome,
		mode:        modeHome,
		getwd:       os.Getwd,
	}
	if err := m.sessions["codex"].Start(); err != nil {
		t.Skipf("tmux sessions cannot be started in this environment: %v", err)
	}
	if err := m.sessions["codex-2"].Start(); err != nil {
		_ = m.sessions["codex"].Stop()
		t.Skipf("tmux sessions cannot be started in this environment: %v", err)
	}
	defer m.sessions["codex"].Stop()
	defer m.sessions["codex-2"].Stop()

	updatedModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(cfg.Codex.Key)})
	m, ok := updatedModel.(model)
	if !ok {
		t.Fatal("Update should return a model")
	}
	if cmd != nil {
		t.Fatal("attach key with multiple sessions should not quit immediately")
	}
	if m.mode != modePickAttach {
		t.Fatalf("expected modePickAttach, got %v", m.mode)
	}
	if m.shouldAttach {
		t.Fatal("attach key with multiple sessions should not immediately attach")
	}
	if len(m.pickerTargets) != 2 {
		t.Fatalf("expected 2 picker targets, got %d", len(m.pickerTargets))
	}
}

func TestKDoesNotEnterKillModeWhenNothingRunning(t *testing.T) {
	m := model{
		config:      config.DefaultConfig(),
		sessions:    map[string]*tmux.Session{},
		bindings:    map[string]commandBinding{},
		windowWidth: 80,
		viewState:   viewHome,
		mode:        modeHome,
	}

	updatedModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m, ok := updatedModel.(model)
	if !ok {
		t.Fatal("Update should return a model")
	}
	if cmd != nil {
		t.Fatal("k with nothing running should not quit")
	}
	if m.mode != modeHome {
		t.Fatal("k with nothing running should stay in home mode")
	}
	if !contains(m.homeNotice, "no running sessions") {
		t.Fatalf("expected no-running notice, got %q", m.homeNotice)
	}
}

func TestYoloCommandForTool(t *testing.T) {
	tests := []struct {
		name    string
		tool    string
		command string
		want    string
	}{
		{
			name:    "claude default command",
			tool:    "claude",
			command: "claude --continue --permission-mode acceptEdits",
			want:    "claude --continue --dangerously-skip-permissions",
		},
		{
			name:    "claude custom command without permission-mode",
			tool:    "claude",
			command: "claude --continue",
			want:    "claude --continue --dangerously-skip-permissions",
		},
		{
			name:    "codex default command",
			tool:    "codex",
			command: "codex resume --last",
			want:    "codex --yolo resume --last",
		},
		{
			name:    "codex custom command",
			tool:    "codex",
			command: "codex --model o4-mini",
			want:    "codex --yolo --model o4-mini",
		},
		{
			name:    "cursor unchanged (no yolo flag)",
			tool:    "cursor",
			command: "agent resume",
			want:    "agent resume",
		},
		{
			name:    "unknown tool unchanged",
			tool:    "other",
			command: "sometool --flag",
			want:    "sometool --flag",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := yoloCommandForTool(tt.tool, tt.command)
			if got != tt.want {
				t.Fatalf("yoloCommandForTool(%q, %q) = %q, want %q", tt.tool, tt.command, got, tt.want)
			}
		})
	}
}

func TestYKeyTogglesYoloInNewMode(t *testing.T) {
	m := model{
		config:      config.DefaultConfig(),
		sessions:    map[string]*tmux.Session{},
		bindings:    map[string]commandBinding{},
		windowWidth: 80,
		viewState:   viewHome,
		mode:        modeNewTool,
	}

	// Toggle on
	updatedModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = updatedModel.(model)
	if cmd != nil {
		t.Fatal("y should not quit")
	}
	if m.mode != modeNewTool {
		t.Fatal("y should stay in new-tool mode")
	}
	if !m.newToolYolo {
		t.Fatal("y should enable yolo mode")
	}
	if !contains(m.View(), "yolo: ON") {
		t.Fatalf("expected yolo ON indicator in view, got: %s", m.View())
	}

	// Toggle off
	updatedModel, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = updatedModel.(model)
	if cmd != nil {
		t.Fatal("y should not quit")
	}
	if m.newToolYolo {
		t.Fatal("second y press should disable yolo mode")
	}
	if contains(m.View(), "yolo: ON") {
		t.Fatalf("expected yolo off after toggle, got: %s", m.View())
	}
}

func TestDetailedRowsShowsPerSessionYoloBadge(t *testing.T) {
	cfg := config.DefaultConfig()
	m := model{
		config: cfg,
		bindings: map[string]commandBinding{
			"codex": {SessionName: "codex", Cwd: "/repo", Running: true, Yolo: true},
		},
		sessions: map[string]*tmux.Session{},
	}

	rows := m.detailedRows("codex", []string{"codex"})
	if len(rows) != 1 {
		t.Fatalf("expected one row, got %d", len(rows))
	}
	if !contains(rows[0], "(yolo)") {
		t.Fatalf("expected per-session yolo badge, got: %s", rows[0])
	}
}

func TestEscResetsYoloInNewMode(t *testing.T) {
	m := model{
		config:      config.DefaultConfig(),
		sessions:    map[string]*tmux.Session{},
		bindings:    map[string]commandBinding{},
		windowWidth: 80,
		viewState:   viewHome,
		mode:        modeNewTool,
		newToolYolo: true,
	}

	updatedModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updatedModel.(model)
	if m.newToolYolo {
		t.Fatal("esc should reset yolo flag")
	}
	if m.mode != modeHome {
		t.Fatal("esc should return to home mode")
	}
}

func TestDResetsYoloInNewMode(t *testing.T) {
	m := model{
		config:      config.DefaultConfig(),
		sessions:    map[string]*tmux.Session{},
		bindings:    map[string]commandBinding{},
		windowWidth: 80,
		viewState:   viewHome,
		mode:        modeNewTool,
		newToolYolo: true,
	}

	updatedModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = updatedModel.(model)
	if m.newToolYolo {
		t.Fatal("d should reset yolo flag")
	}
	if m.mode != modeHome {
		t.Fatal("d should return to home mode")
	}
}

func TestFallbackCommand(t *testing.T) {
	tests := []struct {
		name    string
		tool    string
		command string
		want    string
	}{
		{
			name:    "claude resume fallback",
			tool:    "claude",
			command: "claude --continue --permission-mode acceptEdits",
			want:    "claude --continue --permission-mode acceptEdits || claude --permission-mode acceptEdits",
		},
		{
			name:    "codex resume fallback",
			tool:    "codex",
			command: "codex resume --last",
			want:    "codex resume --last || codex",
		},
		{
			name:    "cursor resume fallback",
			tool:    "cursor",
			command: "agent resume",
			want:    "agent resume || agent",
		},
		{
			name:    "custom command unchanged",
			tool:    "codex",
			command: "codex --model gpt-5",
			want:    "codex --model gpt-5",
		},
		{
			name:    "claude yolo fallback",
			tool:    "claude",
			command: "claude --continue --dangerously-skip-permissions",
			want:    "claude --continue --dangerously-skip-permissions || claude --dangerously-skip-permissions",
		},
		{
			name:    "codex yolo fallback",
			tool:    "codex",
			command: "codex --yolo resume --last",
			want:    "codex --yolo resume --last || codex --yolo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fallbackCommand(tt.tool, tt.command)
			if got != tt.want {
				t.Fatalf("fallbackCommand(%q, %q) = %q, want %q", tt.tool, tt.command, got, tt.want)
			}
		})
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
	if !contains(view, "claude") || !contains(view, "codex") || !contains(view, "not running") {
		t.Error("Should show claude/codex not-running rows when no sessions are active")
	}
	if !contains(view, "cursor") {
		t.Error("Should show cursor not-running row when no sessions are active")
	}
	if !contains(view, "dir:") {
		t.Error("Should include current directory")
	}
	if strings.Count(view, "\n\n") < 2 {
		t.Error("Should include blank lines between dir/agents and agents/hotkeys")
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
	if !contains(view, "claude") {
		t.Error("Should show claude row when session is running")
	}
	for _, line := range strings.Split(view, "\n") {
		if contains(line, "claude") && contains(line, "not running") {
			t.Error("Should not show claude as not running when session is active")
			break
		}
	}
}

func TestDetailedRowsShowsTaskCountWhenPresent(t *testing.T) {
	cfg := config.DefaultConfig()
	m := model{
		config:       cfg,
		sessions:     map[string]*tmux.Session{},
		bindings:     map[string]commandBinding{},
		taskCounts:   map[string]int{"claude": 2},
		taskCommands: map[string][]string{"claude": {"sleep 300"}},
	}

	rows := m.detailedRows("claude", []string{"claude"})
	if len(rows) == 0 {
		t.Fatal("expected detailed row")
	}
	if !contains(rows[0], "tasks:2") {
		t.Fatalf("expected tasks count in row, got: %s", rows[0])
	}
}

func TestSummaryRowShowsTaskTotalWhenPresent(t *testing.T) {
	m := model{
		taskCounts: map[string]int{
			"claude":   2,
			"claude-2": 1,
		},
		sessions: map[string]*tmux.Session{},
	}

	row := m.summaryRow("claude", []string{"claude", "claude-2"})
	if !contains(row, "tasks:3") {
		t.Fatalf("expected summary row to include task total, got: %s", row)
	}
}

func TestDetailedRowsShowsTaskLinesWhenEnabled(t *testing.T) {
	cfg := config.DefaultConfig()
	m := model{
		config:          cfg,
		sessions:        map[string]*tmux.Session{},
		bindings:        map[string]commandBinding{},
		taskCounts:      map[string]int{"claude": 1},
		taskCommands:    map[string][]string{"claude": {"sleep 300"}},
		showTaskDetails: true,
	}

	rows := m.detailedRows("claude", []string{"claude"})
	if len(rows) < 2 {
		t.Fatalf("expected task detail line, got rows: %#v", rows)
	}
	if !contains(rows[1], "task: sleep 300") {
		t.Fatalf("expected task detail content, got: %s", rows[1])
	}
}

func TestDetailedRowsHidesTaskCountWhenTaskLinesEnabled(t *testing.T) {
	cfg := config.DefaultConfig()
	m := model{
		config:          cfg,
		sessions:        map[string]*tmux.Session{},
		bindings:        map[string]commandBinding{},
		taskCounts:      map[string]int{"claude": 2},
		taskCommands:    map[string][]string{"claude": {"sleep 300"}},
		showTaskDetails: true,
	}

	rows := m.detailedRows("claude", []string{"claude"})
	if len(rows) == 0 {
		t.Fatal("expected detailed row")
	}
	if contains(rows[0], "tasks:2") {
		t.Fatalf("did not expect task count on parent row when task lines shown, got: %s", rows[0])
	}
}

func TestTTogglesTaskLinesInHomeMode(t *testing.T) {
	m := model{
		config:      config.DefaultConfig(),
		sessions:    map[string]*tmux.Session{},
		bindings:    map[string]commandBinding{},
		windowWidth: 80,
		viewState:   viewHome,
		mode:        modeHome,
	}

	updatedModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	m, ok := updatedModel.(model)
	if !ok {
		t.Fatal("Update should return a model")
	}
	if cmd != nil {
		t.Fatal("t should not quit")
	}
	if !m.showTaskDetails {
		t.Fatal("t should enable task details")
	}
}

func TestModePickKillTaskKillsSelectedPID(t *testing.T) {
	m := model{
		config: config.DefaultConfig(),
		mode:   modePickKillTask,
		taskKillTargets: map[string]taskKillTarget{
			"a": {Session: "claude", PID: 4242, Command: "sleep 300"},
		},
	}

	originalKill := killTaskPIDFn
	defer func() { killTaskPIDFn = originalKill }()
	killed := 0
	killTaskPIDFn = func(pid int) error {
		killed = pid
		return nil
	}

	updatedModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m, ok := updatedModel.(model)
	if !ok {
		t.Fatal("Update should return a model")
	}
	if cmd != nil {
		t.Fatal("task kill selection should not quit")
	}
	if killed != 4242 {
		t.Fatalf("expected pid 4242 to be killed, got %d", killed)
	}
	if m.mode != modeHome {
		t.Fatalf("expected modeHome after killing task, got %v", m.mode)
	}
	if !contains(m.homeNotice, "killed pid 4242") {
		t.Fatalf("expected killed notice, got %q", m.homeNotice)
	}
}

func TestModePickKillTaskShowsErrorOnKillFailure(t *testing.T) {
	m := model{
		config: config.DefaultConfig(),
		mode:   modePickKillTask,
		taskKillTargets: map[string]taskKillTarget{
			"a": {Session: "claude", PID: 4242, Command: "sleep 300"},
		},
	}

	originalKill := killTaskPIDFn
	defer func() { killTaskPIDFn = originalKill }()
	killTaskPIDFn = func(pid int) error {
		return errors.New("denied")
	}

	updatedModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = updatedModel.(model)
	if !contains(m.homeNotice, "failed to kill pid 4242") {
		t.Fatalf("expected kill failure notice, got %q", m.homeNotice)
	}
}

func TestCreateAndAttachToolReusesSessionInCurrentDirectory(t *testing.T) {
	cfg := config.DefaultConfig()
	m := model{
		config:    cfg,
		sessions:  map[string]*tmux.Session{"claude": tmux.NewSession("claude", cfg.Claude.Command)},
		bindings:  map[string]commandBinding{"claude": {SessionName: "claude", Cwd: "/repo", Running: true}},
		viewState: viewHome,
		mode:      modeHome,
		getwd: func() (string, error) {
			return "/repo", nil
		},
	}

	updatedModel, cmd := m.createAndAttachTool("claude")
	if cmd == nil {
		t.Fatal("expected quit command for attach request")
	}
	if !updatedModel.shouldAttach {
		t.Fatal("expected shouldAttach=true")
	}
	if updatedModel.sessionToAttach != "claude" {
		t.Fatalf("expected attach target claude, got %q", updatedModel.sessionToAttach)
	}
}

func TestCreateAndAttachToolShowsPickerWhenMultipleSessionsInCurrentDirectory(t *testing.T) {
	cfg := config.DefaultConfig()
	m := model{
		config: cfg,
		sessions: map[string]*tmux.Session{
			"claude":   tmux.NewSession("claude", cfg.Claude.Command),
			"claude-2": tmux.NewSession("claude-2", cfg.Claude.Command),
		},
		bindings: map[string]commandBinding{
			"claude":   {SessionName: "claude", Cwd: "/repo", Running: true},
			"claude-2": {SessionName: "claude-2", Cwd: "/repo", Running: true},
		},
		viewState: viewHome,
		mode:      modeHome,
		getwd: func() (string, error) {
			return "/repo", nil
		},
	}

	updatedModel, cmd := m.createAndAttachTool("claude")
	if cmd != nil {
		t.Fatal("did not expect immediate quit when picker is shown")
	}
	if updatedModel.mode != modePickAttach {
		t.Fatalf("expected modePickAttach, got %v", updatedModel.mode)
	}
	if len(updatedModel.pickerTargets) != 2 {
		t.Fatalf("expected 2 picker targets, got %d", len(updatedModel.pickerTargets))
	}
}

func TestDirectoryBindingAllowsAttachInDifferentDirectory(t *testing.T) {
	requireTmuxSessionCreation(t)

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
	requireTmuxSessionCreation(t)

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

func TestPrintToolTasksFallsBackToRootSocketWhenNested(t *testing.T) {
	originalLevel := os.Getenv("PB_LEVEL")
	_ = os.Setenv("PB_LEVEL", "1")
	defer func() {
		if originalLevel == "" {
			_ = os.Unsetenv("PB_LEVEL")
			return
		}
		_ = os.Setenv("PB_LEVEL", originalLevel)
	}()

	originalListSessions := listSessionsFn
	originalSessionTasks := sessionUserTasksFn
	defer func() {
		listSessionsFn = originalListSessions
		sessionUserTasksFn = originalSessionTasks
	}()

	listSessionsFn = func() []string {
		if os.Getenv("PB_LEVEL") == "1" {
			return nil
		}
		return []string{"claude"}
	}
	sessionUserTasksFn = func(sessionName string) ([]tmux.Task, error) {
		if sessionName != "claude" {
			t.Fatalf("unexpected session: %s", sessionName)
		}
		return []tmux.Task{{PID: 42, PPID: 1, State: "S", Command: "echo hi"}}, nil
	}

	var buf bytes.Buffer
	if !printToolTasksForSocket(&buf) {
		// nested socket should have no sessions in this test setup
	} else {
		t.Fatal("expected nested socket pass to find no tool sessions")
	}

	// Simulate root fallback pass.
	_ = os.Unsetenv("PB_LEVEL")
	defer os.Setenv("PB_LEVEL", "1")
	found := printToolTasksForSocket(&buf)
	if !found {
		t.Fatal("expected fallback socket to find claude session")
	}
	if !contains(buf.String(), "claude: 1 task process(es)") {
		t.Fatalf("expected claude task line, got: %s", buf.String())
	}
}

func TestPrintToolTasksCapsPerAgentOutput(t *testing.T) {
	originalListSessions := listSessionsFn
	originalSessionTasks := sessionUserTasksFn
	defer func() {
		listSessionsFn = originalListSessions
		sessionUserTasksFn = originalSessionTasks
	}()

	listSessionsFn = func() []string { return []string{"codex"} }
	sessionUserTasksFn = func(sessionName string) ([]tmux.Task, error) {
		if sessionName != "codex" {
			t.Fatalf("unexpected session: %s", sessionName)
		}
		var tasks []tmux.Task
		for i := 0; i < 8; i++ {
			tasks = append(tasks, tmux.Task{
				PID:     1000 + i,
				PPID:    1,
				State:   "S",
				Command: fmt.Sprintf("sleep %d", i),
			})
		}
		return tasks, nil
	}

	var buf bytes.Buffer
	if !printToolTasksForSocket(&buf) {
		t.Fatal("expected tasks to be found")
	}
	out := buf.String()
	if !contains(out, "codex: 8 task process(es)") {
		t.Fatalf("expected total count header, got: %s", out)
	}
	if !contains(out, "+2 more") {
		t.Fatalf("expected overflow marker, got: %s", out)
	}
	if contains(out, "pid=1007") {
		t.Fatalf("expected pid=1007 to be hidden by cap, got: %s", out)
	}
}

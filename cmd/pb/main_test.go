package main

import (
	"bytes"
	"fmt"
	"os"
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
	if !contains(view, "instances: 0") {
		t.Error("Should show zero instances when no sessions are running")
	}
	if !contains(view, "claude") || !contains(view, "codex") || !contains(view, "not running") {
		t.Error("Should show claude/codex not-running rows when no sessions are active")
	}
	if !contains(view, "cursor") {
		t.Error("Should show cursor not-running row when no sessions are active")
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
	// Should show either "● active" or "○ idle" when running
	hasStatus := contains(view, "● active") || contains(view, "○ idle")
	if !hasStatus {
		t.Error("Should show '● active' or '○ idle' when session is running")
	}
	if !contains(view, "claude") {
		t.Error("Should show claude row when session is running")
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

func TestCtrlTTogglesTaskLinesInHomeMode(t *testing.T) {
	m := model{
		config:      config.DefaultConfig(),
		sessions:    map[string]*tmux.Session{},
		bindings:    map[string]commandBinding{},
		windowWidth: 80,
		viewState:   viewHome,
		mode:        modeHome,
	}

	updatedModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	m, ok := updatedModel.(model)
	if !ok {
		t.Fatal("Update should return a model")
	}
	if cmd != nil {
		t.Fatal("ctrl+t should not quit")
	}
	if !m.showTaskDetails {
		t.Fatal("ctrl+t should enable task details")
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

func TestPrintClaudeTasksFallsBackToRootSocketWhenNested(t *testing.T) {
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
	if !printClaudeTasksForSocket(&buf) {
		// nested socket should have no sessions in this test setup
	} else {
		t.Fatal("expected nested socket pass to find no claude sessions")
	}

	// Simulate root fallback pass.
	_ = os.Unsetenv("PB_LEVEL")
	defer os.Setenv("PB_LEVEL", "1")
	found := printClaudeTasksForSocket(&buf)
	if !found {
		t.Fatal("expected fallback socket to find claude session")
	}
	if !contains(buf.String(), "claude: 1 task process(es)") {
		t.Fatalf("expected claude task line, got: %s", buf.String())
	}
}

package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/zakandrewking/pocketbot/internal/config"
	"github.com/zakandrewking/pocketbot/internal/tmux"
)

var (
	listSessionsFn     = tmux.ListSessions
	sessionUserTasksFn = tmux.SessionUserTasks
	renameSessionFn    = tmux.RenameSession
	getSessionToolFn   = tmux.GetSessionTool
	setSessionToolFn   = tmux.SetSessionTool
	killTaskPIDFn      = func(pid int) error {
		return syscall.Kill(pid, syscall.SIGTERM)
	}
)

const maxTasksShownPerAgent = 6

type viewState int

const (
	viewHome viewState = iota
	viewAttached
)

type uiMode int

const (
	modeHome uiMode = iota
	modeNewTool
	modeKillTool
	modeRenameTool
	modePickAttach
	modePickKill
	modePickRename
	modePickKillTask
	modeRenameInput
	modeDirJump
)

type tickMsg time.Time

func tickCmd() tea.Msg {
	time.Sleep(1 * time.Second)
	return tickMsg(time.Now())
}

type commandBinding struct {
	SessionName string
	Cwd         string
	Running     bool
	Yolo        bool
	Tool        string
	LastSeen    time.Time
}

type taskKillTarget struct {
	Session string
	PID     int
	Command string
}

type model struct {
	config          *config.Config
	sessions        map[string]*tmux.Session
	sessionTools    map[string]string
	bindings        map[string]commandBinding
	taskCounts      map[string]int
	taskCommands    map[string][]string
	taskRefreshAt   time.Time
	showTaskDetails bool
	taskKillTargets map[string]taskKillTarget
	windowWidth     int
	viewState       viewState
	mode            uiMode
	pickerTool      string
	pickerTargets   map[string]string
	renameTarget    string
	renameInput     string
	shouldAttach    bool
	sessionToAttach string // Name of session to attach to
	homeNotice      string
	newToolYolo     bool
	dirQuery        string
	dirSuggestions  []string
	dirSelection    int
	hasFasder       bool
	getwd           func() (string, error)
	chdir           func(string) error
	lookupDirs      func(string) ([]string, error)
}

func initialModel() model {
	// Check for tmux
	if !tmux.Available() {
		fmt.Fprintf(os.Stderr, "Error: tmux is required but not found in PATH\n")
		fmt.Fprintf(os.Stderr, "Install with: brew install tmux\n")
		os.Exit(1)
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		fmt.Fprintf(os.Stderr, "Using default configuration\n")
		cfg = config.DefaultConfig()
	}

	// Create tmux sessions for each configured session
	sessions := make(map[string]*tmux.Session)
	for _, sess := range cfg.AllSessions() {
		sessions[sess.Name] = tmux.NewSession(sess.Name, sess.Command)
	}
	for _, running := range tmux.ListSessions() {
		if _, exists := sessions[running]; !exists {
			sessions[running] = tmux.NewSession(running, "")
		}
	}

	return model{
		config:          cfg,
		sessions:        sessions,
		sessionTools:    make(map[string]string),
		bindings:        make(map[string]commandBinding),
		taskCounts:      make(map[string]int),
		taskCommands:    make(map[string][]string),
		taskKillTargets: make(map[string]taskKillTarget),
		windowWidth:     80,
		viewState:       viewHome,
		mode:            modeHome,
		pickerTargets:   make(map[string]string),
		getwd:           os.Getwd,
		chdir:           os.Chdir,
		lookupDirs:      lookupDirectoriesWithFasder,
		hasFasder:       fasderAvailable(),
	}
}

func normalizeToolName(tool string) string {
	switch tool {
	case "claude", "codex", "cursor":
		return tool
	default:
		return ""
	}
}

func (m *model) rememberSessionTool(name, tool string) {
	tool = normalizeToolName(tool)
	if tool == "" {
		return
	}
	if m.sessionTools == nil {
		m.sessionTools = make(map[string]string)
	}
	m.sessionTools[name] = tool
}

func (m *model) configuredSessionNameSet() map[string]bool {
	names := make(map[string]bool)
	if m.config == nil {
		return names
	}
	for _, sess := range m.config.AllSessions() {
		names[sess.Name] = true
	}
	return names
}

func (m *model) syncSessionsWithTmux() {
	if m.sessions == nil {
		m.sessions = make(map[string]*tmux.Session)
	}
	if m.sessionTools == nil {
		m.sessionTools = make(map[string]string)
	}

	configured := m.configuredSessionNameSet()
	if m.config != nil {
		for _, sess := range m.config.AllSessions() {
			if _, exists := m.sessions[sess.Name]; !exists {
				m.sessions[sess.Name] = tmux.NewSession(sess.Name, sess.Command)
			}
			if inferred := toolFromSessionName(sess.Name); inferred != "" {
				m.rememberSessionTool(sess.Name, inferred)
			}
		}
	}
	live := make(map[string]bool)
	for _, name := range listSessionsFn() {
		if strings.TrimSpace(name) == "" {
			continue
		}
		live[name] = true
		if _, exists := m.sessions[name]; !exists {
			command := ""
			tool := m.sessionTool(name)
			if tool != "" {
				command = m.commandForTool(tool)
			}
			m.sessions[name] = tmux.NewSession(name, command)
		}
		if tool := normalizeToolName(getSessionToolFn(name)); tool != "" {
			m.sessionTools[name] = tool
			continue
		}
		if _, ok := m.sessionTools[name]; ok {
			continue
		}
		if inferred := toolFromSessionName(name); inferred != "" {
			m.sessionTools[name] = inferred
		}
	}

	for name := range m.sessions {
		if live[name] || configured[name] {
			continue
		}
		delete(m.sessions, name)
		delete(m.sessionTools, name)
	}
}

func (m *model) currentDir() string {
	if m.getwd == nil {
		cwd, _ := os.Getwd()
		return cwd
	}
	cwd, err := m.getwd()
	if err != nil {
		return ""
	}
	return cwd
}

func (m *model) refreshBindings() {
	m.syncSessionsWithTmux()
	if m.bindings == nil {
		m.bindings = make(map[string]commandBinding)
	}

	live := make(map[string]bool)
	for name, tmuxSess := range m.sessions {
		if tmuxSess == nil || !tmuxSess.IsRunning() {
			continue
		}

		m.bindings[name] = commandBinding{
			SessionName: name,
			Cwd:         tmux.GetSessionCwd(name),
			Running:     true,
			Yolo:        tmux.GetSessionYolo(name),
			Tool:        m.sessionTool(name),
			LastSeen:    time.Now(),
		}
		live[name] = true
	}

	for sessionName := range m.bindings {
		if !live[sessionName] {
			delete(m.bindings, sessionName)
		}
	}
}

func (m model) sessionTool(name string) string {
	if tool := normalizeToolName(m.sessionTools[name]); tool != "" {
		return tool
	}
	if tool := normalizeToolName(getSessionToolFn(name)); tool != "" {
		return tool
	}
	return toolFromSessionName(name)
}

func checkDirectoryMismatch() {
	cwd, err := os.Getwd()
	if err != nil {
		return
	}

	existingSessions := tmux.ListSessions()
	if len(existingSessions) == 0 {
		return
	}

	var mismatches []string
	for _, name := range existingSessions {
		sessionCwd := tmux.GetSessionCwd(name)
		if sessionCwd != "" && sessionCwd != cwd {
			mismatches = append(mismatches, fmt.Sprintf("  - %s (from %s)", name, sessionCwd))
		}
	}

	if len(mismatches) > 0 {
		fmt.Fprintf(os.Stderr, "\n⚠️  Warning: Sessions running from different directory:\n")
		for _, m := range mismatches {
			fmt.Fprintf(os.Stderr, "%s\n", m)
		}
		fmt.Fprintf(os.Stderr, "\nCurrent directory: %s\n", cwd)
		fmt.Fprintf(os.Stderr, "Use 'pb kill-all' to stop existing sessions, or Ctrl+C to exit.\n\n")
	}
}

func toolFromSessionName(name string) string {
	switch {
	case name == "claude" || strings.HasPrefix(name, "claude-"):
		return "claude"
	case name == "codex" || strings.HasPrefix(name, "codex-"):
		return "codex"
	case name == "cursor" || strings.HasPrefix(name, "cursor-"):
		return "cursor"
	default:
		return ""
	}
}

func alphaKey(i int) string {
	if i < 0 || i >= 26 {
		return ""
	}
	return string(rune('a' + i))
}

func pickerKey(i int) string {
	chars := "abcdefghijklmnopqrstuvwxyz"
	if i < 0 || i >= len(chars) {
		return ""
	}
	return string(chars[i])
}

func (m model) runningToolSessions(tool string) []string {
	var out []string
	for name, binding := range m.bindings {
		bindingTool := binding.Tool
		if bindingTool == "" {
			bindingTool = m.sessionTool(name)
		}
		if bindingTool != tool || !binding.Running {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func (m model) toolSessionsInDir(tool, cwd string) []string {
	var out []string
	for name, binding := range m.bindings {
		bindingTool := binding.Tool
		if bindingTool == "" {
			bindingTool = m.sessionTool(name)
		}
		if bindingTool != tool {
			continue
		}
		if !binding.Running || binding.Cwd != cwd {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func (m model) toolAlreadyRunningInDir(tool, cwd string) bool {
	return len(m.toolSessionsInDir(tool, cwd)) > 0
}

func (m model) commandForTool(tool string) string {
	switch tool {
	case "claude":
		return m.config.Claude.Command
	case "codex":
		return m.config.Codex.Command
	case "cursor":
		return m.config.Cursor.Command
	default:
		return ""
	}
}

func (m model) keyForTool(tool string) string {
	switch tool {
	case "claude":
		return m.config.Claude.Key
	case "codex":
		return m.config.Codex.Key
	case "cursor":
		return m.config.Cursor.Key
	default:
		return ""
	}
}

func (m model) toolEnabled(tool string) bool {
	switch tool {
	case "claude":
		return m.config.Claude.Enabled
	case "codex":
		return m.config.Codex.Enabled
	case "cursor":
		return m.config.Cursor.Enabled
	default:
		return false
	}
}

func (m model) toolForKey(key string) string {
	for _, tool := range []string{"claude", "codex", "cursor"} {
		if !m.toolEnabled(tool) {
			continue
		}
		if m.keyForTool(tool) == key {
			return tool
		}
	}
	return ""
}

func (m model) disabledToolKey(key string) bool {
	for _, tool := range []string{"claude", "codex", "cursor"} {
		if m.toolEnabled(tool) {
			continue
		}
		if m.keyForTool(tool) == key {
			return true
		}
	}
	return false
}

func (m model) nextSessionName(tool string) string {
	names := m.runningToolSessions(tool)
	used := make(map[string]bool)
	for _, n := range names {
		used[n] = true
	}
	if !used[tool] {
		return tool
	}
	max := 1
	for name := range used {
		if strings.HasPrefix(name, tool+"-") {
			var n int
			if _, err := fmt.Sscanf(name, tool+"-%d", &n); err == nil && n > max {
				max = n
			}
		}
	}
	return fmt.Sprintf("%s-%d", tool, max+1)
}

func repoFromCwd(cwd string) string {
	if cwd == "" {
		return "-"
	}
	return filepath.Base(cwd)
}

func lookupDirectoryWithFasder(query string) (string, error) {
	args := []string{"-d"}
	if strings.TrimSpace(query) != "" {
		args = append(args, query)
	}
	out, err := exec.Command("fasder", args...).Output()
	if err != nil {
		return "", err
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return "", fmt.Errorf("no matching directory")
	}
	lines := strings.Split(trimmed, "\n")
	return strings.TrimSpace(lines[0]), nil
}

func lookupDirectoriesWithFasder(query string) ([]string, error) {
	args := []string{"-d", "-l"}
	if strings.TrimSpace(query) != "" {
		args = append(args, query)
	}
	out, err := exec.Command("fasder", args...).Output()
	if err != nil {
		// Fallback to single-result lookup on older fasder variants.
		one, oneErr := lookupDirectoryWithFasder(query)
		if oneErr != nil {
			return nil, err
		}
		return []string{one}, nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var dirs []string
	for _, line := range lines {
		p := strings.TrimSpace(line)
		if p == "" {
			continue
		}
		dirs = append(dirs, p)
	}
	if len(dirs) == 0 {
		return nil, fmt.Errorf("no matching directories")
	}
	// fasder list output is oldest/least-relevant first in practice; invert for top-first UX.
	reverseStrings(dirs)
	return dirs, nil
}

func reverseStrings(items []string) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}

func fasderAvailable() bool {
	_, err := exec.LookPath("fasder")
	return err == nil
}

func (m *model) refreshDirSuggestions() {
	lookup := m.lookupDirs
	if lookup == nil {
		lookup = lookupDirectoriesWithFasder
	}
	suggestions, err := lookup(m.dirQuery)
	if err != nil {
		m.dirSuggestions = nil
		return
	}
	if len(suggestions) > 9 {
		suggestions = suggestions[:9]
	}
	m.dirSuggestions = suggestions
	if len(m.dirSuggestions) == 0 {
		m.dirSelection = 0
	} else if m.dirSelection >= len(m.dirSuggestions) {
		m.dirSelection = len(m.dirSuggestions) - 1
	}
}

func (m *model) applyDirChange(target string) (model, tea.Cmd) {
	chdir := m.chdir
	if chdir == nil {
		chdir = os.Chdir
	}
	if err := chdir(target); err != nil {
		m.homeNotice = fmt.Sprintf("cd failed: %v", err)
		return *m, nil
	}
	m.mode = modeHome
	m.homeNotice = ""
	m.dirQuery = ""
	m.dirSuggestions = nil
	m.dirSelection = 0
	return *m, nil
}

func (m model) mismatchCountForCurrentDir() int {
	cwd := m.currentDir()
	if cwd == "" {
		return 0
	}
	count := 0
	for _, name := range tmux.ListSessions() {
		sessionCwd := tmux.GetSessionCwd(name)
		if sessionCwd != "" && sessionCwd != cwd {
			count++
		}
	}
	return count
}

func fallbackCommand(tool, command string) string {
	switch tool {
	case "claude":
		if command == "claude --continue --permission-mode acceptEdits" {
			return "claude --continue --permission-mode acceptEdits || claude --permission-mode acceptEdits"
		}
		if command == "claude --continue --dangerously-skip-permissions" {
			return "claude --continue --dangerously-skip-permissions || claude --dangerously-skip-permissions"
		}
	case "codex":
		if command == "codex resume --last" {
			return "codex resume --last || codex"
		}
		if command == "codex --yolo resume --last" {
			return "codex --yolo resume --last || codex --yolo"
		}
	case "cursor":
		if command == "agent resume" {
			return "agent resume || agent"
		}
	}
	return command
}

// yoloCommandForTool returns the command modified to run in yolo/auto-approve mode.
// Claude uses --dangerously-skip-permissions (replaces --permission-mode acceptEdits).
// Codex uses --yolo (global flag placed before subcommand).
// Cursor agent has no CLI yolo flag; the command is returned unchanged.
func yoloCommandForTool(tool, command string) string {
	switch tool {
	case "claude":
		cmd := strings.ReplaceAll(command, "--permission-mode acceptEdits", "--dangerously-skip-permissions")
		if cmd == command {
			cmd = strings.TrimSpace(command) + " --dangerously-skip-permissions"
		}
		return strings.TrimSpace(cmd)
	case "codex":
		if strings.HasPrefix(command, "codex ") {
			return "codex --yolo " + command[len("codex "):]
		}
		return command
	}
	return command
}

func (m model) startAndAttachSession(name, command string) (model, tea.Cmd) {
	sess, exists := m.sessions[name]
	if !exists {
		sess = tmux.NewSession(name, command)
		m.sessions[name] = sess
	}
	if !sess.IsRunning() {
		if command == "" {
			command = m.commandForTool(toolFromSessionName(name))
		}
		if command == "" {
			m.homeNotice = fmt.Sprintf("session %s is not running", name)
			return m, nil
		}
		launchCommand := fallbackCommand(toolFromSessionName(name), command)
		if err := tmux.CreateSession(name, launchCommand); err != nil {
			m.homeNotice = fmt.Sprintf("failed to start %s: %v", name, err)
			return m, nil
		}
	}
	m.refreshBindings()
	m.shouldAttach = true
	m.sessionToAttach = name
	m.homeNotice = ""
	m.mode = modeHome
	return m, tea.Quit
}

func (m model) requestAttachSession(name string) (model, tea.Cmd) {
	m.shouldAttach = true
	m.sessionToAttach = name
	m.homeNotice = ""
	m.mode = modeHome
	return m, tea.Quit
}

func (m model) createAndAttachTool(tool string) (model, tea.Cmd) {
	cwd := m.currentDir()
	if cwd != "" {
		inDir := m.toolSessionsInDir(tool, cwd)
		switch len(inDir) {
		case 1:
			return m.requestAttachSession(inDir[0])
		default:
			if len(inDir) == 0 {
				break
			}
			m.mode = modePickAttach
			m.pickerTool = tool
			m.pickerTargets = make(map[string]string)
			for i, name := range inDir {
				m.pickerTargets[pickerKey(i)] = name
			}
			m.homeNotice = "session already running in this directory"
			return m, nil
		}
	}

	command := m.commandForTool(tool)
	if command == "" {
		m.homeNotice = fmt.Sprintf("%s is not configured", tool)
		return m, nil
	}
	yoloEnabled := m.newToolYolo
	if m.newToolYolo {
		command = yoloCommandForTool(tool, command)
		m.newToolYolo = false
	}
	name := m.nextSessionName(tool)
	launchCommand := fallbackCommand(tool, command)
	if err := tmux.CreateSession(name, launchCommand); err != nil {
		m.homeNotice = fmt.Sprintf("failed to create %s: %v", tool, err)
		return m, nil
	}
	_ = setSessionToolFn(name, tool)
	m.rememberSessionTool(name, tool)
	if err := tmux.SetSessionYolo(name, yoloEnabled); err != nil {
		// Non-fatal: session still starts even if metadata cannot be persisted.
	}
	m.sessions[name] = tmux.NewSession(name, command)
	return m.startAndAttachSession(name, command)
}

func (m model) preparePicker(tool string, pickMode uiMode) model {
	targets := m.runningToolSessions(tool)
	m.mode = pickMode
	m.pickerTool = tool
	m.pickerTargets = make(map[string]string)
	limit := len(targets)
	maxKeys := len("abcdefghijklmnopqrstuvwxyz")
	if limit > maxKeys {
		limit = maxKeys
		m.homeNotice = "showing first 26 sessions"
	} else {
		m.homeNotice = ""
	}
	for i := 0; i < limit; i++ {
		m.pickerTargets[pickerKey(i)] = targets[i]
	}
	return m
}

func (m model) handleToolAttach(tool string) (model, tea.Cmd) {
	targets := m.runningToolSessions(tool)
	switch len(targets) {
	case 0:
		return m.createAndAttachTool(tool)
	case 1:
		return m.startAndAttachSession(targets[0], "")
	default:
		m = m.preparePicker(tool, modePickAttach)
		return m, nil
	}
}

func (m model) handleToolKill(tool string) (model, tea.Cmd) {
	targets := m.runningToolSessions(tool)
	switch len(targets) {
	case 0:
		m.homeNotice = fmt.Sprintf("no %s sessions running", tool)
		m.mode = modeHome
		return m, nil
	case 1:
		if err := tmux.KillSession(targets[0]); err != nil {
			m.homeNotice = fmt.Sprintf("failed to stop %s: %v", targets[0], err)
		} else {
			m.homeNotice = fmt.Sprintf("stopped %s", targets[0])
			delete(m.sessions, targets[0])
			delete(m.sessionTools, targets[0])
		}
		m.refreshBindings()
		m.mode = modeHome
		return m, nil
	default:
		m = m.preparePicker(tool, modePickKill)
		return m, nil
	}
}

func validSessionName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			continue
		}
		switch r {
		case '-', '_', '.', ' ':
			continue
		default:
			return false
		}
	}
	return true
}

func (m model) beginRenameTarget(name string) model {
	m.mode = modeRenameInput
	m.renameTarget = name
	m.renameInput = name
	m.homeNotice = ""
	return m
}

func (m model) applyRenameTarget() model {
	oldName := strings.TrimSpace(m.renameTarget)
	newName := strings.TrimSpace(m.renameInput)
	if oldName == "" {
		m.mode = modeHome
		m.homeNotice = "no rename target selected"
		return m
	}
	if newName == "" {
		m.homeNotice = "name cannot be empty"
		return m
	}
	if newName == oldName {
		m.mode = modeHome
		m.homeNotice = "name unchanged"
		return m
	}
	if !validSessionName(newName) {
		m.homeNotice = "name can only use letters, numbers, ., _, -"
		return m
	}
	if _, exists := m.sessions[newName]; exists {
		m.homeNotice = fmt.Sprintf("session %s already exists", newName)
		return m
	}
	if tmux.SessionExists(newName) {
		m.homeNotice = fmt.Sprintf("session %s already exists", newName)
		return m
	}
	if err := renameSessionFn(oldName, newName); err != nil {
		m.homeNotice = fmt.Sprintf("failed to rename %s: %v", oldName, err)
		return m
	}
	tool := m.sessionTool(oldName)

	if _, ok := m.sessions[oldName]; ok {
		delete(m.sessions, oldName)
	}
	delete(m.sessionTools, oldName)
	command := m.commandForTool(tool)
	m.sessions[newName] = tmux.NewSession(newName, command)
	_ = setSessionToolFn(newName, tool)
	m.rememberSessionTool(newName, tool)
	delete(m.bindings, oldName)
	m.renameTarget = ""
	m.renameInput = ""
	m.mode = modeHome
	m.refreshBindings()
	m.homeNotice = fmt.Sprintf("renamed %s to %s", oldName, newName)
	return m
}

func (m model) Init() tea.Cmd {
	return tickCmd
}

func (m *model) refreshTaskCounts() {
	if m.taskCounts == nil {
		m.taskCounts = make(map[string]int)
	}
	if m.taskCommands == nil {
		m.taskCommands = make(map[string][]string)
	}
	now := time.Now()
	if !m.taskRefreshAt.IsZero() && now.Sub(m.taskRefreshAt) < 900*time.Millisecond {
		return
	}

	next := make(map[string]int)
	nextCommands := make(map[string][]string)
	for name, sess := range m.sessions {
		if sess == nil || !sess.IsRunning() {
			continue
		}
		tasks, err := sessionUserTasksFn(name)
		if err != nil {
			continue
		}
		next[name] = len(tasks)
		if len(tasks) > 0 {
			nextCommands[name] = summarizeTaskCommands(tasks, 2)
		}
	}
	m.taskCounts = next
	m.taskCommands = nextCommands
	m.taskRefreshAt = now
}

func summarizeTaskCommands(tasks []tmux.Task, max int) []string {
	if max <= 0 || len(tasks) == 0 {
		return nil
	}
	out := make([]string, 0, max+1)
	for i, t := range tasks {
		if i >= max {
			out = append(out, fmt.Sprintf("+%d more", len(tasks)-max))
			break
		}
		out = append(out, t.Command)
	}
	return out
}

func (m model) runningSessionNames() []string {
	var names []string
	for name, binding := range m.bindings {
		if !binding.Running {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (m model) enterTaskKillPicker() (model, tea.Cmd) {
	targets := make([]taskKillTarget, 0)
	for _, name := range m.runningSessionNames() {
		tasks, err := sessionUserTasksFn(name)
		if err != nil {
			continue
		}
		for _, task := range tasks {
			targets = append(targets, taskKillTarget{
				Session: name,
				PID:     task.PID,
				Command: task.Command,
			})
		}
	}

	if len(targets) == 0 {
		m.mode = modeHome
		m.homeNotice = "no tasks to kill"
		return m, nil
	}

	m.mode = modePickKillTask
	m.taskKillTargets = make(map[string]taskKillTarget)
	limit := len(targets)
	maxKeys := len("abcdefghijklmnopqrstuvwxyz")
	if limit > maxKeys {
		limit = maxKeys
		m.homeNotice = "showing first 26 tasks"
	} else {
		m.homeNotice = ""
	}
	for i := 0; i < limit; i++ {
		m.taskKillTargets[pickerKey(i)] = targets[i]
	}
	return m, nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Handle keys based on current view state
		switch m.viewState {
		case viewHome:
			return m.updateHome(msg)
		case viewAttached:
			return m.updateAttached(msg)
		}
	case tickMsg:
		m.refreshBindings()
		// Periodic update to refresh activity status
		for _, sess := range m.sessions {
			sess.UpdateActivity()
		}
		m.refreshTaskCounts()
		return m, tickCmd
	case tea.WindowSizeMsg:
		m.windowWidth = msg.Width
		return m, nil
	}
	return m, nil
}

func (m model) updateHome(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	m.refreshBindings()

	switch key {
	case "ctrl+c":
		// Kill all tmux sessions and exit
		tmux.KillServer()
		return m, tea.Quit
	case "d":
		if m.mode == modeHome {
			// Quit without killing sessions
			return m, tea.Quit
		}
		if m.mode == modeNewTool || m.mode == modeKillTool || m.mode == modeRenameTool || m.mode == modeRenameInput {
			m.mode = modeHome
			m.homeNotice = ""
			m.newToolYolo = false
			m.renameTarget = ""
			m.renameInput = ""
			return m, nil
		}
	case "esc":
		if m.mode != modeHome {
			m.mode = modeHome
			m.homeNotice = ""
			m.newToolYolo = false
			m.renameTarget = ""
			m.renameInput = ""
			return m, nil
		}
	}

	switch m.mode {
	case modeRenameInput:
		switch msg.Type {
		case tea.KeyEnter:
			m = m.applyRenameTarget()
			return m, nil
		case tea.KeyBackspace, tea.KeyDelete:
			if len(m.renameInput) > 0 {
				m.renameInput = m.renameInput[:len(m.renameInput)-1]
			}
			return m, nil
		case tea.KeyRunes:
			m.renameInput += string(msg.Runes)
			return m, nil
		default:
			return m, nil
		}
	case modeDirJump:
		switch msg.Type {
		case tea.KeyEsc:
			m.mode = modeHome
			m.dirQuery = ""
			m.dirSuggestions = nil
			m.dirSelection = 0
			m.homeNotice = ""
			return m, nil
		case tea.KeyEnter:
			if len(m.dirSuggestions) == 0 {
				m.refreshDirSuggestions()
			}
			if len(m.dirSuggestions) == 0 {
				m.homeNotice = "no matching directories"
				return m, nil
			}
			if m.dirSelection < 0 || m.dirSelection >= len(m.dirSuggestions) {
				m.dirSelection = 0
			}
			return m.applyDirChange(m.dirSuggestions[m.dirSelection])
		case tea.KeyUp:
			if len(m.dirSuggestions) > 0 {
				if m.dirSelection <= 0 {
					m.dirSelection = len(m.dirSuggestions) - 1
				} else {
					m.dirSelection--
				}
			}
			return m, nil
		case tea.KeyDown:
			if len(m.dirSuggestions) > 0 {
				m.dirSelection = (m.dirSelection + 1) % len(m.dirSuggestions)
			}
			return m, nil
		case tea.KeyBackspace, tea.KeyDelete:
			if len(m.dirQuery) > 0 {
				m.dirQuery = m.dirQuery[:len(m.dirQuery)-1]
			}
			m.dirSelection = 0
			m.refreshDirSuggestions()
			return m, nil
		case tea.KeyRunes:
			m.dirQuery += string(msg.Runes)
			m.dirSelection = 0
			m.refreshDirSuggestions()
			return m, nil
		default:
			return m, nil
		}
	case modeNewTool:
		if key == "y" {
			m.newToolYolo = !m.newToolYolo
			return m, nil
		}
		cwd := m.currentDir()
		tool := m.toolForKey(key)
		if tool == "" {
			if m.disabledToolKey(key) {
				return m, nil
			}
			m.homeNotice = fmt.Sprintf("Unknown new target %q.", key)
			return m, nil
		}
		if m.toolAlreadyRunningInDir(tool, cwd) {
			m.homeNotice = fmt.Sprintf("%s already running in this directory", tool)
			return m, nil
		}
		return m.createAndAttachTool(tool)
	case modeKillTool:
		claudeTargets := m.runningToolSessions("claude")
		codexTargets := m.runningToolSessions("codex")
		cursorTargets := m.runningToolSessions("cursor")
		runningClaude := len(claudeTargets) > 0
		runningCodex := len(codexTargets) > 0
		runningCursor := len(cursorTargets) > 0
		if !runningClaude && !runningCodex && !runningCursor {
			m.mode = modeHome
			m.homeNotice = "no kill targets are running"
			return m, nil
		}
		switch key {
		case "t":
			return m.enterTaskKillPicker()
		default:
			tool := m.toolForKey(key)
			if tool == "" {
				if m.disabledToolKey(key) {
					return m, nil
				}
				m.homeNotice = fmt.Sprintf("Unknown kill target %q.", key)
				return m, nil
			}
			var targets []string
			switch tool {
			case "claude":
				targets = claudeTargets
			case "codex":
				targets = codexTargets
			case "cursor":
				targets = cursorTargets
			}
			if len(targets) == 0 {
				m.homeNotice = fmt.Sprintf("%s is not running", tool)
				return m, nil
			}
			if len(targets) > 1 {
				m = m.preparePicker(tool, modePickKill)
				return m, nil
			}
			return m.handleToolKill(tool)
		}
	case modeRenameTool:
		tools := []string{"claude", "codex", "cursor"}
		targetsByTool := make(map[string][]string, len(tools))
		runningAny := false
		for _, tool := range tools {
			targetsByTool[tool] = m.runningToolSessions(tool)
			if len(targetsByTool[tool]) > 0 {
				runningAny = true
			}
		}
		if !runningAny {
			m.mode = modeHome
			m.homeNotice = "no rename targets are running"
			return m, nil
		}
		tool := m.toolForKey(key)
		if tool == "" {
			if m.disabledToolKey(key) {
				return m, nil
			}
			m.homeNotice = fmt.Sprintf("Unknown rename target %q.", key)
			return m, nil
		}
		targets := targetsByTool[tool]
		if len(targets) == 0 {
			m.homeNotice = fmt.Sprintf("%s is not running", tool)
			return m, nil
		}
		if len(targets) > 1 {
			m = m.preparePicker(tool, modePickRename)
			return m, nil
		}
		m = m.beginRenameTarget(targets[0])
		return m, nil
	case modePickAttach:
		target, ok := m.pickerTargets[key]
		if !ok {
			m.homeNotice = fmt.Sprintf("Unknown target %q.", key)
			return m, nil
		}
		return m.startAndAttachSession(target, "")
	case modePickKill:
		target, ok := m.pickerTargets[key]
		if !ok {
			m.homeNotice = fmt.Sprintf("Unknown target %q.", key)
			return m, nil
		}
		if err := tmux.KillSession(target); err != nil {
			m.homeNotice = fmt.Sprintf("failed to stop %s: %v", target, err)
		} else {
			m.homeNotice = fmt.Sprintf("stopped %s", target)
			delete(m.sessions, target)
			delete(m.sessionTools, target)
		}
		m.mode = modeHome
		m.refreshBindings()
		return m, nil
	case modePickRename:
		target, ok := m.pickerTargets[key]
		if !ok {
			m.homeNotice = fmt.Sprintf("Unknown target %q.", key)
			return m, nil
		}
		m = m.beginRenameTarget(target)
		return m, nil
	case modePickKillTask:
		target, ok := m.taskKillTargets[key]
		if !ok {
			m.homeNotice = fmt.Sprintf("Unknown task target %q.", key)
			return m, nil
		}
		if err := killTaskPIDFn(target.PID); err != nil {
			m.homeNotice = fmt.Sprintf("failed to kill pid %d: %v", target.PID, err)
		} else {
			m.homeNotice = fmt.Sprintf("killed pid %d", target.PID)
		}
		m.mode = modeHome
		m.refreshTaskCounts()
		return m, nil
	}

	switch key {
	case "z":
		if !m.hasFasder {
			m.homeNotice = "fasder not found; install fasder to use z"
			return m, nil
		}
		m.mode = modeDirJump
		m.homeNotice = ""
		m.dirQuery = ""
		m.dirSuggestions = nil
		m.dirSelection = 0
		m.refreshDirSuggestions()
		return m, nil
	case "n":
		m.mode = modeNewTool
		m.homeNotice = ""
		return m, nil
	case "k":
		if !m.hasAnyRunningSessions() {
			m.homeNotice = "no running sessions to kill"
			return m, nil
		}
		m.mode = modeKillTool
		m.homeNotice = ""
		return m, nil
	case "r":
		if !m.hasAnyRunningSessions() {
			m.homeNotice = "no running sessions to rename"
			return m, nil
		}
		m.mode = modeRenameTool
		m.homeNotice = ""
		return m, nil
	}

	if tool := m.toolForKey(key); tool != "" {
		return m.handleToolAttach(tool)
	}

	// Keep custom configured sessions working with their own keys.
	for _, sess := range m.config.Sessions {
		if sess.Key != key {
			continue
		}
		return m.startAndAttachSession(sess.Name, sess.Command)
	}

	if key == "t" && m.mode == modeHome {
		m.showTaskDetails = !m.showTaskDetails
		return m, nil
	}

	return m, nil
}

func (m model) stopSession(name string) model {
	tmuxSess, exists := m.sessions[name]
	if !exists {
		m.homeNotice = fmt.Sprintf("%s session is not configured", name)
		return m
	}

	if !tmuxSess.IsRunning() {
		m.homeNotice = fmt.Sprintf("%s session is not running", name)
		return m
	}

	if err := tmuxSess.Stop(); err != nil {
		m.homeNotice = fmt.Sprintf("failed to stop %s: %v", name, err)
		return m
	}

	delete(m.sessions, name)
	delete(m.sessionTools, name)
	m.refreshBindings()
	m.homeNotice = fmt.Sprintf("stopped %s session", name)
	return m
}

func (m model) updateAttached(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	// This view state is no longer used
	// Attach happens outside of Bubble Tea
	return m, nil
}

func (m model) View() string {
	switch m.viewState {
	case viewHome:
		return m.viewHome()
	case viewAttached:
		return m.viewAttached()
	default:
		return ""
	}
}

func (m model) viewHome() string {
	m.refreshBindings()

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D56F4"))
	metaStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888"))
	keyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#4DA3FF"))
	activeStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#04B575")).
		Bold(true)
	idleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#999999"))
	repoNameStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7D56F4")).
		Bold(true)
	alertStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#4DA3FF"))
	title := "Welcome to PocketBot"
	if level := os.Getenv("PB_LEVEL"); level != "" {
		title = fmt.Sprintf("Welcome to PocketBot (level %s)", level)
	}
	lines := []string{
		titleStyle.Render("🤖 " + title),
		metaStyle.Render(fmt.Sprintf("dir: %s", m.currentDir())),
	}

	if m.homeNotice != "" {
		lines = append(lines, alertStyle.Render(m.homeNotice))
	}

	switch m.mode {
	case modeDirJump:
		jumpTitleStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7D56F4")).
			Bold(true)
		searchLabelStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#4DA3FF"))
		hintStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#AAAAAA"))
		selectedStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#04B575")).
			Bold(true)
		suggestionStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#BBBBBB"))

		lines = append(lines,
			jumpTitleStyle.Render("z fasder jump"),
			fmt.Sprintf("%s%s", searchLabelStyle.Render("search: "), m.dirQuery),
			hintStyle.Render("up/down move   enter select   esc cancel"),
		)
		for i, suggestion := range m.dirSuggestions {
			row := fmt.Sprintf("  %s", suggestion)
			if i == m.dirSelection {
				row = fmt.Sprintf("> %s", suggestion)
				lines = append(lines, selectedStyle.Render(row))
				continue
			}
			lines = append(lines, suggestionStyle.Render(row))
		}
	case modeNewTool:
		yoloStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF8A00")).Bold(true)
		cwd := m.currentDir()
		if m.toolEnabled("claude") {
			if m.toolAlreadyRunningInDir("claude", cwd) {
				lines = append(lines, metaStyle.Render("claude already running"))
			} else {
				lines = append(lines, fmt.Sprintf("%s new claude", keyStyle.Render(m.keyForTool("claude"))))
			}
		}
		if m.toolEnabled("codex") {
			if m.toolAlreadyRunningInDir("codex", cwd) {
				lines = append(lines, metaStyle.Render("codex already running"))
			} else {
				lines = append(lines, fmt.Sprintf("%s new codex", keyStyle.Render(m.keyForTool("codex"))))
			}
		}
		if m.toolEnabled("cursor") {
			if m.toolAlreadyRunningInDir("cursor", cwd) {
				lines = append(lines, metaStyle.Render("cursor already running"))
			} else {
				lines = append(lines, fmt.Sprintf("%s new cursor", keyStyle.Render(m.keyForTool("cursor"))))
			}
		}
		if !m.toolEnabled("claude") && !m.toolEnabled("codex") && !m.toolEnabled("cursor") {
			lines = append(lines, metaStyle.Render("all built-in tools are disabled"))
		}
		if m.newToolYolo {
			lines = append(lines, fmt.Sprintf("%s yolo: %s", keyStyle.Render("y"), yoloStyle.Render("ON (skip all permissions)")))
		} else {
			lines = append(lines, fmt.Sprintf("%s yolo: off", keyStyle.Render("y")))
		}
		lines = append(lines, "esc cancel")
	case modeKillTool:
		runningClaude := len(m.runningToolSessions("claude")) > 0
		runningCodex := len(m.runningToolSessions("codex")) > 0
		runningCursor := len(m.runningToolSessions("cursor")) > 0
		renderKillRows := func(tool, key string) {
			names := m.runningToolSessions(tool)
			if len(names) == 0 {
				return
			}
			if len(names) == 1 {
				lines = append(lines, fmt.Sprintf("%s kill %s", keyStyle.Render(key), names[0]))
				return
			}
			for i, name := range names {
				letter := alphaKey(i)
				if letter == "" {
					break
				}
				repo := "-"
				if binding, ok := m.bindings[name]; ok {
					repo = repoFromCwd(binding.Cwd)
				}
				lines = append(lines, fmt.Sprintf("%s %s repo:%s", keyStyle.Render("("+key+" "+letter+")"), name, repoNameStyle.Render(repo)))
			}
		}
		if runningClaude && m.toolEnabled("claude") {
			renderKillRows("claude", m.keyForTool("claude"))
		}
		if runningCodex && m.toolEnabled("codex") {
			renderKillRows("codex", m.keyForTool("codex"))
		}
		if runningCursor && m.toolEnabled("cursor") {
			renderKillRows("cursor", m.keyForTool("cursor"))
		}
		lines = append(lines, fmt.Sprintf("%s kill task", keyStyle.Render("t")))
		lines = append(lines, "esc cancel")
	case modeRenameTool:
		runningClaude := len(m.runningToolSessions("claude")) > 0
		runningCodex := len(m.runningToolSessions("codex")) > 0
		runningCursor := len(m.runningToolSessions("cursor")) > 0
		renderRenameRows := func(tool, key string) {
			names := m.runningToolSessions(tool)
			if len(names) == 0 {
				return
			}
			if len(names) == 1 {
				lines = append(lines, fmt.Sprintf("%s rename %s", keyStyle.Render(key), names[0]))
				return
			}
			for i, name := range names {
				letter := alphaKey(i)
				if letter == "" {
					break
				}
				repo := "-"
				if binding, ok := m.bindings[name]; ok {
					repo = repoFromCwd(binding.Cwd)
				}
				lines = append(lines, fmt.Sprintf("%s %s repo:%s", keyStyle.Render("("+key+" "+letter+")"), name, repoNameStyle.Render(repo)))
			}
		}
		if runningClaude && m.toolEnabled("claude") {
			renderRenameRows("claude", m.keyForTool("claude"))
		}
		if runningCodex && m.toolEnabled("codex") {
			renderRenameRows("codex", m.keyForTool("codex"))
		}
		if runningCursor && m.toolEnabled("cursor") {
			renderRenameRows("cursor", m.keyForTool("cursor"))
		}
		lines = append(lines, "esc cancel")
	case modePickAttach, modePickKill:
		action := "attach"
		if m.mode == modePickKill {
			action = "kill"
		}
		lines = append(lines, metaStyle.Render(fmt.Sprintf("%s %s", action, m.pickerTool)))
		keys := make([]string, 0, len(m.pickerTargets))
		for k := range m.pickerTargets {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if m.mode == modePickKill {
			lines = append(lines, alertStyle.Render("pick one key to kill"))
		} else {
			lines = append(lines, metaStyle.Render("pick one key to attach"))
		}
		for _, k := range keys {
			name := m.pickerTargets[k]
			status := ""
			if sess, ok := m.sessions[name]; ok && sess.ActivityKnown() {
				status = idleStyle.Render("○")
				if sess.IsActive() {
					status = activeStyle.Render("●")
				}
			}
			repo := "-"
			if binding, ok := m.bindings[name]; ok {
				repo = repoFromCwd(binding.Cwd)
			}
			rowParts := []string{keyStyle.Render("(" + k + ")"), name}
			if status != "" {
				rowParts = append(rowParts, status)
			}
			rowParts = append(rowParts, repoNameStyle.Render(repo))
			lines = append(lines, strings.Join(rowParts, " "))
		}
		lines = append(lines, "esc cancel")
	case modePickRename:
		lines = append(lines, metaStyle.Render("rename "+m.pickerTool))
		keys := make([]string, 0, len(m.pickerTargets))
		for k := range m.pickerTargets {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		lines = append(lines, alertStyle.Render("pick one key"))
		for _, k := range keys {
			name := m.pickerTargets[k]
			repo := "-"
			if binding, ok := m.bindings[name]; ok {
				repo = repoFromCwd(binding.Cwd)
			}
			lines = append(lines, fmt.Sprintf("%s %s %s",
				keyStyle.Render("("+k+")"),
				name,
				repoNameStyle.Render(repo),
			))
		}
		lines = append(lines, "esc cancel")
	case modePickKillTask:
		lines = append(lines, metaStyle.Render("kill task"))
		keys := make([]string, 0, len(m.taskKillTargets))
		for k := range m.taskKillTargets {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		lines = append(lines, alertStyle.Render("pick one key to kill task"))
		for _, k := range keys {
			target := m.taskKillTargets[k]
			lines = append(lines, fmt.Sprintf("%s %s pid:%d %s",
				keyStyle.Render("("+k+")"),
				target.Session,
				target.PID,
				target.Command,
			))
		}
		lines = append(lines, "esc cancel")
	case modeRenameInput:
		lines = append(lines, metaStyle.Render(fmt.Sprintf("rename %s", m.renameTarget)))
		cursorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#4DA3FF")).Bold(true)
		lines = append(lines, fmt.Sprintf("new name: %s%s", m.renameInput, cursorStyle.Render("▌")))
		lines = append(lines, "enter confirm   esc cancel")
	default:
		claude := m.runningToolSessions("claude")
		codex := m.runningToolSessions("codex")
		cursor := m.runningToolSessions("cursor")
		total := len(claude) + len(codex) + len(cursor)
		lines = append(lines, "")
		if total < 10 {
			lines = append(lines, m.detailedRows("claude", claude)...)
			lines = append(lines, m.detailedRows("codex", codex)...)
			lines = append(lines, m.detailedRows("cursor", cursor)...)
		} else {
			lines = append(lines, m.summaryRow("claude", claude))
			lines = append(lines, m.summaryRow("codex", codex))
			lines = append(lines, m.summaryRow("cursor", cursor))
		}
		lines = append(lines, "")
		lines = append(lines,
			fmt.Sprintf("%s jump-dir   %s new   %s kill", keyStyle.Render("z"), keyStyle.Render("n"), keyStyle.Render("k")),
			fmt.Sprintf("%s %s   %s rename", keyStyle.Render("t"), map[bool]string{true: "hide tasks", false: "show tasks"}[m.showTaskDetails], keyStyle.Render("r")),
		)
		if m.hasAnyRunningSessions() {
			lines = append(lines, fmt.Sprintf("%s quit   %s kill-all", keyStyle.Render("d"), keyStyle.Render("^c")))
		} else {
			lines = append(lines, fmt.Sprintf("%s quit    %s kill-all", keyStyle.Render("d"), keyStyle.Render("^c")))
		}
	}

	return strings.Join(capLines(lines, 20), "\n") + "\n"
}

func (m model) detailedRows(tool string, names []string) []string {
	var rows []string
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#4DA3FF"))
	activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#04B575")).Bold(true)
	idleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#999999"))
	repoLabelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	repoNameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4")).Bold(true)
	yoloStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF8A00")).Bold(true)
	taskStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#4DA3FF"))
	taskDetailStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#AAAAAA"))
	key := m.keyForTool(tool)
	if len(names) == 0 {
		if !m.toolEnabled(tool) || key == "" {
			return nil
		}
		repoText := repoLabelStyle.Render("repo:") + repoNameStyle.Render("-")
		rows = append(rows, fmt.Sprintf("%s %s %s %s",
			keyStyle.Render("("+key+")"),
			tool,
			repoText,
			idleStyle.Render("○ not running"),
		))
		return rows
	}
	for i, name := range names {
		join := key
		if len(names) > 1 {
			letter := alphaKey(i)
			if letter == "" {
				continue
			}
			join = key + " " + letter
		}
		status := ""
		if sess, ok := m.sessions[name]; ok && sess.ActivityKnown() {
			status = idleStyle.Render("○ idle")
			if sess.IsActive() {
				status = activeStyle.Render("● active")
			}
		}
		repo := "-"
		if binding, ok := m.bindings[name]; ok {
			repo = repoFromCwd(binding.Cwd)
		}
		repoText := repoLabelStyle.Render("repo:") + repoNameStyle.Render(repo)
		rowParts := []string{keyStyle.Render("(" + join + ")"), name, repoText}
		if binding, ok := m.bindings[name]; ok && binding.Yolo {
			rowParts = append(rowParts, yoloStyle.Render("(yolo)"))
		}
		if !m.showTaskDetails {
			if n := m.taskCounts[name]; n > 0 {
				rowParts = append(rowParts, taskStyle.Render(fmt.Sprintf("tasks:%d", n)))
			}
		}
		if status != "" {
			rowParts = append(rowParts, status)
		}
		rows = append(rows, strings.Join(rowParts, " "))
		if m.showTaskDetails {
			for _, cmd := range m.taskCommands[name] {
				rows = append(rows, taskDetailStyle.Render("  task: "+cmd))
			}
		}
	}
	return rows
}

func (m model) summaryRow(tool string, names []string) string {
	active := 0
	taskTotal := 0
	for _, name := range names {
		if sess, ok := m.sessions[name]; ok && sess.IsActive() {
			active++
		}
		taskTotal += m.taskCounts[name]
	}
	metaStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#04B575")).Bold(true)
	parts := []string{
		tool,
		fmt.Sprintf("%d", len(names)),
		activeStyle.Render(fmt.Sprintf("active:%d", active)),
		metaStyle.Render(fmt.Sprintf("idle:%d", len(names)-active)),
	}
	if taskTotal > 0 {
		parts = append(parts, metaStyle.Render(fmt.Sprintf("tasks:%d", taskTotal)))
	}
	return strings.Join(parts, " ")
}

func (m model) hasAnyRunningSessions() bool {
	for _, sess := range m.sessions {
		if sess != nil && sess.IsRunning() {
			return true
		}
	}
	return false
}

func capLines(lines []string, max int) []string {
	if len(lines) <= max {
		return lines
	}
	if max <= 0 {
		return []string{}
	}
	out := append([]string{}, lines[:max]...)
	out[max-1] = "..."
	return out
}

func (m model) viewAttached() string {
	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888")).
		Italic(true)

	help := helpStyle.Render("[Press Ctrl+D to detach]")

	// This view is not actually used - attach happens outside Bubble Tea
	return fmt.Sprintf("%s\n\n[Attached to Claude]\n", help)
}

func main() {
	// Handle subcommands
	if len(os.Args) > 1 {
		handleSubcommand(os.Args[1])
		return
	}

	m := initialModel()

	// Note: We don't kill tmux sessions on exit - they persist in background
	// User can manually kill with: tmux -L pocketbot kill-server

	// Main loop: run UI, attach when requested, repeat
	for {
		m.shouldAttach = false
		m.sessionToAttach = ""
		m.viewState = viewHome

		// Run Bubble Tea UI with alternate screen buffer
		p := tea.NewProgram(m, tea.WithAltScreen())
		finalModel, err := p.Run()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		// Get the final model state
		m = finalModel.(model)

		// Check if we should attach
		if !m.shouldAttach || m.sessionToAttach == "" {
			// User quit normally
			break
		}

		// Attach to requested tmux session
		tmuxSess, exists := m.sessions[m.sessionToAttach]
		if !exists || tmuxSess == nil {
			tmuxSess = tmux.NewSession(m.sessionToAttach, "")
			m.sessions[m.sessionToAttach] = tmuxSess
		}
		if !tmuxSess.IsRunning() {
			fmt.Fprintf(os.Stderr, "Session %q is not running\n", m.sessionToAttach)
			continue
		}

		// Note: No delay needed. The original bug was an invalid claude flag,
		// not a race condition. See TestClaudeCommandFlag for regression test.

		// tmux attach - returns when user detaches (prefix+d)
		if err := tmuxSess.Attach(); err != nil {
			fmt.Fprintf(os.Stderr, "Attach error: %v\n", err)
			// Check if session died
			if !tmuxSess.IsRunning() {
				fmt.Fprintf(os.Stderr, "Session exited. Check: tmux -L pocketbot list-sessions\n")
			}
		}

		// Always return to home screen after detach
	}
}

func handleSubcommand(cmd string) {
	switch cmd {
	case "test":
		runCommand("go", "test", "./...")
	case "build":
		runCommand("go", "build", "-o", "pb", "./cmd/pb")
	case "install":
		runCommand("go", "install", "./cmd/pb")
	case "run":
		runCommand("go", "run", "./cmd/pb")
	case "demo":
		// Run a simple demo session for testing
		runDemoSession()
	case "sessions":
		// Show sessions for current nesting level
		socket := "pocketbot"
		if level := os.Getenv("PB_LEVEL"); level != "" {
			socket = "pocketbot-" + level
		}
		runCommand("tmux", "-L", socket, "list-sessions")
	case "tasks":
		printToolTasks()
	case "kill-all":
		// Kill sessions for current nesting level
		socket := "pocketbot"
		if level := os.Getenv("PB_LEVEL"); level != "" {
			socket = "pocketbot-" + level
		}
		runCommand("tmux", "-L", socket, "kill-server")
	case "help", "-h", "--help":
		printHelp()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		fmt.Fprintf(os.Stderr, "Run 'pb help' for usage\n")
		os.Exit(1)
	}
}

func printToolTasksForSocket(w io.Writer) bool {
	names := listSessionsFn()
	sort.Strings(names)

	seen := false
	for _, name := range names {
		tool := toolFromSessionName(name)
		if tool != "claude" && tool != "codex" && tool != "cursor" {
			continue
		}
		seen = true
		tasks, err := sessionUserTasksFn(name)
		if err != nil {
			fmt.Fprintf(w, "%s: error reading tasks: %v\n", name, err)
			continue
		}
		fmt.Fprintf(w, "%s: %d task process(es)\n", name, len(tasks))
		if len(tasks) == 0 {
			fmt.Fprintln(w, "  (none)")
			continue
		}
		limit := len(tasks)
		if limit > maxTasksShownPerAgent {
			limit = maxTasksShownPerAgent
		}
		for _, task := range tasks[:limit] {
			fmt.Fprintf(w, "  pid=%d ppid=%d state=%s cmd=%s\n", task.PID, task.PPID, task.State, task.Command)
		}
		if len(tasks) > limit {
			fmt.Fprintf(w, "  +%d more\n", len(tasks)-limit)
		}
	}
	return seen
}

func printToolTasks() {
	if printToolTasksForSocket(os.Stdout) {
		return
	}

	// If running nested inside a session, PB_LEVEL points at the nested socket.
	// Fall back to root socket so `pb tasks` still sees top-level agent sessions.
	level := os.Getenv("PB_LEVEL")
	if level != "" {
		_ = os.Unsetenv("PB_LEVEL")
		found := printToolTasksForSocket(os.Stdout)
		_ = os.Setenv("PB_LEVEL", level)
		if found {
			return
		}
	}

	fmt.Println("No claude/codex/cursor sessions are running.")
}

func runCommand(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.Exit(1)
	}
}

func runDemoSession() {
	fmt.Println("Creating demo session...")

	// Create a simple test session
	if err := tmux.CreateSession("demo", "echo 'Demo session started'; echo 'Press Ctrl+D to detach'; sleep 30; echo 'Demo session ending...'"); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating demo session: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Demo session created. Attaching...")

	// Attach to it
	if err := tmux.AttachSession("demo"); err != nil {
		fmt.Fprintf(os.Stderr, "Error attaching: %v\n", err)
		os.Exit(1)
	}

	// Clean up
	fmt.Println("\nCleaning up demo session...")
	tmux.KillSession("demo")
}

func printHelp() {
	fmt.Println(`pocketbot - Mobile-friendly tmux session manager

Usage:
  pb              Start interactive session manager
  pb test         Run tests
  pb build        Build binary
  pb install      Install to $GOPATH/bin
  pb run          Run development version
  pb demo         Run a simple demo session (for testing)
  pb sessions     List active tmux sessions
  pb tasks        List descendant processes for running claude/codex/cursor sessions (spike)
  pb kill-all     Kill all sessions
  pb help         Show this help

Interactive mode keybindings:
  c               Attach claude (picker if multiple, create if none)
  x               Attach codex (picker if multiple, create if none)
  u               Attach cursor (picker if multiple, create if none)
  z               Jump directory with fasder query
  n               New instance (then y to toggle yolo, then c/x/u)
  k               Kill one instance (then c/x/u and picker if needed)
  r               Rename one instance (same flow as k)
  t               Toggle per-session task lines on home screen
  Esc             Go back/cancel in menus
  Ctrl+D          Detach from session (back to pb)
  d               Quit pb (sessions keep running)
  Ctrl+C          Kill all sessions and quit

Config:
  ~/.config/pocketbot/config.yaml`)
}

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/zakandrewking/pocketbot/internal/config"
	"github.com/zakandrewking/pocketbot/internal/tmux"
)

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
	modePickAttach
	modePickKill
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
	LastSeen    time.Time
}

type model struct {
	config          *config.Config
	sessions        map[string]*tmux.Session
	bindings        map[string]commandBinding
	windowWidth     int
	viewState       viewState
	mode            uiMode
	pickerTool      string
	pickerTargets   map[string]string
	shouldAttach    bool
	sessionToAttach string // Name of session to attach to
	homeNotice      string
	getwd           func() (string, error)
}

func initialModel() model {
	// Check for tmux
	if !tmux.Available() {
		fmt.Fprintf(os.Stderr, "Error: tmux is required but not found in PATH\n")
		fmt.Fprintf(os.Stderr, "Install with: brew install tmux\n")
		os.Exit(1)
	}

	// Check for directory mismatches with existing sessions
	checkDirectoryMismatch()

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
		config:        cfg,
		sessions:      sessions,
		bindings:      make(map[string]commandBinding),
		windowWidth:   80,
		viewState:     viewHome,
		mode:          modeHome,
		pickerTargets: make(map[string]string),
		getwd:         os.Getwd,
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
	for name, sess := range m.sessions {
		if toolFromSessionName(name) != tool {
			continue
		}
		if sess != nil && sess.IsRunning() {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func (m model) commandForTool(tool string) string {
	switch tool {
	case "claude":
		return m.config.Claude.Command
	case "codex":
		return m.config.Codex.Command
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
	default:
		return false
	}
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
		if err := tmux.CreateSession(name, command); err != nil {
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

func (m model) createAndAttachTool(tool string) (model, tea.Cmd) {
	command := m.commandForTool(tool)
	if command == "" {
		m.homeNotice = fmt.Sprintf("%s is not configured", tool)
		return m, nil
	}
	name := m.nextSessionName(tool)
	if err := tmux.CreateSession(name, command); err != nil {
		m.homeNotice = fmt.Sprintf("failed to create %s: %v", tool, err)
		return m, nil
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
		}
		m.refreshBindings()
		m.mode = modeHome
		return m, nil
	default:
		m = m.preparePicker(tool, modePickKill)
		return m, nil
	}
}

func (m model) Init() tea.Cmd {
	return tickCmd
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
		if m.mode == modeNewTool || m.mode == modeKillTool {
			m.mode = modeHome
			m.homeNotice = ""
			return m, nil
		}
	case "esc":
		if m.mode != modeHome {
			m.mode = modeHome
			m.homeNotice = ""
			return m, nil
		}
	}

	switch m.mode {
	case modeNewTool:
		switch key {
		case "c":
			return m.createAndAttachTool("claude")
		case "x":
			return m.createAndAttachTool("codex")
		default:
			m.homeNotice = fmt.Sprintf("Unknown new target %q. Use c or x.", key)
			return m, nil
		}
	case modeKillTool:
		switch key {
		case "c":
			return m.handleToolKill("claude")
		case "x":
			return m.handleToolKill("codex")
		default:
			m.homeNotice = fmt.Sprintf("Unknown kill target %q. Use c or x.", key)
			return m, nil
		}
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
		}
		m.mode = modeHome
		m.refreshBindings()
		return m, nil
	}

	switch key {
	case "c":
		return m.handleToolAttach("claude")
	case "x":
		return m.handleToolAttach("codex")
	case "n":
		m.mode = modeNewTool
		m.homeNotice = ""
		return m, nil
	case "k":
		m.mode = modeKillTool
		m.homeNotice = ""
		return m, nil
	}

	// Keep custom configured sessions working with their own keys.
	for _, sess := range m.config.Sessions {
		if sess.Key != key {
			continue
		}
		return m.startAndAttachSession(sess.Name, sess.Command)
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
		metaStyle.Render(fmt.Sprintf("mode: %s", m.modeLabel())),
	}

	if m.homeNotice != "" {
		lines = append(lines, alertStyle.Render(m.homeNotice))
	}

	switch m.mode {
	case modeNewTool:
		lines = append(lines,
			fmt.Sprintf("%s new claude", keyStyle.Render("c")),
			fmt.Sprintf("%s new codex", keyStyle.Render("x")),
			"esc cancel",
		)
	case modeKillTool:
		lines = append(lines,
			fmt.Sprintf("%s kill claude", keyStyle.Render("c")),
			fmt.Sprintf("%s kill codex", keyStyle.Render("x")),
			"esc cancel",
		)
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
			status := idleStyle.Render("○")
			if sess, ok := m.sessions[name]; ok && sess.IsActive() {
				status = activeStyle.Render("●")
			}
			repo := "-"
			if binding, ok := m.bindings[name]; ok {
				repo = repoFromCwd(binding.Cwd)
			}
			lines = append(lines, fmt.Sprintf("%s %s %s %s", keyStyle.Render("("+k+")"), name, status, repoNameStyle.Render(repo)))
		}
		lines = append(lines, "esc cancel")
	default:
		claude := m.runningToolSessions("claude")
		codex := m.runningToolSessions("codex")
		total := len(claude) + len(codex)
		lines = append(lines, metaStyle.Render(fmt.Sprintf("instances: %d", total)))
		if total < 10 {
			lines = append(lines, m.detailedRows("claude", claude)...)
			lines = append(lines, m.detailedRows("codex", codex)...)
		} else {
			lines = append(lines, m.summaryRow("claude", claude))
			lines = append(lines, m.summaryRow("codex", codex))
		}
		lines = append(lines,
			fmt.Sprintf("%s new", keyStyle.Render("n")),
			fmt.Sprintf("%s kill    %s quit   %s kill-all", keyStyle.Render("k"), keyStyle.Render("d"), keyStyle.Render("^c")),
		)
	}

	return strings.Join(capLines(lines, 20), "\n") + "\n"
}

func (m model) modeLabel() string {
	switch m.mode {
	case modeNewTool:
		return "new"
	case modeKillTool:
		return "kill"
	case modePickAttach:
		return "pick-attach"
	case modePickKill:
		return "pick-kill"
	default:
		return "home"
	}
}

func (m model) detailedRows(tool string, names []string) []string {
	var rows []string
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#4DA3FF"))
	activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#04B575")).Bold(true)
	idleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#999999"))
	repoLabelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	repoNameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4")).Bold(true)
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
		status := idleStyle.Render("○ idle")
		if sess, ok := m.sessions[name]; ok && sess.IsActive() {
			status = activeStyle.Render("● active")
		}
		repo := "-"
		if binding, ok := m.bindings[name]; ok {
			repo = repoFromCwd(binding.Cwd)
		}
		repoText := repoLabelStyle.Render("repo:") + repoNameStyle.Render(repo)
		rows = append(rows, fmt.Sprintf("%s %s %s %s",
			keyStyle.Render("("+join+")"),
			name,
			repoText,
			status,
		))
	}
	return rows
}

func (m model) summaryRow(tool string, names []string) string {
	active := 0
	for _, name := range names {
		if sess, ok := m.sessions[name]; ok && sess.IsActive() {
			active++
		}
	}
	metaStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#04B575")).Bold(true)
	return fmt.Sprintf("%s %d %s %s",
		tool,
		len(names),
		activeStyle.Render(fmt.Sprintf("active:%d", active)),
		metaStyle.Render(fmt.Sprintf("idle:%d", len(names)-active)),
	)
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
  pb kill-all     Kill all sessions
  pb help         Show this help

Interactive mode keybindings:
  c               Attach claude (picker if multiple, create if none)
  x               Attach codex (picker if multiple, create if none)
  n               New instance (then c/x)
  k               Kill one instance (then c/x and picker if needed)
  Ctrl+D          Detach from session (back to pb)
  d               Quit pb (sessions keep running)
  Ctrl+C          Kill all sessions and quit

Config:
  ~/.config/pocketbot/config.yaml`)
}

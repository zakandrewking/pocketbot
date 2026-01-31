package main

import (
	"fmt"
	"os"
	"os/exec"
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

type tickMsg time.Time

func tickCmd() tea.Msg {
	time.Sleep(1 * time.Second)
	return tickMsg(time.Now())
}

type model struct {
	config          *config.Config
	sessions        map[string]*tmux.Session
	viewState       viewState
	shouldAttach    bool
	sessionToAttach string // Name of session to attach to
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

	return model{
		config:    cfg,
		sessions:  sessions,
		viewState: viewHome,
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
		// Periodic update to refresh activity status
		for _, sess := range m.sessions {
			sess.UpdateActivity()
		}
		return m, tickCmd
	}
	return m, nil
}

func (m model) updateHome(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "ctrl+c":
		// Kill all tmux sessions and exit
		tmux.KillServer()
		return m, tea.Quit
	case "d":
		// Quit without killing sessions
		return m, tea.Quit
	}

	// Check if key matches any configured session
	for _, sess := range m.config.AllSessions() {
		if sess.Key == key {
			// Get tmux session
			tmuxSess, exists := m.sessions[sess.Name]
			if !exists {
				continue
			}

			// Start session if not running
			if !tmuxSess.IsRunning() {
				if err := tmuxSess.Start(); err != nil {
					// Error starting session, skip
					continue
				}
			}

			// Signal that we want to attach to this session
			m.shouldAttach = true
			m.sessionToAttach = sess.Name
			return m, tea.Quit // Exit Bubble Tea to attach
		}
	}

	return m, nil
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
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D56F4")).
		MarginTop(1).
		MarginBottom(1)

	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888"))

	runningStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#04B575")).
		Bold(true)

	stoppedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#666666"))

	instructionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#AAAAAA")).
		Italic(true)

	// Show nesting level if we're nested
	titleText := "🤖 Welcome to PocketBot!"
	if level := os.Getenv("PB_LEVEL"); level != "" {
		titleText = fmt.Sprintf("🤖 Welcome to PocketBot! (level %s)", level)
	}
	title := titleStyle.Render(titleText)

	// Build status lines for all sessions
	var sb strings.Builder
	for _, sess := range m.config.AllSessions() {
		tmuxSess, exists := m.sessions[sess.Name]
		if !exists {
			continue
		}

		var status string
		if tmuxSess.IsRunning() {
			if tmuxSess.IsActive() {
				status = runningStyle.Render("● active")
			} else {
				// Running but idle
				idleStyle := lipgloss.NewStyle().
					Foreground(lipgloss.Color("#888888"))
				status = idleStyle.Render("● idle")
			}
		} else {
			status = stoppedStyle.Render("○ not running")
		}

		// Format: name (key): status
		keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#5555FF"))
		line := fmt.Sprintf("%s %s %s\n",
			labelStyle.Render(fmt.Sprintf("%s:", sess.Name)),
			status,
			keyStyle.Render(fmt.Sprintf("[%s]", sess.Key)))
		sb.WriteString(line)
	}

	// Instructions
	instructions := instructionStyle.Render("Ctrl+C to kill all & quit • d to quit")

	return fmt.Sprintf("\n%s\n\n%s\n%s\n", title, sb.String(), instructions)
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
		if !exists {
			fmt.Fprintf(os.Stderr, "Session %q not found\n", m.sessionToAttach)
			continue
		}

		// Check if session is actually running before attaching
		if !tmuxSess.IsRunning() {
			fmt.Fprintf(os.Stderr, "Session %q is not running\n", m.sessionToAttach)
			continue
		}

		// Give the session a moment to fully initialize
		time.Sleep(500 * time.Millisecond)

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
  c               Attach to claude session
  Ctrl+D          Detach from session (back to pb)
  d               Quit pb (sessions keep running)
  Ctrl+C          Kill all sessions and quit

Config:
  ~/.config/pocketbot/config.yaml`)
}

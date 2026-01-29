package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/zakandrewking/pocketbot/internal/config"
	"github.com/zakandrewking/pocketbot/internal/session"
)

type viewState int

const (
	viewHome viewState = iota
	viewAttached
)

type attachMsg struct{}

type tickMsg time.Time

func tickCmd() tea.Msg {
	time.Sleep(1 * time.Second)
	return tickMsg(time.Now())
}

type model struct {
	config           *config.Config
	registry         *session.Registry
	viewState        viewState
	shouldAttach     bool
	sessionToAttach  string // Name of session to attach to
}

func initialModel() model {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		fmt.Fprintf(os.Stderr, "Using default configuration\n")
		cfg = config.DefaultConfig()
	}

	// Create registry and populate with configured sessions
	reg := session.NewRegistry()
	for _, sess := range cfg.AllSessions() {
		if err := reg.Create(sess.Name, sess.Command); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating session %q: %v\n", sess.Name, err)
		}
	}

	return model{
		config:    cfg,
		registry:  reg,
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
		return m, tickCmd
	}
	return m, nil
}

func (m model) updateHome(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "ctrl+c", "q":
		return m, tea.Quit
	}

	// Check if key matches any configured session
	for _, sess := range m.config.AllSessions() {
		if sess.Key == key {
			// Start session if not running
			manager, err := m.registry.Get(sess.Name)
			if err != nil {
				// Session not in registry, skip
				continue
			}

			if !manager.IsRunning() {
				if err := m.registry.Start(sess.Name); err != nil {
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

func (m model) updateAttached(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// This view state is no longer used in Phase 3
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

	title := titleStyle.Render("🤖 Welcome to PocketBot!")

	// Build status lines for all sessions
	var sb strings.Builder
	for _, sess := range m.config.AllSessions() {
		manager, err := m.registry.Get(sess.Name)
		if err != nil {
			continue
		}

		var status string
		if manager.IsRunning() {
			activityState := manager.GetActivityState()
			if activityState == session.StateActive {
				status = runningStyle.Render("● active")
			} else {
				status = runningStyle.Render("● idle")
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
	instructions := instructionStyle.Render("Press key to start/attach • Ctrl+C to quit")

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
	m := initialModel()

	// Ensure session cleanup on exit
	defer func() {
		if err := m.registry.StopAll(); err != nil {
			fmt.Fprintf(os.Stderr, "Error stopping sessions: %v\n", err)
		}
	}()

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

		// Attach to requested session
		// Note: No screen clearing needed - alternate screen handles separation
		result, err := m.registry.Attach(m.sessionToAttach)

		if err != nil {
			fmt.Fprintf(os.Stderr, "Attach error: %v\n", err)
			// Continue to show UI
			continue
		}

		// Handle attach result
		switch result {
		case session.AttachDetached:
			// User pressed Ctrl+D, return to home screen
			continue
		case session.AttachExited:
			// Session exited, return to home screen
			continue
		}
	}
}

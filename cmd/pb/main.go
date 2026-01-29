package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/zakandrewking/pocketbot/internal/session"
)

type viewState int

const (
	viewHome viewState = iota
	viewAttached
)

type attachMsg struct{}

type model struct {
	session      *session.Manager
	viewState    viewState
	shouldAttach bool
}

func initialModel() model {
	return model{
		session:   session.New(),
		viewState: viewHome,
	}
}

func (m model) Init() tea.Cmd {
	return nil
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
	}
	return m, nil
}

func (m model) updateHome(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "c":
		// Start session if not running
		if !m.session.IsRunning() {
			if err := m.session.Start(); err != nil {
				// For now, just ignore errors
				return m, nil
			}
		}
		// Signal that we want to attach
		m.shouldAttach = true
		return m, tea.Quit // Exit Bubble Tea to attach
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

	subtitleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#04B575"))

	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFA500"))

	title := titleStyle.Render("🤖 Welcome to PocketBot!")

	var instructions string
	if m.session.IsRunning() {
		status := statusStyle.Render("● Claude is running")
		instructions = subtitleStyle.Render("Press 'c' to attach, Ctrl+C to quit.")
		return fmt.Sprintf("\n%s\n\n%s\n\n%s\n\n", title, status, instructions)
	} else {
		instructions = subtitleStyle.Render("Press 'c' to start Claude, Ctrl+C to quit.")
		return fmt.Sprintf("\n%s\n\n%s\n\n", title, instructions)
	}
}

func (m model) viewAttached() string {
	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888")).
		Italic(true)

	help := helpStyle.Render("[Press Ctrl+P to detach]")

	// In Phase 3, we'll actually show Claude's output here
	// For now, just show a placeholder
	return fmt.Sprintf("%s\n\n[Attached to Claude - I/O forwarding not yet implemented]\n", help)
}

func main() {
	m := initialModel()

	// Ensure session cleanup on exit
	defer func() {
		if err := m.session.Stop(); err != nil {
			fmt.Fprintf(os.Stderr, "Error stopping session: %v\n", err)
		}
	}()

	// Main loop: run UI, attach when requested, repeat
	for {
		m.shouldAttach = false
		m.viewState = viewHome

		// Run Bubble Tea UI
		p := tea.NewProgram(m)
		finalModel, err := p.Run()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		// Get the final model state
		m = finalModel.(model)

		// Check if we should attach
		if !m.shouldAttach {
			// User quit normally
			break
		}

		// Attach to Claude session
		result, err := m.session.Attach()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Attach error: %v\n", err)
			// Continue to show UI
			continue
		}

		// Handle attach result
		switch result {
		case session.AttachDetached:
			// User pressed Ctrl+P, return to home screen
			continue
		case session.AttachExited:
			// Claude exited, return to home screen
			continue
		}
	}
}

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

type model struct {
	session   *session.Manager
	viewState viewState
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
		// Start or attach to Claude session
		if !m.session.IsRunning() {
			if err := m.session.Start(); err != nil {
				// For now, just ignore errors (we'll add better error handling later)
				return m, nil
			}
		}
		// Switch to attached view
		m.viewState = viewAttached
		return m, nil
	}
	return m, nil
}

func (m model) updateAttached(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+p":
		// Detach from session and return to home
		m.viewState = viewHome
		return m, nil
	}
	// In Phase 3, we'll forward all other keys to Claude
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

	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}
}

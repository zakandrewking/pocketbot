package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/zakandrewking/pocketbot/internal/session"
)

type model struct {
	session *session.Manager
}

func initialModel() model {
	return model{
		session: session.New(),
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m model) View() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D56F4")).
		MarginTop(1).
		MarginBottom(1)

	subtitleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#04B575"))

	title := titleStyle.Render("🤖 Welcome to PocketBot!")
	subtitle := subtitleStyle.Render("Press Ctrl+C to quit.")

	return fmt.Sprintf("\n%s\n\n%s\n\n", title, subtitle)
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

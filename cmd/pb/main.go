package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type model struct{}

func initialModel() model {
	return model{}
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
	p := tea.NewProgram(initialModel())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}
}

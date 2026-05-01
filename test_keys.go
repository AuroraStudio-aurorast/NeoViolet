package main

import (
	"fmt"
	
	tea "charm.land/bubbletea/v2"
)

type model struct{}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		fmt.Printf("KeyPressMsg received: String=%q\n", msg.String())
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m model) View() tea.View {
	return tea.NewView("Press Ctrl+C to quit. Press any key to see its name.")
}

func main() {
	program := tea.NewProgram(model{})
	if _, err := program.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}
package tui

import (
	"fmt"

	"bitbucket-cli/internal/config"

	tea "github.com/charmbracelet/bubbletea"
)

type Model struct {
	profiles       []string
	cursor         int
	selected       string
	configFile     *config.ConfigFile
	shouldQuit     bool
	selectedConfig config.Config
}

func NewWorkspaceSelector(cfg *config.ConfigFile) Model {
	return Model{
		profiles:   cfg.ListProfiles(),
		cursor:     0,
		configFile: cfg,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.shouldQuit = true
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			if m.cursor < len(m.profiles)-1 {
				m.cursor++
			}

		case "enter":
			m.selected = m.profiles[m.cursor]
			profile, err := m.configFile.GetProfile(m.selected)
			if err == nil {
				m.selectedConfig = config.FromProfile(profile)
			}
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m Model) View() string {
	if m.shouldQuit {
		return ""
	}

	s := "Select a workspace:\n\n"

	for i, profile := range m.profiles {
		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}

		s += fmt.Sprintf("%s %s\n", cursor, profile)
	}

	s += "\nPress 'q' to quit\n"
	return s
}

func (m Model) SelectedConfig() config.Config {
	return m.selectedConfig
}

func (m Model) WasQuit() bool {
	return m.shouldQuit && m.selected == ""
}

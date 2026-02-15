package main

import (
	"fmt"
	"os"

	"bitbucket-cli/internal/config"
	"bitbucket-cli/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	configFile, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		fmt.Fprintf(os.Stderr, "Make sure ~/.config/bitbucket-cli/config exists\n")
		os.Exit(1)
	}

	var selectedWorkspace string
	var selectedConfig config.Config

	defaultProfile, err := configFile.GetDefaultProfile()
	if err == nil {
		selectedWorkspace = defaultProfile.Workspace
		selectedConfig = config.FromProfile(defaultProfile)
	} else {
		m := tui.NewWorkspaceSelector(configFile)
		p := tea.NewProgram(m)
		finalModel, err := p.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error running workspace selector: %v\n", err)
			os.Exit(1)
		}

		model := finalModel.(tui.Model)
		if model.WasQuit() {
			fmt.Println("Cancelled")
			os.Exit(0)
		}

		selectedWorkspace = model.SelectedConfig().Workspace
		selectedConfig = model.SelectedConfig()
	}

	app := tui.NewApp(selectedWorkspace, selectedConfig)
	p := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running app: %v\n", err)
		os.Exit(1)
	}
}

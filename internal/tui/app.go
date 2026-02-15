package tui

import (
	"fmt"
	"strings"

	"bitbucket-cli/internal/bitbucket"
	"bitbucket-cli/internal/config"
	"bitbucket-cli/internal/domain"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type pane int

const (
	repoPane pane = iota
	branchPane
)

var (
	activePaneStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42")).
			Bold(true)

	inactivePaneStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("241"))

	cursorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("205"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("63")).
			Padding(1, 2)

	messageStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("211")).
			Bold(true)
)

type AppModel struct {
	workspace         string
	client            *bitbucket.Client
	activePane        pane
	repositories      []domain.Repository
	branches          []domain.Branch
	repoCursor        int
	branchCursor      int
	width             int
	height            int
	loading           bool
	message           string
	selectedRepo      string
	filterMode        bool
	repoFilterQuery   string
	branchFilterQuery string
}

type reposLoadedMsg struct {
	repos []domain.Repository
	err   error
}

type branchesLoadedMsg struct {
	branches []domain.Branch
	err      error
}

func NewApp(workspace string, cfg config.Config) AppModel {
	return AppModel{
		workspace:  workspace,
		client:     bitbucket.NewClient(cfg),
		activePane: repoPane,
		loading:    true,
	}
}

func (m AppModel) Init() tea.Cmd {
	return loadRepositories(m.client)
}

func loadRepositories(client *bitbucket.Client) tea.Cmd {
	return func() tea.Msg {
		repos, err := client.ListRepositories()
		return reposLoadedMsg{repos: repos, err: err}
	}
}

func loadBranches(client *bitbucket.Client, repoSlug string) tea.Cmd {
	return func() tea.Msg {
		branches, err := client.ListBranches(repoSlug)
		return branchesLoadedMsg{branches: branches, err: err}
	}
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case reposLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.message = fmt.Sprintf("Error loading repos: %v", msg.err)
		} else {
			m.repositories = msg.repos
			m.message = ""
		}

	case branchesLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.message = fmt.Sprintf("Error loading branches: %v", msg.err)
		} else {
			m.branches = msg.branches
			m.branchCursor = 0
			m.message = ""
		}

	case tea.KeyMsg:
		m.message = ""

		if m.filterMode {
			currentFilter := &m.repoFilterQuery
			currentCursor := &m.repoCursor
			if m.activePane == branchPane {
				currentFilter = &m.branchFilterQuery
				currentCursor = &m.branchCursor
			}

			switch msg.String() {
			case "esc":
				m.filterMode = false
				*currentFilter = ""
				*currentCursor = 0

			case "enter":
				m.filterMode = false

			case "backspace":
				if len(*currentFilter) > 0 {
					*currentFilter = (*currentFilter)[:len(*currentFilter)-1]
					*currentCursor = 0
				}

			default:
				if len(msg.String()) == 1 {
					*currentFilter += msg.String()
					*currentCursor = 0
				}
			}
			return m, nil
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "/":
			m.filterMode = true

		case "h":
			if !m.filterMode {
				m.activePane = repoPane
			}

		case "l":
			if !m.filterMode && m.activePane == repoPane && len(m.getFilteredRepos()) > 0 {
				m.activePane = branchPane
				m.loading = true
				m.branches = nil
				m.branchFilterQuery = ""
				m.branchCursor = 0
				repos := m.getFilteredRepos()
				repo := repos[m.repoCursor]
				m.selectedRepo = repo.Name
				return m, loadBranches(m.client, repo.Slug)
			}

		case "j", "down":
			if !m.filterMode {
				if m.activePane == repoPane {
					filtered := m.getFilteredRepos()
					if m.repoCursor < len(filtered)-1 {
						m.repoCursor++
					}
				} else {
					filtered := m.getFilteredBranches()
					if m.branchCursor < len(filtered)-1 {
						m.branchCursor++
					}
				}
			}

		case "k", "up":
			if !m.filterMode {
				if m.activePane == repoPane {
					if m.repoCursor > 0 {
						m.repoCursor--
					}
				} else {
					if m.branchCursor > 0 {
						m.branchCursor--
					}
				}
			}

		case "p":
			if !m.filterMode && m.activePane == branchPane && len(m.getFilteredBranches()) > 0 {
				filtered := m.getFilteredBranches()
				selectedBranch := filtered[m.branchCursor].Name
				m.message = fmt.Sprintf("Creating pull request for %s!", selectedBranch)
			}
		}
	}

	return m, nil
}

func (m AppModel) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	leftPane := m.renderRepoPane()
	rightPane := m.renderBranchPane()

	content := lipgloss.JoinHorizontal(
		lipgloss.Top,
		leftPane,
		rightPane,
	)

	helpText := "h/l: switch panes  j/k/↑/↓: navigate  l: load branches  /: filter  p: create PR  q: quit"
	if m.filterMode {
		currentFilter := m.repoFilterQuery
		if m.activePane == branchPane {
			currentFilter = m.branchFilterQuery
		}
		helpText = fmt.Sprintf("Filter: %s  (esc: cancel, enter: apply)", currentFilter)
		helpText = activePaneStyle.Render(helpText)
	} else if m.message != "" {
		helpText = messageStyle.Render(m.message)
	}

	fullContent := lipgloss.JoinVertical(
		lipgloss.Left,
		content,
		"",
		helpStyle.Render(helpText),
	)

	return fullContent
}

func (m AppModel) renderRepoPane() string {
	paneWidth := (m.width - 10) / 2
	if paneWidth < 20 {
		paneWidth = 20
	}

	availableHeight := m.height - 6
	if availableHeight < 5 {
		availableHeight = 5
	}

	title := "Repositories"
	if m.repoFilterQuery != "" {
		title = fmt.Sprintf("Repositories [/%s]", m.repoFilterQuery)
	}
	if m.activePane == repoPane {
		title = activePaneStyle.Render(title)
	} else {
		title = inactivePaneStyle.Render(title)
	}

	var items []string
	items = append(items, title)
	items = append(items, "")

	if m.loading && len(m.repositories) == 0 {
		items = append(items, "Loading...")
	} else if len(m.repositories) == 0 {
		items = append(items, "No repositories")
	} else {
		filtered := m.getFilteredRepos()
		if len(filtered) == 0 {
			items = append(items, "No matches")
		} else {
			start, end := m.calculateWindow(m.repoCursor, len(filtered), availableHeight-2)

			for i := start; i < end; i++ {
				repo := filtered[i]
				cursor := " "
				if m.activePane == repoPane && i == m.repoCursor {
					cursor = cursorStyle.Render(">")
				}
				items = append(items, fmt.Sprintf("%s %s", cursor, repo.Name))
			}

			if start > 0 {
				items[1] = inactivePaneStyle.Render("  ↑ more")
			}
			if end < len(filtered) {
				items = append(items, inactivePaneStyle.Render("  ↓ more"))
			}
		}
	}

	content := strings.Join(items, "\n")
	style := lipgloss.NewStyle().
		Width(paneWidth).
		Height(availableHeight).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("63")).
		Padding(0, 1)

	return style.Render(content)
}

func (m AppModel) renderBranchPane() string {
	paneWidth := (m.width - 10) / 2
	if paneWidth < 20 {
		paneWidth = 20
	}

	availableHeight := m.height - 6
	if availableHeight < 5 {
		availableHeight = 5
	}

	title := "Branches"
	if m.selectedRepo != "" {
		title = fmt.Sprintf("Branches (%s)", m.selectedRepo)
	}
	if m.branchFilterQuery != "" {
		title = fmt.Sprintf("Branches [/%s]", m.branchFilterQuery)
	}

	if m.activePane == branchPane {
		title = activePaneStyle.Render(title)
	} else {
		title = inactivePaneStyle.Render(title)
	}

	var items []string
	items = append(items, title)
	items = append(items, "")

	if m.loading && m.activePane == branchPane {
		items = append(items, "Loading...")
	} else if len(m.branches) == 0 {
		items = append(items, "← Select a repo")
	} else {
		filtered := m.getFilteredBranches()
		if len(filtered) == 0 {
			items = append(items, "No matches")
		} else {
			start, end := m.calculateWindow(m.branchCursor, len(filtered), availableHeight-2)

			for i := start; i < end; i++ {
				branch := filtered[i]
				cursor := " "
				if m.activePane == branchPane && i == m.branchCursor {
					cursor = cursorStyle.Render(">")
				}
				items = append(items, fmt.Sprintf("%s %s", cursor, branch.Name))
			}

			if start > 0 {
				items[1] = inactivePaneStyle.Render("  ↑ more")
			}
			if end < len(filtered) {
				items = append(items, inactivePaneStyle.Render("  ↓ more"))
			}
		}
	}

	content := strings.Join(items, "\n")
	style := lipgloss.NewStyle().
		Width(paneWidth).
		Height(availableHeight).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("63")).
		Padding(0, 1)

	return style.Render(content)
}

func (m AppModel) getFilteredRepos() []domain.Repository {
	if m.repoFilterQuery == "" {
		return m.repositories
	}

	var filtered []domain.Repository
	query := strings.ToLower(m.repoFilterQuery)
	for _, repo := range m.repositories {
		if strings.Contains(strings.ToLower(repo.Name), query) ||
			strings.Contains(strings.ToLower(repo.Slug), query) {
			filtered = append(filtered, repo)
		}
	}
	return filtered
}

func (m AppModel) getFilteredBranches() []domain.Branch {
	if m.branchFilterQuery == "" {
		return m.branches
	}

	var filtered []domain.Branch
	query := strings.ToLower(m.branchFilterQuery)
	for _, branch := range m.branches {
		if strings.Contains(strings.ToLower(branch.Name), query) {
			filtered = append(filtered, branch)
		}
	}
	return filtered
}

func (m AppModel) calculateWindow(cursor, total, height int) (int, int) {
	if total <= height {
		return 0, total
	}

	half := height / 2
	start := cursor - half
	end := cursor + half

	if start < 0 {
		start = 0
		end = height
	}

	if end > total {
		end = total
		start = total - height
	}

	if start < 0 {
		start = 0
	}

	return start, end
}

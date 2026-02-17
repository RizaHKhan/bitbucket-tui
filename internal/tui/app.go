package tui

import (
	"fmt"
	"os/exec"
	"runtime"
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
	prPane
)

type viewMode int

const (
	noSelection viewMode = iota
	branchesView
	prView
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
	currentView       viewMode
	repositories      []domain.Repository
	branches          []domain.Branch
	pullRequests      []domain.PullRequest
	repoCursor        int
	branchCursor      int
	prCursor          int
	width             int
	height            int
	loading           bool
	message           string
	selectedRepo      string
	selectedRepoSlug  string
	filterMode        bool
	repoFilterQuery   string
	branchFilterQuery string
	prFilterQuery     string
}

type reposLoadedMsg struct {
	repos []domain.Repository
	err   error
}

type branchesLoadedMsg struct {
	branches []domain.Branch
	err      error
}

type pullRequestsLoadedMsg struct {
	prs []domain.PullRequest
	err error
}

func NewApp(workspace string, cfg config.Config) AppModel {
	return AppModel{
		workspace:   workspace,
		client:      bitbucket.NewClient(cfg),
		activePane:  repoPane,
		currentView: noSelection,
		loading:     true,
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

func loadPullRequests(client *bitbucket.Client, repoSlug string) tea.Cmd {
	return func() tea.Msg {
		prs, err := client.ListPullRequests(repoSlug)
		return pullRequestsLoadedMsg{prs: prs, err: err}
	}
}

func openURL(url string) tea.Cmd {
	return func() tea.Msg {
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "linux":
			cmd = exec.Command("xdg-open", url)
		case "darwin":
			cmd = exec.Command("open", url)
		case "windows":
			cmd = exec.Command("cmd", "/c", "start", url)
		default:
			return nil
		}
		_ = cmd.Start()
		return nil
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

	case pullRequestsLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.message = fmt.Sprintf("Error loading pull requests: %v", msg.err)
		} else {
			m.pullRequests = msg.prs
			m.prCursor = 0
			m.message = ""
		}

	case tea.KeyMsg:
		m.message = ""

		if m.filterMode {
			currentFilter := &m.repoFilterQuery
			currentCursor := &m.repoCursor
			if m.activePane == branchPane {
				if m.currentView == branchesView {
					currentFilter = &m.branchFilterQuery
					currentCursor = &m.branchCursor
				} else if m.currentView == prView {
					currentFilter = &m.prFilterQuery
					currentCursor = &m.prCursor
				}
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

		case "esc":
			if m.activePane == branchPane {
				m.activePane = repoPane
			}

		case "/":
			m.filterMode = true

		case "enter":
			if !m.filterMode && m.activePane == repoPane && len(m.getFilteredRepos()) > 0 {
				m.currentView = prView
				m.activePane = branchPane
				m.loading = true
				m.pullRequests = nil
				m.prFilterQuery = ""
				m.prCursor = 0
				repos := m.getFilteredRepos()
				repo := repos[m.repoCursor]
				m.selectedRepo = repo.Name
				m.selectedRepoSlug = repo.Slug
				return m, loadPullRequests(m.client, repo.Slug)
			}

		case "h":
			if !m.filterMode && m.activePane == branchPane && m.currentView == branchesView && m.selectedRepoSlug != "" {
				m.currentView = prView
				m.loading = true
				m.pullRequests = nil
				m.prFilterQuery = ""
				m.prCursor = 0
				return m, loadPullRequests(m.client, m.selectedRepoSlug)
			}

		case "l":
			if !m.filterMode && m.activePane == repoPane && m.currentView != noSelection {
				m.activePane = branchPane
			} else if !m.filterMode && m.activePane == branchPane && m.currentView == prView && m.selectedRepoSlug != "" {
				m.currentView = branchesView
				m.loading = true
				m.branches = nil
				m.branchFilterQuery = ""
				m.branchCursor = 0
				return m, loadBranches(m.client, m.selectedRepoSlug)
			}

		case "b":
			if !m.filterMode && m.activePane == repoPane && len(m.getFilteredRepos()) > 0 {
				m.currentView = branchesView
				m.activePane = branchPane
				m.loading = true
				m.branches = nil
				m.branchFilterQuery = ""
				m.branchCursor = 0
				repos := m.getFilteredRepos()
				repo := repos[m.repoCursor]
				m.selectedRepo = repo.Name
				m.selectedRepoSlug = repo.Slug
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
					if m.currentView == branchesView {
						filtered := m.getFilteredBranches()
						if m.branchCursor < len(filtered)-1 {
							m.branchCursor++
						}
					} else if m.currentView == prView {
						filtered := m.getFilteredPRs()
						if m.prCursor < len(filtered)-1 {
							m.prCursor++
						}
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
					if m.currentView == branchesView {
						if m.branchCursor > 0 {
							m.branchCursor--
						}
					} else if m.currentView == prView {
						if m.prCursor > 0 {
							m.prCursor--
						}
					}
				}
			}

		case "p":
			if !m.filterMode && m.activePane == repoPane && len(m.getFilteredRepos()) > 0 {
				m.currentView = prView
				m.activePane = branchPane
				m.loading = true
				m.pullRequests = nil
				m.prFilterQuery = ""
				m.prCursor = 0
				repos := m.getFilteredRepos()
				repo := repos[m.repoCursor]
				m.selectedRepo = repo.Name
				m.selectedRepoSlug = repo.Slug
				return m, loadPullRequests(m.client, repo.Slug)
			}

		case "o":
			if !m.filterMode && m.activePane == branchPane && m.currentView == prView && len(m.getFilteredPRs()) > 0 {
				filtered := m.getFilteredPRs()
				selectedPR := filtered[m.prCursor]
				if selectedPR.URL != "" {
					return m, openURL(selectedPR.URL)
				}
			}
		}
	}

	return m, nil
}

func (m AppModel) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	showRepoPane := m.currentView == noSelection || m.activePane == repoPane

	var content string
	if showRepoPane {
		leftPane := m.renderRepoPane()

		var rightPane string
		if m.currentView == noSelection {
			rightPane = ""
		} else {
			rightPane = m.renderRightPane()
		}

		content = lipgloss.JoinHorizontal(
			lipgloss.Top,
			leftPane,
			rightPane,
		)
	} else {
		content = m.renderRightPane()
	}

	helpText := "j/k/↑/↓: navigate  enter: select repo  /: filter  q: quit"
	if m.currentView != noSelection {
		helpText = "h/l: switch tabs  esc: back  j/k/↑/↓: navigate  /: filter  q: quit"
	}
	if m.currentView == prView && m.activePane == branchPane {
		helpText = "h/l: switch tabs  esc: back  j/k/↑/↓: navigate  o: open in browser  /: filter  q: quit"
	}
	if m.filterMode {
		currentFilter := m.repoFilterQuery
		if m.activePane == branchPane {
			if m.currentView == branchesView {
				currentFilter = m.branchFilterQuery
			} else if m.currentView == prView {
				currentFilter = m.prFilterQuery
			}
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

func (m AppModel) renderRightPane() string {
	if m.currentView == branchesView {
		return m.renderBranchPane()
	} else if m.currentView == prView {
		return m.renderPRPane()
	}
	return ""
}

func (m AppModel) renderRightTabs() string {
	baseTab := lipgloss.NewStyle().Padding(0, 2)

	activeTab := baseTab.
		Foreground(lipgloss.Color("0")).
		Background(lipgloss.Color("42")).
		Bold(true)

	inactiveTab := baseTab.
		Foreground(lipgloss.Color("241"))

	prsTab := inactiveTab.Render("Pull Requests")
	branchesTab := inactiveTab.Render("Branches")

	if m.currentView == prView {
		prsTab = activeTab.Render("Pull Requests")
	} else if m.currentView == branchesView {
		branchesTab = activeTab.Render("Branches")
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, prsTab, branchesTab)
}

func (m AppModel) renderRepoPane() string {
	paneWidth := (m.width - 10) / 3
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
	showRepoPane := m.currentView == noSelection || m.activePane == repoPane

	paneWidth := m.width - 4
	if showRepoPane {
		repoPaneWidth := (m.width - 10) / 3
		if repoPaneWidth < 20 {
			repoPaneWidth = 20
		}
		paneWidth = m.width - repoPaneWidth - 10
	}
	if paneWidth < 30 {
		paneWidth = 30
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
	if !showRepoPane {
		title = fmt.Sprintf("%s (esc: back)", title)
	}

	if m.activePane == branchPane {
		title = activePaneStyle.Render(title)
	} else {
		title = inactivePaneStyle.Render(title)
	}

	var items []string
	items = append(items, m.renderRightTabs())
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
			start, end := m.calculateWindow(m.branchCursor, len(filtered), availableHeight-3)

			for i := start; i < end; i++ {
				branch := filtered[i]
				cursor := " "
				if m.activePane == branchPane && i == m.branchCursor {
					cursor = cursorStyle.Render(">")
				}
				items = append(items, fmt.Sprintf("%s %s", cursor, branch.Name))
			}

			if start > 0 {
				items[2] = inactivePaneStyle.Render("  ↑ more")
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

func (m AppModel) renderPRPane() string {
	showRepoPane := m.currentView == noSelection || m.activePane == repoPane

	paneWidth := m.width - 4
	if showRepoPane {
		repoPaneWidth := (m.width - 10) / 3
		if repoPaneWidth < 20 {
			repoPaneWidth = 20
		}
		paneWidth = m.width - repoPaneWidth - 10
	}
	if paneWidth < 30 {
		paneWidth = 30
	}

	availableHeight := m.height - 6
	if availableHeight < 5 {
		availableHeight = 5
	}

	title := "Pull Requests"
	if m.selectedRepo != "" {
		title = fmt.Sprintf("Pull Requests (%s)", m.selectedRepo)
	}
	if m.prFilterQuery != "" {
		title = fmt.Sprintf("Pull Requests [/%s]", m.prFilterQuery)
	}
	if !showRepoPane {
		title = fmt.Sprintf("%s (esc: back)", title)
	}

	if m.activePane == branchPane {
		title = activePaneStyle.Render(title)
	} else {
		title = inactivePaneStyle.Render(title)
	}

	var items []string
	items = append(items, m.renderRightTabs())
	items = append(items, title)
	items = append(items, "")

	if m.loading && m.activePane == branchPane && m.currentView == prView {
		items = append(items, "Loading...")
	} else if len(m.pullRequests) == 0 {
		items = append(items, "No pull requests")
	} else {
		filtered := m.getFilteredPRs()
		if len(filtered) == 0 {
			items = append(items, "No matches")
		} else {
			start, end := m.calculateWindow(m.prCursor, len(filtered), availableHeight-3)

			for i := start; i < end; i++ {
				pr := filtered[i]
				cursor := " "
				if m.activePane == branchPane && i == m.prCursor {
					cursor = cursorStyle.Render(">")
				}

				stateBadge := formatPRState(pr.State, pr.Draft)

				authorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("99"))
				author := authorStyle.Render(fmt.Sprintf("@%s", pr.Author))

				const cursorIDStateAuthorPadding = 35
				maxTitleWidth := paneWidth - cursorIDStateAuthorPadding - len(pr.Author)
				prTitle := pr.Title
				if len(prTitle) > maxTitleWidth {
					prTitle = prTitle[:maxTitleWidth-3] + "..."
				}

				items = append(items, fmt.Sprintf("%s #%d %s %s %s", cursor, pr.ID, stateBadge, author, prTitle))
			}

			if start > 0 {
				items[2] = inactivePaneStyle.Render("  ↑ more")
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

func formatPRState(state string, draft bool) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "open":
		if draft {
			return lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("[DRAFT]")
		}
		return lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("[OPEN]")
	case "merged":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("99")).Render("[MERGED]")
	case "declined":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("[DECLINED]")
	case "superseded":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("[SUPERSEDED]")
	default:
		return fmt.Sprintf("[%s]", strings.ToUpper(state))
	}
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

func (m AppModel) getFilteredPRs() []domain.PullRequest {
	if m.prFilterQuery == "" {
		return m.pullRequests
	}

	var filtered []domain.PullRequest
	query := strings.ToLower(m.prFilterQuery)
	for _, pr := range m.pullRequests {
		if strings.Contains(strings.ToLower(pr.Title), query) ||
			strings.Contains(strings.ToLower(pr.Author), query) ||
			strings.Contains(strings.ToLower(pr.SourceBranch), query) {
			filtered = append(filtered, pr)
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

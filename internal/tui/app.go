package tui

import (
	"fmt"
	"hash/fnv"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"bitbucket-cli/internal/bitbucket"
	"bitbucket-cli/internal/config"
	"bitbucket-cli/internal/domain"

	"github.com/charmbracelet/bubbles/spinner"
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
	pipelinesView
	pipelineStepsView
	pipelineStepLogView
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
	workspace             string
	client                *bitbucket.Client
	spinner               spinner.Model
	activePane            pane
	currentView           viewMode
	repositories          []domain.Repository
	branches              []domain.Branch
	pullRequests          []domain.PullRequest
	pipelines             []domain.Pipeline
	pipelineSteps         []domain.PipelineStep
	pipelineStepLog       string
	pipelineStepLogLines  []string
	repoCursor            int
	branchCursor          int
	prCursor              int
	pipelineCursor        int
	pipelineStepCursor    int
	pipelineStepLogCursor int
	width                 int
	height                int
	loading               bool
	message               string
	selectedRepo          string
	selectedRepoSlug      string
	selectedPipelineRef   string
	selectedPipelineUUID  string
	selectedStepName      string
	filterMode            bool
	repoFilterQuery       string
	branchFilterQuery     string
	prFilterQuery         string
	pipelineFilterQuery   string
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

type pipelinesLoadedMsg struct {
	pipelines []domain.Pipeline
	err       error
}

type pipelineStepsLoadedMsg struct {
	steps []domain.PipelineStep
	err   error
}

type pipelineStepLogLoadedMsg struct {
	log string
	err error
}

type editorClosedMsg struct {
	err error
}

type urlOpenedMsg struct {
	err error
}

type pipelinePollTickMsg struct{}

type pipelinePolledMsg struct {
	pipeline domain.Pipeline
	err      error
}

const pipelinePollInterval = 8 * time.Second

func NewApp(workspace string, cfg config.Config) AppModel {
	s := spinner.New()
	s.Spinner = spinner.MiniDot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	return AppModel{
		workspace:   workspace,
		client:      bitbucket.NewClient(cfg),
		spinner:     s,
		activePane:  repoPane,
		currentView: noSelection,
		loading:     true,
	}
}

func (m AppModel) Init() tea.Cmd {
	return tea.Batch(loadRepositories(m.client), m.spinner.Tick)
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

func loadPipelines(client *bitbucket.Client, repoSlug string) tea.Cmd {
	return func() tea.Msg {
		pipelines, err := client.ListPipelines(repoSlug)
		return pipelinesLoadedMsg{pipelines: pipelines, err: err}
	}
}

func pollPipelineUpdates() tea.Cmd {
	return tea.Tick(pipelinePollInterval, func(time.Time) tea.Msg {
		return pipelinePollTickMsg{}
	})
}

func loadPipeline(client *bitbucket.Client, repoSlug, pipelineUUID string) tea.Cmd {
	return func() tea.Msg {
		pipeline, err := client.GetPipeline(repoSlug, pipelineUUID)
		return pipelinePolledMsg{pipeline: pipeline, err: err}
	}
}

func loadPipelineSteps(client *bitbucket.Client, repoSlug, pipelineUUID string) tea.Cmd {
	return func() tea.Msg {
		steps, err := client.ListPipelineSteps(repoSlug, pipelineUUID)
		return pipelineStepsLoadedMsg{steps: steps, err: err}
	}
}

func loadPipelineStepLog(client *bitbucket.Client, repoSlug, pipelineUUID, stepUUID string) tea.Cmd {
	return func() tea.Msg {
		log, err := client.GetPipelineStepLog(repoSlug, pipelineUUID, stepUUID)
		return pipelineStepLogLoadedMsg{log: log, err: err}
	}
}

func openURL(url string) tea.Cmd {
	return func() tea.Msg {
		var commands [][]string
		switch runtime.GOOS {
		case "linux":
			commands = [][]string{
				{"xdg-open", url},
				{"gio", "open", url},
				{"wslview", url},
				{"cmd.exe", "/c", "start", "", url},
				{"powershell.exe", "-NoProfile", "-Command", "Start-Process", url},
			}
		case "darwin":
			commands = [][]string{{"open", url}}
		case "windows":
			commands = [][]string{{"cmd", "/c", "start", "", url}}
		default:
			return urlOpenedMsg{err: fmt.Errorf("opening URLs is not supported on %s", runtime.GOOS)}
		}

		var lastErr error
		for _, parts := range commands {
			if _, err := exec.LookPath(parts[0]); err != nil {
				lastErr = err
				continue
			}

			cmd := exec.Command(parts[0], parts[1:]...)
			output, err := cmd.CombinedOutput()
			if err != nil {
				trimmedOutput := strings.TrimSpace(string(output))
				if trimmedOutput != "" {
					lastErr = fmt.Errorf("%s failed: %w (%s)", parts[0], err, trimmedOutput)
				} else {
					lastErr = fmt.Errorf("%s failed: %w", parts[0], err)
				}
				continue
			}

			return urlOpenedMsg{}
		}

		if lastErr == nil {
			lastErr = fmt.Errorf("no URL opener found")
		}

		return urlOpenedMsg{err: lastErr}
	}
}

func openLogInEditor(logContent, stepName string) tea.Cmd {
	content := logContent
	if strings.TrimSpace(content) == "" {
		content = "No log output returned for this step."
	}

	title := "pipeline-log"
	if strings.TrimSpace(stepName) != "" {
		title = strings.ReplaceAll(strings.TrimSpace(stepName), " ", "-")
	}

	tmpFile, err := os.CreateTemp("", fmt.Sprintf("bb-%s-*.log", title))
	if err != nil {
		return func() tea.Msg { return editorClosedMsg{err: err} }
	}

	filePath := tmpFile.Name()
	if _, writeErr := tmpFile.WriteString(content); writeErr != nil {
		_ = tmpFile.Close()
		_ = os.Remove(filePath)
		return func() tea.Msg { return editorClosedMsg{err: writeErr} }
	}
	_ = tmpFile.Close()

	var cmd *exec.Cmd
	if _, lookErr := exec.LookPath("nvim"); lookErr == nil {
		cmd = exec.Command("nvim", filePath)
	} else if _, lookErr := exec.LookPath("less"); lookErr == nil {
		cmd = exec.Command("less", filePath)
	} else {
		_ = os.Remove(filePath)
		return func() tea.Msg { return editorClosedMsg{err: fmt.Errorf("neither nvim nor less is installed")} }
	}

	return tea.ExecProcess(cmd, func(execErr error) tea.Msg {
		_ = os.Remove(filePath)
		return editorClosedMsg{err: execErr}
	})
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

	case pipelinesLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.message = fmt.Sprintf("Error loading pipelines: %v", msg.err)
		} else {
			previousCursor := m.pipelineCursor
			m.pipelines = msg.pipelines
			if len(m.pipelines) == 0 {
				m.pipelineCursor = 0
			} else if previousCursor >= 0 && previousCursor < len(m.pipelines) {
				m.pipelineCursor = previousCursor
			} else {
				m.pipelineCursor = len(m.pipelines) - 1
			}
			m.message = ""

			if m.activePane == branchPane && m.currentView == pipelinesView && selectedRunningPipelineUUID(m) != "" {
				return m, pollPipelineUpdates()
			}
		}

	case pipelinePollTickMsg:
		if m.activePane == branchPane && m.currentView == pipelinesView && m.selectedRepoSlug != "" {
			pipelineUUID := selectedRunningPipelineUUID(m)
			if pipelineUUID != "" {
				return m, loadPipeline(m.client, m.selectedRepoSlug, pipelineUUID)
			}
		}

	case pipelinePolledMsg:
		if msg.err != nil {
			m.message = fmt.Sprintf("Error polling pipeline: %v", msg.err)
			if m.activePane == branchPane && m.currentView == pipelinesView && selectedRunningPipelineUUID(m) != "" {
				return m, pollPipelineUpdates()
			}
			break
		}

		for i := range m.pipelines {
			if m.pipelines[i].UUID == msg.pipeline.UUID {
				m.pipelines[i] = msg.pipeline
				break
			}
		}

		if m.activePane == branchPane && m.currentView == pipelinesView && isPipelineRunning(msg.pipeline) {
			return m, pollPipelineUpdates()
		}

	case pipelineStepsLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.message = fmt.Sprintf("Error loading pipeline steps: %v", msg.err)
		} else {
			m.pipelineSteps = msg.steps
			m.pipelineStepCursor = 0
			m.message = ""
		}

	case pipelineStepLogLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.message = fmt.Sprintf("Error loading pipeline log: %v", msg.err)
		} else {
			m.pipelineStepLog = msg.log
			if strings.TrimSpace(msg.log) == "" {
				m.pipelineStepLogLines = []string{"No log output returned for this step."}
			} else {
				m.pipelineStepLogLines = strings.Split(msg.log, "\n")
			}
			m.pipelineStepLogCursor = 0
			m.message = ""
		}

	case editorClosedMsg:
		if msg.err != nil {
			m.message = fmt.Sprintf("Editor error: %v", msg.err)
		} else {
			m.message = "Closed log viewer"
		}

	case urlOpenedMsg:
		if msg.err != nil {
			m.message = fmt.Sprintf("Open URL error: %v", msg.err)
		} else {
			m.message = "Opened PR in browser"
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

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
				} else if m.currentView == pipelinesView {
					currentFilter = &m.pipelineFilterQuery
					currentCursor = &m.pipelineCursor
				} else if m.currentView == pipelineStepsView || m.currentView == pipelineStepLogView {
					return m, nil
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
			if m.activePane == branchPane && m.currentView == pipelineStepLogView {
				m.currentView = pipelineStepsView
				m.pipelineStepLog = ""
				m.pipelineStepLogLines = nil
				m.pipelineStepLogCursor = 0
			} else if m.activePane == branchPane && m.currentView == pipelineStepsView {
				m.currentView = pipelinesView
				m.pipelineStepCursor = 0
				m.pipelineSteps = nil
			} else if m.activePane == branchPane {
				m.activePane = repoPane
				m.currentView = noSelection
			}

		case "/":
			if m.currentView != pipelineStepsView && m.currentView != pipelineStepLogView {
				m.filterMode = true
			}

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
			if !m.filterMode && m.activePane == branchPane && m.currentView == pipelinesView && len(m.getFilteredPipelines()) > 0 {
				filtered := m.getFilteredPipelines()
				selectedPipeline := filtered[m.pipelineCursor]
				if selectedPipeline.UUID == "" {
					m.message = "Selected pipeline has no UUID"
					return m, nil
				}
				m.selectedPipelineRef = fmt.Sprintf("#%d", selectedPipeline.BuildNumber)
				m.selectedPipelineUUID = selectedPipeline.UUID
				m.currentView = pipelineStepsView
				m.loading = true
				m.pipelineSteps = nil
				m.pipelineStepCursor = 0
				return m, loadPipelineSteps(m.client, m.selectedRepoSlug, selectedPipeline.UUID)
			}
			if !m.filterMode && m.activePane == branchPane && m.currentView == pipelineStepsView && len(m.pipelineSteps) > 0 && m.selectedPipelineUUID != "" {
				selectedStep := m.pipelineSteps[m.pipelineStepCursor]
				if selectedStep.UUID == "" {
					m.message = "Selected step has no UUID"
					return m, nil
				}
				m.selectedStepName = selectedStep.Name
				if m.selectedStepName == "" {
					m.selectedStepName = selectedStep.UUID
				}
				m.currentView = pipelineStepLogView
				m.loading = true
				m.pipelineStepLog = ""
				m.pipelineStepLogLines = nil
				m.pipelineStepLogCursor = 0
				return m, loadPipelineStepLog(m.client, m.selectedRepoSlug, m.selectedPipelineUUID, selectedStep.UUID)
			}

		case "h":
			if !m.filterMode && m.activePane == branchPane && m.selectedRepoSlug != "" && m.currentView != pipelineStepsView && m.currentView != pipelineStepLogView {
				switch m.currentView {
				case branchesView:
					m.currentView = prView
					m.loading = true
					m.pullRequests = nil
					m.prFilterQuery = ""
					m.prCursor = 0
					return m, loadPullRequests(m.client, m.selectedRepoSlug)
				case prView:
					m.currentView = pipelinesView
					m.loading = true
					m.pipelines = nil
					m.pipelineFilterQuery = ""
					m.pipelineCursor = 0
					return m, loadPipelines(m.client, m.selectedRepoSlug)
				case pipelinesView:
					m.currentView = branchesView
					m.loading = true
					m.branches = nil
					m.branchFilterQuery = ""
					m.branchCursor = 0
					return m, loadBranches(m.client, m.selectedRepoSlug)
				}
			}

		case "l":
			if !m.filterMode && m.activePane == branchPane && m.selectedRepoSlug != "" && m.currentView != pipelineStepsView && m.currentView != pipelineStepLogView {
				switch m.currentView {
				case prView:
					m.currentView = branchesView
					m.loading = true
					m.branches = nil
					m.branchFilterQuery = ""
					m.branchCursor = 0
					return m, loadBranches(m.client, m.selectedRepoSlug)
				case branchesView:
					m.currentView = pipelinesView
					m.loading = true
					m.pipelines = nil
					m.pipelineFilterQuery = ""
					m.pipelineCursor = 0
					return m, loadPipelines(m.client, m.selectedRepoSlug)
				case pipelinesView:
					m.currentView = prView
					m.loading = true
					m.pullRequests = nil
					m.prFilterQuery = ""
					m.prCursor = 0
					return m, loadPullRequests(m.client, m.selectedRepoSlug)
				}
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
				cursorChanged := false
				if m.activePane == repoPane {
					filtered := m.getFilteredRepos()
					if m.repoCursor < len(filtered)-1 {
						m.repoCursor++
						cursorChanged = true
					}
				} else {
					if m.currentView == branchesView {
						filtered := m.getFilteredBranches()
						if m.branchCursor < len(filtered)-1 {
							m.branchCursor++
							cursorChanged = true
						}
					} else if m.currentView == prView {
						filtered := m.getFilteredPRs()
						if m.prCursor < len(filtered)-1 {
							m.prCursor++
							cursorChanged = true
						}
					} else if m.currentView == pipelinesView {
						filtered := m.getFilteredPipelines()
						if m.pipelineCursor < len(filtered)-1 {
							m.pipelineCursor++
							cursorChanged = true
						}
					} else if m.currentView == pipelineStepsView {
						if m.pipelineStepCursor < len(m.pipelineSteps)-1 {
							m.pipelineStepCursor++
							cursorChanged = true
						}
					} else if m.currentView == pipelineStepLogView {
						if m.pipelineStepLogCursor < len(m.pipelineStepLogLines)-1 {
							m.pipelineStepLogCursor++
							cursorChanged = true
						}
					}
				}

				if cursorChanged && m.activePane == branchPane && m.currentView == pipelinesView && selectedRunningPipelineUUID(m) != "" {
					return m, pollPipelineUpdates()
				}
			}

		case "k", "up":
			if !m.filterMode {
				cursorChanged := false
				if m.activePane == repoPane {
					if m.repoCursor > 0 {
						m.repoCursor--
						cursorChanged = true
					}
				} else {
					if m.currentView == branchesView {
						if m.branchCursor > 0 {
							m.branchCursor--
							cursorChanged = true
						}
					} else if m.currentView == prView {
						if m.prCursor > 0 {
							m.prCursor--
							cursorChanged = true
						}
					} else if m.currentView == pipelinesView {
						if m.pipelineCursor > 0 {
							m.pipelineCursor--
							cursorChanged = true
						}
					} else if m.currentView == pipelineStepsView {
						if m.pipelineStepCursor > 0 {
							m.pipelineStepCursor--
							cursorChanged = true
						}
					} else if m.currentView == pipelineStepLogView {
						if m.pipelineStepLogCursor > 0 {
							m.pipelineStepLogCursor--
							cursorChanged = true
						}
					}
				}

				if cursorChanged && m.activePane == branchPane && m.currentView == pipelinesView && selectedRunningPipelineUUID(m) != "" {
					return m, pollPipelineUpdates()
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
				prURL := strings.TrimSpace(selectedPR.URL)
				if !strings.HasPrefix(prURL, "https://") && !strings.HasPrefix(prURL, "http://") {
					prURL = ""
				}
				if prURL == "" && selectedPR.ID > 0 && m.workspace != "" && m.selectedRepoSlug != "" {
					prURL = fmt.Sprintf("https://bitbucket.org/%s/%s/pull-requests/%d", m.workspace, m.selectedRepoSlug, selectedPR.ID)
				}
				if prURL != "" {
					return m, openURL(prURL)
				}
				m.message = "Selected PR has no URL"
				return m, nil
			}

		case "v":
			if !m.filterMode && m.activePane == branchPane && m.currentView == pipelineStepLogView && !m.loading {
				return m, openLogInEditor(m.pipelineStepLog, m.selectedStepName)
			}

		case "r":
			if !m.filterMode && m.activePane == branchPane && m.selectedRepoSlug != "" {
				switch m.currentView {
				case branchesView:
					m.loading = true
					m.branches = nil
					m.branchCursor = 0
					return m, loadBranches(m.client, m.selectedRepoSlug)
				case prView:
					m.loading = true
					m.pullRequests = nil
					m.prCursor = 0
					return m, loadPullRequests(m.client, m.selectedRepoSlug)
				case pipelinesView:
					m.loading = true
					m.pipelines = nil
					m.pipelineCursor = 0
					return m, loadPipelines(m.client, m.selectedRepoSlug)
				case pipelineStepsView:
					if m.selectedPipelineUUID != "" {
						m.loading = true
						m.pipelineSteps = nil
						m.pipelineStepCursor = 0
						return m, loadPipelineSteps(m.client, m.selectedRepoSlug, m.selectedPipelineUUID)
					}
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
	if m.currentView != noSelection && m.activePane == branchPane {
		helpText = "h/l: switch tabs  esc: back  j/k/↑/↓: navigate  r: refresh  /: filter  q: quit"
	}
	if m.currentView == prView && m.activePane == branchPane {
		helpText = "h/l: switch tabs  esc: back  j/k/↑/↓: navigate  o: open in browser  r: refresh  /: filter  q: quit"
	}
	if m.currentView == pipelinesView && m.activePane == branchPane {
		helpText = "h/l: switch tabs  enter: view steps  esc: back  j/k/↑/↓: navigate  r: refresh  /: filter  q: quit"
	}
	if m.currentView == pipelineStepsView && m.activePane == branchPane {
		helpText = "enter: view logs  esc: back to pipelines  j/k/↑/↓: navigate  r: refresh  q: quit"
	}
	if m.currentView == pipelineStepLogView && m.activePane == branchPane {
		helpText = "v: open in nvim/less  esc: back to steps  j/k/↑/↓: scroll logs  q: quit"
	}
	if m.filterMode {
		currentFilter := m.repoFilterQuery
		if m.activePane == branchPane {
			if m.currentView == branchesView {
				currentFilter = m.branchFilterQuery
			} else if m.currentView == prView {
				currentFilter = m.prFilterQuery
			} else if m.currentView == pipelinesView {
				currentFilter = m.pipelineFilterQuery
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
	} else if m.currentView == pipelinesView {
		return m.renderPipelinePane()
	} else if m.currentView == pipelineStepsView {
		return m.renderPipelineStepsPane()
	} else if m.currentView == pipelineStepLogView {
		return m.renderPipelineStepLogPane()
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
	pipelinesTab := inactiveTab.Render("Pipelines")

	if m.currentView == prView {
		prsTab = activeTab.Render("Pull Requests")
	} else if m.currentView == branchesView {
		branchesTab = activeTab.Render("Branches")
	} else if m.currentView == pipelinesView || m.currentView == pipelineStepsView || m.currentView == pipelineStepLogView {
		pipelinesTab = activeTab.Render("Pipelines")
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, prsTab, branchesTab, pipelinesTab)
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
		items = append(items, m.spinner.View()+" Loading...")
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
		items = append(items, m.spinner.View()+" Loading...")
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
		items = append(items, m.spinner.View()+" Loading...")
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
		Padding(0, 1)

	return style.Render(content)
}

func (m AppModel) renderPipelinePane() string {
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

	title := "Pipelines"
	if m.selectedRepo != "" {
		title = fmt.Sprintf("Pipelines (%s)", m.selectedRepo)
	}
	title = fmt.Sprintf("%s [develop/staging/main/master]", title)
	if m.pipelineFilterQuery != "" {
		title = fmt.Sprintf("%s [/%s]", title, m.pipelineFilterQuery)
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

	if m.loading && m.activePane == branchPane && m.currentView == pipelinesView {
		items = append(items, m.spinner.View()+" Loading...")
	} else if len(m.pipelines) == 0 {
		items = append(items, "No pipelines")
	} else {
		filtered := m.getFilteredPipelines()
		if len(filtered) == 0 {
			if m.pipelineFilterQuery == "" {
				items = append(items, "No pipelines for tracked branches")
			} else {
				items = append(items, "No matches")
			}
		} else {
			start, end := m.calculateWindow(m.pipelineCursor, len(filtered), availableHeight-3)

			for i := start; i < end; i++ {
				pipeline := filtered[i]
				cursor := " "
				if m.activePane == branchPane && i == m.pipelineCursor {
					cursor = cursorStyle.Render(">")
				}

				stateBadge := formatPipelineState(pipeline.State)
				resultBadge := formatPipelineResult(pipeline.Result)
				branch := renderPipelineBranchColumn(pipeline.BranchName)
				created := shortTimestamp(pipeline.CreatedOn)
				duration := pipelineDuration(pipeline.StartedOn, pipeline.CompletedOn)
				ago := timeAgo(pipeline.CompletedOn)

				line := fmt.Sprintf("%s #%d %s %s %s created: %s", cursor, pipeline.BuildNumber, branch, stateBadge, resultBadge, created)
				if duration != "" {
					line = fmt.Sprintf("%s duration: %s", line, duration)
				}
				if ago != "" {
					line = fmt.Sprintf("%s completed: %s", line, ago)
				}

				items = append(items, line)
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
		Padding(0, 1)

	return style.Render(content)
}

func (m AppModel) renderPipelineStepsPane() string {
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

	title := "Pipeline Steps"
	if m.selectedRepo != "" {
		title = fmt.Sprintf("Pipeline Steps (%s)", m.selectedRepo)
	}
	if m.selectedPipelineRef != "" {
		title = fmt.Sprintf("%s %s", title, m.selectedPipelineRef)
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

	if m.loading && m.currentView == pipelineStepsView {
		items = append(items, m.spinner.View()+" Loading...")
	} else if len(m.pipelineSteps) == 0 {
		items = append(items, "No steps")
	} else {
		start, end := m.calculateWindow(m.pipelineStepCursor, len(m.pipelineSteps), availableHeight-3)
		for i := start; i < end; i++ {
			step := m.pipelineSteps[i]
			cursor := " "
			if m.activePane == branchPane && i == m.pipelineStepCursor {
				cursor = cursorStyle.Render(">")
			}

			stateBadge := formatPipelineState(step.State)
			resultBadge := formatPipelineResult(step.Result)
			duration := pipelineDuration(step.StartedOn, step.CompletedOn)
			line := fmt.Sprintf("%s %s %s %s", cursor, stateBadge, resultBadge, step.Name)
			if duration != "" {
				line = fmt.Sprintf("%s (%s)", line, duration)
			}
			items = append(items, line)
		}

		if start > 0 {
			items[2] = inactivePaneStyle.Render("  ↑ more")
		}
		if end < len(m.pipelineSteps) {
			items = append(items, inactivePaneStyle.Render("  ↓ more"))
		}
	}

	content := strings.Join(items, "\n")
	style := lipgloss.NewStyle().
		Width(paneWidth).
		Height(availableHeight).
		Padding(0, 1)

	return style.Render(content)
}

func (m AppModel) renderPipelineStepLogPane() string {
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

	title := "Pipeline Logs"
	if m.selectedRepo != "" {
		title = fmt.Sprintf("Pipeline Logs (%s)", m.selectedRepo)
	}
	if m.selectedPipelineRef != "" {
		title = fmt.Sprintf("%s %s", title, m.selectedPipelineRef)
	}
	if m.selectedStepName != "" {
		title = fmt.Sprintf("%s - %s", title, m.selectedStepName)
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

	if m.loading && m.currentView == pipelineStepLogView {
		items = append(items, m.spinner.View()+" Loading...")
	} else if len(m.pipelineStepLogLines) == 0 {
		items = append(items, "No logs")
	} else {
		start, end := m.calculateWindow(m.pipelineStepLogCursor, len(m.pipelineStepLogLines), availableHeight-3)
		for i := start; i < end; i++ {
			line := m.pipelineStepLogLines[i]
			cursor := " "
			if m.activePane == branchPane && i == m.pipelineStepLogCursor {
				cursor = cursorStyle.Render(">")
			}
			items = append(items, fmt.Sprintf("%s %s", cursor, line))
		}

		if start > 0 {
			items[2] = inactivePaneStyle.Render("  ↑ more")
		}
		if end < len(m.pipelineStepLogLines) {
			items = append(items, inactivePaneStyle.Render("  ↓ more"))
		}
	}

	content := strings.Join(items, "\n")
	style := lipgloss.NewStyle().
		Width(paneWidth).
		Height(availableHeight).
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

func formatPipelineState(state string) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "completed":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("99")).Render("[COMPLETED]")
	case "in_progress":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Render("[RUNNING]")
	case "pending":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("[PENDING]")
	case "paused":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("[PAUSED]")
	case "error":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("[ERROR]")
	default:
		return fmt.Sprintf("[%s]", strings.ToUpper(state))
	}
}

func formatPipelineResult(result string) string {
	switch strings.ToLower(strings.TrimSpace(result)) {
	case "successful", "success":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("[SUCCESS]")
	case "failed", "error":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("[FAILED]")
	case "stopped":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("[STOPPED]")
	case "expired":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("[EXPIRED]")
	case "":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("[N/A]")
	default:
		return fmt.Sprintf("[%s]", strings.ToUpper(result))
	}
}

func selectedRunningPipelineUUID(m AppModel) string {
	if m.activePane != branchPane || m.currentView != pipelinesView {
		return ""
	}

	filtered := m.getFilteredPipelines()
	if len(filtered) == 0 || m.pipelineCursor < 0 || m.pipelineCursor >= len(filtered) {
		return ""
	}

	selected := filtered[m.pipelineCursor]
	if !isPipelineRunning(selected) {
		return ""
	}

	return selected.UUID
}

func isPipelineRunning(pipeline domain.Pipeline) bool {
	state := strings.ToLower(strings.TrimSpace(pipeline.State))
	return state == "in_progress" || state == "running"
}

func shortTimestamp(value string) string {
	if value == "" {
		return "-"
	}

	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value
	}

	return t.Local().Format("2006-01-02 15:04")
}

func pipelineDuration(startedOn, completedOn string) string {
	if startedOn == "" {
		return ""
	}

	start, err := time.Parse(time.RFC3339, startedOn)
	if err != nil {
		return ""
	}

	end := time.Now().UTC()
	if completedOn != "" {
		parsedEnd, parseErr := time.Parse(time.RFC3339, completedOn)
		if parseErr == nil {
			end = parsedEnd
		}
	}

	if end.Before(start) {
		return ""
	}

	duration := end.Sub(start)
	if duration < time.Minute {
		return fmt.Sprintf("%ds", int(duration.Seconds()))
	}
	if duration < time.Hour {
		return fmt.Sprintf("%dm", int(duration.Minutes()))
	}
	return fmt.Sprintf("%dh%dm", int(duration.Hours()), int(duration.Minutes())%60)
}

func timeAgo(completedOn string) string {
	if completedOn == "" {
		return ""
	}

	completedAt, err := time.Parse(time.RFC3339, completedOn)
	if err != nil {
		return ""
	}

	elapsed := time.Now().UTC().Sub(completedAt)
	if elapsed < time.Minute {
		return "just now"
	}

	if elapsed < time.Hour {
		minutes := int(elapsed.Minutes())
		if minutes == 1 {
			return "1 min ago"
		}
		return fmt.Sprintf("%d mins ago", minutes)
	}

	if elapsed < 24*time.Hour {
		hours := int(elapsed.Hours())
		if hours == 1 {
			return "1 hr ago"
		}
		return fmt.Sprintf("%d hrs ago", hours)
	}

	days := int(elapsed.Hours() / 24)
	if days == 1 {
		return "1 day ago"
	}
	return fmt.Sprintf("%d days ago", days)
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

func (m AppModel) getFilteredPipelines() []domain.Pipeline {
	query := strings.ToLower(m.pipelineFilterQuery)
	if query == "" {
		var tracked []domain.Pipeline
		for _, pipeline := range m.pipelines {
			if isTrackedPipelineBranch(pipeline.BranchName) {
				tracked = append(tracked, pipeline)
			}
		}
		return tracked
	}

	var filtered []domain.Pipeline
	for _, pipeline := range m.pipelines {
		if !isTrackedPipelineBranch(pipeline.BranchName) {
			continue
		}

		buildNumber := fmt.Sprintf("%d", pipeline.BuildNumber)
		if strings.Contains(strings.ToLower(pipeline.State), query) ||
			strings.Contains(strings.ToLower(pipeline.Result), query) ||
			strings.Contains(strings.ToLower(buildNumber), query) ||
			strings.Contains(strings.ToLower(pipeline.BranchName), query) {
			filtered = append(filtered, pipeline)
		}
	}
	return filtered
}

func isTrackedPipelineBranch(branchName string) bool {
	branch := strings.ToLower(formatPipelineBranch(branchName))
	switch branch {
	case "develop", "staging", "main", "master":
		return true
	default:
		return false
	}
}

func formatPipelineBranch(branchName string) string {
	branch := strings.TrimSpace(branchName)
	branch = strings.TrimPrefix(branch, "refs/heads/")
	branch = strings.TrimPrefix(branch, "/")
	if branch == "" {
		return "-"
	}
	return branch
}

func renderPipelineBranchColumn(branchName string) string {
	branch := formatPipelineBranch(branchName)
	color := pipelineBranchColor(branch)

	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(color)).
		Width(12).
		Render(branch)
}

func pipelineBranchColor(branch string) string {
	switch strings.ToLower(strings.TrimSpace(branch)) {
	case "develop":
		return "45"
	case "staging":
		return "220"
	case "main":
		return "42"
	case "master":
		return "39"
	case "-":
		return "241"
	}

	palette := []string{"33", "69", "81", "111", "147", "177", "207", "214", "179", "44", "75", "109"}
	h := fnv.New32a()
	_, _ = h.Write([]byte(branch))
	return palette[h.Sum32()%uint32(len(palette))]
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

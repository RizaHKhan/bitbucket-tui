package tui

import (
	"fmt"
	"strings"

	"bitbucket-cli/internal/bitbucket"
	"bitbucket-cli/internal/domain"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func loadPullRequestCommits(client *bitbucket.Client, repoSlug string, pullRequestID int) tea.Cmd {
	return func() tea.Msg {
		commits, err := client.ListPullRequestCommits(repoSlug, pullRequestID)
		return prCommitsLoadedMsg{commits: commits, err: err}
	}
}

func loadCommitChanges(client *bitbucket.Client, repoSlug, commitHash string) tea.Cmd {
	return func() tea.Msg {
		changes, err := client.ListCommitChanges(repoSlug, commitHash)
		return prCommitChangesLoadedMsg{hash: commitHash, changes: changes, err: err}
	}
}

func loadCommitDiff(client *bitbucket.Client, repoSlug, commitHash string) tea.Cmd {
	return func() tea.Msg {
		diff, err := client.GetCommitDiff(repoSlug, commitHash)
		return prCommitDiffLoadedMsg{hash: commitHash, diff: diff, err: err}
	}
}

func updateSelectedCommitDetails(m *AppModel) tea.Cmd {
	if m.currentView != prCommitsView || m.activePane != branchPane || len(m.prCommits) == 0 {
		m.selectedCommitHash = ""
		m.prCommitChanges = nil
		m.prCommitDiff = ""
		return nil
	}
	if m.prCommitCursor < 0 || m.prCommitCursor >= len(m.prCommits) {
		m.selectedCommitHash = ""
		m.prCommitChanges = nil
		m.prCommitDiff = ""
		return nil
	}

	selected := m.prCommits[m.prCommitCursor]
	hash := strings.TrimSpace(selected.Hash)
	m.selectedCommitHash = hash
	if hash == "" {
		m.prCommitChanges = nil
		m.prCommitDiff = ""
		return nil
	}

	hasChanges := false
	if cached, ok := m.prCommitChangesCache[hash]; ok {
		m.prCommitChanges = cached
		hasChanges = true
	} else {
		m.prCommitChanges = nil
	}

	hasDiff := false
	if cached, ok := m.prCommitDiffCache[hash]; ok {
		m.prCommitDiff = cached
		hasDiff = true
	} else {
		m.prCommitDiff = ""
	}

	if hasChanges && hasDiff {
		return nil
	}

	if m.selectedRepoSlug == "" {
		return nil
	}

	if !hasChanges && !hasDiff {
		return tea.Batch(
			loadCommitChanges(m.client, m.selectedRepoSlug, hash),
			loadCommitDiff(m.client, m.selectedRepoSlug, hash),
		)
	}
	if !hasChanges {
		return loadCommitChanges(m.client, m.selectedRepoSlug, hash)
	}
	return loadCommitDiff(m.client, m.selectedRepoSlug, hash)
}

func (m AppModel) renderPRCommitsPane() string {
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

	title := fmt.Sprintf("PR #%d commits", m.selectedPullRequestID)
	if strings.TrimSpace(m.selectedPullRequest) != "" {
		title = fmt.Sprintf("PR #%d commits (%s)", m.selectedPullRequestID, m.selectedPullRequest)
	}
	if !showRepoPane {
		title = fmt.Sprintf("%s (esc: back)", title)
	}

	title = activePaneStyle.Render(title)

	listWidth := int(float64(paneWidth) * 0.55)
	if listWidth < 40 {
		listWidth = 40
	}
	detailsWidth := paneWidth - listWidth - 1
	if detailsWidth < 30 {
		detailsWidth = 30
		listWidth = paneWidth - detailsWidth - 1
		if listWidth < 30 {
			listWidth = 30
			detailsWidth = paneWidth - listWidth - 1
		}
	}

	listContentHeight := availableHeight - 3
	if listContentHeight < 1 {
		listContentHeight = 1
	}

	var listItems []string
	listItems = append(listItems, "Commits")
	listItems = append(listItems, "")

	if m.loading && m.activePane == branchPane && m.currentView == prCommitsView {
		listItems = append(listItems, m.spinner.View()+" Loading...")
	} else if len(m.prCommits) == 0 {
		listItems = append(listItems, "No commits")
	} else {
		start, end := m.calculateWindow(m.prCommitCursor, len(m.prCommits), listContentHeight)

		for i := start; i < end; i++ {
			commit := m.prCommits[i]
			cursor := " "
			if m.activePane == branchPane && i == m.prCommitCursor {
				cursor = cursorStyle.Render(">")
			}

			hash := commit.Hash
			if len(hash) > 8 {
				hash = hash[:8]
			}

			message := strings.Split(commit.Message, "\n")[0]
			author := strings.TrimSpace(commit.Author)
			if author == "" {
				author = "unknown"
			}

			const rowPadding = 20
			maxMessageWidth := listWidth - rowPadding - len(author)
			if maxMessageWidth < 8 {
				maxMessageWidth = 8
			}
			if len(message) > maxMessageWidth {
				message = message[:maxMessageWidth-3] + "..."
			}

			authorText := lipgloss.NewStyle().Foreground(lipgloss.Color("99")).Render(fmt.Sprintf("@%s", author))
			listItems = append(listItems, fmt.Sprintf("%s %s %s %s", cursor, hash, authorText, message))
		}

		if start > 0 {
			listItems[1] = inactivePaneStyle.Render("  ↑ more")
		}
		if end < len(m.prCommits) {
			listItems = append(listItems, inactivePaneStyle.Render("  ↓ more"))
		}
	}

	detailsItems := []string{lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Render("Diff"), ""}
	if m.selectedCommitHash == "" {
		detailsItems = append(detailsItems, "Select a commit")
	} else {
		hash := m.selectedCommitHash
		if len(hash) > 12 {
			hash = hash[:12]
		}
		detailsItems = append(detailsItems, fmt.Sprintf("commit %s", hash))
		if _, ok := m.prCommitChangesCache[m.selectedCommitHash]; ok {
			detailsItems = append(detailsItems, fmt.Sprintf("files changed: %d", len(m.prCommitChanges)))
		}
		detailsItems = append(detailsItems, "")

		if _, ok := m.prCommitDiffCache[m.selectedCommitHash]; !ok {
			detailsItems = append(detailsItems, m.spinner.View()+" Loading diff...")
		} else if strings.TrimSpace(m.prCommitDiff) == "" {
			detailsItems = append(detailsItems, "No textual diff")
		} else {
			lines := strings.Split(m.prCommitDiff, "\n")
			maxRows := availableHeight - 8
			if maxRows < 1 {
				maxRows = 1
			}
			maxLineWidth := detailsWidth - 2
			if maxLineWidth < 10 {
				maxLineWidth = 10
			}

			for i := 0; i < len(lines) && i < maxRows; i++ {
				line := lines[i]
				if len(line) > maxLineWidth {
					line = line[:maxLineWidth-3] + "..."
				}
				detailsItems = append(detailsItems, line)
			}
			if len(lines) > maxRows {
				detailsItems = append(detailsItems, inactivePaneStyle.Render(fmt.Sprintf("  +%d more diff lines", len(lines)-maxRows)))
			}
		}
	}

	listStyle := lipgloss.NewStyle().Width(listWidth)
	detailsStyle := lipgloss.NewStyle().Width(detailsWidth)
	split := lipgloss.JoinHorizontal(lipgloss.Top, listStyle.Render(strings.Join(listItems, "\n")), detailsStyle.Render(strings.Join(detailsItems, "\n")))

	content := strings.Join([]string{m.renderRightTabs(), title, "", split}, "\n")
	style := lipgloss.NewStyle().
		Width(paneWidth).
		Height(availableHeight).
		Padding(0, 1)

	return style.Render(content)
}

var _ = domain.Commit{}

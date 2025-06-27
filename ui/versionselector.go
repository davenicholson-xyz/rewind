package ui

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/davenicholson-xyz/rewind/app"
)

// VersionSelectorModel holds the application state
type VersionSelectorModel struct {
	versions     []*app.FileVersion
	cursor       int
	selected     *app.FileVersion
	showDetails  bool
	showDiff     bool
	fileName     string
	viewport     viewportState
	diffLines    []string
	diffViewport viewportState
	rootDir      string
}

type viewportState struct {
	offset int
	height int
}

// Styles for the version selector
var (
	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("205")).
			Bold(true).
			Margin(0, 0, 1, 0)

	selectedVersionStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("212")).
				Background(lipgloss.Color("57")).
				Bold(true).
				Padding(0, 1)

	versionStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Padding(0, 1)

	currentVersionStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("46")).
				Bold(true).
				Padding(0, 1)

	headerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("33")).
			Bold(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottom(true).
			BorderForeground(lipgloss.Color("240")).
			Margin(0, 0, 1, 0)

	detailsStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(1).
			Margin(1, 0)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Margin(1, 0, 0, 0)

	// Diff styles
	diffHeaderStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("33")).
			Bold(true)

	diffContainerStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("240")).
				Padding(1).
				Margin(1, 0)

	diffLineStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))
)

// NewVersionSelector creates a new version selector model
func NewVersionSelector(versions []*app.FileVersion, fileName, rootDir string) VersionSelectorModel {
	// Sort versions by version number (descending - newest first)
	sortedVersions := make([]*app.FileVersion, len(versions))
	copy(sortedVersions, versions)
	sort.SliceStable(sortedVersions, func(i, j int) bool {
		return sortedVersions[i].VersionNumber > sortedVersions[j].VersionNumber
	})

	return VersionSelectorModel{
		versions:    sortedVersions,
		cursor:      0,
		fileName:    fileName,
		rootDir:     rootDir,
		showDetails: false,
		showDiff:    false,
		viewport: viewportState{
			height: 20,
		},
		diffViewport: viewportState{
			height: 15,
		},
	}
}

func (m VersionSelectorModel) Init() tea.Cmd {
	return nil
}

func (m VersionSelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Adjust viewport height based on terminal size
		if m.showDiff {
			m.viewport.height = (msg.Height - 12) / 2
			m.diffViewport.height = (msg.Height - 12) / 2
		} else {
			m.viewport.height = msg.Height - 10
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit

		case "up", "k":
			if m.showDiff {
				if m.diffViewport.offset > 0 {
					m.diffViewport.offset--
				}
			} else {
				if m.cursor > 0 {
					m.cursor--
					m.adjustViewport()
				}
			}

		case "down", "j":
			if m.showDiff {
				if m.diffViewport.offset < len(m.diffLines)-m.diffViewport.height {
					m.diffViewport.offset++
				}
			} else {
				if m.cursor < len(m.versions)-1 {
					m.cursor++
					m.adjustViewport()
				}
			}

		case "left", "h":
			if m.showDiff {
				m.showDiff = false
				m.viewport.height = 20
			}

		case "right", "l":
			if !m.showDiff && len(m.versions) > 0 {
				m.showDiff = true
				m.generateDiff()
				m.viewport.height = 10
				m.diffViewport.height = 15
			}

		case "home", "g":
			if m.showDiff {
				m.diffViewport.offset = 0
			} else {
				m.cursor = 0
				m.viewport.offset = 0
			}

		case "end", "G":
			if m.showDiff {
				if len(m.diffLines) > m.diffViewport.height {
					m.diffViewport.offset = len(m.diffLines) - m.diffViewport.height
				}
			} else {
				m.cursor = len(m.versions) - 1
				m.adjustViewport()
			}

		case "pageup", "ctrl+u":
			if m.showDiff {
				m.diffViewport.offset -= m.diffViewport.height / 2
				if m.diffViewport.offset < 0 {
					m.diffViewport.offset = 0
				}
			} else {
				m.cursor -= m.viewport.height / 2
				if m.cursor < 0 {
					m.cursor = 0
				}
				m.adjustViewport()
			}

		case "pagedown", "ctrl+d":
			if m.showDiff {
				m.diffViewport.offset += m.diffViewport.height / 2
				if m.diffViewport.offset > len(m.diffLines)-m.diffViewport.height {
					m.diffViewport.offset = len(m.diffLines) - m.diffViewport.height
				}
				if m.diffViewport.offset < 0 {
					m.diffViewport.offset = 0
				}
			} else {
				m.cursor += m.viewport.height / 2
				if m.cursor >= len(m.versions) {
					m.cursor = len(m.versions) - 1
				}
				m.adjustViewport()
			}

		case "enter", " ":
			if len(m.versions) > 0 && !m.showDiff {
				m.selected = m.versions[m.cursor]
			}

		case "tab", "i":
			if !m.showDiff {
				m.showDetails = !m.showDetails
			}

		case "d":
			if len(m.versions) > 0 {
				m.showDiff = !m.showDiff
				if m.showDiff {
					m.generateDiff()
					m.viewport.height = 10
					m.diffViewport.height = 15
				} else {
					m.viewport.height = 20
				}
			}

		case "s":
			if !m.showDiff {
				if len(m.versions) > 1 {
					newestFirst := m.versions[0].VersionNumber > m.versions[len(m.versions)-1].VersionNumber

					if newestFirst {
						sort.SliceStable(m.versions, func(i, j int) bool {
							return m.versions[i].VersionNumber < m.versions[j].VersionNumber
						})
					} else {
						sort.SliceStable(m.versions, func(i, j int) bool {
							return m.versions[i].VersionNumber > m.versions[j].VersionNumber
						})
					}

					m.cursor = 0
					m.viewport.offset = 0
				}
			}
		}
	}

	return m, nil
}

// generateDiff runs the Unix diff command and captures its output
func (m *VersionSelectorModel) generateDiff() {
	if len(m.versions) == 0 {
		return
	}

	currentVersion := m.versions[m.cursor]

	// Paths for current file and stored version
	currentFilePath := filepath.Join(m.rootDir, currentVersion.FilePath)
	storedVersionPath := filepath.Join(m.rootDir, ".rewind", "versions", currentVersion.StoragePath)

	// Check if files exist
	if _, err := os.Stat(currentFilePath); os.IsNotExist(err) {
		m.diffLines = []string{fmt.Sprintf("Error: Current file not found: %s", currentFilePath)}
		return
	}

	if _, err := os.Stat(storedVersionPath); os.IsNotExist(err) {
		m.diffLines = []string{fmt.Sprintf("Error: Stored version not found: %s", storedVersionPath)}
		return
	}

	// Run diff command with unified format, colors, and labels
	cmd := exec.Command("diff", "-u", "--color=always",
		"--label", fmt.Sprintf("Version %d", currentVersion.VersionNumber),
		"--label", "Current",
		storedVersionPath, currentFilePath)

	output, err := cmd.Output()

	// diff returns exit code 1 when files differ, which is expected
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			// Exit code 1 means files differ (normal case)
			// Exit code 2 means error occurred
			if exitError.ExitCode() == 2 {
				m.diffLines = []string{fmt.Sprintf("Error running diff: %v", err)}
				return
			}
		} else {
			m.diffLines = []string{fmt.Sprintf("Error running diff: %v", err)}
			return
		}
	}

	// Parse output into lines
	if len(output) == 0 {
		m.diffLines = []string{"No differences found"}
	} else {
		scanner := bufio.NewScanner(strings.NewReader(string(output)))
		m.diffLines = []string{}
		for scanner.Scan() {
			m.diffLines = append(m.diffLines, scanner.Text())
		}
	}

	m.diffViewport.offset = 0
}

// adjustViewport ensures the cursor is visible within the viewport
func (m *VersionSelectorModel) adjustViewport() {
	if m.cursor < m.viewport.offset {
		m.viewport.offset = m.cursor
	} else if m.cursor >= m.viewport.offset+m.viewport.height {
		m.viewport.offset = m.cursor - m.viewport.height + 1
	}
}

// formatFileSize converts bytes to human-readable format
func formatFileSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// getTimeDiff returns a human-readable time difference
func getTimeDiff(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)

	switch {
	case diff < 10*time.Second:
		return "just now"
	case diff < time.Minute:
		seconds := int(diff.Seconds())
		if seconds == 1 {
			return "1 second ago"
		}
		return fmt.Sprintf("%d seconds ago", seconds)
	case diff < time.Hour:
		minutes := int(diff.Minutes())
		if minutes == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", minutes)
	case diff < 24*time.Hour:
		hours := int(diff.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	case diff < 7*24*time.Hour:
		days := int(diff.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	case diff < 30*24*time.Hour:
		weeks := int(diff.Hours() / (24 * 7))
		if weeks == 1 {
			return "1 week ago"
		}
		return fmt.Sprintf("%d weeks ago", weeks)
	case diff < 365*24*time.Hour:
		months := int(diff.Hours() / (24 * 30))
		if months == 1 {
			return "1 month ago"
		}
		return fmt.Sprintf("%d months ago", months)
	default:
		years := int(diff.Hours() / (24 * 365))
		if years == 1 {
			return "1 year ago"
		}
		return fmt.Sprintf("%d years ago", years)
	}
}

func (m VersionSelectorModel) View() string {
	var b strings.Builder

	// Title
	title := fmt.Sprintf("Version Selector - %s", m.fileName)
	if m.showDiff {
		title += " (Diff Mode)"
	}
	b.WriteString(titleStyle.Render(title))
	b.WriteString("\n")

	if m.showDiff {
		// Split view: versions list on top, diff on bottom
		b.WriteString(m.renderVersionList())
		b.WriteString("\n")
		b.WriteString(m.renderDiff())
	} else {
		// Full version list view
		b.WriteString(m.renderVersionList())

		if m.showDetails && len(m.versions) > 0 {
			b.WriteString(m.renderDetails())
		}
	}

	// Selection info
	if m.selected != nil {
		selectedInfo := fmt.Sprintf("✓ Selected: Version %d (ID: %d)", m.selected.VersionNumber, m.selected.ID)
		b.WriteString(selectedVersionStyle.Render(selectedInfo))
		b.WriteString("\n")
	}

	// Help text
	var help string
	if m.showDiff {
		help = "Diff Mode: ↑/↓,j/k=scroll diff, h=back to list, l/d=toggle diff, g/G=top/bottom, PgUp/PgDn=page, q/Esc=quit"
	} else {
		help = "Navigation: ↑/↓,j/k=move, Enter/Space=select, Tab/i=details, s=sort, d/l=diff, g/G=top/bottom, PgUp/PgDn=scroll, q/Esc=quit"
	}
	b.WriteString(helpStyle.Render(help))

	return b.String()
}

func (m VersionSelectorModel) renderVersionList() string {
	var b strings.Builder

	// Header
	header := fmt.Sprintf("%-8s %-12s %-15s %-12s %s", "Version", "Size", "Age", "Hash", "Storage Path")
	b.WriteString(headerStyle.Render(header))
	b.WriteString("\n")

	// Version list with viewport
	visibleVersions := m.versions[m.viewport.offset:]
	if len(visibleVersions) > m.viewport.height {
		visibleVersions = visibleVersions[:m.viewport.height]
	}

	for i, version := range visibleVersions {
		actualIndex := m.viewport.offset + i

		// Format the version info
		versionStr := fmt.Sprintf("v%-7d", version.VersionNumber)
		sizeStr := fmt.Sprintf("%-12s", formatFileSize(version.FileSize))
		ageStr := fmt.Sprintf("%-15s", getTimeDiff(version.Timestamp))
		hashStr := version.FileHash
		if len(hashStr) > 12 {
			hashStr = hashStr[:12] + "..."
		}
		hashStr = fmt.Sprintf("%-12s", hashStr)
		storageStr := version.StoragePath

		line := fmt.Sprintf("%s %s %s %s %s", versionStr, sizeStr, ageStr, hashStr, storageStr)

		// Apply styling based on state
		if actualIndex == m.cursor {
			line = selectedVersionStyle.Render(line)
		} else if actualIndex == 0 && m.versions[0].VersionNumber > m.versions[len(m.versions)-1].VersionNumber {
			line = currentVersionStyle.Render(line)
		} else {
			line = versionStyle.Render(line)
		}

		b.WriteString(line)
		b.WriteString("\n")
	}

	// Show scroll indicators
	if len(m.versions) > m.viewport.height {
		totalVersions := len(m.versions)
		scrollInfo := fmt.Sprintf("Showing %d-%d of %d versions",
			m.viewport.offset+1,
			min(m.viewport.offset+len(visibleVersions), totalVersions),
			totalVersions)
		b.WriteString(helpStyle.Render(scrollInfo))
		b.WriteString("\n")
	}

	return b.String()
}

func (m VersionSelectorModel) renderDiff() string {
	var b strings.Builder

	diffTitle := "Diff View"
	if len(m.versions) > 0 {
		currentVersion := m.versions[m.cursor]
		diffTitle = fmt.Sprintf("Diff: Version %d vs Current", currentVersion.VersionNumber)
	}
	b.WriteString(diffHeaderStyle.Render(diffTitle))
	b.WriteString("\n")

	if len(m.diffLines) == 0 {
		b.WriteString(diffContainerStyle.Render("No diff available"))
		return b.String()
	}

	// Show diff lines with viewport
	visibleLines := m.diffLines[m.diffViewport.offset:]
	if len(visibleLines) > m.diffViewport.height {
		visibleLines = visibleLines[:m.diffViewport.height]
	}

	var diffContent strings.Builder
	for _, line := range visibleLines {
		diffContent.WriteString(diffLineStyle.Render(line))
		diffContent.WriteString("\n")
	}

	b.WriteString(diffContainerStyle.Render(diffContent.String()))

	// Show diff scroll indicators
	if len(m.diffLines) > m.diffViewport.height {
		scrollInfo := fmt.Sprintf("Diff: %d-%d of %d lines",
			m.diffViewport.offset+1,
			min(m.diffViewport.offset+len(visibleLines), len(m.diffLines)),
			len(m.diffLines))
		b.WriteString(helpStyle.Render(scrollInfo))
		b.WriteString("\n")
	}

	return b.String()
}

func (m VersionSelectorModel) renderDetails() string {
	currentVersion := m.versions[m.cursor]
	details := fmt.Sprintf(
		"Detailed Information:\n"+
			"ID: %d\n"+
			"File Path: %s\n"+
			"Version: %d\n"+
			"Timestamp: %s\n"+
			"File Hash: %s\n"+
			"File Size: %s (%d bytes)\n"+
			"Storage Path: %s",
		currentVersion.ID,
		currentVersion.FilePath,
		currentVersion.VersionNumber,
		currentVersion.Timestamp.Format("2006-01-02 15:04:05 MST"),
		currentVersion.FileHash,
		formatFileSize(currentVersion.FileSize),
		currentVersion.FileSize,
		currentVersion.StoragePath,
	)
	return detailsStyle.Render(details) + "\n"
}

// Helper functions
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Updated function signature to include rootDir
func FileVersionSelector(versions []*app.FileVersion, rootDir string) {
	fileName := versions[0].FilePath
	model := NewVersionSelector(versions, fileName, rootDir)

	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v", err)
	}
}

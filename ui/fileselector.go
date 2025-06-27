package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/davenicholson-xyz/rewind/app"
)

// TreeNode represents a node in the file tree
type TreeNode struct {
	Name        string
	IsDir       bool
	Children    []*TreeNode
	Parent      *TreeNode
	Expanded    bool
	Depth       int
	FileVersion *app.FileVersion // Store file version info for files
}

// Model holds the application state
type Model struct {
	tree     *TreeNode
	cursor   int
	flatList []*TreeNode
	selected *TreeNode
}

// Styles for the tree view
var (
	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("212")).
			Background(lipgloss.Color("57")).
			Bold(true)

	dirStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("33")).
			Bold(true)

	fileStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))
)

// buildTreeFromFileVersions creates a tree structure from FileVersion slice
func buildTreeFromFileVersions(fileVersions []*app.FileVersion) *TreeNode {
	root := &TreeNode{
		Name:     "root",
		IsDir:    true,
		Expanded: true,
		Depth:    0,
		Children: []*TreeNode{},
	}

	// Map to keep track of created directory nodes
	dirMap := make(map[string]*TreeNode)
	dirMap[""] = root // Empty path maps to root

	for _, fv := range fileVersions {
		parts := strings.Split(filepath.Clean(fv.FilePath), string(filepath.Separator))
		if parts[0] == "." {
			parts = parts[1:] // Remove leading "." if present
		}

		currentParent := root
		currentPath := ""

		// Create intermediate directories
		for i, part := range parts[:len(parts)-1] {
			if currentPath == "" {
				currentPath = part
			} else {
				currentPath = filepath.Join(currentPath, part)
			}

			// Check if directory already exists
			if existingDir, exists := dirMap[currentPath]; exists {
				currentParent = existingDir
			} else {
				// Create new directory node
				newDir := &TreeNode{
					Name:     part,
					IsDir:    true,
					Expanded: false,
					Parent:   currentParent,
					Depth:    i + 1,
					Children: []*TreeNode{},
				}

				currentParent.Children = append(currentParent.Children, newDir)
				dirMap[currentPath] = newDir
				currentParent = newDir
			}
		}

		// Create the file node
		fileName := parts[len(parts)-1]
		fileNode := &TreeNode{
			Name:        fileName,
			IsDir:       false,
			Parent:      currentParent,
			Depth:       len(parts),
			FileVersion: fv, // Store the file version info
		}

		currentParent.Children = append(currentParent.Children, fileNode)
	}

	return root
}

func flattenTree(node *TreeNode, list *[]*TreeNode) {
	*list = append(*list, node)

	if node.IsDir && node.Expanded {
		for _, child := range node.Children {
			flattenTree(child, list)
		}
	}
}

// updateFlatList rebuilds the flat navigation list
func (m *Model) updateFlatList() {
	m.flatList = []*TreeNode{}
	flattenTree(m.tree, &m.flatList)

	// Ensure cursor is within bounds
	if m.cursor >= len(m.flatList) {
		m.cursor = len(m.flatList) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

// getTreePrefix returns the tree drawing characters for a node
func getTreePrefix(node *TreeNode) string {
	if node.Depth == 0 {
		return ""
	}

	prefix := ""
	for i := range node.Depth {
		if i == node.Depth-1 {
			prefix += "├── "
		} else {
			prefix += "│   "
		}
	}
	return prefix
}

// getDirIcon returns the appropriate icon for directories
func getDirIcon(node *TreeNode) string {
	if !node.IsDir {
		return ""
	}
	if node.Expanded {
		return "📂 "
	}
	return "📁 "
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			if m.cursor < len(m.flatList)-1 {
				m.cursor++
			}

		case "enter", " ":
			if len(m.flatList) > 0 {
				currentNode := m.flatList[m.cursor]
				if currentNode.IsDir {
					// Toggle directory expansion
					currentNode.Expanded = !currentNode.Expanded
					m.updateFlatList()
				} else {
					// Select file
					m.selected = currentNode
				}
			}

		case "right", "l":
			if len(m.flatList) > 0 {
				currentNode := m.flatList[m.cursor]
				if currentNode.IsDir && !currentNode.Expanded {
					currentNode.Expanded = true
					m.updateFlatList()
				}
			}

		case "left", "h":
			if len(m.flatList) > 0 {
				currentNode := m.flatList[m.cursor]
				if currentNode.IsDir && currentNode.Expanded {
					currentNode.Expanded = false
					m.updateFlatList()
				}
			}
		}
	}

	return m, nil
}

func (m Model) View() string {
	var b strings.Builder

	b.WriteString("File Tree Navigator\n")
	b.WriteString("Use ↑/↓ or j/k to navigate, Enter/Space to select/toggle, ←/→ or h/l to collapse/expand\n")
	b.WriteString("Press q or Ctrl+C to quit\n\n")

	for i, node := range m.flatList {
		prefix := getTreePrefix(node)
		icon := getDirIcon(node)

		line := prefix + icon + node.Name

		// Add file version info for files
		if !node.IsDir && node.FileVersion != nil {
			fv := node.FileVersion
			line += fmt.Sprintf(" (v%d, %s)", fv.VersionNumber, fv.Timestamp.Format("2006-01-02"))
		}

		if i == m.cursor {
			// Highlight current selection
			line = selectedStyle.Render(line)
		} else if node.IsDir {
			line = dirStyle.Render(line)
		} else {
			line = fileStyle.Render(line)
		}

		b.WriteString(line + "\n")
	}

	if m.selected != nil {
		b.WriteString(fmt.Sprintf("\nSelected: %s", m.selected.Name))
		if m.selected.FileVersion != nil {
			fv := m.selected.FileVersion
			b.WriteString(fmt.Sprintf("\nFile Details:"))
			b.WriteString(fmt.Sprintf("\n  Version: %d", fv.VersionNumber))
			b.WriteString(fmt.Sprintf("\n  Size: %d bytes", fv.FileSize))
			b.WriteString(fmt.Sprintf("\n  Hash: %s", fv.FileHash))
			b.WriteString(fmt.Sprintf("\n  Timestamp: %s", fv.Timestamp.Format("2006-01-02 15:04:05")))
			b.WriteString(fmt.Sprintf("\n  Storage Path: %s", fv.StoragePath))
		}
	}

	return b.String()
}

func FileSelector(versions []*app.FileVersion) {
	tree := buildTreeFromFileVersions(versions)
	model := Model{
		tree:   tree,
		cursor: 0,
	}
	model.updateFlatList()

	p := tea.NewProgram(model)
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v", err)
	}
}

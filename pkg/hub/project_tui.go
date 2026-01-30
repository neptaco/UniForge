package hub

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/neptaco/uniforge/pkg/ui"
)

// Key bindings
type keyMap struct {
	Up       key.Binding
	Down     key.Binding
	Enter    key.Binding
	Editor   key.Binding
	CopyPath key.Binding
	Quit     key.Binding
}

var keys = keyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "ctrl+k"),
		key.WithHelp("↑", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "ctrl+j"),
		key.WithHelp("↓", "down"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("Enter", "open in Unity"),
	),
	Editor: key.NewBinding(
		key.WithKeys("ctrl+e"),
		key.WithHelp("^E", "open in editor"),
	),
	CopyPath: key.NewBinding(
		key.WithKeys("ctrl+p"),
		key.WithHelp("^P", "copy path"),
	),
	Quit: key.NewBinding(
		key.WithKeys("esc", "ctrl+c"),
		key.WithHelp("Esc", "quit"),
	),
}

// Styles
var (
	selectedStyle = lipgloss.NewStyle().
			Bold(true).
			Background(lipgloss.Color("237"))

	normalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	versionStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	gitBranchStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("77"))

	gitDirtyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214"))

	gitCleanStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	counterStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	promptStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("76"))

	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))
)

// OpenProjectFunc is a function type for opening a project in Unity
type OpenProjectFunc func(path, version string) error

// projectModel is the bubbletea model for project TUI
type projectModel struct {
	projects      []ProjectInfo
	filtered      []ProjectInfo // filtered projects based on search
	cursor        int
	status        string
	quitting      bool
	loading       bool
	launching     bool   // true when launching Unity/editor
	launchMsg     string // message to show while launching
	err           error
	openProjectFn OpenProjectFunc
	editorName    string // detected editor name for help display
	filterInput   textinput.Model
}

type projectsLoadedMsg struct {
	projects []ProjectInfo
	err      error
}

type actionDoneMsg struct {
	message string
	err     error
}

func initialProjectModel(openFn OpenProjectFunc) projectModel {
	ti := textinput.New()
	ti.Focus()
	ti.CharLimit = 100
	ti.Width = 50
	ti.Prompt = ""

	return projectModel{
		loading:       true,
		openProjectFn: openFn,
		editorName:    getExternalEditor(),
		filterInput:   ti,
	}
}

func (m projectModel) Init() tea.Cmd {
	return loadProjects
}

func loadProjects() tea.Msg {
	client := NewClient()
	projects, err := client.ListProjectsWithGit()
	return projectsLoadedMsg{projects: projects, err: err}
}

func (m projectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case projectsLoadedMsg:
		m.loading = false
		m.projects = msg.projects
		m.filtered = msg.projects
		m.err = msg.err
		return m, nil

	case actionDoneMsg:
		m.launching = false
		if msg.err != nil {
			m.status = fmt.Sprintf("Error: %s", msg.err)
			return m, nil
		}
		// Success - quit TUI and show message
		m.status = msg.message
		m.quitting = true
		return m, tea.Quit

	case tea.KeyMsg:
		if m.loading {
			return m, nil
		}

		switch {
		case key.Matches(msg, keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case key.Matches(msg, keys.Down):
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}
			return m, nil
		case key.Matches(msg, keys.Enter):
			if len(m.filtered) > 0 {
				p := m.filtered[m.cursor]
				m.launching = true
				m.launchMsg = fmt.Sprintf("Starting Unity %s for %s...", p.Version, p.Title)
				return m, openInUnity(p, m.openProjectFn)
			}
		case key.Matches(msg, keys.Editor):
			if len(m.filtered) > 0 {
				p := m.filtered[m.cursor]
				m.launching = true
				m.launchMsg = fmt.Sprintf("Opening %s in editor...", p.Title)
				return m, openInEditor(p)
			}
		case key.Matches(msg, keys.CopyPath):
			if len(m.filtered) > 0 {
				return m, copyPath(m.filtered[m.cursor])
			}
			return m, nil
		case key.Matches(msg, keys.Quit):
			// If filter has text, clear it first
			if m.filterInput.Value() != "" {
				m.filterInput.SetValue("")
				m.filtered = m.projects
				m.cursor = 0
				return m, nil
			}
			// Otherwise quit
			m.quitting = true
			return m, tea.Quit
		}

		// Update text input for filtering
		var cmd tea.Cmd
		m.filterInput, cmd = m.filterInput.Update(msg)

		// Filter projects based on input
		m.filtered = m.filterProjects(m.filterInput.Value())
		// Reset cursor if out of bounds
		if m.cursor >= len(m.filtered) {
			m.cursor = max(0, len(m.filtered)-1)
		}
		return m, cmd
	}
	return m, nil
}

// filterProjects filters projects by name (case-insensitive)
func (m projectModel) filterProjects(query string) []ProjectInfo {
	if query == "" {
		return m.projects
	}
	query = strings.ToLower(query)
	var result []ProjectInfo
	for _, p := range m.projects {
		if strings.Contains(strings.ToLower(p.Title), query) {
			result = append(result, p)
		}
	}
	return result
}

func (m projectModel) View() string {
	if m.quitting {
		if m.status != "" {
			return statusStyle.Render(m.status) + "\n"
		}
		return ""
	}

	if m.launching {
		return m.launchMsg + "\n"
	}

	if m.loading {
		return "Loading projects..."
	}

	if m.err != nil {
		return fmt.Sprintf("Error: %s\n", m.err)
	}

	if len(m.projects) == 0 {
		return "No projects registered in Unity Hub.\n"
	}

	var b strings.Builder

	// Calculate max widths for alignment
	maxTitleLen := 0
	maxVersionLen := 0
	maxBranchLen := 0
	for _, p := range m.filtered {
		if len(p.Title) > maxTitleLen {
			maxTitleLen = len(p.Title)
		}
		if len(p.Version) > maxVersionLen {
			maxVersionLen = len(p.Version)
		}
		if len(p.GitBranch) > maxBranchLen {
			maxBranchLen = len(p.GitBranch)
		}
	}

	// Project list
	for i, p := range m.filtered {
		// Build line content
		title := p.Title + strings.Repeat(" ", maxTitleLen-len(p.Title))
		version := p.Version + strings.Repeat(" ", maxVersionLen-len(p.Version))

		var gitInfo string
		if p.GitBranch != "" {
			branch := p.GitBranch + strings.Repeat(" ", maxBranchLen-len(p.GitBranch))
			if p.GitStatus == "+0,-0" {
				gitInfo = gitBranchStyle.Render(branch) + " " + gitCleanStyle.Render("("+p.GitStatus+")")
			} else {
				gitInfo = gitBranchStyle.Render(branch) + " " + gitDirtyStyle.Render("("+p.GitStatus+")")
			}
		} else {
			gitInfo = versionStyle.Render(strings.Repeat(" ", maxBranchLen) + "—")
		}

		line := " " + title + "  " + versionStyle.Render(version) + "  " + gitInfo

		if i == m.cursor {
			b.WriteString(selectedStyle.Render(line))
		} else {
			b.WriteString(normalStyle.Render(line))
		}
		b.WriteString("\n")
	}

	// Show message if no matches
	if len(m.filtered) == 0 {
		b.WriteString(versionStyle.Render("  No matching projects"))
		b.WriteString("\n")
	}

	// Counter and help
	editorLabel := strings.ToUpper(m.editorName[:1]) + m.editorName[1:]
	counter := fmt.Sprintf("  %d/%d", len(m.filtered), len(m.projects))
	help := fmt.Sprintf("  Enter:Unity ^E:%s ^P:Copy Esc:Quit", editorLabel)
	b.WriteString(counterStyle.Render(counter + help))
	b.WriteString("\n")

	// Prompt
	b.WriteString(promptStyle.Render("> "))
	b.WriteString(m.filterInput.View())

	return b.String()
}

func openInUnity(p ProjectInfo, openFn OpenProjectFunc) tea.Cmd {
	return func() tea.Msg {
		if openFn == nil {
			return actionDoneMsg{err: fmt.Errorf("no Unity open function configured")}
		}
		err := openFn(p.Path, p.Version)
		if err != nil {
			return actionDoneMsg{err: err}
		}
		return actionDoneMsg{message: fmt.Sprintf("Opening %s in Unity %s", p.Title, p.Version)}
	}
}

func openInEditor(p ProjectInfo) tea.Cmd {
	return func() tea.Msg {
		editorCmd := getExternalEditor()
		cmd := exec.Command(editorCmd, p.Path)
		err := cmd.Start()
		if err != nil {
			return actionDoneMsg{err: fmt.Errorf("failed to open editor: %w", err)}
		}
		return actionDoneMsg{message: fmt.Sprintf("Opening %s in %s", p.Title, editorCmd)}
	}
}

func copyPath(p ProjectInfo) tea.Cmd {
	return func() tea.Msg {
		err := copyToClipboard(p.Path)
		if err != nil {
			return actionDoneMsg{err: fmt.Errorf("failed to copy path: %w", err)}
		}
		return actionDoneMsg{message: fmt.Sprintf("Copied: %s", p.Path)}
	}
}

func getExternalEditor() string {
	// Explicit override
	if editor := os.Getenv("UNIFORGE_EDITOR"); editor != "" {
		return editor
	}
	// Auto-detect Unity-friendly IDEs (preferred for Unity projects)
	for _, cmd := range []string{"rider", "cursor", "code"} {
		if isCommandAvailable(cmd) {
			return cmd
		}
	}
	// Fallback to general EDITOR
	if editor := os.Getenv("EDITOR"); editor != "" {
		return editor
	}
	return "code"
}

func copyToClipboard(text string) error {
	var cmd *exec.Cmd

	switch {
	case isCommandAvailable("pbcopy"):
		cmd = exec.Command("pbcopy")
	case isCommandAvailable("xclip"):
		cmd = exec.Command("xclip", "-selection", "clipboard")
	case isCommandAvailable("xsel"):
		cmd = exec.Command("xsel", "--clipboard", "--input")
	case isCommandAvailable("clip"):
		cmd = exec.Command("clip")
	default:
		return fmt.Errorf("no clipboard utility available")
	}

	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

func isCommandAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// RunProjectTUI launches the interactive project selector TUI
// openFn is called when user selects a project to open in Unity
func RunProjectTUI(client *Client, openFn OpenProjectFunc) error {
	ui.Debug("Starting project TUI")

	p := tea.NewProgram(initialProjectModel(openFn))
	_, err := p.Run()
	return err
}

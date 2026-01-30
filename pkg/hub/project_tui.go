package hub

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/key"
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
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "open in Unity"),
	),
	Editor: key.NewBinding(
		key.WithKeys("e"),
		key.WithHelp("e", "open in editor"),
	),
	CopyPath: key.NewBinding(
		key.WithKeys("p"),
		key.WithHelp("p", "copy path"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "esc", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
}

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("75")).
			MarginBottom(1)

	selectedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("212"))

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

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			MarginTop(1)

	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42")).
			MarginTop(1)
)

// OpenProjectFunc is a function type for opening a project in Unity
type OpenProjectFunc func(path, version string) error

// projectModel is the bubbletea model for project TUI
type projectModel struct {
	projects       []ProjectInfo
	cursor         int
	status         string
	quitting       bool
	loading        bool
	err            error
	openProjectFn  OpenProjectFunc
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
	return projectModel{
		loading:       true,
		openProjectFn: openFn,
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
		m.err = msg.err
		return m, nil

	case actionDoneMsg:
		if msg.err != nil {
			m.status = fmt.Sprintf("Error: %s", msg.err)
		} else {
			m.status = msg.message
		}
		return m, nil

	case tea.KeyMsg:
		if m.loading {
			return m, nil
		}

		switch {
		case key.Matches(msg, keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
		case key.Matches(msg, keys.Down):
			if m.cursor < len(m.projects)-1 {
				m.cursor++
			}
		case key.Matches(msg, keys.Enter):
			if len(m.projects) > 0 {
				return m, openInUnity(m.projects[m.cursor], m.openProjectFn)
			}
		case key.Matches(msg, keys.Editor):
			if len(m.projects) > 0 {
				return m, openInEditor(m.projects[m.cursor])
			}
		case key.Matches(msg, keys.CopyPath):
			if len(m.projects) > 0 {
				return m, copyPath(m.projects[m.cursor])
			}
		case key.Matches(msg, keys.Quit):
			m.quitting = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m projectModel) View() string {
	if m.quitting {
		return ""
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

	b.WriteString(titleStyle.Render("Unity Hub Projects"))
	b.WriteString("\n")

	for i, p := range m.projects {
		cursor := "  "
		style := normalStyle
		if i == m.cursor {
			cursor = "> "
			style = selectedStyle
		}

		// Project name
		line := cursor + style.Render(p.Title)

		// Version
		line += "  " + versionStyle.Render(p.Version)

		// Git info
		if p.GitBranch != "" {
			line += "  " + gitBranchStyle.Render(p.GitBranch)
			if p.GitStatus != "" && p.GitStatus != "clean" {
				line += " " + gitDirtyStyle.Render("("+p.GitStatus+")")
			} else if p.GitStatus == "clean" {
				line += " " + gitCleanStyle.Render("(clean)")
			}
		} else {
			line += "  " + versionStyle.Render("—")
		}

		b.WriteString(line + "\n")
	}

	// Status message
	if m.status != "" {
		b.WriteString(statusStyle.Render(m.status))
		b.WriteString("\n")
	}

	// Help
	help := helpStyle.Render("[Enter] Unity  [e] Editor  [p] Copy path  [q] Quit")
	b.WriteString(help)

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
	if editor := os.Getenv("UNIFORGE_EDITOR"); editor != "" {
		return editor
	}
	if editor := os.Getenv("EDITOR"); editor != "" {
		return editor
	}
	// Fallback: use first available editor
	for _, cmd := range []string{"rider", "cursor", "code"} {
		if isCommandAvailable(cmd) {
			return cmd
		}
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

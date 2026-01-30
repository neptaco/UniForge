package hub

import (
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/neptaco/uniforge/pkg/ui"
)

// editorTUIState represents the current state of the editor install TUI
type editorTUIState int

const (
	stateStreamSelect editorTUIState = iota
	stateVersionSelect
	stateInstalledSelect // インストール済みバージョン一覧
	stateModuleSelect
	stateInstalling
	stateComplete
)

// editorKeyMap defines key bindings for the editor TUI
type editorKeyMap struct {
	Up              key.Binding
	Down            key.Binding
	Enter           key.Binding
	Space           key.Binding
	Escape          key.Binding
	Tab             key.Binding
	OpenNotes       key.Binding
	FilterInstalled key.Binding
}

var editorKeys = editorKeyMap{
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
		key.WithHelp("Enter", "select/install"),
	),
	Space: key.NewBinding(
		key.WithKeys(" "),
		key.WithHelp("Space", "toggle"),
	),
	Escape: key.NewBinding(
		key.WithKeys("esc", "ctrl+c"),
		key.WithHelp("Esc", "back/quit"),
	),
	Tab: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("Tab", "toggle all"),
	),
	OpenNotes: key.NewBinding(
		key.WithKeys("o"),
		key.WithHelp("o", "open release notes"),
	),
	FilterInstalled: key.NewBinding(
		key.WithKeys("ctrl+l"),
		key.WithHelp("C-l", "installed"),
	),
}

// Styles for editor TUI
var (
	editorSelectedStyle = lipgloss.NewStyle().
				Bold(true).
				Background(lipgloss.Color("237"))

	editorNormalStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252"))

	editorLTSStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("77")).
			Bold(true)

	editorInstalledStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("42"))

	editorMutedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("241"))

	editorCheckboxStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("76"))

	editorDisabledStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("240"))

	editorHeaderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("255")).
				Bold(true)

	editorCountStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245"))

	editorRecommendedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("220")).
				Bold(true)

	editorNewBadgeStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("214")).
				Bold(true)

	editorDateStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	editorSizeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243"))

	editorSecurityAlertStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("196")).
					Bold(true)
)

// editorInstallModel is the bubbletea model for editor install TUI
type editorInstallModel struct {
	client *Client
	state  editorTUIState

	// Loading states
	loadingStreams  bool
	loadingReleases bool

	// Streams
	streams         []VersionStream
	filteredStreams []VersionStream
	streamCursor    int

	// Releases (background loaded)
	allReleases      []UnityRelease
	filteredReleases []UnityRelease
	versionCursor    int

	// Selected stream for version view
	selectedStream *VersionStream

	// Filter
	filterInput textinput.Model

	// Module selection
	modules         []ModuleInfo
	moduleCursor    int
	selectedModules map[string]bool
	selectedVersion *UnityRelease

	// Install
	architecture   string
	quitting       bool
	err            error
	installResult  string
	pendingInstall *InstallOptions // Set when user confirms install, executed after TUI exits

	// Project counts per version
	projectCounts map[string]int
}

// Message types
type streamsLoadedMsg struct {
	streams []VersionStream
	err     error
}

type releasesLoadedMsg struct {
	releases []UnityRelease
	err      error
}

type installCompleteMsg struct {
	message string
	err     error
}

func initialEditorInstallModel(client *Client) editorInstallModel {
	ti := textinput.New()
	ti.Focus()
	ti.CharLimit = 50
	ti.Width = 40
	ti.Prompt = ""

	// Load project counts per version
	projectCounts := make(map[string]int)
	projects, err := client.ListProjects()
	if err == nil {
		for _, p := range projects {
			projectCounts[p.Version]++
		}
	}

	return editorInstallModel{
		client:          client,
		state:           stateStreamSelect,
		loadingStreams:  true,
		loadingReleases: true,
		filterInput:     ti,
		selectedModules: make(map[string]bool),
		architecture:    client.detectArchitecture(),
		projectCounts:   projectCounts,
	}
}

func (m editorInstallModel) Init() tea.Cmd {
	return tea.Batch(
		m.loadStreams(),
		m.loadAllReleases(),
	)
}

func (m editorInstallModel) loadStreams() tea.Cmd {
	return func() tea.Msg {
		streams, err := m.client.FetchStreams()
		return streamsLoadedMsg{streams: streams, err: err}
	}
}

func (m editorInstallModel) loadAllReleases() tea.Cmd {
	return func() tea.Msg {
		// Try cache first
		cache, err := m.client.LoadCache()
		if err == nil && cache != nil {
			// Check if cache is valid by fetching current stream metadata
			currentStreams, streamErr := m.client.FetchStreams()
			if streamErr == nil && m.client.CheckCacheValidity(cache, currentStreams) {
				ui.Debug("Using cached releases")
				releases := m.client.ConvertCacheToReleases(cache)
				releases = m.client.EnrichReleasesWithInstallStatus(releases)
				return releasesLoadedMsg{releases: releases}
			}
		}

		// Fetch from API
		ui.Debug("Fetching releases from API")
		releases, err := m.client.GetAllReleases()
		if err != nil {
			return releasesLoadedMsg{err: err}
		}

		// Enrich with install status
		releases = m.client.EnrichReleasesWithInstallStatus(releases)

		// Save to cache (get streams for metadata)
		streams, _ := m.client.FetchStreams()
		if len(streams) > 0 {
			_ = m.client.SaveCache(streams, releases)
		}

		return releasesLoadedMsg{releases: releases}
	}
}

func (m editorInstallModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case streamsLoadedMsg:
		m.loadingStreams = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.streams = msg.streams
		m.filteredStreams = msg.streams
		return m, nil

	case releasesLoadedMsg:
		m.loadingReleases = false
		if msg.err != nil {
			ui.Debug("Failed to load releases", "error", msg.err)
			m.err = fmt.Errorf("failed to load releases: %w", msg.err)
			return m, nil
		}
		m.allReleases = msg.releases
		// Update filtered releases if we're already in version select state
		if m.state == stateVersionSelect && m.selectedStream != nil {
			m.updateFilteredReleases()
		}
		return m, nil

	case installCompleteMsg:
		m.state = stateComplete
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.installResult = msg.message
		}
		return m, nil

	case tea.KeyMsg:
		switch m.state {
		case stateStreamSelect:
			return m.updateStreamSelect(msg)
		case stateVersionSelect:
			return m.updateVersionSelect(msg)
		case stateInstalledSelect:
			return m.updateInstalledSelect(msg)
		case stateModuleSelect:
			return m.updateModuleSelect(msg)
		case stateComplete:
			m.quitting = true
			return m, tea.Quit
		}
	}
	return m, nil
}

// isVersionSearchMode returns true if filter looks like a version (2+ dots)
func (m editorInstallModel) isVersionSearchMode() bool {
	filter := m.filterInput.Value()
	return strings.Count(filter, ".") >= 2
}

func (m editorInstallModel) updateStreamSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.loadingStreams {
		return m, nil
	}

	switch {
	case key.Matches(msg, editorKeys.Up):
		if m.isVersionSearchMode() {
			if m.versionCursor > 0 {
				m.versionCursor--
			}
		} else {
			if m.streamCursor > 0 {
				m.streamCursor--
			}
		}
		return m, nil

	case key.Matches(msg, editorKeys.Down):
		if m.isVersionSearchMode() {
			if m.versionCursor < len(m.filteredReleases)-1 {
				m.versionCursor++
			}
		} else {
			if m.streamCursor < len(m.filteredStreams)-1 {
				m.streamCursor++
			}
		}
		return m, nil

	case key.Matches(msg, editorKeys.Enter):
		if m.isVersionSearchMode() {
			// Direct version selection
			if len(m.filteredReleases) > 0 {
				selected := m.filteredReleases[m.versionCursor]
				return m.selectVersion(&selected)
			}
		} else {
			// Stream selection - go to version list
			if len(m.filteredStreams) > 0 {
				m.selectedStream = &m.filteredStreams[m.streamCursor]
				m.state = stateVersionSelect
				m.filterInput.SetValue("")
				m.versionCursor = 0
				m.updateFilteredReleases()
			}
		}
		return m, nil

	case key.Matches(msg, editorKeys.Escape):
		if m.filterInput.Value() != "" {
			m.filterInput.SetValue("")
			m.filteredStreams = m.streams
			m.streamCursor = 0
			m.versionCursor = 0
			return m, nil
		}
		m.quitting = true
		return m, tea.Quit

	case key.Matches(msg, editorKeys.OpenNotes):
		// Open release notes (only in version search mode)
		if m.isVersionSearchMode() && len(m.filteredReleases) > 0 {
			selected := m.filteredReleases[m.versionCursor]
			if selected.ReleaseNotesURL != "" {
				_ = openURL(selected.ReleaseNotesURL)
			}
		}
		return m, nil

	case key.Matches(msg, editorKeys.FilterInstalled):
		// Show installed versions flat list
		m.state = stateInstalledSelect
		m.versionCursor = 0
		// Set filtered releases to installed only
		m.filteredReleases = filterInstalledReleases(m.allReleases)
		sort.Slice(m.filteredReleases, func(i, j int) bool {
			if !m.filteredReleases[i].ReleaseDate.IsZero() && !m.filteredReleases[j].ReleaseDate.IsZero() {
				return m.filteredReleases[i].ReleaseDate.After(m.filteredReleases[j].ReleaseDate)
			}
			return compareVersions(m.filteredReleases[i].Version, m.filteredReleases[j].Version) > 0
		})
		return m, nil
	}

	// Update text input for filtering
	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(msg)

	// Apply filter
	m.applyFilter()

	return m, cmd
}

func (m *editorInstallModel) applyFilter() {
	filter := m.filterInput.Value()

	if m.isVersionSearchMode() {
		// Version search mode
		if !m.loadingReleases {
			m.filteredReleases = FilterReleasesByVersion(m.allReleases, filter)
			if m.versionCursor >= len(m.filteredReleases) {
				m.versionCursor = max(0, len(m.filteredReleases)-1)
			}
		}
	} else {
		// Stream filter mode
		if filter == "" {
			m.filteredStreams = m.streams
		} else {
			filter = strings.ToLower(filter)
			var result []VersionStream
			for _, s := range m.streams {
				if strings.Contains(strings.ToLower(s.DisplayName), filter) ||
					strings.Contains(strings.ToLower(s.MajorMinor), filter) {
					result = append(result, s)
				}
			}
			m.filteredStreams = result
		}
		if m.streamCursor >= len(m.filteredStreams) {
			m.streamCursor = max(0, len(m.filteredStreams)-1)
		}
	}
}

func (m *editorInstallModel) updateFilteredReleases() {
	if m.selectedStream == nil || m.loadingReleases {
		return
	}

	var result []UnityRelease
	for _, r := range m.allReleases {
		if GetMajorMinorFromVersion(r.Version) == m.selectedStream.MajorMinor {
			result = append(result, r)
		}
	}

	// Sort by release date (newest first), fallback to version comparison
	sort.Slice(result, func(i, j int) bool {
		if !result[i].ReleaseDate.IsZero() && !result[j].ReleaseDate.IsZero() {
			return result[i].ReleaseDate.After(result[j].ReleaseDate)
		}
		return compareVersions(result[i].Version, result[j].Version) > 0
	})

	m.filteredReleases = result
}

// filterInstalledReleases returns only installed releases
func filterInstalledReleases(releases []UnityRelease) []UnityRelease {
	var result []UnityRelease
	for _, r := range releases {
		if r.Installed {
			result = append(result, r)
		}
	}
	return result
}

func (m editorInstallModel) updateVersionSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, editorKeys.Up):
		if m.versionCursor > 0 {
			m.versionCursor--
		}
		return m, nil

	case key.Matches(msg, editorKeys.Down):
		if m.versionCursor < len(m.filteredReleases)-1 {
			m.versionCursor++
		}
		return m, nil

	case key.Matches(msg, editorKeys.Enter):
		if len(m.filteredReleases) > 0 {
			selected := m.filteredReleases[m.versionCursor]
			return m.selectVersion(&selected)
		}
		return m, nil

	case key.Matches(msg, editorKeys.OpenNotes):
		// Open release notes in browser
		if len(m.filteredReleases) > 0 {
			selected := m.filteredReleases[m.versionCursor]
			if selected.ReleaseNotesURL != "" {
				_ = openURL(selected.ReleaseNotesURL)
			}
		}
		return m, nil

	case key.Matches(msg, editorKeys.Escape):
		if m.filterInput.Value() != "" {
			m.filterInput.SetValue("")
			m.updateFilteredReleases()
			m.versionCursor = 0
			return m, nil
		}
		// Go back to stream select
		m.state = stateStreamSelect
		m.selectedStream = nil
		m.filterInput.SetValue("")
		return m, nil
	}

	// Update text input for filtering within stream
	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(msg)

	// Filter versions within stream
	filter := m.filterInput.Value()
	if filter == "" {
		m.updateFilteredReleases()
	} else {
		m.filteredReleases = FilterReleasesByVersion(m.filteredReleases, filter)
	}
	if m.versionCursor >= len(m.filteredReleases) {
		m.versionCursor = max(0, len(m.filteredReleases)-1)
	}

	return m, cmd
}

func (m editorInstallModel) selectVersion(selected *UnityRelease) (tea.Model, tea.Cmd) {
	m.selectedVersion = selected
	m.state = stateModuleSelect

	// Prepare modules list
	if len(selected.Modules) > 0 {
		m.modules = selected.Modules
	} else {
		m.modules = GetCommonModules()
	}

	// Filter to visible platform modules only
	var filteredModules []ModuleInfo
	for _, mod := range m.modules {
		if mod.IsVisible() {
			filteredModules = append(filteredModules, mod)
		}
	}
	if len(filteredModules) > 0 {
		m.modules = filteredModules
	}

	// Mark installed modules (always check if version is installed)
	if selected.Installed && selected.InstalledPath != "" {
		for i := range m.modules {
			m.modules[i].Installed = m.client.IsModuleInstalled(selected.InstalledPath, m.modules[i].ID)
		}
	}

	m.moduleCursor = 0
	m.selectedModules = make(map[string]bool)

	return m, nil
}

// updateInstalledSelect handles key input for installed versions list
func (m editorInstallModel) updateInstalledSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, editorKeys.Up):
		if m.versionCursor > 0 {
			m.versionCursor--
		}
		return m, nil

	case key.Matches(msg, editorKeys.Down):
		if m.versionCursor < len(m.filteredReleases)-1 {
			m.versionCursor++
		}
		return m, nil

	case key.Matches(msg, editorKeys.Enter):
		if len(m.filteredReleases) > 0 {
			selected := m.filteredReleases[m.versionCursor]
			return m.selectVersion(&selected)
		}
		return m, nil

	case key.Matches(msg, editorKeys.OpenNotes):
		if len(m.filteredReleases) > 0 {
			selected := m.filteredReleases[m.versionCursor]
			if selected.ReleaseNotesURL != "" {
				_ = openURL(selected.ReleaseNotesURL)
			}
		}
		return m, nil

	case key.Matches(msg, editorKeys.Escape):
		m.state = stateStreamSelect
		return m, nil
	}

	return m, nil
}

func (m editorInstallModel) updateModuleSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, editorKeys.Up):
		if m.moduleCursor > 0 {
			m.moduleCursor--
		}
		return m, nil

	case key.Matches(msg, editorKeys.Down):
		if m.moduleCursor < len(m.modules)-1 {
			m.moduleCursor++
		}
		return m, nil

	case key.Matches(msg, editorKeys.Space):
		if len(m.modules) > 0 {
			mod := m.modules[m.moduleCursor]
			if !mod.Installed {
				m.selectedModules[mod.ID] = !m.selectedModules[mod.ID]
			}
		}
		return m, nil

	case key.Matches(msg, editorKeys.Tab):
		allSelected := true
		for _, mod := range m.modules {
			if !mod.Installed && !m.selectedModules[mod.ID] {
				allSelected = false
				break
			}
		}
		for _, mod := range m.modules {
			if !mod.Installed {
				m.selectedModules[mod.ID] = !allSelected
			}
		}
		return m, nil

	case key.Matches(msg, editorKeys.Enter):
		// Prepare install options and quit TUI (install runs after TUI exits)
		if m.selectedVersion == nil {
			return m, nil
		}

		var modules []string
		for modID, selected := range m.selectedModules {
			if selected {
				modules = append(modules, modID)
			}
		}

		// Check if already installed with no new modules
		if m.selectedVersion.Installed && len(modules) == 0 {
			m.installResult = fmt.Sprintf("Unity %s is already installed", m.selectedVersion.Version)
			m.quitting = true
			return m, tea.Quit
		}

		m.pendingInstall = &InstallOptions{
			Version:      m.selectedVersion.Version,
			Changeset:    m.selectedVersion.Changeset,
			Modules:      modules,
			Architecture: m.architecture,
		}
		m.quitting = true
		return m, tea.Quit

	case key.Matches(msg, editorKeys.Escape):
		m.selectedVersion = nil
		// Return to the appropriate state based on where we came from
		if m.selectedStream != nil {
			m.state = stateVersionSelect
			m.updateFilteredReleases()
		} else {
			// Came from installed versions list
			m.state = stateInstalledSelect
		}
		return m, nil
	}

	return m, nil
}

func (m editorInstallModel) View() string {
	if m.quitting {
		if m.installResult != "" {
			return editorInstalledStyle.Render(m.installResult) + "\n"
		}
		return ""
	}

	switch m.state {
	case stateStreamSelect:
		return m.viewStreamSelect()
	case stateVersionSelect:
		return m.viewVersionSelect()
	case stateInstalledSelect:
		return m.viewInstalledSelect()
	case stateModuleSelect:
		return m.viewModuleSelect()
	case stateInstalling:
		return m.viewInstalling()
	case stateComplete:
		return m.viewComplete()
	}

	return ""
}

func (m editorInstallModel) viewStreamSelect() string {
	if m.loadingStreams {
		return "Loading version streams..."
	}

	if m.err != nil {
		return fmt.Sprintf("Error: %s\n", m.err)
	}

	var b strings.Builder

	// Check if in version search mode
	if m.isVersionSearchMode() {
		return m.viewVersionSearch(&b)
	}

	// Header
	b.WriteString(editorHeaderStyle.Render("Select Unity Version"))
	b.WriteString("\n\n")

	if len(m.streams) == 0 {
		b.WriteString("No version streams found.\n")
		return b.String()
	}

	// Stream list
	maxDisplay := 15
	start := 0
	if m.streamCursor >= maxDisplay {
		start = m.streamCursor - maxDisplay + 1
	}
	end := min(start+maxDisplay, len(m.filteredStreams))

	for i := start; i < end; i++ {
		s := m.filteredStreams[i]
		line := m.formatStreamLine(s)

		if i == m.streamCursor {
			b.WriteString(editorSelectedStyle.Render(line))
		} else {
			b.WriteString(editorNormalStyle.Render(line))
		}
		b.WriteString("\n")
	}

	if len(m.filteredStreams) == 0 {
		b.WriteString(editorMutedStyle.Render("  No matching streams"))
		b.WriteString("\n")
	}

	// Help
	b.WriteString("\n")
	counter := fmt.Sprintf("  %d/%d", len(m.filteredStreams), len(m.streams))
	b.WriteString(editorMutedStyle.Render(counter))
	help := "  C-l:Installed  Enter:Select  Esc:Quit"
	b.WriteString(editorMutedStyle.Render(help))
	b.WriteString("\n")

	// Prompt
	b.WriteString(promptStyle.Render("> "))
	b.WriteString(m.filterInput.View())

	return b.String()
}

func (m editorInstallModel) viewVersionSearch(b *strings.Builder) string {
	// Header
	b.WriteString(editorHeaderStyle.Render("Search Unity Version"))
	b.WriteString("\n\n")

	if m.loadingReleases {
		b.WriteString(editorMutedStyle.Render("  Loading versions..."))
		b.WriteString("\n")
	} else if m.err != nil && len(m.allReleases) == 0 {
		b.WriteString(editorMutedStyle.Render("  Failed to load versions. Check your network connection."))
		b.WriteString("\n")
	} else if len(m.filteredReleases) == 0 {
		b.WriteString(editorMutedStyle.Render("  No matching versions"))
		b.WriteString("\n")
	} else {
		maxDisplay := 15
		start := 0
		if m.versionCursor >= maxDisplay {
			start = m.versionCursor - maxDisplay + 1
		}
		end := min(start+maxDisplay, len(m.filteredReleases))

		for i := start; i < end; i++ {
			r := m.filteredReleases[i]
			line := m.formatVersionLine(r)

			if i == m.versionCursor {
				b.WriteString(editorSelectedStyle.Render(line))
			} else {
				b.WriteString(editorNormalStyle.Render(line))
			}
			b.WriteString("\n")
		}
	}

	// Help
	b.WriteString("\n")
	if !m.loadingReleases {
		counter := fmt.Sprintf("  %d/%d", len(m.filteredReleases), len(m.allReleases))
		b.WriteString(editorMutedStyle.Render(counter))
	}
	help := "  Enter:Select  o:Notes  Esc:Clear"
	b.WriteString(editorMutedStyle.Render(help))
	b.WriteString("\n")

	// Prompt
	b.WriteString(promptStyle.Render("> "))
	b.WriteString(m.filterInput.View())

	return b.String()
}

func (m editorInstallModel) formatStreamLine(s VersionStream) string {
	var parts []string

	// Display name with padding
	name := fmt.Sprintf(" %-20s", s.DisplayName)
	parts = append(parts, name)

	// Version count
	count := editorCountStyle.Render(fmt.Sprintf("(%d versions)", s.TotalCount))
	parts = append(parts, count)

	return strings.Join(parts, " ")
}

func (m editorInstallModel) viewVersionSelect() string {
	if m.selectedStream == nil {
		return "No stream selected\n"
	}

	var b strings.Builder

	// Header
	header := fmt.Sprintf("Select Version - %s", m.selectedStream.DisplayName)
	b.WriteString(editorHeaderStyle.Render(header))
	b.WriteString("\n\n")

	if m.loadingReleases {
		b.WriteString("Loading versions...\n")
		return b.String()
	}

	// Version list
	maxDisplay := 15
	start := 0
	if m.versionCursor >= maxDisplay {
		start = m.versionCursor - maxDisplay + 1
	}
	end := min(start+maxDisplay, len(m.filteredReleases))

	for i := start; i < end; i++ {
		r := m.filteredReleases[i]
		line := m.formatVersionLine(r)

		if i == m.versionCursor {
			b.WriteString(editorSelectedStyle.Render(line))
		} else {
			b.WriteString(editorNormalStyle.Render(line))
		}
		b.WriteString("\n")
	}

	if len(m.filteredReleases) == 0 {
		b.WriteString(editorMutedStyle.Render("  No versions found"))
		b.WriteString("\n")
	}

	// Help
	b.WriteString("\n")
	counter := fmt.Sprintf("  %d/%d", len(m.filteredReleases), m.selectedStream.TotalCount)
	b.WriteString(editorMutedStyle.Render(counter))
	help := "  Enter:Select  o:Notes  Esc:Back"
	b.WriteString(editorMutedStyle.Render(help))
	b.WriteString("\n")

	// Prompt
	b.WriteString(promptStyle.Render("> "))
	b.WriteString(m.filterInput.View())

	return b.String()
}

// viewInstalledSelect displays the installed versions list
func (m editorInstallModel) viewInstalledSelect() string {
	var b strings.Builder

	b.WriteString(editorHeaderStyle.Render("Installed Versions"))
	b.WriteString("\n\n")

	if m.loadingReleases {
		b.WriteString("Loading versions...\n")
		return b.String()
	}

	if len(m.filteredReleases) == 0 {
		b.WriteString(editorMutedStyle.Render("  No installed versions"))
		b.WriteString("\n")
	} else {
		maxDisplay := 15
		start := 0
		if m.versionCursor >= maxDisplay {
			start = m.versionCursor - maxDisplay + 1
		}
		end := min(start+maxDisplay, len(m.filteredReleases))

		for i := start; i < end; i++ {
			r := m.filteredReleases[i]
			line := m.formatInstalledVersionLine(r)

			if i == m.versionCursor {
				b.WriteString(editorSelectedStyle.Render(line))
			} else {
				b.WriteString(editorNormalStyle.Render(line))
			}
			b.WriteString("\n")
		}
	}

	// Help
	b.WriteString("\n")
	counter := fmt.Sprintf("  %d installed", len(m.filteredReleases))
	b.WriteString(editorMutedStyle.Render(counter))
	help := "  Enter:Select  o:Notes  Esc:Back"
	b.WriteString(editorMutedStyle.Render(help))
	b.WriteString("\n")

	return b.String()
}

// formatInstalledVersionLine formats a version line with project count
func (m editorInstallModel) formatInstalledVersionLine(r UnityRelease) string {
	var parts []string

	parts = append(parts, fmt.Sprintf(" %-16s", r.Version))

	// LTS badge
	if r.LTS {
		parts = append(parts, editorLTSStyle.Render("[LTS]"))
	} else {
		parts = append(parts, "     ")
	}

	// Security alert badge
	if r.SecurityAlert != "" {
		parts = append(parts, editorSecurityAlertStyle.Render(" [!"+r.SecurityAlert+"]"))
	}

	// Project count
	if count, ok := m.projectCounts[r.Version]; ok && count > 0 {
		parts = append(parts, editorCountStyle.Render(fmt.Sprintf(" %d projects", count)))
	}

	return strings.Join(parts, "")
}

func (m editorInstallModel) formatVersionLine(r UnityRelease) string {
	var parts []string

	parts = append(parts, fmt.Sprintf(" %-16s", r.Version))

	// LTS badge
	if r.LTS {
		parts = append(parts, editorLTSStyle.Render("[LTS]"))
	} else {
		parts = append(parts, "     ")
	}

	// Recommended badge
	if r.Recommended {
		parts = append(parts, editorRecommendedStyle.Render(" [★]"))
	}

	// Security alert badge
	if r.SecurityAlert != "" {
		parts = append(parts, editorSecurityAlertStyle.Render(" [!"+r.SecurityAlert+"]"))
	}

	// Installed badge
	if r.Installed {
		parts = append(parts, editorInstalledStyle.Render(" [installed]"))
	}

	// NEW badge (within 14 days)
	if !r.ReleaseDate.IsZero() && isNewRelease(r.ReleaseDate) {
		parts = append(parts, editorNewBadgeStyle.Render(" NEW"))
	}

	// Release date
	if !r.ReleaseDate.IsZero() {
		dateStr := r.ReleaseDate.Format("2006-01-02")
		parts = append(parts, editorDateStyle.Render(" "+dateStr))
	}

	// Download size
	if r.DownloadSize > 0 {
		sizeStr := formatBytes(r.DownloadSize)
		parts = append(parts, editorSizeStyle.Render(" ("+sizeStr+")"))
	}

	return strings.Join(parts, "")
}

// isNewRelease returns true if the release is within 14 days
func isNewRelease(releaseDate time.Time) bool {
	return time.Since(releaseDate) < 14*24*time.Hour
}

// formatBytes formats bytes to human readable format
func formatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.0f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func (m editorInstallModel) viewModuleSelect() string {
	if m.selectedVersion == nil {
		return "No version selected\n"
	}

	var b strings.Builder

	header := fmt.Sprintf("Select Modules for Unity %s", m.selectedVersion.Version)
	b.WriteString(editorHeaderStyle.Render(header))
	b.WriteString("\n\n")

	b.WriteString(editorMutedStyle.Render("  Platforms:"))
	b.WriteString("\n")

	for i, mod := range m.modules {
		line := m.formatModuleLine(mod)

		if i == m.moduleCursor {
			b.WriteString(editorSelectedStyle.Render(line))
		} else {
			b.WriteString(editorNormalStyle.Render(line))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	help := "  Space:Toggle  Tab:Toggle All  Enter:Install  Esc:Back"
	b.WriteString(editorMutedStyle.Render(help))
	b.WriteString("\n")

	return b.String()
}

func (m editorInstallModel) formatModuleLine(mod ModuleInfo) string {
	var checkbox string
	if mod.Installed {
		checkbox = editorDisabledStyle.Render("[x]")
	} else if m.selectedModules[mod.ID] {
		checkbox = editorCheckboxStyle.Render("[x]")
	} else {
		checkbox = "[ ]"
	}

	name := mod.Name
	if mod.Name == "" {
		name = mod.ID
	}

	var extras []string
	if mod.Installed {
		extras = append(extras, editorDisabledStyle.Render("installed"))
	}
	if mod.DownloadSize > 0 {
		extras = append(extras, editorSizeStyle.Render(formatBytes(mod.DownloadSize)))
	}

	var suffix string
	if len(extras) > 0 {
		suffix = " (" + strings.Join(extras, ", ") + ")"
	}

	return fmt.Sprintf("  %s %s%s", checkbox, name, suffix)
}

func (m editorInstallModel) viewInstalling() string {
	if m.selectedVersion == nil {
		return "Installing...\n"
	}

	var b strings.Builder

	b.WriteString(fmt.Sprintf("Installing Unity %s...\n", m.selectedVersion.Version))

	var modules []string
	for modID, selected := range m.selectedModules {
		if selected {
			modules = append(modules, modID)
		}
	}
	if len(modules) > 0 {
		b.WriteString(fmt.Sprintf("Modules: %s\n", strings.Join(modules, ", ")))
	}

	return b.String()
}

func (m editorInstallModel) viewComplete() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %s\n\nPress any key to exit.\n", m.err)
	}

	return fmt.Sprintf("%s\n\nPress any key to exit.\n", m.installResult)
}

// RunEditorInstallTUI launches the interactive editor install TUI
func RunEditorInstallTUI(client *Client) error {
	ui.Debug("Starting editor install TUI")

	p := tea.NewProgram(initialEditorInstallModel(client))
	m, err := p.Run()
	if err != nil {
		return err
	}

	// Check if there's a pending install to execute
	model, ok := m.(editorInstallModel)
	if !ok {
		return nil
	}

	// Show install result if set (e.g., "already installed")
	if model.installResult != "" {
		fmt.Println(model.installResult)
		return nil
	}

	// Execute pending install after TUI has exited
	if model.pendingInstall != nil {
		ui.Info("Installing Unity %s...", model.pendingInstall.Version)
		if len(model.pendingInstall.Modules) > 0 {
			ui.Muted("Modules: %s", strings.Join(model.pendingInstall.Modules, ", "))
		}

		// Check if this is just adding modules to existing install
		if model.selectedVersion != nil && model.selectedVersion.Installed {
			if err := client.InstallModules(model.pendingInstall.Version, model.pendingInstall.Modules); err != nil {
				return fmt.Errorf("failed to install modules: %w", err)
			}
			fmt.Printf("Successfully added modules to Unity %s: %s\n",
				model.pendingInstall.Version, strings.Join(model.pendingInstall.Modules, ", "))
		} else {
			if err := client.InstallEditorWithOptions(*model.pendingInstall); err != nil {
				return fmt.Errorf("failed to install Unity: %w", err)
			}
			msg := fmt.Sprintf("Successfully installed Unity %s", model.pendingInstall.Version)
			if len(model.pendingInstall.Modules) > 0 {
				msg += fmt.Sprintf(" with modules: %s", strings.Join(model.pendingInstall.Modules, ", "))
			}
			fmt.Println(msg)
		}
	}

	return nil
}

// openURL opens a URL in the default browser
func openURL(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
	return cmd.Start()
}

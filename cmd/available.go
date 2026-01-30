package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/mattn/go-isatty"
	"github.com/neptaco/uniforge/pkg/hub"
	"github.com/neptaco/uniforge/pkg/ui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	availLTSStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	availVersionStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("43"))
	availInstalledStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	availStreamStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("75"))
	availArchStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

var (
	availableFormat       string
	availableLTS          bool
	availableStream       string
	availableInstalled    bool
	availableNotInstalled bool
	availableMajor        string
	availableLatest       bool
	availableCount        bool
)

var editorAvailableCmd = &cobra.Command{
	Use:   "available",
	Short: "List available Unity Editor versions for installation",
	Long: `List all Unity Editor versions that can be installed.

Examples:
  # Table format (default for TTY)
  uniforge editor available

  # JSON format for scripting
  uniforge editor available --format json

  # LTS versions only
  uniforge editor available --lts

  # Filter by major version
  uniforge editor available --major 6000

  # Latest version per stream
  uniforge editor available --latest

  # Show only not installed versions
  uniforge editor available --not-installed`,
	Aliases: []string{"avail"},
	RunE:    runAvailable,
}

func init() {
	editorCmd.AddCommand(editorAvailableCmd)

	editorAvailableCmd.Flags().StringVar(&availableFormat, "format", "", "Output format: table, json, tsv (auto-detected if not specified)")
	editorAvailableCmd.Flags().BoolVar(&availableLTS, "lts", false, "Show only LTS versions")
	editorAvailableCmd.Flags().StringVar(&availableStream, "stream", "", "Filter by stream: LTS, TECH, BETA, ALPHA")
	editorAvailableCmd.Flags().BoolVar(&availableInstalled, "installed", false, "Show only installed versions")
	editorAvailableCmd.Flags().BoolVar(&availableNotInstalled, "not-installed", false, "Show only not installed versions")
	editorAvailableCmd.Flags().StringVar(&availableMajor, "major", "", "Filter by major version (e.g., 6000, 2022)")
	editorAvailableCmd.Flags().BoolVar(&availableLatest, "latest", false, "Show only latest version per major version")
	editorAvailableCmd.Flags().BoolVar(&availableCount, "count", false, "Show only count of matching versions")
}

func runAvailable(cmd *cobra.Command, args []string) error {
	ui.Debug("Fetching available Unity Editor versions")

	hubClient := hub.NewClient()
	hubClient.NoCache = viper.GetBool("no-cache")

	releases, err := fetchReleasesWithCache(hubClient)
	if err != nil {
		return fmt.Errorf("failed to fetch available releases: %w", err)
	}

	// Apply filters
	releases = filterReleases(releases)

	// Apply --latest filter (after other filters)
	if availableLatest {
		releases = latestPerMajor(releases)
	}

	// Count mode
	if availableCount {
		fmt.Println(len(releases))
		return nil
	}

	if len(releases) == 0 {
		ui.Info("No Unity Editor releases found")
		return nil
	}

	// Determine format
	format := availableFormat
	if format == "" {
		if isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd()) {
			format = "table"
		} else {
			format = "tsv"
		}
	}

	switch format {
	case "json":
		return printAvailableJSON(releases)
	case "tsv":
		return printAvailableTSV(releases)
	case "table":
		return printAvailableTable(releases)
	default:
		return fmt.Errorf("unknown format: %s", format)
	}
}

func fetchReleasesWithCache(client *hub.Client) ([]hub.UnityRelease, error) {
	// Try cache first (unless --no-cache)
	if !client.NoCache {
		cache, err := client.LoadCache()
		if err == nil && cache != nil {
			// Check if cache is valid
			currentStreams, streamErr := client.FetchStreams()
			if streamErr == nil && client.CheckCacheValidity(cache, currentStreams) {
				ui.Debug("Using cached releases")
				releases := client.ConvertCacheToReleases(cache)
				return client.EnrichReleasesWithInstallStatus(releases), nil
			}
		}
	}

	// Fetch from API
	releases, err := ui.WithSpinner("Fetching available releases...", func() ([]hub.UnityRelease, error) {
		return client.GetAllReleases()
	})
	if err != nil {
		return nil, err
	}

	// Save to cache
	streams, _ := client.FetchStreams()
	if len(streams) > 0 {
		_ = client.SaveCache(streams, releases)
	}

	return releases, nil
}

func filterReleases(releases []hub.UnityRelease) []hub.UnityRelease {
	var filtered []hub.UnityRelease
	for _, r := range releases {
		// --lts filter
		if availableLTS && !r.LTS {
			continue
		}
		// --stream filter
		if availableStream != "" && !strings.EqualFold(r.Stream, availableStream) {
			continue
		}
		// --installed filter
		if availableInstalled && !r.Installed {
			continue
		}
		// --not-installed filter
		if availableNotInstalled && r.Installed {
			continue
		}
		// --major filter
		if availableMajor != "" {
			parts := strings.Split(r.Version, ".")
			if len(parts) == 0 || parts[0] != availableMajor {
				continue
			}
		}
		filtered = append(filtered, r)
	}
	return filtered
}

func latestPerMajor(releases []hub.UnityRelease) []hub.UnityRelease {
	// Group by major.minor
	latest := make(map[string]hub.UnityRelease)
	for _, r := range releases {
		parts := strings.Split(r.Version, ".")
		if len(parts) < 2 {
			continue
		}
		key := parts[0] + "." + parts[1]
		if _, exists := latest[key]; !exists {
			latest[key] = r // Already sorted, first is latest
		}
	}

	var result []hub.UnityRelease
	for _, r := range latest {
		result = append(result, r)
	}

	// Sort result
	sort.Slice(result, func(i, j int) bool {
		return compareVersionStrings(result[i].Version, result[j].Version) > 0
	})

	return result
}

func compareVersionStrings(a, b string) int {
	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")
	for i := 0; i < len(aParts) && i < len(bParts); i++ {
		if aParts[i] > bParts[i] {
			return 1
		}
		if aParts[i] < bParts[i] {
			return -1
		}
	}
	return len(aParts) - len(bParts)
}

func printAvailableJSON(releases []hub.UnityRelease) error {
	type jsonRelease struct {
		Version      string `json:"version"`
		Changeset    string `json:"changeset,omitempty"`
		Stream       string `json:"stream"`
		LTS          bool   `json:"lts"`
		Installed    bool   `json:"installed"`
		Architecture string `json:"architecture,omitempty"`
	}

	var output []jsonRelease
	for _, r := range releases {
		output = append(output, jsonRelease{
			Version:      r.Version,
			Changeset:    r.Changeset,
			Stream:       r.Stream,
			LTS:          r.LTS,
			Installed:    r.Installed,
			Architecture: r.Architecture,
		})
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func printAvailableTSV(releases []hub.UnityRelease) error {
	for _, r := range releases {
		installed := "no"
		if r.Installed {
			installed = "yes"
		}
		lts := ""
		if r.LTS {
			lts = "LTS"
		}
		fmt.Printf("%s\t%s\t%s\t%s\t%s\n", r.Version, r.Stream, lts, installed, r.Changeset)
	}
	return nil
}

func printAvailableTable(releases []hub.UnityRelease) error {
	rows := make([][]string, 0, len(releases))
	for _, r := range releases {
		stream := r.Stream
		if r.LTS {
			stream = "LTS"
		}
		installed := ""
		if r.Installed {
			installed = "✓"
		}
		rows = append(rows, []string{r.Version, stream, installed, r.Architecture})
	}

	t := table.New().
		Headers("VERSION", "STREAM", "INSTALLED", "ARCH").
		Rows(rows...).
		Border(lipgloss.HiddenBorder()).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return headerStyle
			}
			switch col {
			case 0:
				return availVersionStyle
			case 1:
				if rows[row][col] == "LTS" {
					return availLTSStyle
				}
				return availStreamStyle
			case 2:
				return availInstalledStyle
			case 3:
				return availArchStyle
			}
			return lipgloss.NewStyle()
		})

	fmt.Println(t)
	return nil
}

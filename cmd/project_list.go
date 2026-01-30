package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/mattn/go-isatty"
	"github.com/neptaco/uniforge/pkg/hub"
	"github.com/neptaco/uniforge/pkg/ui"
	"github.com/spf13/cobra"
)

var (
	projectListFormat   string
	projectListPathOnly bool
	projectListNoGit    bool
)

var projectListCmd = &cobra.Command{
	Use:   "list",
	Short: "List Unity Hub projects",
	Long: `List all Unity projects registered in Unity Hub.

Examples:
  # Table format (default for TTY)
  uniforge project list

  # JSON format
  uniforge project list --format=json

  # TSV format (default for non-TTY)
  uniforge project list --format=tsv

  # Path only (for scripting)
  uniforge project list --path-only

  # Without Git information (faster)
  uniforge project list --no-git`,
	RunE: runProjectList,
}

func init() {
	projectCmd.AddCommand(projectListCmd)

	projectListCmd.Flags().StringVar(&projectListFormat, "format", "", "output format: table, json, tsv (auto-detected if not specified)")
	projectListCmd.Flags().BoolVar(&projectListPathOnly, "path-only", false, "output only project paths")
	projectListCmd.Flags().BoolVar(&projectListNoGit, "no-git", false, "skip Git information (faster)")
}

func runProjectList(cmd *cobra.Command, args []string) error {
	hubClient := hub.NewClient()

	var projects []hub.ProjectInfo
	var err error

	if projectListNoGit {
		projects, err = hubClient.ListProjects()
	} else {
		projects, err = ui.WithSpinner("Fetching projects...", func() ([]hub.ProjectInfo, error) {
			return hubClient.ListProjectsWithGit()
		})
	}

	if err != nil {
		return fmt.Errorf("failed to list projects: %w", err)
	}

	if len(projects) == 0 {
		if projectListFormat == "json" {
			fmt.Println("[]")
		} else {
			ui.Info("No projects registered in Unity Hub")
		}
		return nil
	}

	// Path only mode
	if projectListPathOnly {
		for _, p := range projects {
			fmt.Println(p.Path)
		}
		return nil
	}

	// Determine format
	format := projectListFormat
	if format == "" {
		if isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd()) {
			format = "table"
		} else {
			format = "tsv"
		}
	}

	switch format {
	case "json":
		return printProjectsJSON(projects)
	case "tsv":
		return printProjectsTSV(projects)
	case "table":
		return printProjectsTable(projects)
	default:
		return fmt.Errorf("unknown format: %s", format)
	}
}

func printProjectsJSON(projects []hub.ProjectInfo) error {
	type jsonProject struct {
		Name      string `json:"name"`
		Path      string `json:"path"`
		Version   string `json:"version"`
		GitBranch string `json:"git_branch,omitempty"`
		GitStatus string `json:"git_status,omitempty"`
	}

	var output []jsonProject
	for _, p := range projects {
		output = append(output, jsonProject{
			Name:      p.Title,
			Path:      p.Path,
			Version:   p.Version,
			GitBranch: p.GitBranch,
			GitStatus: p.GitStatus,
		})
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func printProjectsTSV(projects []hub.ProjectInfo) error {
	for _, p := range projects {
		gitInfo := ""
		if p.GitBranch != "" {
			gitInfo = p.GitBranch
			if p.GitStatus != "" {
				gitInfo += " (" + p.GitStatus + ")"
			}
		}
		fmt.Printf("%s\t%s\t%s\t%s\n", p.Title, p.Version, gitInfo, p.Path)
	}
	return nil
}

func printProjectsTable(projects []hub.ProjectInfo) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	fmt.Fprintln(w, "NAME\tVERSION\tGIT\tPATH")
	fmt.Fprintln(w, "----\t-------\t---\t----")

	for _, p := range projects {
		gitInfo := formatGitInfo(p.GitBranch, p.GitStatus)
		// Truncate path for display
		displayPath := truncatePath(p.Path, 50)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", p.Title, p.Version, gitInfo, displayPath)
	}

	return w.Flush()
}

func formatGitInfo(branch, status string) string {
	if branch == "" {
		return "—"
	}

	if status == "" || status == "clean" {
		return branch
	}

	return fmt.Sprintf("%s (%s)", branch, status)
}

func truncatePath(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}

	// Try to show meaningful parts
	parts := strings.Split(path, string(os.PathSeparator))
	if len(parts) <= 3 {
		return "..." + path[len(path)-maxLen+3:]
	}

	// Show first and last parts
	result := parts[0] + "/.../" + strings.Join(parts[len(parts)-2:], "/")
	if len(result) > maxLen {
		return "..." + path[len(path)-maxLen+3:]
	}
	return result
}

package cmd

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/neptaco/uniforge/pkg/hub"
	"github.com/neptaco/uniforge/pkg/ui"
	"github.com/spf13/cobra"
)

var (
	editorVersionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("43"))
	editorPathStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

var editorListCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed Unity Editor versions",
	Long:  `List all installed Unity Editor versions managed by Unity Hub.`,
	RunE:  runList,
}

func init() {
	editorCmd.AddCommand(editorListCmd)
}

func runList(cmd *cobra.Command, args []string) error {
	ui.Debug("Listing installed Unity Editor versions")

	editors, err := ui.WithSpinner("Fetching installed editors...", func() ([]hub.EditorInfo, error) {
		hubClient := hub.NewClient()
		return hubClient.ListInstalledEditors()
	})
	if err != nil {
		return fmt.Errorf("failed to list editors: %w", err)
	}

	if len(editors) == 0 {
		ui.Info("No Unity Editor installations found")
		return nil
	}

	rows := make([][]string, 0, len(editors))
	for _, editor := range editors {
		rows = append(rows, []string{editor.Version, editor.Path})
	}

	t := table.New().
		Headers("VERSION", "PATH").
		Rows(rows...).
		Border(lipgloss.HiddenBorder()).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return headerStyle
			}
			switch col {
			case 0:
				return editorVersionStyle
			case 1:
				return editorPathStyle
			}
			return lipgloss.NewStyle()
		})

	fmt.Println(t)
	return nil
}

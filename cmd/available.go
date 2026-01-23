package cmd

import (
	"fmt"
	"strings"

	"github.com/neptaco/uniforge/pkg/hub"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	availableAll bool
)

var editorAvailableCmd = &cobra.Command{
	Use:   "available",
	Short: "List available Unity Editor versions for installation",
	Long: `List all Unity Editor versions that can be installed through Unity Hub.
This shows the official releases available in Unity Hub's release list.`,
	Aliases: []string{"avail"},
	RunE:    runAvailable,
}

func init() {
	editorCmd.AddCommand(editorAvailableCmd)

	editorAvailableCmd.Flags().BoolVar(&availableAll, "all", false, "Show all available versions including alpha/beta")
}

func runAvailable(cmd *cobra.Command, args []string) error {
	logrus.Debug("Fetching available Unity Editor versions")

	hubClient := hub.NewClient()
	releases, err := hubClient.ListAvailableReleases()
	if err != nil {
		return fmt.Errorf("failed to fetch available releases: %w", err)
	}

	if len(releases) == 0 {
		fmt.Println("No Unity Editor releases found")
		return nil
	}

	// Group by major version
	grouped := make(map[string][]hub.ReleaseInfo)
	for _, release := range releases {
		// Extract major version (e.g., "2022.3" from "2022.3.10f1")
		parts := strings.Split(release.Version, ".")
		if len(parts) >= 2 {
			majorVersion := parts[0] + "." + parts[1]
			grouped[majorVersion] = append(grouped[majorVersion], release)
		} else {
			grouped["other"] = append(grouped["other"], release)
		}
	}

	// Display grouped versions
	fmt.Println("Available Unity Editor versions:")
	fmt.Println()

	// Sort and display LTS versions first
	for version, releases := range grouped {
		if strings.Contains(version, "2022.3") || strings.Contains(version, "2021.3") {
			fmt.Printf("LTS %s:\n", version)
			for _, release := range releases {
				displayRelease(release)
			}
			fmt.Println()
		}
	}

	// Display other versions
	for version, releases := range grouped {
		if !strings.Contains(version, "2022.3") && !strings.Contains(version, "2021.3") {
			fmt.Printf("%s:\n", version)
			for _, release := range releases {
				displayRelease(release)
			}
			fmt.Println()
		}
	}

	fmt.Println("\nTo install a specific version, use:")
	fmt.Println("  uniforge editor install --version <version>")

	return nil
}

func displayRelease(release hub.ReleaseInfo) {
	if release.Architecture != "" {
		fmt.Printf("  - %s (%s)\n", release.Version, release.Architecture)
	} else {
		fmt.Printf("  - %s\n", release.Version)
	}
	
	if release.Changeset != "" {
		fmt.Printf("    Changeset: %s\n", release.Changeset)
	}
}
package cmd

import (
	"fmt"

	"github.com/neptaco/uniforge/pkg/hub"
	"github.com/spf13/cobra"
)

var cacheClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear the release cache",
	Long: `Clear the cached Unity release information.

This removes the local cache file that stores Unity release data.
The cache will be rebuilt on the next command that fetches release information.`,
	RunE: runCacheClear,
}

func init() {
	cacheCmd.AddCommand(cacheClearCmd)
}

func runCacheClear(cmd *cobra.Command, args []string) error {
	hubClient := hub.NewClient()

	if err := hubClient.ClearCache(); err != nil {
		return fmt.Errorf("failed to clear cache: %w", err)
	}

	fmt.Println("Cache cleared successfully")
	return nil
}

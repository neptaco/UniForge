package cmd

import (
	"github.com/spf13/cobra"
)

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Manage UniForge cache",
	Long:  `Commands to manage the UniForge cache for Unity releases.`,
}

func init() {
	rootCmd.AddCommand(cacheCmd)
}

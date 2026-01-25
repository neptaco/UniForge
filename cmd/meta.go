package cmd

import (
	"github.com/spf13/cobra"
)

var metaCmd = &cobra.Command{
	Use:   "meta",
	Short: "Manage Unity .meta files",
	Long:  `Commands for managing Unity .meta files.`,
}

func init() {
	rootCmd.AddCommand(metaCmd)
}

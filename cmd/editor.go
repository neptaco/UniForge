package cmd

import (
	"github.com/spf13/cobra"
)

var editorCmd = &cobra.Command{
	Use:   "editor",
	Short: "Manage Unity Editor installations",
	Long:  `Commands for managing Unity Editor installations via Unity Hub.`,
}

func init() {
	rootCmd.AddCommand(editorCmd)
}
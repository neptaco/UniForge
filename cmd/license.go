package cmd

import (
	"github.com/spf13/cobra"
)

var licenseCmd = &cobra.Command{
	Use:   "license",
	Short: "Manage Unity license",
	Long:  `Commands for managing Unity license activation and return.`,
}

func init() {
	rootCmd.AddCommand(licenseCmd)
}

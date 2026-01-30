package cmd

import (
	"fmt"
	"os"

	"github.com/neptaco/uniforge/pkg/ui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile  string
	logLevel string
	Version  string
)

var rootCmd = &cobra.Command{
	Use:   "uniforge",
	Short: "Command-line tool for Unity development",
	Long: `UniForge is a command-line tool for Unity development.
It provides functionality to manage Unity Editor installations,
build Unity projects, and run Unity in batch mode.`,
}

func Execute(version string) {
	Version = version
	rootCmd.Version = version
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.uniforge.yaml)")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().Bool("no-color", false, "disable colored output")
	rootCmd.PersistentFlags().Bool("no-cache", false, "skip reading from cache (still writes to cache)")

	rootCmd.SetVersionTemplate(`{{printf "%s\n" .Version}}`)

	if err := viper.BindPFlag("log-level", rootCmd.PersistentFlags().Lookup("log-level")); err != nil {
		ui.Error("Failed to bind log-level flag: %v", err)
		os.Exit(1)
	}
	if err := viper.BindPFlag("no-color", rootCmd.PersistentFlags().Lookup("no-color")); err != nil {
		ui.Error("Failed to bind no-color flag: %v", err)
		os.Exit(1)
	}
	if err := viper.BindPFlag("no-cache", rootCmd.PersistentFlags().Lookup("no-cache")); err != nil {
		ui.Error("Failed to bind no-cache flag: %v", err)
		os.Exit(1)
	}
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		viper.AddConfigPath(home)
		viper.SetConfigType("yaml")
		viper.SetConfigName(".uniforge")
	}

	viper.SetEnvPrefix("UNIFORGE")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err == nil {
		ui.Debug("Using config file", "path", viper.ConfigFileUsed())
	}

	// Set debug mode based on log level
	logLevel := viper.GetString("log-level")
	ui.SetDebugMode(logLevel == "debug")
}

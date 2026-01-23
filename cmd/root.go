package cmd

import (
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
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
	Short: "Unity CI/CD command-line tool",
	Long: `Uniforge is a command-line tool for Unity CI/CD operations.
It provides functionality to manage Unity Editor installations,
build Unity projects, and run Unity in batch mode for CI/CD pipelines.`,
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

	rootCmd.SetVersionTemplate(`{{printf "%s\n" .Version}}`)

	if err := viper.BindPFlag("log-level", rootCmd.PersistentFlags().Lookup("log-level")); err != nil {
		logrus.Fatalf("Failed to bind log-level flag: %v", err)
	}
	if err := viper.BindPFlag("no-color", rootCmd.PersistentFlags().Lookup("no-color")); err != nil {
		logrus.Fatalf("Failed to bind no-color flag: %v", err)
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
		logrus.Debugf("Using config file: %s", viper.ConfigFileUsed())
	}

	level, err := logrus.ParseLevel(viper.GetString("log-level"))
	if err != nil {
		level = logrus.InfoLevel
	}
	logrus.SetLevel(level)

	if viper.GetBool("no-color") || os.Getenv("NO_COLOR") != "" {
		logrus.SetFormatter(&logrus.TextFormatter{
			DisableColors: true,
		})
	}
}

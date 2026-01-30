package cmd

import (
	"fmt"
	"strings"

	"github.com/neptaco/uniforge/pkg/hub"
	"github.com/neptaco/uniforge/pkg/ui"
	"github.com/neptaco/uniforge/pkg/unity"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	installModules      string
	installChangeset    string
	installArchitecture string
	installForce        bool
	installProject      string
)

var editorInstallCmd = &cobra.Command{
	Use:   "install [version]",
	Short: "Install Unity Editor version",
	Long: `Install a specific Unity Editor version with optional modules.
You can specify a version directly or let it detect from a Unity project.
If no version is specified and not in a Unity project, launches interactive TUI.

If the editor is already installed:
  - Without --modules: skips installation (use --force to reinstall)
  - With --modules: checks if modules are installed and adds missing ones

Examples:
  # Interactive mode - select version and modules from TUI
  uniforge editor install

  # Install from current directory's project
  uniforge editor install -p .

  # Install specific version
  uniforge editor install 2022.3.10f1

  # Install from specific project path
  uniforge editor install -p /path/to/project

  # Install with modules
  uniforge editor install 2022.3.10f1 --modules ios,android

  # Add modules to existing editor (only installs missing modules)
  uniforge editor install 2022.3.10f1 --modules webgl`,
	Args:         cobra.MaximumNArgs(1),
	RunE:         runInstall,
	SilenceUsage: true,
}

func init() {
	editorCmd.AddCommand(editorInstallCmd)

	editorInstallCmd.Flags().StringVarP(&installProject, "project", "p", "", "Path to Unity project (enables project detection mode)")
	editorInstallCmd.Flags().StringVar(&installModules, "modules", "", "Comma-separated list of modules to install (e.g., ios,android)")
	editorInstallCmd.Flags().StringVar(&installChangeset, "changeset", "", "Changeset for versions not in release list")
	editorInstallCmd.Flags().StringVar(&installArchitecture, "architecture", "", "Architecture to install (x86_64 or arm64, auto-detect if not specified)")
	editorInstallCmd.Flags().BoolVar(&installForce, "force", false, "Force reinstall even if already installed")
}

func runInstall(cmd *cobra.Command, args []string) error {
	var version string
	var changeset string

	hubClient := hub.NewClient()
	hubClient.NoCache = viper.GetBool("no-cache")

	if len(args) > 0 {
		// Version specified as positional argument
		version = args[0]
	} else if installProject != "" {
		// Project path specified - detect from project
		ui.Debug("Detecting Unity version from project", "path", installProject)

		project, err := unity.LoadProject(installProject)
		if err != nil {
			return fmt.Errorf("failed to load project: %w", err)
		}

		version = project.UnityVersion
		ui.Info("Detected Unity version: %s", version)

		// Use changeset from project if not specified via flag
		if installChangeset == "" && project.Changeset != "" {
			changeset = project.Changeset
			ui.Muted("Detected changeset: %s", changeset)
		}
	} else {
		// No version and no project specified - launch interactive TUI
		return hub.RunEditorInstallTUI(hubClient)
	}

	// Override with flag if provided
	if installChangeset != "" {
		changeset = installChangeset
	}

	// Parse modules early so we can check if they're installed
	modules := []string{}
	if installModules != "" {
		modules = strings.Split(installModules, ",")
		for i := range modules {
			modules[i] = strings.TrimSpace(modules[i])
		}
	}

	// Check if already installed (do this once and reuse the result)
	var isInstalled bool
	var installedPath string
	if !installForce {
		var err error
		isInstalled, installedPath, err = hubClient.IsEditorInstalled(version)
		if err != nil {
			ui.Warn("Failed to check if editor is installed: %v", err)
		} else if isInstalled {
			// If already installed and no changeset was provided, try to get it from the installed editor
			if changeset == "" {
				installedChangeset := hubClient.GetEditorChangeset(installedPath)
				if installedChangeset != "" {
					changeset = installedChangeset
					ui.Muted("Found changeset from installed editor: %s", changeset)
				}
			}

			// Check if requested modules are installed
			if len(modules) > 0 {
				missingModules := hubClient.GetMissingModules(installedPath, modules)
				if len(missingModules) > 0 {
					ui.Info("Unity Editor %s is installed, but missing modules: %s", version, strings.Join(missingModules, ", "))
					ui.Info("Installing missing modules...")

					if err := hubClient.InstallModules(version, missingModules); err != nil {
						return fmt.Errorf("failed to install modules: %w", err)
					}

					fmt.Printf("Successfully installed modules: %s\n", strings.Join(missingModules, ", "))
					return nil
				}
			}

			fmt.Printf("Unity Editor %s is already installed at: %s\n", version, installedPath)
			if changeset != "" {
				fmt.Printf("Changeset: %s\n", changeset)
			}
			if len(modules) > 0 {
				fmt.Printf("All requested modules are already installed: %s\n", strings.Join(modules, ", "))
			}
			fmt.Println("Use --force to reinstall")
			return nil
		}
	}

	// If no changeset and not installed, try to fetch from Unity API
	if changeset == "" && version != "" && !isInstalled {
		apiChangeset, err := ui.WithSpinner("Fetching changeset from Unity API...", func() (string, error) {
			return unity.GetChangesetForVersion(version)
		})
		if err != nil {
			ui.Warn("Failed to fetch changeset from API: %v", err)
			ui.Muted("You may need to provide --changeset manually")
		} else {
			changeset = apiChangeset
			ui.Muted("Found changeset: %s", changeset)
		}
	}

	ui.Info("Installing Unity Editor %s", version)

	// Configure installation options
	options := hub.InstallOptions{
		Version:      version,
		Changeset:    changeset,
		Modules:      modules,
		Architecture: installArchitecture,
	}

	if err := hubClient.InstallEditorWithOptions(options); err != nil {
		return fmt.Errorf("failed to install Unity Editor: %w", err)
	}

	fmt.Printf("Successfully installed Unity Editor %s\n", version)
	if len(modules) > 0 {
		fmt.Printf("With modules: %s\n", strings.Join(modules, ", "))
	}

	return nil
}

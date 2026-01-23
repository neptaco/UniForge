package cmd

import (
	"fmt"
	"strings"

	"github.com/neptaco/uniforge/pkg/hub"
	"github.com/neptaco/uniforge/pkg/unity"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	installVersion     string
	installFromProject string
	installModules     string
	installChangeset   string
	installArchitecture string
	installForce       bool
)

var editorInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install Unity Editor version",
	Long: `Install a specific Unity Editor version with optional modules.
You can specify a version directly or let it detect from a Unity project.
If no version is specified, it will try to detect from the current directory.
If the editor is already installed, it will skip the installation unless --force is specified.`,
	RunE: runInstall,
}

func init() {
	editorCmd.AddCommand(editorInstallCmd)

	editorInstallCmd.Flags().StringVar(&installVersion, "version", "", "Unity Editor version to install (e.g., 2022.3.10f1)")
	editorInstallCmd.Flags().StringVar(&installFromProject, "from-project", "", "Path to Unity project to detect version from")
	editorInstallCmd.Flags().StringVar(&installModules, "modules", "", "Comma-separated list of modules to install (e.g., ios,android)")
	editorInstallCmd.Flags().StringVar(&installChangeset, "changeset", "", "Changeset for versions not in release list")
	editorInstallCmd.Flags().StringVar(&installArchitecture, "architecture", "", "Architecture to install (x86_64 or arm64, auto-detect if not specified)")
	editorInstallCmd.Flags().BoolVar(&installForce, "force", false, "Force reinstall even if already installed")
}

func runInstall(cmd *cobra.Command, args []string) error {
	var version string
	var changeset string

	if installVersion != "" {
		version = installVersion
	} else {
		// Try to detect from project (explicit path or current directory)
		projectPath := installFromProject
		if projectPath == "" {
			projectPath = "."
		}

		logrus.Debugf("Detecting Unity version from project: %s", projectPath)

		project, err := unity.LoadProject(projectPath)
		if err != nil {
			if installFromProject != "" {
				return fmt.Errorf("failed to load project: %w", err)
			}
			return fmt.Errorf("no version specified and current directory is not a Unity project: %w", err)
		}

		version = project.UnityVersion
		logrus.Infof("Detected Unity version: %s", version)

		// Use changeset from project if not specified via flag
		if installChangeset == "" && project.Changeset != "" {
			changeset = project.Changeset
			logrus.Infof("Detected changeset: %s", changeset)
		}
	}
	
	// Override with flag if provided
	if installChangeset != "" {
		changeset = installChangeset
	}
	
	hubClient := hub.NewClient()
	
	// Check if already installed (do this once and reuse the result)
	var isInstalled bool
	var installedPath string
	if !installForce {
		var err error
		isInstalled, installedPath, err = hubClient.IsEditorInstalled(version)
		if err != nil {
			logrus.Warnf("Failed to check if editor is installed: %v", err)
		} else if isInstalled {
			// If already installed and no changeset was provided, try to get it from the installed editor
			if changeset == "" {
				installedChangeset := hubClient.GetEditorChangeset(installedPath)
				if installedChangeset != "" {
					changeset = installedChangeset
					logrus.Infof("Found changeset from installed editor: %s", changeset)
					fmt.Printf("Unity Editor %s is already installed at: %s\n", version, installedPath)
					fmt.Printf("Changeset: %s\n", changeset)
				} else {
					fmt.Printf("Unity Editor %s is already installed at: %s\n", version, installedPath)
				}
			} else {
				fmt.Printf("Unity Editor %s is already installed at: %s\n", version, installedPath)
				fmt.Printf("Changeset: %s\n", changeset)
			}
			fmt.Println("Use --force to reinstall")
			return nil
		}
	}
	
	// If no changeset and not installed, try to fetch from Unity API
	if changeset == "" && version != "" && !isInstalled {
		logrus.Info("No changeset provided, attempting to fetch from Unity API...")
		apiChangeset, err := unity.GetChangesetForVersion(version)
		if err != nil {
			logrus.Warnf("Failed to fetch changeset from API: %v", err)
			logrus.Info("You may need to provide --changeset manually")
		} else {
			changeset = apiChangeset
			logrus.Infof("Found changeset from Unity API: %s", changeset)
		}
	}

	logrus.Infof("Installing Unity Editor %s", version)
	
	modules := []string{}
	if installModules != "" {
		modules = strings.Split(installModules, ",")
		for i := range modules {
			modules[i] = strings.TrimSpace(modules[i])
		}
	}

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
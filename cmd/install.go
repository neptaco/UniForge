package cmd

import (
	"encoding/json"
	"errors"
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
	installOutput       string
)

// installResult is the terminal event of the JSON Lines stream, emitted after
// the progress and log events that pkg/hub writes.
type installResult struct {
	Type    string   `json:"type"`
	Status  string   `json:"status"`
	Version string   `json:"version"`
	Path    string   `json:"path,omitempty"`
	Modules []string `json:"modules,omitempty"`
}

// installRun carries the resolved inputs for one install invocation.
type installRun struct {
	cmd        *cobra.Command
	hub        *hub.Client
	status     statusPrinter
	jsonOutput bool
	version    string
	changeset  string
	modules    []string
}

func (r *installRun) emitResult(status, path string, modules []string) error {
	return json.NewEncoder(r.cmd.OutOrStdout()).Encode(installResult{
		Type:    "result",
		Status:  status,
		Version: r.version,
		Path:    path,
		Modules: modules,
	})
}

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
  uniforge editor install 2022.3.10f1 --modules webgl

  # JSON output (for programmatic use): one JSON object per line
  uniforge editor install 2022.3.10f1 --output json

Only one install of a given editor version runs at a time, across all
architectures and including module installs. A second run for the same version
exits immediately instead of downloading a duplicate.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runInstall,
}

func init() {
	editorCmd.AddCommand(editorInstallCmd)

	editorInstallCmd.Flags().StringVarP(&installProject, "project", "p", "", "Path to Unity project (enables project detection mode)")
	editorInstallCmd.Flags().StringVar(&installModules, "modules", "", "Comma-separated list of modules to install (e.g., ios,android)")
	editorInstallCmd.Flags().StringVar(&installChangeset, "changeset", "", "Changeset for versions not in release list")
	editorInstallCmd.Flags().StringVar(&installArchitecture, "architecture", "", "Architecture to install (x86_64 or arm64, auto-detect if not specified)")
	editorInstallCmd.Flags().BoolVar(&installForce, "force", false, "Force reinstall even if already installed")
	addOutputFlag(editorInstallCmd, &installOutput, "Output format: text, json")
}

func runInstall(cmd *cobra.Command, args []string) error {
	output := resolveOutputOrDefault(installOutput, "text")

	// The interactive picker renders terminal frames to stdout, which is where
	// the JSON Lines stream goes. Refuse rather than emit a corrupt stream.
	if output == "json" && len(args) == 0 && installProject == "" {
		return fmt.Errorf("--output json requires a version argument or --project: the interactive picker cannot run in json mode")
	}

	hubClient := hub.NewClient()
	hubClient.NoCache = viper.GetBool("no-cache")
	hubClient.ProgressJSON = output == "json"

	run := &installRun{
		cmd:        cmd,
		hub:        hubClient,
		status:     newStatusPrinter(output),
		jsonOutput: output == "json",
	}

	switch {
	case len(args) > 0:
		// Version specified as positional argument
		run.version = args[0]
	case installProject != "":
		// Project path specified - detect from project
		ui.Debug("Detecting Unity version from project", "path", installProject)

		project, err := unity.LoadProject(installProject)
		if err != nil {
			return fmt.Errorf("failed to load project: %w", err)
		}

		run.version = project.UnityVersion
		run.status.Info("Detected Unity version: %s", run.version)

		// Use changeset from project if not specified via flag
		if installChangeset == "" && project.Changeset != "" {
			run.changeset = project.Changeset
			run.status.Muted("Detected changeset: %s", run.changeset)
		}
	default:
		// No version and no project specified - launch interactive TUI
		return hub.RunEditorInstallTUI(hubClient)
	}

	// Override with flag if provided
	if installChangeset != "" {
		run.changeset = installChangeset
	}

	// Parse modules early so we can check if they're installed
	if installModules != "" {
		run.modules = strings.Split(installModules, ",")
		for i := range run.modules {
			run.modules[i] = strings.TrimSpace(run.modules[i])
		}
	}

	if !installForce {
		installed, installedPath, err := hubClient.IsEditorInstalled(run.version)
		switch {
		case err != nil:
			run.status.Warn("Failed to check if editor is installed: %v", err)
		// An explicit --architecture is not satisfied by an editor built for a
		// different one; fall through to the install so Hub fetches it.
		case installed && hubClient.ArchitectureMatches(installedPath, installArchitecture):
			return run.reportExisting(installedPath)
		}
	}

	return run.install()
}

// reportExisting handles an editor that is already present: it installs any
// requested modules that are missing, then reports what was found.
func (r *installRun) reportExisting(installedPath string) error {
	// No changeset provided, so take it from the installed editor.
	if r.changeset == "" {
		if installedChangeset := r.hub.GetEditorChangeset(installedPath); installedChangeset != "" {
			r.changeset = installedChangeset
			r.status.Muted("Found changeset from installed editor: %s", r.changeset)
		}
	}

	if len(r.modules) > 0 {
		missingModules := r.hub.GetMissingModules(installedPath, r.modules)
		if len(missingModules) > 0 {
			r.status.Info("Unity Editor %s is installed, but missing modules: %s", r.version, strings.Join(missingModules, ", "))
			r.status.Info("Installing missing modules...")

			installedModules, err := r.hub.InstallModules(r.version, missingModules)
			if err != nil {
				if errors.Is(err, hub.ErrInstallInProgress) {
					return fmt.Errorf("%w; wait for it to finish or stop it before retrying", err)
				}
				return fmt.Errorf("failed to install modules: %w", err)
			}

			if r.jsonOutput {
				return r.emitResult("modules-installed", installedPath, installedModules)
			}
			fmt.Printf("Successfully installed modules: %s\n", strings.Join(installedModules, ", "))
			return nil
		}
	}

	if r.jsonOutput {
		return r.emitResult("already-installed", installedPath, r.modules)
	}

	fmt.Printf("Unity Editor %s is already installed at: %s\n", r.version, installedPath)
	if r.changeset != "" {
		fmt.Printf("Changeset: %s\n", r.changeset)
	}
	if len(r.modules) > 0 {
		fmt.Printf("All requested modules are already installed: %s\n", strings.Join(r.modules, ", "))
	}
	fmt.Println("Use --force to reinstall")
	return nil
}

func (r *installRun) install() error {
	if r.changeset == "" && r.version != "" {
		r.resolveChangesetFromAPI()
	}

	r.status.Info("Installing Unity Editor %s", r.version)

	outcome, err := r.hub.InstallEditorWithOptions(hub.InstallOptions{
		Version:      r.version,
		Changeset:    r.changeset,
		Modules:      r.modules,
		Architecture: installArchitecture,
		Force:        installForce,
	})
	if err != nil {
		if errors.Is(err, hub.ErrInstallInProgress) {
			return fmt.Errorf("%w; wait for it to finish or stop it before retrying", err)
		}
		return fmt.Errorf("failed to install Unity Editor: %w", err)
	}

	if r.jsonOutput {
		status := "installed"
		switch {
		case outcome.AlreadyInstalled && len(outcome.Modules) > 0:
			// Another process installed the editor; this run added the modules.
			status = "modules-installed"
		case outcome.AlreadyInstalled:
			status = "already-installed"
		}
		return r.emitResult(status, outcome.Path, outcome.Modules)
	}

	if outcome.AlreadyInstalled {
		fmt.Printf("Unity Editor %s was already installed at: %s\n", r.version, outcome.Path)
		if len(outcome.Modules) > 0 {
			fmt.Printf("Added modules: %s\n", strings.Join(outcome.Modules, ", "))
		}
		return nil
	}

	fmt.Printf("Successfully installed Unity Editor %s\n", r.version)
	if len(outcome.Modules) > 0 {
		fmt.Printf("With modules: %s\n", strings.Join(outcome.Modules, ", "))
	}

	return nil
}

func (r *installRun) resolveChangesetFromAPI() {
	fetch := func() (string, error) {
		return unity.GetChangesetForVersion(r.version)
	}

	var changeset string
	var err error
	if r.jsonOutput {
		// The spinner would write animation frames into the JSON stream.
		changeset, err = fetch()
	} else {
		changeset, err = ui.WithSpinner("Fetching changeset from Unity API...", fetch)
	}

	if err != nil {
		r.status.Warn("Failed to fetch changeset from API: %v", err)
		r.status.Muted("You may need to provide --changeset manually")
		return
	}

	r.changeset = changeset
	r.status.Muted("Found changeset: %s", r.changeset)
}

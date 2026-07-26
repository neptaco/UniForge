# UniForge

UniForge is a command-line tool for Unity development. With the Unity package installed, you can inspect and change an open Unity Editor from the terminal. When the Editor is closed, you can compile projects and run tests in batch mode.

## What You Can Do

- Work with an open Editor: inspect scenes, change GameObjects and components, control Play Mode, and run tests.
- Compile and test projects with the Editor closed.
- Check `.meta` files for missing files, orphan files, and duplicate GUIDs.
- Read Unity logs and diagnose stale runtime state.
- Read tool results as YAML, or request JSON for scripts and CI.
- Install the included skill so supported coding agents can follow the same UniForge workflow.
- Install Unity versions and open Unity Hub projects when needed.
- Use the same commands on macOS, Windows, and Linux.

## Quick Start

To use UniForge with an open Unity Editor, first install the CLI. On macOS or Linux:

```bash
curl -fsSL https://github.com/neptaco/uniforge/releases/latest/download/install.sh | sh
```

Next, move to the Unity project root—the directory that contains `Assets`, `Packages`, and `ProjectSettings`:

```bash
cd /path/to/MyUnityProject
```

Then add the [UniForge Unity package](https://github.com/neptaco/uniforge-unity):

```bash
uniforge package add neptaco/uniforge-unity/Packages/dev.crysta.uniforge
```

When the project argument is omitted, UniForge detects the Unity project containing the current directory. The GitHub package source is expanded to an HTTPS Git URL. When `--tag` is omitted, UniForge selects the highest semantic-version tag from the repository.

Before changing `Packages/manifest.json`, UniForge reads the package metadata at the resolved tag and compares its minimum Unity version with the project's `ProjectSettings/ProjectVersion.txt`. Newer Unity streams, including alpha and beta releases, are accepted when their version is at least the declared minimum. An older or unparseable version stops the installation without changing the project. Use `--force` only when you intentionally want to bypass this check.

To run the command from another directory, pass the project path explicitly:

```bash
uniforge package add /path/to/MyUnityProject neptaco/uniforge-unity/Packages/dev.crysta.uniforge
```

In an interactive terminal, UniForge shows the project Unity version, package compatibility, resolved URL, tag, package reference, and manifest path before making the change. Enter `n` to cancel, or add `--yes` to skip this confirmation. Non-interactive commands skip the prompt so CI and coding agents do not wait for input, but they still run the compatibility check.

To pin a release:

```bash
uniforge package add neptaco/uniforge-unity/Packages/dev.crysta.uniforge --tag v0.11.0
```

You can also pass the full package URL without a fragment:

```bash
uniforge package add "https://github.com/neptaco/uniforge-unity.git?path=Packages/dev.crysta.uniforge" --tag v0.11.0
```

If you prefer the Unity interface, use **Window > Package Management > Package Manager > Install package from git URL** and paste:

```text
https://github.com/neptaco/uniforge-unity.git?path=Packages/dev.crysta.uniforge#v0.11.0
```

Open the project in Unity. The package connects automatically, and UniForge starts its local daemon when a tool command needs it.

See the connected Editor:

```bash
uniforge tool projects
```

Inspect the active scene:

```bash
uniforge tool call hierarchy
```

List the tools available in the project:

```bash
uniforge tool list
```

Check the arguments accepted by a tool:

```bash
uniforge tool describe create-gameobject
```

Windows installation is covered in [Installation](#installation). See [Live Unity tools](#live-unity-tools-editor-open) for more examples, [Batch mode](#compile-test-and-run-in-batch-mode-editor-closed) for closed-Editor commands, and [GitHub Actions](#github-actions) for CI setup.

## Coding Agent Skill

This repository includes a skill that teaches supported coding agents how to choose between live Editor tools and batch mode, run Unity tests, inspect logs, and diagnose common Unity problems. Install it from the root of the project where you use your coding agent:

```bash
npx skills add neptaco/uniforge --skill uniforge
```

The command requires Node.js. The installer detects supported coding agents and asks where to install the skill. Project installation is the default, so the skill can be committed and shared with the project. Add `--global` if you want it available in all of your projects.

## Installation

### macOS / Linux

```bash
curl -fsSL https://github.com/neptaco/uniforge/releases/latest/download/install.sh | sh
```

The installer verifies the release checksum and installs to `~/.local/bin` by default. Set `UNIFORGE_INSTALL_DIR` or pass `--install-dir` to choose another directory.

### Windows

```powershell
irm https://github.com/neptaco/uniforge/releases/latest/download/install.ps1 | iex
```

After installation, verify that `uniforge` is available:

```bash
uniforge --version
```

### Update

Standalone installations can update themselves:

```bash
uniforge update
uniforge update --check
uniforge update --version v0.11.1
```

Package-manager and development builds are not modified by `uniforge update`.

Released builds also check for new UniForge versions in the background. The
check runs at most once every 24 hours and never delays the command being run.
If an update is found, a later successful interactive command prints a short
notice to stderr. The same release is not mentioned again for seven days.

Automatic checks and notices are disabled in CI and for machine-readable or
protocol output, including JSON, YAML, TSV, `tool`, and shell
completion. This protection still applies when notifications are configured as
`always`, because PTYs and some process runners combine stdout and stderr.

Configure the behavior in `~/.uniforge.yaml`:

```yaml
update:
  check: true
  check_interval: 24h
  notify: auto # auto, always, or never
  remind_interval: 168h
```

The equivalent environment variables are
`UNIFORGE_UPDATE_CHECK`, `UNIFORGE_UPDATE_CHECK_INTERVAL`,
`UNIFORGE_UPDATE_NOTIFY`, and `UNIFORGE_UPDATE_REMIND_INTERVAL`. Set
`UNIFORGE_NO_UPDATE_CHECK=1` to disable automatic network checks entirely.
Checks only request public release metadata from GitHub; UniForge does not send
an identifier or telemetry with the request.

### Manual download

Download the latest release from [GitHub Releases](https://github.com/neptaco/uniforge/releases).

### Shell Completion

```bash
# zsh (~/.zshrc)
eval "$(uniforge completion zsh)"

# bash (~/.bashrc)
eval "$(uniforge completion bash)"

# fish
uniforge completion fish | source
```

## Requirements

- Unity 6.0 LTS or later for live Unity tools
- The [UniForge Unity package](https://github.com/neptaco/uniforge-unity) in projects you want to control with `uniforge tool`
- Unity UI (`com.unity.ugui`), resolved as a package dependency
- Unity Hub only when using Editor installation or Unity Hub project-management commands

## Usage

### Live Unity Tools (Editor open)

Install the [Unity-side UPM package](https://github.com/neptaco/uniforge-unity), then open your project. The package connects to the local daemon automatically; `tool` commands start the daemon when needed.

```bash
# See connected Unity projects
uniforge tool projects

# Browse available tools
uniforge tool list

# Inspect a tool's input schema
uniforge tool describe gameobject

# Inspect the Editor and active scene
uniforge tool call editor-state
uniforge tool call hierarchy

# Inspect a GameObject
uniforge tool call gameobject '{"path":"Main Camera"}'

# Run EditMode tests inside the open Editor
uniforge tool call run-tests '{"mode":"EditMode"}'
```

Default output is YAML. Use `-o json` when machine-readable output is needed. If multiple Editors are connected, select one with `--project <name>`.

The Unity package exposes tools for scene and GameObject inspection, component editing, asset and prefab operations, Play Mode control, test execution, screenshots, input simulation, and more. Run `uniforge tool list` against your project for the authoritative list.

### Update the Unity Package

After the package is installed, update it from the Unity project root with:

```bash
uniforge package update
```

`package update` updates an existing UniForge package reference. Before asking an open Editor to update—or changing `Packages/manifest.json` while the Editor is closed—it checks the target release's minimum Unity version against the project. Later Unity streams, including alpha and beta releases, are accepted when they meet the declared minimum. If the check fails or cannot be completed, no update starts; use `--force` only when you intentionally want to bypass the check. Use the command in [Quick Start](#quick-start) for the first installation.

### Compile, Test, and Run in Batch Mode (Editor closed)

```bash
# Compile project
uniforge compile ./MyProject

# Run EditMode tests
uniforge test ./MyProject --platform editmode

# Run PlayMode tests with filter and results
uniforge test ./MyProject --platform playmode \
  --filter MyTestClass \
  --results ./test-results.xml

# Run custom method
uniforge run ./MyProject -- -executeMethod Build.Execute

# CI mode (optimized output)
uniforge run ./MyProject --ci -- -executeMethod Build.Execute
```

**Options:**
- `--ci`: CI mode (GitHub Actions annotations, log grouping, stack trace filtering)
- `--log-file <path>`: Save log to file
- `--timeout <seconds>`: Timeout in seconds
- `-t, --timestamp`: Show timestamps

### Manage Unity Editor

```bash
# Interactive TUI (when no version specified)
uniforge editor install

# Install from project (auto-detect version)
uniforge editor install -p .

# Install specific version with modules
uniforge editor install 6000.0.40f1 --modules ios,android

# Install with machine-readable progress (one JSON object per line)
uniforge editor install 6000.0.40f1 --output json

# List installed Unity Editors
uniforge editor list

# List available versions (for scripting)
uniforge editor available --lts --latest -o json

# Clear cached Unity release information
uniforge cache clear
```

Editor installation is optional if the required Unity version is already installed. These commands use Unity Hub.

### Daemon Management

The daemon is normally managed automatically by `tool` commands. Use these commands for diagnostics or manual lifecycle control:

```bash
uniforge daemon status
uniforge daemon restart
uniforge daemon stop
```

### Check .meta File Integrity

```bash
# Check for missing/orphan .meta files and duplicate GUIDs
uniforge meta check ./MyProject

# Auto-fix (generate missing, remove orphans)
uniforge meta check ./MyProject --fix

# Fix without confirmation (for CI)
uniforge meta check ./MyProject --fix --force
```

### Manage Unity Hub Projects

```bash
# Interactive TUI
uniforge project list

# List in JSON format
uniforge project list -o json

# Get project path (for shell scripts)
uniforge project path my-game

# Show project details (Unity version, packages, asmdefs)
uniforge project info my-game
```

### Open/Close Unity Editor

```bash
# Open Unity Editor with a project
uniforge open ./MyProject

# Open by project name
uniforge open my-game

# Close / force close / restart
uniforge close ./MyProject
uniforge close ./MyProject --force
uniforge restart ./MyProject
```

### View Unity Logs

```bash
# Show last 100 lines
uniforge logs

# Follow in real-time
uniforge logs -f

# Show project stack traces
uniforge logs --trace

# Show full stack traces (including Unity internals)
uniforge logs --full-trace

# Open in text editor
uniforge logs --editor
```

### Diagnose Stale Unity Runtime State

```bash
# Read-only diagnosis
uniforge doctor ./MyProject

# Remove only verified stale files and stop orphan licensing clients
uniforge doctor ./MyProject --fix
```

The doctor never removes runtime files when the process state cannot be verified or a matching Unity process is active. Use `uniforge clean ./MyProject --target lockfile` when you explicitly want to remove a selected runtime file:

```bash
# Remove Temp/UnityLockfile after verifying the editor is not running
uniforge clean ./MyProject --target lockfile

# Preview what would be removed
uniforge clean ./MyProject --target lockfile --dry-run
```

### Manage Unity License

```bash
# Check license status
uniforge license status

# Activate (Personal: no serial, Plus/Pro: serial required)
uniforge license activate -u user@example.com -p password -s SERIAL-KEY

# Return license
uniforge license return
```

**Environment variables:** `UNITY_USERNAME`, `UNITY_PASSWORD`, `UNITY_SERIAL`

## GitHub Actions

Use [`neptaco/setup-uniforge`](https://github.com/neptaco/setup-uniforge) to install uniforge on self-hosted runners:

```yaml
- uses: neptaco/setup-uniforge@v1
```

### CI Workflow (Self-hosted Runner)

Run tests on every push:

```yaml
name: CI

on: [push, pull_request]

jobs:
  test:
    runs-on: unity
    steps:
      - uses: actions/checkout@v4

      - uses: neptaco/setup-uniforge@v1

      - name: Install Unity
        run: uniforge editor install

      - name: Check .meta files
        run: uniforge meta check .

      - name: Run Tests
        run: uniforge test . --platform editmode --ci
```

### Build Workflow (Self-hosted Runner)

Build for multiple platforms using matrix strategy:

```yaml
name: Build

on:
  push:
    tags: ['v*']

jobs:
  build:
    strategy:
      matrix:
        include:
          - runner: unity-windows
            target: StandaloneWindows64
            modules: windows-il2cpp
          - runner: unity-mac
            target: StandaloneOSX
            modules: mac-il2cpp

    runs-on: ${{ matrix.runner }}
    steps:
      - uses: actions/checkout@v4

      - uses: neptaco/setup-uniforge@v1

      - name: Install Unity
        run: uniforge editor install --modules ${{ matrix.modules }}

      - name: Build
        run: uniforge run . --ci -- -executeMethod Build.Perform -buildTarget ${{ matrix.target }}
```

## Configuration

### Environment Variables

```bash
UNIFORGE_HUB_PATH           # Path to Unity Hub executable
UNIFORGE_EDITOR_BASE_PATH   # Custom Unity Editor base directory
UNIFORGE_EDITOR              # External editor for "project" TUI
UNIFORGE_BIN                 # Go binary path for daemon auto-start
UNIFORGE_LOG_LEVEL           # Log level (debug, info, warn, error)
UNIFORGE_TIMEOUT             # Default timeout in seconds
UNIFORGE_NO_COLOR            # Disable colored output
```

## Development

### Prerequisites

- [Go](https://go.dev/) 1.25+
- [Task](https://taskfile.dev/) - Task runner

### Setup

```bash
git clone https://github.com/neptaco/uniforge.git
cd uniforge
task deps
```

### Commands

```bash
task build    # Build binary to dist/uniforge
task test     # Run tests
task check    # Run all checks (fmt, vet, lint, test)
```

### Notes

- Use `./dist/uniforge` for testing, not `go run .` — `task build` embeds version info via ldflags, and the daemon requires a real binary path.
- Root-level `run|compile|test` are the canonical forms. The previous `batch run|compile|test` forms remain as deprecated compatibility aliases.

## License

MIT License - see [LICENSE](LICENSE) file for details.

## Contributing

Contributions are welcome! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

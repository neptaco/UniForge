# UniForge

Command-line tool for Unity CI/CD automation. Build Unity projects for multiple platforms with simple commands.

## Features

- 🤖 **CI/CD optimized** - GitHub Actions annotations, log grouping, noise filtering
- 🖥️ **Cross-platform** - Same commands work on macOS, Windows, Linux
- 🧪 **Test Runner** - Run EditMode/PlayMode tests with XML results
- 📦 **Editor management** - Install Unity versions via Unity Hub CLI
- 📁 **Project management** - Browse and open Unity Hub projects with TUI or CLI
- 📋 **Meta file check** - Detect missing .meta files and duplicate GUIDs
- 🔑 **License management** - Activate/return licenses for CI runners

## Quick Start: GitHub Actions

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

## Installation

### GitHub Actions

```yaml
- uses: neptaco/setup-uniforge@v1
```

### Using Homebrew (macOS/Linux)

```bash
brew tap neptaco/tap
brew install uniforge
```

### Using Scoop (Windows)

```powershell
scoop bucket add neptaco https://github.com/neptaco/scoop-bucket
scoop install uniforge
```

### Download Binary

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

## Prerequisites

- Unity Hub installed

## Usage

### Manage Unity Editor

```bash
# Interactive TUI (when no version specified)
uniforge editor install

# Install from project (auto-detect version)
uniforge editor install -p .

# Install specific version
uniforge editor install 2022.3.10f1

# Install with modules
uniforge editor install 2022.3.10f1 --modules ios,android

# Install specific architecture
uniforge editor install 2022.3.10f1 --architecture arm64

# Force reinstall
uniforge editor install 2022.3.10f1 --force

# Install with changeset (for versions not in release list)
uniforge editor install 2022.3.10f1 --changeset abc123def456

# List installed Unity Editors
uniforge editor list

# List available versions (for scripting)
uniforge editor available --lts --latest --format json
```

#### Available Versions

The `editor available` command supports various filters and output formats for scripting:

```bash
# Table format (default for TTY)
uniforge editor available

# JSON format for scripting
uniforge editor available --format json

# LTS versions only
uniforge editor available --lts

# Filter by major version
uniforge editor available --major 6000

# Latest version per stream
uniforge editor available --latest

# Show only not installed versions
uniforge editor available --not-installed

# Count matching versions
uniforge editor available --lts --count

# Combine filters
uniforge editor available --major 2022 --lts --latest --format tsv
```

**Options:**
- `--format <table|json|tsv>`: Output format (auto-detected based on TTY)
- `--lts`: Show only LTS versions
- `--stream <name>`: Filter by stream (LTS, TECH, BETA, ALPHA)
- `--major <version>`: Filter by major version (e.g., 6000, 2022)
- `--installed`: Show only installed versions
- `--not-installed`: Show only not installed versions
- `--latest`: Show only latest version per major version
- `--count`: Output count only

#### Interactive TUI

When running `uniforge editor install` without arguments, an interactive TUI is launched:

- **Stream selection**: Browse available Unity versions by stream (LTS, Tech, Beta)
- **Version search**: Type version number (e.g., `2022.3.`) to filter
- **Module selection**: Choose platform modules to install
- **Ctrl+l**: View installed versions with project counts for module updates

### Run Unity in Batch Mode

```bash
# Run custom method
uniforge run ./MyProject -- -executeMethod Build.Execute

# Run with CI mode (optimized output)
uniforge run ./MyProject --ci -- -executeMethod Build.Execute

# Save log to file
uniforge run ./MyProject --log-file ./build.log -- -executeMethod Build.Execute

# Show timestamps
uniforge run ./MyProject -t -- -executeMethod Build.Execute
```

**Options:**
- `--ci`: CI mode (optimized output format)
- `--log-file <path>`: Path to save log file
- `--timeout <seconds>`: Timeout in seconds (default: 3600)
- `-t, --timestamp`: Show timestamp for each line

#### CI Mode Features

The `--ci` flag optimizes output for CI/CD environments:

- **GitHub Actions annotations**: Errors and warnings are prefixed with `::error::` and `::warning::` for inline display
- **Log grouping**: Verbose logs (Licensing, Package Manager, Assembly Reload, etc.) are collapsed into expandable groups
- **Stack trace filtering**: All stack traces are hidden to reduce noise

### Run Tests

```bash
# Run EditMode tests
uniforge test ./MyProject --platform editmode

# Run PlayMode tests
uniforge test ./MyProject --platform playmode

# Run with filter and save results
uniforge test ./MyProject --platform editmode \
  --filter MyTestClass \
  --results ./test-results.xml

# CI mode with custom timeout
uniforge test ./MyProject --platform editmode --ci --timeout 1800

# Save log to file
uniforge test ./MyProject --platform editmode --log-file ./test.log

# Show timestamps
uniforge test ./MyProject --platform editmode -t
```

**Options:**
- `--platform <editmode|playmode>`: Test platform (required)
- `--filter <expression>`: Test filter expression
- `--results <path>`: Path to save test results (XML)
- `--log-file <path>`: Path to save log file
- `--timeout <seconds>`: Test timeout in seconds (default: 600)
- `--ci`: CI mode (optimized output format)
- `-t, --timestamp`: Show timestamp for each line

### Check .meta File Integrity

```bash
# Check for missing/orphan .meta files and duplicate GUIDs
uniforge meta check ./MyProject

# Fix orphan .meta files (with confirmation)
uniforge meta check ./MyProject --fix

# Fix without confirmation (for CI)
uniforge meta check ./MyProject --fix --force
```

### Manage Unity Hub Projects

```bash
# Interactive TUI (when in terminal)
uniforge project

# List all registered projects
uniforge project list

# List in different formats
uniforge project list --format=json
uniforge project list --format=tsv
uniforge project list --path-only

# List without Git information (faster)
uniforge project list --no-git

# Open project by name (partial match supported)
uniforge project open my-game

# Get project path (for shell scripts)
cd $(uniforge project path my-game)
```

#### Shell Integration (fzf)

Add to your `.zshrc` or `.bashrc`:

```bash
# cd to Unity project with fzf
ucd() {
  local project
  project=$(uniforge project list --format=tsv | fzf --delimiter='\t' --with-nth=1,2 | cut -f4)
  [ -n "$project" ] && cd "$project"
}
```

### Open/Close Unity Editor

```bash
# Open Unity Editor with a project path
uniforge open ./MyProject

# Open by project name (searches Unity Hub projects)
uniforge open my-game

# Close running Unity Editor
uniforge close ./MyProject

# Force close (without save prompt)
uniforge close ./MyProject --force

# Restart Unity Editor
uniforge restart ./MyProject
```

### View Unity Logs

```bash
# Show last 100 lines (default)
uniforge logs

# Show last 500 lines
uniforge logs -n 500

# Follow log in real-time
uniforge logs -f

# Follow with timestamps
uniforge logs -f -t

# Show raw output without colors or filtering
uniforge logs --raw

# Show project stack traces (Assets/, Packages/)
uniforge logs --trace

# Show full stack traces (including Unity internals)
uniforge logs --full-trace

# Open in text editor ($EDITOR or vim)
uniforge logs --editor
```

**Options:**
- `-f, --follow`: Follow log output in real-time
- `-n, --lines <count>`: Number of lines to show (default: 100)
- `-t, --timestamp`: Show timestamp for each line
- `--raw`: Show raw output without colors or filtering
- `--trace`: Show project stack traces (Assets/, Packages/)
- `--full-trace`: Show full stack traces including Unity internals
- `--editor`: Open log in text editor ($EDITOR or vim)

### Manage Release Cache

```bash
# Clear cached Unity release information
uniforge cache clear

# Skip cache when fetching releases (still writes to cache)
uniforge editor install --no-cache
```

### Manage Unity License

For CI environments that require license activation:

```bash
# Check license status
uniforge license status

# Activate license (Personal: no serial, Plus/Pro: serial required)
uniforge license activate

# Activate with explicit credentials
uniforge license activate -u user@example.com -p password -s SERIAL-KEY

# Activate using a specific Unity version
uniforge license activate --version 2022.3.10f1

# Return license
uniforge license return

# Return using a specific Unity version
uniforge license return --version 2022.3.10f1
```

**Activate options:**
- `-u, --username <email>`: Unity ID email (or `UNITY_USERNAME` env)
- `-p, --password <password>`: Password (or `UNITY_PASSWORD` env)
- `-s, --serial <key>`: Serial key for Plus/Pro license (or `UNITY_SERIAL` env)
- `--version <version>`: Unity version to use for activation
- `--timeout <seconds>`: Timeout in seconds (default: 300)

**Return options:**
- `--version <version>`: Unity version to use for return
- `--timeout <seconds>`: Timeout in seconds (default: 300)

#### Supported License Types

| Type | Detection Method |
|------|------------------|
| Serial | `Unity_lic.ulf` file |
| Unity Hub | `userInfoKey.json` (logged in via Hub) |
| Licensing Server | `UNITY_LICENSING_SERVER` env or `services-config.json` |
| Build Server | `enableFloatingApi: true` in config |

#### Environment Variables

```bash
UNITY_USERNAME    # Unity ID email
UNITY_PASSWORD    # Password
UNITY_SERIAL      # Serial key (Plus/Pro only)
```

## Configuration

### Environment Variables

```bash
UNIFORGE_HUB_PATH           # Path to Unity Hub executable
UNIFORGE_EDITOR_BASE_PATH   # Custom Unity Editor base directory
UNIFORGE_EDITOR             # External editor for "project" TUI (auto-detect: rider > cursor > code)
UNIFORGE_LOG_LEVEL          # Log level (debug, info, warn, error)
UNIFORGE_TIMEOUT            # Default timeout in seconds
UNIFORGE_NO_COLOR           # Disable colored output
```

### Editor Location

UniForge automatically detects Unity Editors from:
- **Unity Hub settings** (Preferences → Installs location)
- **Default OS paths**:
  - macOS: `/Applications/Unity/Hub/Editor`
  - Windows: `C:\Program Files\Unity\Hub\Editor`
  - Linux: `~/Unity/Hub/Editor`

Use `UNIFORGE_EDITOR_BASE_PATH` only if Unity Hub settings are not detected:

```bash
# macOS/Linux
export UNIFORGE_EDITOR_BASE_PATH=/Volumes/ExternalSSD/Unity/Hub/Editor

# Windows
set UNIFORGE_EDITOR_BASE_PATH=D:\Unity\Hub\Editor
```

## Development

### Prerequisites

- [Go](https://go.dev/) 1.24+
- [Task](https://taskfile.dev/) - Task runner

### Setup

```bash
# Clone and setup (installs tools and git hooks)
git clone https://github.com/neptaco/uniforge.git
cd uniforge
task setup
```

### Commands

```bash
task build    # Build
task test     # Run tests
task lint     # Run linters
task check    # Run all checks (fmt, vet, lint, test)
```

## License

MIT License - see [LICENSE](LICENSE) file for details.

## Contributing

Contributions are welcome! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## Support

For issues and feature requests, please use [GitHub Issues](https://github.com/neptaco/uniforge/issues).

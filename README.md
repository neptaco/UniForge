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

## Prerequisites

- Unity Hub installed

## Usage

### Manage Unity Editor

```bash
# Install from project (auto-detect version)
uniforge editor install

# Install specific version
uniforge editor install 2022.3.10f1

# Install with modules
uniforge editor install 2022.3.10f1 --modules ios,android

# List installed Unity Editors
uniforge editor list
```

### Run Unity in Batch Mode

```bash
# Run custom method
uniforge run ./MyProject -- -executeMethod Build.Execute

# Run with CI mode (optimized output)
uniforge run ./MyProject --ci -- -executeMethod Build.Execute
```

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
```

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

# Follow log in real-time
uniforge logs -f

# Show with timestamps
uniforge logs -f -t
```

### Manage Unity License

For CI environments that require license activation:

```bash
# Check license status
uniforge license status

# Activate license (Personal: no serial, Plus/Pro: serial required)
uniforge license activate

# Return license
uniforge license return
```

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

### Custom Editor Location

```bash
# Example: External SSD (macOS)
export UNIFORGE_EDITOR_BASE_PATH=/Volumes/ExternalSSD/Unity/Hub/Editor

# Example: Custom location (Windows)
set UNIFORGE_EDITOR_BASE_PATH=D:\Unity\Hub\Editor
```

Default locations:
- **macOS**: `/Applications/Unity/Hub/Editor`
- **Windows**: `C:\Program Files\Unity\Hub\Editor`
- **Linux**: `~/Unity/Hub/Editor`

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

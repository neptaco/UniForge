# UniForge

Command-line tool for Unity development. Manage Unity Editor installations, build projects, and run Unity in batch mode.

## Features

- 🎮 Detect Unity version from project
- 📦 Install Unity Editor versions via Unity Hub
- 🔨 Build Unity projects for multiple platforms
- 🤖 Run Unity in batch mode for CI/CD
- 🧪 Run Unity Test Runner (EditMode/PlayMode)
- 🔑 Manage Unity license for CI/CD environments
- 📋 Check .meta file integrity
- 📝 Comprehensive logging with CI-friendly output
- 🖥️ Cross-platform support (macOS, Windows, Linux)

## Installation

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

### Build from Source

```bash
git clone https://github.com/neptaco/uniforge.git
cd uniforge
task build
```

## Prerequisites

- Unity Hub installed
- Go 1.24+ (for building from source)
- Task (for building from source)

### Windows-specific Requirements

- PowerShell 5.1+ or PowerShell Core 6+
- Unity Hub for Windows
- Visual Studio Build Tools (for certain build targets)
- Windows SDK (for Windows builds)

## Usage

### Manage Unity Editor

```bash
# List installed Unity Editors
uniforge editor list

# Install specific version
uniforge editor install 2022.3.10f1

# Install with modules
uniforge editor install 2022.3.10f1 --modules ios,android

# Install from project (auto-detect version)
uniforge editor install -p ./MyUnityProject
```

### Open/Close Unity Editor

```bash
# Open Unity Editor with a project
uniforge open ./MyProject

# Close running Unity Editor
uniforge close ./MyProject

# Force close (without save prompt)
uniforge close ./MyProject --force

# Restart Unity Editor
uniforge restart ./MyProject
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

### Manage Unity License (CI/CD)

```bash
# Activate license
uniforge license activate \
  --username user@example.com \
  --password $UNITY_PASSWORD \
  --serial $UNITY_SERIAL

# Check license status
uniforge license status

# Return license (important for CI)
uniforge license return \
  --username user@example.com \
  --password $UNITY_PASSWORD
```

### View Unity Logs

```bash
# Show last 100 lines (default)
uniforge logs

# Follow log in real-time
uniforge logs -f

# Show with timestamps
uniforge logs -f -t

# Show project stack traces
uniforge logs --trace
```

## Configuration

### Environment Variables

```bash
UNIFORGE_HUB_PATH           # Path to Unity Hub executable
UNIFORGE_EDITOR_BASE_PATH   # Custom Unity Editor base directory (see below)
UNIFORGE_LOG_LEVEL          # Log level (debug, info, warn, error)
UNIFORGE_TIMEOUT            # Default timeout in seconds
UNIFORGE_NO_COLOR           # Disable colored output
```

#### Custom Editor Location

Unity Editors installed in non-default locations are automatically detected via Unity Hub CLI. However, setting `UNIFORGE_EDITOR_BASE_PATH` improves performance by skipping the Hub CLI call.

```bash
# Example: External SSD (macOS)
export UNIFORGE_EDITOR_BASE_PATH=/Volumes/ExternalSSD/Unity/Hub/Editor

# Example: Custom location (Windows)
set UNIFORGE_EDITOR_BASE_PATH=D:\Unity\Hub\Editor
```

Default locations (detected without configuration):
- **macOS**: `/Applications/Unity/Hub/Editor`
- **Windows**: `C:\Program Files\Unity\Hub\Editor`
- **Linux**: `~/Unity/Hub/Editor`

### Configuration File

Create `.uniforge.yaml` in your home directory:

```yaml
log-level: info
no-color: false
```

## CI/CD Integration

### GitHub Actions

```yaml
name: Unity CI

on: [push]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: neptaco/setup-uniforge@v1

      - name: Activate License
        run: |
          uniforge license activate \
            --username ${{ secrets.UNITY_EMAIL }} \
            --password ${{ secrets.UNITY_PASSWORD }} \
            --serial ${{ secrets.UNITY_SERIAL }}

      - name: Install Unity
        run: uniforge editor install --modules ios,android

      - name: Check .meta files
        run: uniforge meta check .

      - name: Run Tests
        run: uniforge test . --platform editmode --ci --results ./results.xml

      - name: Return License
        if: always()
        run: |
          uniforge license return \
            --username ${{ secrets.UNITY_EMAIL }} \
            --password ${{ secrets.UNITY_PASSWORD }}
```

Works on `ubuntu-latest`, `macos-latest`, and `windows-latest`.

## Development

### Prerequisites

- Go 1.24+
- Task

### Building

```bash
# Build for current platform
task build

# Build for all platforms
task build-all

# Run tests
task test

# Run linter
task lint
```

### Project Structure

```
uniforge/
├── cmd/           # CLI commands
├── pkg/           # Core packages
│   ├── unity/     # Unity-related functionality
│   ├── hub/       # Unity Hub integration
│   ├── platform/  # Platform-specific code
│   └── logger/    # Logging system
├── scripts/       # Build and installation scripts
└── .github/       # GitHub Actions workflows
```

## Troubleshooting

### Windows-specific Issues

#### PowerShell Execution Policy
If you encounter execution policy errors:
```powershell
Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser
```

#### Unity Hub Path Issues
If Unity Hub is not found automatically:
```powershell
$env:UNIFORGE_HUB_PATH = "C:\Program Files\Unity\Hub\Unity Hub.exe"
```

#### Long Path Support
For projects with deep directory structures:
```powershell
# Enable long path support (requires admin)
New-ItemProperty -Path "HKLM:\SYSTEM\CurrentControlSet\Control\FileSystem" -Name "LongPathsEnabled" -Value 1 -PropertyType DWORD -Force
```

#### Build Failures
- Ensure Visual Studio Build Tools are installed
- Check Windows SDK version compatibility
- Verify Unity Editor modules are properly installed

## License

MIT License - see [LICENSE](LICENSE) file for details.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

See [CONTRIBUTING.md](CONTRIBUTING.md) for development guidelines.

## Support

For issues and feature requests, please use the [GitHub Issues](https://github.com/neptaco/uniforge/issues) page.

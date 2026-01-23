# Uniforge

Unity CI/CD command-line tool for managing Unity Editor installations and building Unity projects.

## Features

- 🎮 Detect Unity version from project
- 📦 Install Unity Editor versions via Unity Hub
- 🔨 Build Unity projects for multiple platforms
- 🤖 Run Unity in batch mode for CI/CD
- 📝 Comprehensive logging with CI-friendly output
- 🖥️ Cross-platform support (macOS, Windows, Linux)

## Installation

### Using Homebrew (macOS)

```bash
brew tap neptaco/uniforge
brew install uniforge
```

### Using Scoop (Windows)

```bash
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
- Go 1.21+ (for building from source)
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
uniforge editor install --version 2022.3.10f1

# Install with modules
uniforge editor install --version 2022.3.10f1 --modules ios,android

# Install from project (auto-detect version)
uniforge editor install --from-project ./MyUnityProject
```

### Build Unity Project

```bash
# Basic build
uniforge build --project ./MyProject --target ios --output ./Build/iOS

# Build with custom method
uniforge build \
  --project ./MyProject \
  --target android \
  --method MyCompany.Builder.BuildAndroid \
  --output ./Build/Android \
  --log-file ./build.log

# CI mode with fail on warning
uniforge build \
  --project ./MyProject \
  --target windows \
  --ci-mode \
  --fail-on-warning
```

### Run Unity with Custom Methods

```bash
# Run tests
uniforge run \
  --project ./MyProject \
  --execute-method TestRunner.RunAllTests \
  --test-results ./test-results.xml \
  --quit

# Run multiple methods
uniforge run \
  --project ./MyProject \
  --execute-method "Setup.Initialize;Build.Execute;Cleanup.Finish" \
  --quit
```

## Configuration

### Environment Variables

```bash
UNIFORGE_HUB_PATH        # Path to Unity Hub executable
UNITY_HUB_INSTALL_PATH    # Custom Unity Editor installation directory (speeds up detection)
UNIFORGE_LOG_LEVEL       # Log level (debug, info, warn, error)
UNIFORGE_TIMEOUT         # Default timeout in seconds
UNIFORGE_NO_COLOR        # Disable colored output
```

#### Performance Optimization

If you have Unity Editors installed in a custom location, set `UNITY_HUB_INSTALL_PATH` to avoid Unity Hub CLI calls:

```bash
# Example: Custom installation directory
export UNITY_HUB_INSTALL_PATH=/Volumes/ExternalSSD/Applications/Unity/Hub/Editor

# This will make editor detection instant (< 0.04 seconds)
uniforge editor install --version 2022.3.60f1
```

### Configuration File

Create `.uniforge.yaml` in your home directory:

```yaml
log-level: info
no-color: false
```

## CI/CD Integration

### GitHub Actions

#### Linux/macOS

```yaml
name: Build Unity Project

on: [push]

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Setup Unity CLI
        run: |
          curl -L https://github.com/neptaco/uniforge/releases/latest/download/uniforge-linux-amd64 -o uniforge
          chmod +x uniforge
          sudo mv uniforge /usr/local/bin/
      
      - name: Install Unity
        run: uniforge editor install --from-project . --modules android
      
      - name: Run Tests
        run: |
          uniforge run \
            --project . \
            --execute-method CI.TestRunner.RunTests \
            --test-results ./test-results.xml \
            --quit
      
      - name: Build Android
        run: |
          uniforge build \
            --project . \
            --target android \
            --method CI.Builder.BuildAndroid \
            --output ./Build/Android \
            --ci-mode
```

#### Windows

```yaml
name: Build Unity Project (Windows)

on: [push]

jobs:
  build:
    runs-on: windows-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Setup Unity CLI (Windows)
        run: |
          # Download Windows binary
          $url = "https://github.com/neptaco/uniforge/releases/latest/download/uniforge_windows_amd64.zip"
          $output = "uniforge.zip"
          Invoke-WebRequest -Uri $url -OutFile $output
          
          # Extract and install
          Expand-Archive -Path $output -DestinationPath "."
          Move-Item "uniforge_windows_amd64/uniforge.exe" "uniforge.exe"
          Remove-Item $output -Force
          Remove-Item "uniforge_windows_amd64" -Recurse -Force
          
          # Add to PATH
          $env:PATH = "$env:PATH;$pwd"
      
      - name: Install Unity
        run: |
          .\uniforge.exe editor install --from-project . --modules windows
      
      - name: Run Tests
        run: |
          .\uniforge.exe run `
            --project . `
            --execute-method CI.TestRunner.RunTests `
            --test-results .\test-results.xml `
            --quit
      
      - name: Build Windows
        run: |
          .\uniforge.exe build `
            --project . `
            --target windows `
            --method CI.Builder.BuildWindows `
            --output .\Build\Windows `
            --ci-mode
```

### GitLab CI

```yaml
build:
  image: ubuntu:latest
  script:
    - curl -L https://github.com/neptaco/uniforge/releases/latest/download/uniforge-linux-amd64 -o uniforge
    - chmod +x uniforge
    - ./uniforge build --project . --target android --ci-mode
```

## Build Targets

Supported build targets:
- `windows` - Windows 64-bit
- `macos` - macOS Universal
- `linux` - Linux 64-bit
- `android` - Android APK
- `ios` - iOS Xcode project
- `webgl` - WebGL build

## Development

### Prerequisites

- Go 1.21+
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

## Support

For issues and feature requests, please use the [GitHub Issues](https://github.com/neptaco/uniforge/issues) page.
# Contributing to UniForge

## Development Setup

### Prerequisites

- Go 1.24+
- [Task](https://taskfile.dev/) (task runner)
- [golangci-lint](https://golangci-lint.run/) (optional, for linting)
- [lefthook](https://github.com/evilmartians/lefthook) (optional, for Git hooks)

### Quick Start

```bash
# Clone repository
git clone https://github.com/neptaco/uniforge.git
cd uniforge

# Install dependencies
task deps

# Build
task build

# Install to GOPATH
task install
```

### During Development

```bash
# Run without building
go run . editor list
go run . test ./path/to/project --platform editmode

# Or use Task dev command
task dev -- editor list
```

### Uninstall

```bash
# If installed via go install
rm $(go env GOPATH)/bin/uniforge

# If installed via task install
rm $(go env GOPATH)/bin/uniforge
```

## Code Quality

### Commands

| Command | Description |
|---------|-------------|
| `task fmt` | Format code with go fmt |
| `task vet` | Run go vet |
| `task lint` | Run golangci-lint |
| `task test` | Run tests with coverage |
| `task check` | Run all checks (fmt, vet, lint, test) |

### Before Committing

Always run checks before committing:

```bash
task check
```

### Git Hooks (Optional)

Install lefthook for automatic pre-commit checks:

```bash
# Install lefthook
go install github.com/evilmartians/lefthook@latest

# Setup hooks
lefthook install
```

## Testing

### Running Tests

```bash
# Run all tests
task test

# Run specific package tests
go test ./pkg/unity
go test ./pkg/hub

# Run with verbose output
go test -v ./...

# Run specific test
go test -v -run TestLoadProject ./pkg/unity
```

### Debug Mode

```bash
# Enable debug logging
./dist/uniforge --log-level debug editor list

# Or via environment variable
UNIFORGE_LOG_LEVEL=debug ./dist/uniforge editor list

# Disable colored output
./dist/uniforge --no-color editor list
```

## Commit Guidelines

We follow [Conventional Commits](https://www.conventionalcommits.org/).

### User-facing changes (included in CHANGELOG)

| Type | Description | Example |
|------|-------------|---------|
| `feat` | New feature for users | `feat: add test command` |
| `fix` | Bug fix for users | `fix: handle missing .meta files` |

### Internal changes (excluded from CHANGELOG)

| Type | Description | Example |
|------|-------------|---------|
| `ci` | CI/CD configuration | `ci: add golangci-lint to workflow` |
| `chore` | Maintenance, tooling | `chore: update dependencies` |
| `docs` | Documentation | `docs: update README` |
| `refactor` | Code refactoring | `refactor: extract validation logic` |
| `style` | Code formatting | `style: format with go fmt` |
| `test` | Adding tests | `test: add unit tests for meta checker` |

## Project Structure

```
uniforge/
├── cmd/           # CLI commands (Cobra)
├── pkg/
│   ├── hub/       # Unity Hub integration
│   ├── license/   # License management
│   ├── logger/    # Log formatting
│   ├── platform/  # Platform detection
│   ├── ui/        # Terminal UI (Charm)
│   └── unity/     # Unity project/editor operations
├── Taskfile.yml   # Task definitions
└── lefthook.yml   # Git hooks config
```

## CI/CD

GitHub Actions runs on every PR:
- Tests on Ubuntu, macOS, Windows
- Linting with golangci-lint
- Format check

# Contributing to UniForge

## Development Setup

### Prerequisites

- Go 1.24+
- [Task](https://taskfile.dev/) (task runner)
- [golangci-lint](https://golangci-lint.run/) (optional, for linting)
- [lefthook](https://github.com/evilmartians/lefthook) (optional, for Git hooks)

### Quick Start

```bash
# Install dependencies
task deps

# Build
task build

# Install to GOPATH
task install
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

## Commit Guidelines

We follow [Conventional Commits](https://www.conventionalcommits.org/):

| Type | Description |
|------|-------------|
| `feat` | New feature |
| `fix` | Bug fix |
| `refactor` | Code refactoring |
| `docs` | Documentation |
| `style` | Code style (formatting, etc.) |
| `test` | Adding tests |
| `chore` | Maintenance |

Example:
```
feat: add license management commands
fix: handle missing .meta files correctly
```

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

## Testing

- Add tests for new functionality in `*_test.go` files
- Run tests: `task test`
- View coverage: `task test-coverage`

## CI/CD

GitHub Actions runs on every PR:
- Tests on Ubuntu, macOS, Windows
- Linting with golangci-lint
- Format check

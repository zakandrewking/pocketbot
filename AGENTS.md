# CLAUDE.md

This file contains context and information for Claude Code when working on this project.

## Project: pocketbot

A Go CLI tool that installs as `pb`.

### Tech Stack

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - Terminal UI framework

## Installation

```bash
go install github.com/zakandrewking/pocketbot/cmd/pb@latest
```

## Development

```bash
# Run locally
go run cmd/pb/main.go

# Build
go build -o pb cmd/pb/main.go

# Install locally
go install ./cmd/pb
```

### Meta-Development: Using pb to test pb

pb includes CLI commands for development, enabling you to use pb itself to test and build pb:

```bash
pb test         # Run tests
pb build        # Build binary
pb install      # Install to $GOPATH/bin
pb sessions     # List active tmux sessions
pb kill-all     # Kill all sessions
```

**Workflow:** You can configure pb sessions to run these commands interactively. Add to your `~/.config/pocketbot/config.yaml`:

```yaml
sessions:
  - name: "test"
    command: "pb test"
    key: "t"
  - name: "build"
    command: "pb build && pb install"
    key: "b"
```

Then run `pb`, press `t` to run tests in tmux, `Ctrl+D` to detach, etc. This creates a meta-workflow where pb manages its own development.

## Project Structure

- `cmd/pb/` - Main CLI application entry point
- `go.mod` - Go module definition

## Workflow

- After finishing work and verifying it, always commit changes and run `go install ./cmd/pb`
- Wait for explicit confirmation before pushing
- Always work in small testable steps (unit tests, integration tests, or manual testing)
- **All new features require tests** - either unit tests, integration tests, or clear manual testing steps
- For larger work, create a markdown project plan before beginning
- Track bugs in a committed markdown file (BUGS.md)

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

## Project Structure

- `cmd/pb/` - Main CLI application entry point
- `go.mod` - Go module definition

## Workflow

- It's OK to push straight to main if changes are tested
- Always commit & push working improvements
- Always work in small testable steps (unit tests, integration tests, or manual testing)
- For larger work, create a markdown project plan before beginning
- Track bugs in a committed markdown file (BUGS.md)

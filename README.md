# PocketBot

A lightweight session manager for terminal workflows. Manage Claude Code and custom dev tools with simple keybindings.

## Features

- **Claude Code Integration**: Quick access to Claude Code sessions with `c`
- **Codex CLI Integration**: Quick access to Codex CLI sessions with `x`
- **Custom Sessions**: Configure any command (dev servers, logs, etc.) with custom keybindings
- **Activity Monitoring**: See if sessions are active or idle in real-time
- **Detach/Attach**: Like tmux, but simpler - press Ctrl+D to detach, press key to reattach
- **YAML Configuration**: Simple config file for all your workflows

## Installation

```bash
go install github.com/zakandrewking/pocketbot/cmd/pb@latest
```

## Quick Start

```bash
pb
```

**Default Usage:**
- Press `c` to start/attach to Claude Code
- Press `x` to start/attach to Codex CLI
- While attached, press Ctrl+D to detach (returns to pocketbot)
- Press Ctrl+C to quit pocketbot

## Configuration

Create `~/.config/pocketbot/config.yaml` to customize sessions:

```yaml
# Claude (default, can be disabled)
claude:
  command: "claude --continue"
  key: "c"
  enabled: true

# Codex (default, can be disabled)
codex:
  command: "codex"
  key: "x"
  enabled: true

# Add custom sessions
sessions:
  - name: "dev-server"
    command: "npm run dev"
    key: "d"

  - name: "logs"
    command: "tail -f logs/app.log"
    key: "l"
```

See [config.example.yaml](config.example.yaml) for more examples.

**Session Display:**
```
🤖 Welcome to PocketBot!

claude: ● active [c]
codex: ○ not running [x]
dev-server: ○ not running [d]
logs: ● idle [l]

Press key to start/attach • Ctrl+C to quit
```

## Development

### Run locally

```bash
go run cmd/pb/main.go
```

### Build

```bash
go build -o pb cmd/pb/main.go
```

### Install locally

```bash
go install ./cmd/pb
```

### Live reload with Air

Install Air:

```bash
go install github.com/air-verse/air@latest
```

Run with live reload:

```bash
air --build.cmd "go install ./cmd/pb" --build.bin "/usr/bin/true"
```

This will automatically rebuild and reinstall `pb` whenever you save changes to the source files.

## Tech Stack

- [Go](https://go.dev/) - Programming language
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - Terminal UI framework

## License

MIT

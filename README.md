# PocketBot

A terminal UI (TUI) tool built with Go and Bubble Tea.

## Installation

```bash
go install github.com/zakandrewking/pocketbot/cmd/pb@latest
```

## Usage

```bash
pb
```

Navigate the menu with:
- Arrow keys or `j`/`k` (vim-style)
- Select items with Enter or Space
- Quit with `q` or Ctrl+C

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

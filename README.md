# PocketBot

PocketBot (`pb`) is a tmux-backed launcher for Claude, Codex, Cursor, and other long-running terminal workflows.

<img width="381" height="164" alt="image" src="https://github.com/user-attachments/assets/61be48d1-b85e-4233-a3f4-099b7bebdf59" />

## What It Does Well

- Keeps sessions alive in the background (tmux-backed).
- Lets you detach and reattach quickly (`Ctrl+D` to detach from a session).
- Supports multiple Claude/Codex/Cursor instances per machine.

## Install

Prerequisites:

- `go` (to build/install `pb`)
- `tmux` (required at runtime)
- `fasder` (optional, enables `z` directory jump)

```bash
go install github.com/zakandrewking/pocketbot/cmd/pb@latest
```

## Default Keys

- `c`: attach Claude (create if none, picker if multiple)
- `x`: attach Codex (create if none, picker if multiple)
- `u`: attach Cursor (create if none, picker if multiple)
- `z`: directory jump using `fasder` search + Enter
- `n`: create new instance, then choose `c`, `x`, or `u`
- `k`: kill one instance, then choose `c`, `x`, or `u` (picker appears if needed)
- `d`: back or quit UI (sessions keep running)
- `Esc`: go back/cancel in picker-style flows
- `Ctrl+C`: kill all sessions and quit

## Quick Start

```bash
pb
```

Typical loop:

1. Press `c`, `x`, or `u` to jump into a coding session.
2. Press `Ctrl+D` to detach back to `pb`.
3. Press `n` to spin up another instance for a parallel task.
4. Press `k` to clean up a specific instance.

## Configuration

Create `~/.config/pocketbot/config.yaml`:

```yaml
claude:
  command: "claude --continue --permission-mode acceptEdits"
  key: "c"
  enabled: true

codex:
  command: "codex resume --last"
  key: "x"
  enabled: true

cursor:
  command: "agent resume"
  key: "u"
  enabled: true

sessions:
  - name: "dev-server"
    command: "npm run dev"
    key: "v"
  - name: "logs"
    command: "tail -f logs/app.log"
    key: "l"
```

Reserved keys in the default UI: `c`, `x`, `u`, `z`, `n`, `k`, `d`, `Esc`.

See `config.example.yaml` for more examples.

## Development

```bash
go run cmd/pb/main.go
go test ./...
go install ./cmd/pb
```

## License

Apache License 2.0

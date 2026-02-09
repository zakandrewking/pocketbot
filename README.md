# PocketBot

PocketBot (`pb`) is a tmux-backed launcher for Claude, Codex, Cursor, and other long-running terminal workflows.

It is built for fast, repeatable keyboard control on small screens: single letters, letter-pair pickers, and `Ctrl+<letter>` globals.

## What It Does Well

- Keeps sessions alive in the background (tmux-backed).
- Lets you detach and reattach quickly (`Ctrl+D` to detach from a session).
- Supports multiple Claude/Codex/Cursor instances per machine.
- Uses a compact 20-line mobile-first home view.
- Shows the current working directory at the top of home.
- Switches views automatically:
  - fewer than 10 Claude+Codex+Cursor instances: detailed rows
  - 10 or more: consolidated summary with drill-down picker
- Supports custom commands as first-class sessions.

## Install

```bash
go install github.com/zakandrewking/pocketbot/cmd/pb@latest
```

## Default Keys

- `c`: attach Claude (create if none, picker if multiple)
- `x`: attach Codex (create if none, picker if multiple)
- `u`: attach Cursor (create if none, picker if multiple)
- `z`: directory jump using `fasder` query + Enter
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

MIT

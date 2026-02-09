# Mobile UI Constraints

- Use only lowercase single-letter shortcuts, lowercase letter-pair flows (with a picker for letter 2), and `ctrl+<letter>` commands.
- Do not require uppercase letter shortcuts.
- Do not require arrow keys, function keys, or multi-key tmux prefix sequences.
- Target a 20-line terminal viewport.
- When total Claude+Codex instances are fewer than 10, show per-instance status, repo, and key-chord info.
- When total Claude+Codex instances are 10 or more, show consolidated summary info on home and require drill-down to per-instance lists.
- Keep messages and hints readable on very small screens (single-column layout).
- `Escape` is allowed and should be used as the general go-back/cancel keybinding.

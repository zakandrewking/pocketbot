# Plan: tmux Backend for Pocketbot

## Problem

The custom PTY + VT100 emulation approach has issues:
1. VT100 library doesn't handle all of Claude's escape sequences
2. Screen rendering is complex and buggy
3. We're reimplementing what tmux already does well

## Solution

Use tmux as the backend for session management. pocketbot becomes a thin wrapper that:
1. Manages tmux sessions (create, attach, detach)
2. Provides the Bubble Tea home screen UI
3. Monitors session activity via tmux APIs

## Benefits

- Battle-tested terminal handling
- Proper scrollback support
- Clean attach/detach
- Solves the keychain issue (tmux + reattach-to-user-namespace)
- Less code to maintain

## Tradeoffs

- Requires tmux to be installed
- Less control over exact rendering
- Need to handle "nested tmux" case

---

## Phases

### Phase 1: Basic tmux Integration

**Goal:** Replace custom PTY with tmux session management.

**Changes:**

1. Check for tmux on startup, error if not found
2. Create a tmux session per configured command
3. Attach to tmux session when user selects it
4. Return to pb home screen on detach (Ctrl+D still works in tmux via prefix)

**New file: `internal/tmux/tmux.go`**
```go
package tmux

import (
    "os/exec"
    "fmt"
)

// SessionExists checks if a tmux session exists
func SessionExists(name string) bool {
    cmd := exec.Command("tmux", "has-session", "-t", name)
    return cmd.Run() == nil
}

// CreateSession creates a new tmux session running the given command
func CreateSession(name, command string) error {
    // Create detached session
    cmd := exec.Command("tmux", "new-session", "-d", "-s", name, command)
    return cmd.Run()
}

// AttachSession attaches to an existing tmux session
// This replaces the current process
func AttachSession(name string) error {
    cmd := exec.Command("tmux", "attach-session", "-t", name)
    cmd.Stdin = os.Stdin
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    return cmd.Run()
}

// KillSession terminates a tmux session
func KillSession(name string) error {
    cmd := exec.Command("tmux", "kill-session", "-t", name)
    return cmd.Run()
}
```

**Test:** Run `pb`, select claude, verify tmux session starts and attaches.

---

### Phase 2: Activity Monitoring

**Goal:** Detect if a session is active or idle (for status display).

**Options:**

A. **`tmux capture-pane`** - Periodic polling
   ```go
   func CapturePane(session string) (string, error) {
       cmd := exec.Command("tmux", "capture-pane", "-t", session, "-p")
       out, err := cmd.Output()
       return string(out), err
   }
   ```
   - Compare captures to detect changes
   - Simple but requires polling

B. **`tmux pipe-pane`** - Stream output to file
   ```go
   func StartPipePane(session, pipePath string) error {
       cmd := exec.Command("tmux", "pipe-pane", "-t", session, fmt.Sprintf("cat > %s", pipePath))
       return cmd.Run()
   }
   ```
   - Watch file for writes to detect activity
   - More efficient but more complex

C. **tmux hooks** - Event-driven
   ```bash
   tmux set-hook -t session after-send-keys 'run-shell "touch /tmp/pb-activity-session"'
   ```
   - React to key events
   - Most efficient but complex setup

**Recommendation:** Start with option A (polling capture-pane) for simplicity.

---

### Phase 3: Session Lifecycle

**Goal:** Proper session start/stop/restart handling.

**Changes:**

1. On pb start: check for existing sessions, show status
2. Auto-start sessions marked as `enabled: true`
3. Handle session death (command exits)
4. Clean up sessions on pb exit (optional, configurable)

**Session states:**
- `not_created` - tmux session doesn't exist
- `running` - session exists and command is running
- `exited` - session exists but command has exited

```go
func GetSessionState(name string) SessionState {
    if !SessionExists(name) {
        return StateNotCreated
    }
    // Check if pane is still running
    cmd := exec.Command("tmux", "list-panes", "-t", name, "-F", "#{pane_dead}")
    out, _ := cmd.Output()
    if strings.TrimSpace(string(out)) == "1" {
        return StateExited
    }
    return StateRunning
}
```

---

### Phase 4: Detach Key Handling

**Goal:** Make Ctrl+D detach from tmux back to pb (not close the pane).

**Problem:** By default, Ctrl+D sends EOF to the shell, not tmux detach.

**Solutions:**

A. **Use tmux prefix + d** - Default tmux behavior (Ctrl+B, d)
   - No code needed
   - User must know tmux

B. **Rebind Ctrl+D in tmux** - Custom keybinding
   ```bash
   tmux bind-key -n C-d detach-client
   ```
   - Only for pb sessions
   - May conflict with shell usage of Ctrl+D

C. **Wrapper script** - Intercept Ctrl+D
   - Complex, not recommended

D. **Use a different key** - e.g., Ctrl+\\ or Escape
   - Document it clearly

**Recommendation:** Option A (standard tmux prefix+d) with clear documentation. Power users already know this.

---

### Phase 5: Nested tmux Handling

**Goal:** Work correctly when user is already in tmux.

**Detection:**
```go
func InTmux() bool {
    return os.Getenv("TMUX") != ""
}
```

**Options:**

A. **Error/warn** - Tell user to exit tmux first
B. **Nested attach** - Use `tmux -L pb` (separate server)
C. **Passthrough** - Just attach (nested tmux works, just awkward)

**Recommendation:** Option B - Use a separate tmux server socket:
```go
const tmuxSocket = "pocketbot"

func tmuxCmd(args ...string) *exec.Cmd {
    fullArgs := append([]string{"-L", tmuxSocket}, args...)
    return exec.Command("tmux", fullArgs...)
}
```

This avoids conflicts with user's existing tmux sessions.

---

### Phase 6: Status Bar / Overlay

**Goal:** Show "Ctrl+B d to detach" or similar hint.

**Options:**

A. **tmux status bar** - Configure per-session status
   ```bash
   tmux set-option -t session status-left "pb: Prefix+d to detach"
   ```

B. **No overlay** - Just document the keybinding
   - Simpler, cleaner

C. **Top status line** - Reserve first line
   - Complex, like the VT100 approach

**Recommendation:** Option A - Use tmux's built-in status bar.

---

## File Changes Summary

| File | Change |
|------|--------|
| `internal/tmux/tmux.go` | New: tmux wrapper functions |
| `internal/session/manager.go` | Refactor: use tmux instead of PTY |
| `internal/session/attach.go` | Simplify: just call tmux attach |
| `internal/session/render.go` | Delete: no longer needed |
| `cmd/pb/main.go` | Update: check for tmux, handle attach flow |

## Migration Path

1. Phase 1 first - get basic attach working
2. Delete old PTY code once tmux approach is validated
3. Add monitoring and polish incrementally

## Risks

1. **tmux not installed** - Clear error message, maybe offer install instructions
2. **User tmux config conflicts** - Use separate socket (`-L pocketbot`)
3. **Detach UX** - Users must learn tmux prefix (document clearly)

## Start Point

Begin with Phase 1 - minimal tmux integration. Get attach/detach working before adding monitoring.

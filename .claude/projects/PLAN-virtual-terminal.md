# Plan: Virtual Terminal Buffer for Pocketbot

## Problem

When Claude runs inside pocketbot, its output overwrites:
1. The CTRL-D overlay at the bottom of the screen
2. The home screen when transitioning back

This happens because pocketbot pipes PTY output directly to stdout without tracking screen state.

## Solution

Implement a virtual terminal buffer (like tmux) that:
1. Intercepts Claude's PTY output
2. Parses ANSI escape sequences to maintain screen state
3. Composes the screen with pocketbot's overlay
4. Renders the composed output

## Library

Use [github.com/vito/vt100](https://pkg.go.dev/github.com/vito/vt100) - a Go VT100 emulator that:
- Parses ANSI escape sequences
- Maintains a `Content [][]rune` grid
- Tracks cursor position and formatting
- Implements `io.Writer` interface

## Phases

### Phase 1: Basic Buffer (No Overlay Yet)

**Goal:** Replace direct stdout passthrough with buffered rendering.

**Changes to `internal/session/attach.go`:**

1. Create a `VT100` instance sized to terminal dimensions
2. Instead of writing PTY output to stdout, write to the VT100
3. After each write, render the VT100's Content to stdout
4. Handle terminal resize (SIGWINCH) by resizing the VT100

**Minimal code change:**
```go
// Before (current)
os.Stdout.Write(buf[:n])

// After (phase 1)
vt.Write(buf[:n])
renderScreen(vt)
```

**Test:** Run `pb`, attach to claude, verify output looks normal.

---

### Phase 2: Add Overlay Compositing

**Goal:** Draw the CTRL-D overlay as part of the composed output.

**Changes:**

1. Create a `renderScreen(vt *vt100.VT100, overlay string)` function
2. When rendering, replace the last line with the overlay
3. Use ANSI codes to position cursor correctly after render

**Minimal code:**
```go
func renderScreen(vt *vt100.VT100, overlay string) {
    // Clear screen and home cursor
    fmt.Print("\033[H\033[2J")

    // Render VT100 content (rows 0 to height-2)
    for y := 0; y < vt.Height-1; y++ {
        for x := 0; x < vt.Width; x++ {
            fmt.Print(string(vt.Content[y][x]))
        }
        fmt.Print("\n")
    }

    // Render overlay on last line
    fmt.Print("\033[7m") // reverse video
    fmt.Print(overlay)
    fmt.Print("\033[0m")

    // Restore cursor to VT100's cursor position
    fmt.Printf("\033[%d;%dH", vt.Cursor.Y+1, vt.Cursor.X+1)
}
```

**Test:** Overlay stays visible even when Claude outputs text.

---

### Phase 3: Optimize Rendering

**Goal:** Reduce flicker and improve performance.

**Changes:**

1. Only redraw changed lines (diff previous vs current)
2. Use terminal's scrolling capabilities when possible
3. Batch output with a write buffer
4. Rate-limit redraws (e.g., max 60fps)

---

### Phase 4: Format/Color Support

**Goal:** Preserve Claude's colors and formatting.

**Changes:**

1. When rendering, also output `vt.Format[y][x]` as ANSI codes
2. Track format changes to minimize escape sequence output
3. Handle all format attributes (bold, underline, colors, etc.)

---

### Phase 5: Clean Transitions

**Goal:** Smooth transition between home screen and attached session.

**Changes:**

1. On detach: clear the VT100 buffer
2. On attach: properly initialize buffer from current terminal state
3. Handle alternate screen buffer compatibility

---

## File Changes Summary

| File | Change |
|------|--------|
| `go.mod` | Add `github.com/vito/vt100` dependency |
| `internal/session/attach.go` | Replace stdout passthrough with VT100 buffer + render |
| `internal/session/render.go` | New file: screen composition and rendering logic |

## Risks

1. **Performance:** Parsing every byte through VT100 adds overhead
2. **Compatibility:** VT100 library may not handle all escape sequences Claude uses
3. **Flicker:** Full redraws may cause visible flicker

## Start Point

Begin with Phase 1 - get the basic buffer working without the overlay first. This validates the approach before adding complexity.

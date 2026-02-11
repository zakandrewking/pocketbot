# Terminal State Management Research

## Problems We're Seeing
1. Claude output overwrites pocketbot home screen
2. Blank screen when reattaching to Claude
3. Terminal state corruption between transitions

## How tmux Handles This

### tmux Architecture
1. **Client/Server Model**: Server manages PTY sessions, client just attaches
2. **Virtual Terminal**: Each session has its own virtual terminal that maintains state
3. **Screen Redraw**: On reattach, tmux redraws the entire virtual terminal
4. **Alternate Screen**: Uses alternate screen buffer for separation

### Key Concepts

#### Alternate Screen Buffer
- Most terminals support two screen buffers: main and alternate
- Escape sequence to switch: `\033[?1049h` (enter alt) and `\033[?1049l` (exit alt)
- TUI apps should use alternate screen so they don't pollute main screen
- When you exit, it restores the original screen

#### Terminal State
- Raw mode vs cooked mode
- Screen buffer contents
- Cursor position
- Window size

### What We Need to Do

1. **Use Alternate Screen for Bubble Tea**
   - Bubble Tea should already do this, but we might be interfering
   - Check if we're properly initializing/deinitializing

2. **Force PTY Redraw on Attach**
   - Send SIGWINCH (window resize) signal to force redraw
   - Or send Ctrl+L (redraw) sequence

3. **Proper State Transitions**
   - Save terminal state before attach
   - Restore terminal state after detach
   - Clear screen at transitions

4. **Don't Clear PTY Output**
   - The PTY maintains Claude's screen state
   - We just need to make Claude redraw it

## Implementation Plan

1. Ensure Bubble Tea uses alternate screen (check tea.WithAltScreen())
2. On attach: trigger PTY redraw with SIGWINCH or resize event
3. On detach: restore terminal properly
4. Remove manual screen clearing (let the tools handle it)

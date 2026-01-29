# Project Plan: Claude Session Management

## Goal
Add ability to launch and manage a Claude Code session from within pocketbot, similar to tmux session management.

## Requirements
1. Press "c" to attach to a Claude Code session
2. Claude runs with `claude --continue` in current directory
3. Press Ctrl+P while in Claude to detach and return to pocketbot home
4. Home screen shows indicator when Claude is running
5. Quitting pocketbot kills the Claude process

## Architecture

### Components
1. **PTY Manager** - Manages pseudo-terminal for Claude process
2. **Session State** - Tracks whether Claude is running and attached/detached
3. **Input Proxy** - Intercepts Ctrl+P when attached to Claude
4. **UI States** - Home screen vs Attached screen

### State Machine
```
[Home - No Session]
    ↓ (press 'c')
[Home - Session Starting]
    ↓
[Attached to Claude]
    ↓ (press Ctrl+P)
[Home - Session Running]
    ↓ (press 'c')
[Attached to Claude]
    ↓ (Claude exits or pocketbot quits)
[Home - No Session]
```

## Implementation Steps

### Phase 1: Basic PTY and Process Management
- [ ] Add `github.com/creack/pty` dependency
- [ ] Create `session` package for managing Claude process
- [ ] Implement `StartClaude()` - spawn Claude in a pty
- [ ] Implement `StopClaude()` - kill Claude process
- [ ] Add process cleanup on pocketbot exit
- [ ] **Test**: Manually verify Claude starts and is killed on exit

### Phase 2: Attach/Detach State Management
- [ ] Add session state to model (not running, running detached, running attached)
- [ ] Update model to handle "c" key press in home state
- [ ] Add view state for "attached to Claude"
- [ ] Implement basic attach (just detect state change for now)
- [ ] Update home screen to show "Claude running" indicator
- [ ] **Test**: Unit test state transitions

### Phase 3: Terminal I/O Forwarding
- [ ] Implement terminal raw mode switching
- [ ] Forward stdin to pty when attached
- [ ] Forward pty stdout to terminal when attached
- [ ] Handle terminal resize events (SIGWINCH)
- [ ] **Test**: Manually verify can interact with Claude

### Phase 4: Ctrl+P Interception
- [ ] Create input proxy that reads stdin byte-by-byte
- [ ] Detect Ctrl+P sequence (0x10)
- [ ] On Ctrl+P, detach from Claude and return to home
- [ ] Pass all other input through to pty
- [ ] **Test**: Manually verify Ctrl+P detaches properly

### Phase 5: Polish and Edge Cases
- [ ] Handle Claude process crashes gracefully
- [ ] Add error handling for pty creation failures
- [ ] Ensure proper cleanup of file descriptors
- [ ] Add logging for debugging
- [ ] **Test**: Manual testing of error scenarios

## Dependencies
- `github.com/creack/pty` - Pseudo-terminal management
- Existing: `github.com/charmbracelet/bubbletea`

## Testing Strategy
- Unit tests for state management
- Manual testing for terminal I/O (hard to automate)
- Process lifecycle tests (spawn, kill, cleanup)

## Risks and Mitigations
- **Risk**: Terminal state corruption if detach fails
  - **Mitigation**: Defer terminal restoration, panic recovery
- **Risk**: Zombie processes if cleanup fails
  - **Mitigation**: Use process groups, ensure cleanup in defer
- **Risk**: Input handling bugs breaking Claude
  - **Mitigation**: Extensive manual testing, logging

## Success Criteria
- Can launch Claude with "c" key
- Can interact with Claude normally (all keys work)
- Ctrl+P returns to pocketbot without killing Claude
- Home screen shows "Claude running" when detached
- Pressing "c" again reattaches to same session
- Quitting pocketbot kills Claude process cleanly

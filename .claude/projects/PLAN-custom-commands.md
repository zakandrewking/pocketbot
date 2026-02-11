# Project Plan: Custom Configurable Commands

## Goal
Allow users to configure custom commands (like dev servers) that can run in background sessions with keybindings, similar to how Claude works.

## Requirements
1. Configuration file to define custom commands
2. Each command has a name, command string, and keybinding
3. Claude remains the default (key 'c')
4. Multiple sessions can run simultaneously
5. UI shows all configured sessions and their states
6. Can attach/detach from any running session
7. Each session gets its own PTY with activity monitoring

## Use Cases
- Press 'd' to start/attach to dev server (npm run dev)
- Press 'a' to start/attach to API server
- Press 'c' for Claude (default)
- Home screen shows all sessions with their active/idle states

## Configuration File Design

### Location
`~/.config/pocketbot/config.yaml`

### Format
```yaml
# Default Claude session (optional, defaults shown)
claude:
  command: "claude --continue"
  key: "c"
  enabled: true

# Custom sessions
sessions:
  - name: "dev-server"
    command: "npm run dev"
    key: "d"

  - name: "api"
    command: "go run cmd/api/main.go"
    key: "a"

  - name: "logs"
    command: "tail -f /var/log/app.log"
    key: "l"
```

## Architecture Changes

### Config Package
- Parse YAML config file
- Validate configuration (no duplicate keys, valid commands)
- Provide default config if file doesn't exist

### Session Manager Changes
- Support multiple named sessions
- Map of session name -> Manager
- Track which session is currently selected for attach
- Allow listing all sessions

### UI Changes
- Show all configured sessions on home screen
- Display state for each (running/not running, active/idle)
- Route keypresses to correct session
- Show which session you're attached to

### Model Changes
- Track map of sessions instead of single session
- Handle multiple keybindings dynamically
- Session selection state

## Implementation Steps

### Phase 1: Config Infrastructure
- [ ] Create config package
- [ ] Define config structs
- [ ] Implement YAML parsing with gopkg.in/yaml.v3
- [ ] Add config validation
- [ ] Add tests for config parsing
- [ ] Load default config if file doesn't exist

### Phase 2: Multi-Session Manager
- [ ] Refactor session.Manager to support multiple sessions
- [ ] Create SessionRegistry to manage multiple sessions
- [ ] Update Start/Stop to work with session names
- [ ] Add GetSession(name) method
- [ ] Add ListSessions() method
- [ ] Add tests for multi-session management

### Phase 3: UI Integration
- [ ] Update model to use SessionRegistry
- [ ] Generate keybindings from config
- [ ] Update home view to show all sessions
- [ ] Handle dynamic keypresses
- [ ] Show session name when attached
- [ ] Add tests for multi-session UI

### Phase 4: Documentation and Polish
- [ ] Update README with config example
- [ ] Add example config file
- [ ] Handle config errors gracefully
- [ ] Add config validation error messages
- [ ] Test with multiple sessions running

## Testing Strategy
- Unit tests for config parsing
- Unit tests for session registry
- Manual testing with multiple sessions
- Test invalid configs
- Test missing config (should use defaults)

## Migration Path
- Existing users with no config: Claude works as before (backwards compatible)
- Config file is optional
- Default behavior unchanged

## Nice-to-Have (Future)
- Config hot-reload (watch file for changes)
- Session naming on CLI: `pb attach dev-server`
- Session logging to files
- Configurable idle timeout per session
- Terminal multiplexer: split screen to show multiple sessions

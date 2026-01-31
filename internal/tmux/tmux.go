package tmux

import (
	"os"
	"os/exec"
	"sync"
)

// Socket name for pocketbot's tmux server (isolated from user's tmux)
const Socket = "pocketbot"

// cmd creates a tmux command using pocketbot's socket
func cmd(args ...string) *exec.Cmd {
	fullArgs := append([]string{"-L", Socket}, args...)
	return exec.Command("tmux", fullArgs...)
}

// Available checks if tmux is installed
func Available() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

// SessionExists checks if a tmux session exists
func SessionExists(name string) bool {
	return cmd("has-session", "-t", name).Run() == nil
}

// CreateSession creates a new detached tmux session running the given command
func CreateSession(name, command string) error {
	if err := cmd("new-session", "-d", "-s", name, command).Run(); err != nil {
		return err
	}

	// Hide status bar to save screen space
	if err := cmd("set-option", "-t", name, "status", "off").Run(); err != nil {
		return err
	}

	// Bind 'd' to detach (no prefix needed)
	// This only affects pocketbot's tmux server, not user's main tmux
	if err := cmd("bind-key", "-n", "d", "detach-client").Run(); err != nil {
		return err
	}

	// Show brief message on attach about Ctrl+D (stays for 3 seconds)
	if err := cmd("set-option", "-t", name, "display-time", "3000").Run(); err != nil {
		return err
	}

	return nil
}

// AttachSession attaches to an existing tmux session
// This takes over stdin/stdout until the user detaches
func AttachSession(name string) error {
	// Show a floating message for 3 seconds when attaching
	// This appears as a small overlay in the center of the screen
	cmd("display-message", "-t", name, "d to detach").Run()

	c := cmd("attach-session", "-t", name)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

// KillSession terminates a tmux session
func KillSession(name string) error {
	return cmd("kill-session", "-t", name).Run()
}

// KillServer kills the entire pocketbot tmux server
func KillServer() error {
	return cmd("kill-server").Run()
}

// Session represents a tmux-backed session
type Session struct {
	name    string
	command string
	mu      sync.Mutex
}

// NewSession creates a new tmux session wrapper
func NewSession(name, command string) *Session {
	return &Session{
		name:    name,
		command: command,
	}
}

// IsRunning returns whether the tmux session exists
func (s *Session) IsRunning() bool {
	return SessionExists(s.name)
}

// Start creates the tmux session if it doesn't exist
func (s *Session) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if SessionExists(s.name) {
		return nil // Already running
	}
	return CreateSession(s.name, s.command)
}

// Stop kills the tmux session
func (s *Session) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !SessionExists(s.name) {
		return nil // Already stopped
	}
	return KillSession(s.name)
}

// Attach attaches to the tmux session
// Returns nil on normal detach, error on failure
func (s *Session) Attach() error {
	return AttachSession(s.name)
}

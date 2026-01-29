package session

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
)

// Manager handles the Claude Code session lifecycle
type Manager struct {
	cmd     *exec.Cmd
	pty     *os.File
	running bool
	mu      sync.Mutex
}

// New creates a new session manager
func New() *Manager {
	return &Manager{}
}

// Start launches a Claude Code session in a PTY
func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return fmt.Errorf("session already running")
	}

	// Create the command
	m.cmd = exec.Command("claude", "--continue")
	m.cmd.Dir, _ = os.Getwd()

	// Start the command with a pty
	ptmx, err := pty.Start(m.cmd)
	if err != nil {
		return fmt.Errorf("failed to start pty: %w", err)
	}

	m.pty = ptmx
	m.running = true

	// Monitor process exit
	go func() {
		m.cmd.Wait()
		m.mu.Lock()
		m.running = false
		m.mu.Unlock()
	}()

	return nil
}

// Stop kills the Claude process and cleans up
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return nil
	}

	// Close the pty first
	if m.pty != nil {
		m.pty.Close()
	}

	// Kill the process
	if m.cmd != nil && m.cmd.Process != nil {
		if err := m.cmd.Process.Kill(); err != nil {
			return fmt.Errorf("failed to kill process: %w", err)
		}
	}

	m.running = false
	return nil
}

// IsRunning returns whether the Claude session is running
func (m *Manager) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// PTY returns the pty file handle for I/O operations
func (m *Manager) PTY() *os.File {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.pty
}

// Read reads from the pty (Claude's output)
func (m *Manager) Read(p []byte) (n int, err error) {
	if m.pty == nil {
		return 0, fmt.Errorf("pty not available")
	}
	return m.pty.Read(p)
}

// Write writes to the pty (input to Claude)
func (m *Manager) Write(p []byte) (n int, err error) {
	if m.pty == nil {
		return 0, fmt.Errorf("pty not available")
	}
	return m.pty.Write(p)
}

// Copy copies data between the pty and the given reader/writer
// This is used when attached to the session
func (m *Manager) Copy(dst io.Writer, src io.Reader) error {
	_, err := io.Copy(dst, src)
	return err
}

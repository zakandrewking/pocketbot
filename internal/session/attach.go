package session

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"
)

// AttachResult indicates how the attach session ended
type AttachResult int

const (
	AttachDetached AttachResult = iota // User pressed Ctrl+D
	AttachExited                       // Claude process exited
)

// Attach connects the current terminal to the Claude session
// Returns when the user presses Ctrl+D (detach) or Claude exits
func (m *Manager) Attach() (AttachResult, error) {
	if !m.IsRunning() {
		return AttachExited, fmt.Errorf("session not running")
	}

	ptmx := m.PTY()
	if ptmx == nil {
		return AttachExited, fmt.Errorf("pty not available")
	}

	// Put terminal in raw mode
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return AttachExited, fmt.Errorf("failed to set raw mode: %w", err)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	// Handle terminal resize
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	go func() {
		for range ch {
			if err := pty.InheritSize(os.Stdin, ptmx); err != nil {
				// Ignore errors during resize
			}
		}
	}()
	defer signal.Stop(ch)

	// Set initial size
	if err := pty.InheritSize(os.Stdin, ptmx); err != nil {
		return AttachExited, fmt.Errorf("failed to set pty size: %w", err)
	}

	// Force redraw by sending a resize signal
	// This ensures Claude redraws its screen when we reattach
	if m.cmd != nil && m.cmd.Process != nil {
		m.cmd.Process.Signal(syscall.SIGWINCH)
	}

	// Create channels for I/O completion and detach signal
	done := make(chan error, 1)
	detach := make(chan struct{})

	// Copy output from pty to stdout
	go func() {
		_, err := io.Copy(os.Stdout, ptmx)
		done <- err
	}()

	// Copy input from stdin to pty, intercepting Ctrl+D
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil {
				done <- err
				return
			}

			// Check for Ctrl+D (0x04) - detach signal
			// We need to check and filter it out before writing to pty
			filtered := make([]byte, 0, n)
			for i := 0; i < n; i++ {
				if buf[i] == 0x04 { // Ctrl+D
					close(detach)
					return
				}
				filtered = append(filtered, buf[i])
			}

			// Write filtered input to pty
			if len(filtered) > 0 {
				if _, err := ptmx.Write(filtered); err != nil {
					done <- err
					return
				}
			}
		}
	}()

	// Wait for detach or error
	select {
	case <-detach:
		// Give a moment for terminal to settle after detach
		time.Sleep(50 * time.Millisecond)
		return AttachDetached, nil
	case err := <-done:
		// Give a moment for terminal to settle after exit
		time.Sleep(50 * time.Millisecond)
		if err != nil && err != io.EOF {
			return AttachExited, err
		}
		return AttachExited, nil
	}
}

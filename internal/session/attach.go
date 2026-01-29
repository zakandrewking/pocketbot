package session

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/creack/pty"
	"golang.org/x/term"
)

// AttachResult indicates how the attach session ended
type AttachResult int

const (
	AttachDetached AttachResult = iota // User pressed Ctrl+P
	AttachExited                       // Claude process exited
)

// Attach connects the current terminal to the Claude session
// Returns when the user presses Ctrl+P (detach) or Claude exits
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

	// Create channels for I/O completion and detach signal
	done := make(chan error, 1)
	detach := make(chan struct{})

	// Copy output from pty to stdout
	go func() {
		_, err := io.Copy(os.Stdout, ptmx)
		done <- err
	}()

	// Copy input from stdin to pty, intercepting Ctrl+P
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil {
				done <- err
				return
			}

			// Check for Ctrl+P (0x10)
			for i := 0; i < n; i++ {
				if buf[i] == 0x10 { // Ctrl+P
					close(detach)
					return
				}
			}

			// Write to pty
			if _, err := ptmx.Write(buf[:n]); err != nil {
				done <- err
				return
			}
		}
	}()

	// Wait for detach or error
	select {
	case <-detach:
		return AttachDetached, nil
	case err := <-done:
		if err != nil && err != io.EOF {
			return AttachExited, err
		}
		return AttachExited, nil
	}
}

package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// IdleTimeout is how long without changes before marking session as idle
const IdleTimeout = 5 * time.Second

const (
	activePollInterval       = 750 * time.Millisecond
	pendingActivityPollDelay = 250 * time.Millisecond
	activityConfirmWindow    = 500 * time.Millisecond
)

// getSocketName returns the tmux socket name for the current nesting level
func getSocketName() string {
	level := os.Getenv("PB_LEVEL")
	if level == "" {
		return "pocketbot"
	}
	return fmt.Sprintf("pocketbot-%s", level)
}

// getNestingLevel returns the current pb nesting level
func getNestingLevel() int {
	level := os.Getenv("PB_LEVEL")
	if level == "" {
		return 0
	}
	n, _ := strconv.Atoi(level)
	return n
}

// cmd creates a tmux command using pocketbot's socket
func cmd(args ...string) *exec.Cmd {
	fullArgs := append([]string{"-L", getSocketName()}, args...)
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
	// Get current working directory to store with session
	cwd, _ := os.Getwd()

	// Set PB_LEVEL environment variable for nested pb instances
	// Also set PB_CWD to track where session was launched from
	nextLevel := getNestingLevel() + 1
	envCmd := fmt.Sprintf("export PB_LEVEL=%d; export PB_CWD='%s'; %s", nextLevel, cwd, command)

	if err := cmd("new-session", "-d", "-s", name, "-c", cwd, "sh", "-c", envCmd).Run(); err != nil {
		return err
	}

	// Store the launch directory as a tmux session option (for easy querying)
	if err := cmd("set-option", "-t", name, "@pb_cwd", cwd).Run(); err != nil {
		// Non-fatal - just means we can't check directory later
	}
	// Store which configured command this session belongs to.
	if err := cmd("set-option", "-t", name, "@pb_command", name).Run(); err != nil {
		// Non-fatal - binding can still fall back to session name.
	}

	// Hide status bar to save screen space
	if err := cmd("set-option", "-t", name, "status", "off").Run(); err != nil {
		return err
	}

	// Bind Ctrl+D to detach (no prefix needed)
	// This only affects pocketbot's tmux server, not user's main tmux
	if err := cmd("bind-key", "-n", "C-d", "detach-client").Run(); err != nil {
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
	showDetachOverlay(name)

	c := cmd("attach-session", "-t", name)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

func detachOverlayMessage(level int) string {
	msg := "Ctrl+D to detach"
	if level > 0 {
		return fmt.Sprintf("%s (pb level %d)", msg, level)
	}
	return msg
}

func showDetachOverlay(name string) {
	msg := detachOverlayMessage(getNestingLevel())
	// Prefer a tiny top-right popup so we don't reserve a full line in the UI.
	// Fall back to display-message when popup is unavailable.
	if err := showDetachPopup(name, msg); err == nil {
		return
	}
	if err := cmd("display-message", "-d", "2500", "-x", "R", "-y", "0", "-t", name, msg).Run(); err == nil {
		return
	}
	cmd("display-message", "-d", "2500", "-t", name, msg).Run()
}

func showDetachPopup(name, msg string) error {
	width := strconv.Itoa(detachPopupWidth(msg))
	command := "printf %s " + shellSingleQuote(msg) + "; sleep 2"
	return cmd(
		"display-popup",
		"-E",
		"-B",
		"-x", "R",
		"-y", "0",
		"-w", width,
		"-h", "1",
		"-t", name,
		command,
	).Run()
}

func detachPopupWidth(msg string) int {
	// Add breathing room around the message while keeping popup compact.
	width := utf8.RuneCountInString(msg) + 4
	if width < 24 {
		return 24
	}
	if width > 96 {
		return 96
	}
	return width
}

func shellSingleQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

// KillSession terminates a tmux session
func KillSession(name string) error {
	return cmd("kill-session", "-t", name).Run()
}

// KillServer kills the entire pocketbot tmux server
func KillServer() error {
	return cmd("kill-server").Run()
}

// CapturePane captures the content of a pane (for testing)
func CapturePane(sessionName string) (string, error) {
	out, err := cmd("capture-pane", "-t", sessionName, "-p").Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// GetSessionCwd returns the working directory where a session was launched
func GetSessionCwd(sessionName string) string {
	out, err := cmd("show-options", "-t", sessionName, "-v", "@pb_cwd").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// GetSessionCommand returns the configured command binding for a session.
func GetSessionCommand(sessionName string) string {
	out, err := cmd("show-options", "-t", sessionName, "-v", "@pb_command").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// ListSessions returns all active session names
func ListSessions() []string {
	out, err := cmd("list-sessions", "-F", "#{session_name}").Output()
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	return lines
}

// Session represents a tmux-backed session
type Session struct {
	name         string
	command      string
	mu           sync.Mutex
	lastCapture  string
	lastActivity time.Time
	nextPollAt   time.Time
	pendingSince time.Time
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

// capturePane captures the current pane content (last 10 lines only for efficiency)
func (s *Session) capturePane() (string, error) {
	// Only capture last 10 lines to reduce overhead
	out, err := cmd("capture-pane", "-t", s.name, "-p", "-S", "-10").Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// UpdateActivity checks for pane changes and updates activity state
// Returns true if active, false if idle
func (s *Session) UpdateActivity() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !SessionExists(s.name) {
		return false
	}
	now := time.Now()
	if !s.nextPollAt.IsZero() && now.Before(s.nextPollAt) {
		return now.Sub(s.lastActivity) < IdleTimeout
	}

	// Capture current pane content
	// Use a shorter capture to reduce overhead (last 10 lines only)
	current, err := s.capturePane()
	if err != nil {
		// On error, assume no change but don't crash
		s.nextPollAt = now.Add(3 * time.Second)
		return now.Sub(s.lastActivity) < IdleTimeout
	}

	// Baseline capture avoids treating initial pane snapshot as activity.
	if s.lastCapture == "" {
		s.lastCapture = current
		s.pendingSince = time.Time{}
		s.nextPollAt = now.Add(activePollInterval)
		return now.Sub(s.lastActivity) < IdleTimeout
	}

	// Check if content changed.
	if current != s.lastCapture {
		if s.pendingSince.IsZero() {
			s.pendingSince = now
			s.nextPollAt = now.Add(pendingActivityPollDelay)
			return now.Sub(s.lastActivity) < IdleTimeout
		}
		if now.Sub(s.pendingSince) >= activityConfirmWindow {
			s.lastCapture = current
			s.lastActivity = now
			s.pendingSince = time.Time{}
			s.nextPollAt = now.Add(activePollInterval)
			return true
		}
		s.nextPollAt = now.Add(pendingActivityPollDelay)
		return now.Sub(s.lastActivity) < IdleTimeout
	}

	s.pendingSince = time.Time{}
	s.nextPollAt = now.Add(nextActivityPollInterval(now.Sub(s.lastActivity)))

	// Content hasn't changed - check if idle timeout exceeded
	return now.Sub(s.lastActivity) < IdleTimeout
}

// IsActive returns whether the session is currently active (has recent activity)
func (s *Session) IsActive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !SessionExists(s.name) {
		return false
	}

	return time.Since(s.lastActivity) < IdleTimeout
}

// ActivityKnown reports whether we've captured enough pane data to classify
// activity for this running session.
func (s *Session) ActivityKnown() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !SessionExists(s.name) {
		return false
	}
	return s.lastCapture != ""
}

func nextActivityPollInterval(idleFor time.Duration) time.Duration {
	switch {
	case idleFor < IdleTimeout:
		return 1 * time.Second
	case idleFor < 30*time.Second:
		return 2 * time.Second
	case idleFor < 2*time.Minute:
		return 5 * time.Second
	default:
		return 10 * time.Second
	}
}

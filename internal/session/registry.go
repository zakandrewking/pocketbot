package session

import (
	"fmt"
	"sync"
	"time"
)

// Registry manages multiple named sessions
type Registry struct {
	sessions map[string]*Manager
	mu       sync.RWMutex
}

// NewRegistry creates a new session registry
func NewRegistry() *Registry {
	return &Registry{
		sessions: make(map[string]*Manager),
	}
}

// Create creates a new session with the given name and command
func (r *Registry) Create(name, command string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.sessions[name]; exists {
		return fmt.Errorf("session %q already exists", name)
	}

	manager := NewWithCommand(command)
	r.sessions[name] = manager
	return nil
}

// Get retrieves a session by name
func (r *Registry) Get(name string) (*Manager, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	manager, exists := r.sessions[name]
	if !exists {
		return nil, fmt.Errorf("session %q not found", name)
	}
	return manager, nil
}

// Start starts a session by name
func (r *Registry) Start(name string) error {
	manager, err := r.Get(name)
	if err != nil {
		return err
	}
	return manager.Start()
}

// Stop stops a session by name
func (r *Registry) Stop(name string) error {
	manager, err := r.Get(name)
	if err != nil {
		return err
	}
	return manager.Stop()
}

// StopAll stops all running sessions
func (r *Registry) StopAll() error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var errs []error
	for name, manager := range r.sessions {
		if err := manager.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("failed to stop %q: %w", name, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors stopping sessions: %v", errs)
	}
	return nil
}

// List returns all session names
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.sessions))
	for name := range r.sessions {
		names = append(names, name)
	}
	return names
}

// SessionInfo contains information about a session
type SessionInfo struct {
	Name          string
	Running       bool
	ActivityState ActivityState
}

// ListInfo returns information about all sessions
func (r *Registry) ListInfo() []SessionInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	infos := make([]SessionInfo, 0, len(r.sessions))
	for name, manager := range r.sessions {
		infos = append(infos, SessionInfo{
			Name:          name,
			Running:       manager.IsRunning(),
			ActivityState: manager.GetActivityState(),
		})
	}
	return infos
}

// Attach attaches to a session by name
func (r *Registry) Attach(name string) (AttachResult, error) {
	manager, err := r.Get(name)
	if err != nil {
		return AttachExited, err
	}
	return manager.Attach()
}

// NewWithCommand creates a new session manager with a custom command
func NewWithCommand(command string) *Manager {
	return &Manager{
		command:         command,
		activityMonitor: NewActivityMonitor(5 * time.Second),
	}
}

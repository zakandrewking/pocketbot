package session

import (
	"sync"
	"time"
)

// ActivityState represents whether the session is active or idle
type ActivityState int

const (
	StateIdle ActivityState = iota
	StateActive
)

// ActivityMonitor tracks I/O activity on the PTY
type ActivityMonitor struct {
	lastActivity time.Time
	mu           sync.RWMutex
	idleTimeout  time.Duration
}

// NewActivityMonitor creates a new activity monitor
func NewActivityMonitor(idleTimeout time.Duration) *ActivityMonitor {
	return &ActivityMonitor{
		lastActivity: time.Now(),
		idleTimeout:  idleTimeout,
	}
}

// RecordActivity updates the last activity timestamp
func (a *ActivityMonitor) RecordActivity() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lastActivity = time.Now()
}

// GetState returns the current activity state
func (a *ActivityMonitor) GetState() ActivityState {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if time.Since(a.lastActivity) < a.idleTimeout {
		return StateActive
	}
	return StateIdle
}

// GetLastActivity returns the time of last activity
func (a *ActivityMonitor) GetLastActivity() time.Time {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.lastActivity
}

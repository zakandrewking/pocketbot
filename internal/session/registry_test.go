package session

import (
	"testing"
)

func TestNewRegistry(t *testing.T) {
	reg := NewRegistry()
	if reg == nil {
		t.Fatal("NewRegistry should return non-nil registry")
	}

	if len(reg.List()) != 0 {
		t.Error("New registry should have no sessions")
	}
}

func TestCreateSession(t *testing.T) {
	reg := NewRegistry()

	err := reg.Create("test", "echo test")
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	sessions := reg.List()
	if len(sessions) != 1 {
		t.Errorf("Expected 1 session, got %d", len(sessions))
	}
}

func TestCreateDuplicateSession(t *testing.T) {
	reg := NewRegistry()

	reg.Create("test", "echo test")
	err := reg.Create("test", "echo test2")

	if err == nil {
		t.Error("Creating duplicate session should error")
	}
}

func TestGetSession(t *testing.T) {
	reg := NewRegistry()
	reg.Create("test", "echo test")

	manager, err := reg.Get("test")
	if err != nil {
		t.Fatalf("Failed to get session: %v", err)
	}

	if manager == nil {
		t.Error("Get should return non-nil manager")
	}
}

func TestGetNonexistentSession(t *testing.T) {
	reg := NewRegistry()

	_, err := reg.Get("nonexistent")
	if err == nil {
		t.Error("Getting nonexistent session should error")
	}
}

func TestStartStopSession(t *testing.T) {
	reg := NewRegistry()
	reg.Create("test", "sleep 10")

	// Start session
	err := reg.Start("test")
	if err != nil {
		t.Fatalf("Failed to start session: %v", err)
	}

	// Check it's running
	manager, _ := reg.Get("test")
	if !manager.IsRunning() {
		t.Error("Session should be running after start")
	}

	// Stop session
	err = reg.Stop("test")
	if err != nil {
		t.Fatalf("Failed to stop session: %v", err)
	}

	// Check it's stopped
	if manager.IsRunning() {
		t.Error("Session should not be running after stop")
	}
}

func TestStopAll(t *testing.T) {
	reg := NewRegistry()
	reg.Create("test1", "sleep 10")
	reg.Create("test2", "sleep 10")

	reg.Start("test1")
	reg.Start("test2")

	err := reg.StopAll()
	if err != nil {
		t.Fatalf("StopAll failed: %v", err)
	}

	m1, _ := reg.Get("test1")
	m2, _ := reg.Get("test2")

	if m1.IsRunning() || m2.IsRunning() {
		t.Error("All sessions should be stopped after StopAll")
	}
}

func TestListInfo(t *testing.T) {
	reg := NewRegistry()
	reg.Create("test1", "echo test1")
	reg.Create("test2", "sleep 10")

	reg.Start("test2")
	defer reg.Stop("test2")

	infos := reg.ListInfo()
	if len(infos) != 2 {
		t.Errorf("Expected 2 session infos, got %d", len(infos))
	}

	// Find test2 info
	var test2Info *SessionInfo
	for _, info := range infos {
		if info.Name == "test2" {
			test2Info = &info
			break
		}
	}

	if test2Info == nil {
		t.Fatal("Could not find test2 in session infos")
	}

	if !test2Info.Running {
		t.Error("test2 should be running")
	}
}

func TestNewWithCommand(t *testing.T) {
	manager := NewWithCommand("echo test")
	if manager == nil {
		t.Fatal("NewWithCommand should return non-nil manager")
	}

	if manager.command != "echo test" {
		t.Errorf("Expected command 'echo test', got %q", manager.command)
	}
}

package tmux

import (
	"reflect"
	"testing"
)

func TestParseProcessSnapshot(t *testing.T) {
	raw := `
  100   1 S+ /bin/zsh
  111 100 R+ claude --continue
  112 111 S+ git status --short
`
	got, err := parseProcessSnapshot(raw)
	if err != nil {
		t.Fatalf("parseProcessSnapshot returned error: %v", err)
	}

	if got[111].command != "claude --continue" {
		t.Fatalf("expected command for pid 111, got %q", got[111].command)
	}
	if got[112].ppid != 111 {
		t.Fatalf("expected pid 112 parent 111, got %d", got[112].ppid)
	}
}

func TestCollectDescendantTasks(t *testing.T) {
	processes := map[int]processInfo{
		100: {pid: 100, ppid: 1, state: "S+", command: "/bin/zsh"},
		111: {pid: 111, ppid: 100, state: "R+", command: "claude --continue"},
		112: {pid: 112, ppid: 111, state: "S+", command: "git status --short"},
		200: {pid: 200, ppid: 1, state: "S+", command: "unrelated"},
	}

	got := collectDescendantTasks([]int{100}, processes)
	want := []Task{
		{PID: 111, PPID: 100, State: "R+", Command: "claude --continue"},
		{PID: 112, PPID: 111, State: "S+", Command: "git status --short"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("collectDescendantTasks mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestFilterUserTasksPrefersLeafNonInfrastructure(t *testing.T) {
	tasks := []Task{
		{PID: 111, PPID: 100, State: "S+", Command: "claude --continue"},
		{PID: 112, PPID: 111, State: "S+", Command: "gopls"},
		{PID: 113, PPID: 111, State: "S+", Command: "sleep 300"},
	}

	got := filterUserTasks(tasks)
	want := []Task{
		{PID: 113, PPID: 111, State: "S+", Command: "sleep 300"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filterUserTasks mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestFilterUserTasksDropsInfrastructureOnlyTrees(t *testing.T) {
	tasks := []Task{
		{PID: 111, PPID: 100, State: "S+", Command: "claude --continue"},
		{PID: 112, PPID: 111, State: "S+", Command: "gopls"},
	}

	got := filterUserTasks(tasks)
	var want []Task
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filterUserTasks infrastructure-only mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

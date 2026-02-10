package tmux

import (
	"reflect"
	"sort"
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
	if len(got) != 0 {
		t.Fatalf("filterUserTasks infrastructure-only mismatch:\n got: %#v\nwant empty", got)
	}
}

func TestFilterUserTasksDropsKnownNodeWorkerNoise(t *testing.T) {
	tasks := []Task{
		{PID: 1753, PPID: 55235, State: "S+", Command: "caffeinate -i -t 300"},
		{PID: 3204, PPID: 3143, State: "Ss", Command: "/opt/homebrew/bin/node /repo/node_modules/nx/src/daemon/server/start.js"},
		{PID: 3269, PPID: 3211, State: "S", Command: "/opt/homebrew/bin/node /repo/node_modules/fork-ts-checker-webpack-plugin/lib/typescript/worker/get-dependencies-worker.js"},
		{PID: 3322, PPID: 3211, State: "S", Command: "/opt/homebrew/bin/node /repo/node_modules/fork-ts-checker-webpack-plugin/lib/typescript/worker/get-issues-worker.js"},
		{PID: 3491, PPID: 3143, State: "S", Command: "/repo/node_modules/@esbuild/darwin-arm64/bin/esbuild --service=0.19.12 --ping"},
		{PID: 4088, PPID: 3143, State: "S", Command: "/opt/homebrew/bin/node --inspect=localhost:9229 /repo/node_modules/@nx/js/src/executors/node/node-with-require-overrides"},
	}

	got := filterUserTasks(tasks)
	if len(got) != 0 {
		t.Fatalf("filterUserTasks node-noise mismatch:\n got: %#v\nwant empty", got)
	}
}

func TestFilterUserTasksKeepsRelevantOrchestrators(t *testing.T) {
	tasks := []Task{
		{PID: 42091, PPID: 42080, State: "S", Command: "/Applications/Xcode.app/Contents/Developer/usr/bin/make integration-test-backend"},
		{PID: 89262, PPID: 89236, State: "S", Command: "/opt/homebrew/bin/node /repo/node_modules/.bin/nx serve backend"},
		{PID: 3087, PPID: 3056, State: "S", Command: "/opt/homebrew/bin/node /repo/node_modules/.bin/nx serve webportal --host=0.0.0.0"},
		{PID: 42094, PPID: 55235, State: "S+", Command: "caffeinate -i -t 300"},
		{PID: 42609, PPID: 42569, State: "S", Command: "/Users/zak/.docker/cli-plugins/docker-buildx bake --file - --progress rawjson"},
	}

	got := filterUserTasks(tasks)
	sort.Slice(got, func(i, j int) bool { return got[i].PID < got[j].PID })
	want := []Task{
		{PID: 3087, PPID: 3056, State: "S", Command: "/opt/homebrew/bin/node /repo/node_modules/.bin/nx serve webportal --host=0.0.0.0"},
		{PID: 42091, PPID: 42080, State: "S", Command: "/Applications/Xcode.app/Contents/Developer/usr/bin/make integration-test-backend"},
		{PID: 89262, PPID: 89236, State: "S", Command: "/opt/homebrew/bin/node /repo/node_modules/.bin/nx serve backend"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filterUserTasks orchestrator mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestFilterUserTasksSkipsShellWrapperWhenNoBetterSignal(t *testing.T) {
	tasks := []Task{
		{PID: 10, PPID: 1, State: "S", Command: "/bin/zsh -c sleep 300"},
		{PID: 11, PPID: 10, State: "S", Command: "sleep 300"},
	}

	got := filterUserTasks(tasks)
	want := []Task{
		{PID: 11, PPID: 10, State: "S", Command: "sleep 300"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filterUserTasks wrapper mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

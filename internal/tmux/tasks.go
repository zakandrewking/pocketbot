package tmux

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Task represents a descendant process running inside a session pane.
type Task struct {
	PID     int
	PPID    int
	State   string
	Command string
}

type processInfo struct {
	pid     int
	ppid    int
	state   string
	command string
}

// SessionTasks returns descendant processes for all panes in a tmux session.
func SessionTasks(sessionName string) ([]Task, error) {
	pids, err := panePIDs(sessionName)
	if err != nil {
		return nil, err
	}
	if len(pids) == 0 {
		return nil, nil
	}

	processes, err := listProcesses()
	if err != nil {
		return nil, err
	}
	return collectDescendantTasks(pids, processes), nil
}

// SessionUserTasks returns a filtered task list intended to represent user work
// instead of agent/editor helper processes.
func SessionUserTasks(sessionName string) ([]Task, error) {
	tasks, err := SessionTasks(sessionName)
	if err != nil {
		return nil, err
	}
	return filterUserTasks(tasks), nil
}

func panePIDs(sessionName string) ([]int, error) {
	out, err := cmd("list-panes", "-t", sessionName, "-F", "#{pane_pid}").Output()
	if err != nil {
		return nil, err
	}
	return parsePIDs(string(out))
}

func listProcesses() (map[int]processInfo, error) {
	out, err := exec.Command("ps", "-axo", "pid=,ppid=,stat=,command=").Output()
	if err != nil {
		return nil, err
	}
	return parseProcessSnapshot(string(out))
}

func parsePIDs(raw string) ([]int, error) {
	var out []int
	seen := make(map[int]bool)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pid, err := strconv.Atoi(line)
		if err != nil {
			return nil, fmt.Errorf("parse pane pid %q: %w", line, err)
		}
		if seen[pid] {
			continue
		}
		seen[pid] = true
		out = append(out, pid)
	}
	return out, nil
}

func parseProcessSnapshot(raw string) (map[int]processInfo, error) {
	processes := make(map[int]processInfo)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 4 {
			return nil, fmt.Errorf("unexpected ps row format: %q", line)
		}
		pid, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, fmt.Errorf("parse pid from %q: %w", line, err)
		}
		ppid, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("parse ppid from %q: %w", line, err)
		}
		processes[pid] = processInfo{
			pid:     pid,
			ppid:    ppid,
			state:   parts[2],
			command: strings.Join(parts[3:], " "),
		}
	}
	return processes, nil
}

func collectDescendantTasks(rootPIDs []int, processes map[int]processInfo) []Task {
	roots := make(map[int]bool, len(rootPIDs))
	for _, pid := range rootPIDs {
		roots[pid] = true
	}

	children := make(map[int][]processInfo)
	for _, p := range processes {
		children[p.ppid] = append(children[p.ppid], p)
	}

	seen := make(map[int]bool)
	queue := append([]int{}, rootPIDs...)
	var tasks []Task
	for len(queue) > 0 {
		parent := queue[0]
		queue = queue[1:]
		for _, child := range children[parent] {
			if seen[child.pid] {
				continue
			}
			seen[child.pid] = true
			queue = append(queue, child.pid)

			if roots[child.pid] {
				continue
			}
			tasks = append(tasks, Task{
				PID:     child.pid,
				PPID:    child.ppid,
				State:   child.state,
				Command: child.command,
			})
		}
	}

	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].PID < tasks[j].PID
	})
	return tasks
}

func filterUserTasks(tasks []Task) []Task {
	if len(tasks) == 0 {
		return nil
	}

	byPID := make(map[int]Task, len(tasks))
	children := make(map[int][]Task, len(tasks))
	for _, t := range tasks {
		byPID[t.PID] = t
		children[t.PPID] = append(children[t.PPID], t)
	}

	roots := make([]Task, 0)
	for _, t := range tasks {
		if _, ok := byPID[t.PPID]; !ok {
			roots = append(roots, t)
		}
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i].PID < roots[j].PID })

	selected := make(map[int]bool)
	out := make([]Task, 0, len(roots))
	for _, root := range roots {
		reps := collectRepresentatives(root, children)
		for _, rep := range reps {
			if selected[rep.PID] {
				continue
			}
			selected[rep.PID] = true
			out = append(out, rep)
		}
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].PID < out[j].PID
	})
	return out
}

func collectRepresentatives(root Task, children map[int][]Task) []Task {
	// Roots with multiple children usually represent independent branches.
	// Split by direct child so parallel tasks are preserved.
	kids := children[root.PID]
	if len(kids) > 1 || isShellWrapper(root.Command) {
		var reps []Task
		for _, child := range kids {
			rep, ok := chooseRepresentative(child, children)
			if !ok {
				continue
			}
			reps = append(reps, rep)
		}
		if len(reps) > 0 {
			return reps
		}
	}

	rep, ok := chooseRepresentative(root, children)
	if !ok {
		return nil
	}
	return []Task{rep}
}

type taskNode struct {
	task  Task
	depth int
}

func chooseRepresentative(root Task, children map[int][]Task) (Task, bool) {
	queue := []taskNode{{task: root, depth: 0}}
	bestScore := -1
	bestDepth := 1 << 20
	var best Task

	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]

		score := taskScore(node.task.Command)
		if score > bestScore ||
			(score == bestScore && isShellWrapper(best.Command) && !isShellWrapper(node.task.Command)) ||
			(score == bestScore && node.depth < bestDepth) {
			bestScore = score
			bestDepth = node.depth
			best = node.task
		}

		for _, child := range children[node.task.PID] {
			queue = append(queue, taskNode{task: child, depth: node.depth + 1})
		}
	}
	if bestScore < 0 {
		return Task{}, false
	}
	return best, true
}

func taskScore(command string) int {
	if isNoiseCommand(command) {
		return -1
	}
	cmd := strings.TrimSpace(strings.ToLower(command))
	words := strings.Fields(cmd)
	if len(words) == 0 {
		return -1
	}

	// Strongly prefer explicit user orchestrators.
	if filepath.Base(words[0]) == "make" {
		return 100
	}
	if strings.Contains(cmd, "/.bin/nx serve ") {
		return 98
	}
	if strings.Contains(cmd, " nx serve ") {
		return 95
	}
	if strings.Contains(cmd, "npm exec nx serve") || strings.Contains(cmd, "npx nx serve") {
		return 90
	}
	if strings.Contains(cmd, "npm exec") {
		return 60
	}
	if isShellWrapper(command) {
		return 10
	}
	return 50
}

func isNoiseCommand(command string) bool {
	cmd := strings.TrimSpace(strings.ToLower(command))
	if cmd == "" {
		return true
	}

	words := strings.Fields(cmd)
	if len(words) == 0 {
		return true
	}
	bin := filepath.Base(words[0])

	// Agent runtimes and helpers are not user-level tasks.
	switch bin {
	case "claude", "codex", "agent":
		return true
	case "gopls", "caffeinate":
		return true
	}
	if strings.Contains(cmd, " pb.test ") || strings.Contains(cmd, "/pb.test ") {
		return true
	}
	if strings.Contains(cmd, " tmux.test ") || strings.Contains(cmd, "/tmux.test ") {
		return true
	}
	if strings.HasPrefix(cmd, "ps -axo ") || strings.Contains(cmd, " ps -axo ") {
		return true
	}
	if strings.Contains(cmd, "go run ./cmd/pb tasks") {
		return true
	}
	if strings.Contains(cmd, "/exe/pb tasks") {
		return true
	}
	if strings.Contains(cmd, "go test ./internal/tmux ./cmd/pb") ||
		strings.Contains(cmd, "go test ./cmd/pb ./internal/tmux") {
		return true
	}
	if strings.Contains(cmd, "gopls ** telemetry **") {
		return true
	}
	// Common build/watch helper workers that are usually noise in task views.
	if strings.Contains(cmd, "fork-ts-checker-webpack-plugin") {
		return true
	}
	if strings.Contains(cmd, "nx/src/daemon/server/start.js") {
		return true
	}
	if strings.Contains(cmd, "@esbuild/") && strings.Contains(cmd, "--service=") {
		return true
	}
	if strings.Contains(cmd, "docker-buildx") && strings.Contains(cmd, " bake ") {
		return true
	}
	if strings.Contains(cmd, "docker-compose compose up") {
		return true
	}
	if strings.Contains(cmd, "worker.js") || strings.Contains(cmd, "/worker/") {
		return true
	}
	if strings.Contains(cmd, "--inspect=localhost:") {
		return true
	}

	return false
}

func isShellWrapper(command string) bool {
	cmd := strings.TrimSpace(strings.ToLower(command))
	words := strings.Fields(cmd)
	if len(words) == 0 {
		return false
	}
	switch filepath.Base(words[0]) {
	case "sh", "bash", "zsh", "fish":
		return true
	}
	return false
}

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

	children := make(map[int]int)
	for _, t := range tasks {
		children[t.PPID]++
	}

	var leaf []Task
	for _, t := range tasks {
		if children[t.PID] == 0 {
			leaf = append(leaf, t)
		}
	}

	var filtered []Task
	for _, t := range leaf {
		if isInfrastructureCommand(t.Command) {
			continue
		}
		filtered = append(filtered, t)
	}
	if len(filtered) > 0 {
		return filtered
	}
	return leaf
}

func isInfrastructureCommand(command string) bool {
	cmd := strings.TrimSpace(strings.ToLower(command))
	if cmd == "" {
		return true
	}

	words := strings.Fields(cmd)
	if len(words) == 0 {
		return true
	}
	bin := filepath.Base(words[0])

	// Shell wrappers and agent runtimes are typically not the "task".
	switch bin {
	case "sh", "bash", "zsh", "fish", "claude", "codex", "agent":
		return true
	case "gopls":
		return true
	}
	if strings.Contains(cmd, "gopls ** telemetry **") {
		return true
	}

	return false
}

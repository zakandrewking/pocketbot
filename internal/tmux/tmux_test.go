package tmux

import (
	"testing"
	"time"
)

func TestNextActivityPollInterval(t *testing.T) {
	tests := []struct {
		name    string
		idleFor time.Duration
		want    time.Duration
	}{
		{
			name:    "active session polls quickly",
			idleFor: 2 * time.Second,
			want:    1 * time.Second,
		},
		{
			name:    "recently idle polls moderately",
			idleFor: 10 * time.Second,
			want:    2 * time.Second,
		},
		{
			name:    "idle polls slower",
			idleFor: 45 * time.Second,
			want:    5 * time.Second,
		},
		{
			name:    "long idle polls slowest",
			idleFor: 3 * time.Minute,
			want:    10 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nextActivityPollInterval(tt.idleFor)
			if got != tt.want {
				t.Fatalf("nextActivityPollInterval(%v)=%v, want %v", tt.idleFor, got, tt.want)
			}
		})
	}
}

func TestDetachOverlayMessage(t *testing.T) {
	tests := []struct {
		name  string
		level int
		want  string
	}{
		{
			name:  "top level",
			level: 0,
			want:  "Ctrl+D to detach",
		},
		{
			name:  "nested level",
			level: 2,
			want:  "Ctrl+D to detach (pb level 2)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detachOverlayMessage(tt.level)
			if got != tt.want {
				t.Fatalf("detachOverlayMessage(%d)=%q, want %q", tt.level, got, tt.want)
			}
		})
	}
}

func TestDetachPopupWidth(t *testing.T) {
	if got := detachPopupWidth("x"); got != 24 {
		t.Fatalf("detachPopupWidth short = %d, want 24", got)
	}
	msg := "Ctrl+D to detach"
	if got := detachPopupWidth(msg); got != 24 {
		t.Fatalf("detachPopupWidth normal = %d, want 24", got)
	}
	long := "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
	if got := detachPopupWidth(long); got != 96 {
		t.Fatalf("detachPopupWidth long = %d, want 96", got)
	}
}

func TestShellSingleQuote(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "", want: "''"},
		{in: "abc", want: "'abc'"},
		{in: "a'b", want: `'a'"'"'b'`},
	}

	for _, tt := range tests {
		if got := shellSingleQuote(tt.in); got != tt.want {
			t.Fatalf("shellSingleQuote(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

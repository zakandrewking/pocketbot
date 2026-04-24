package tmux

import (
	"reflect"
	"testing"
	"time"
)

func TestWithoutEnvRemovesOnlyRequestedKey(t *testing.T) {
	env := []string{
		"PATH=/usr/bin",
		"TMUX=/private/tmp/tmux-501/default,123,0",
		"TMUX_PANE=%1",
		"PB_LEVEL=1",
	}

	got := withoutEnv(env, "TMUX")
	want := []string{
		"PATH=/usr/bin",
		"TMUX_PANE=%1",
		"PB_LEVEL=1",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("withoutEnv()=%v, want %v", got, want)
	}
}

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

package tmux

import (
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"
)

func requireIntegrationEnv(t *testing.T) {
	t.Helper()
	if os.Getenv("PB_INTEGRATION") != "1" {
		t.Skip("set PB_INTEGRATION=1 to run tmux integration tests")
	}
	if !Available() {
		t.Skip("tmux is not available")
	}
}

func useIsolatedSocket(t *testing.T) {
	t.Helper()
	level := strconv.FormatInt(time.Now().UnixNano()%1_000_000_000, 10)
	t.Setenv("PB_LEVEL", level)
}

func waitForConsecutiveState(s *Session, want bool, consecutive int, timeout, interval time.Duration) (time.Time, bool) {
	deadline := time.Now().Add(timeout)
	count := 0
	for time.Now().Before(deadline) {
		got := s.UpdateActivity()
		if got == want {
			count++
			if count >= consecutive {
				return time.Now(), true
			}
		} else {
			count = 0
		}
		time.Sleep(interval)
	}
	return time.Time{}, false
}

func TestIntegrationQuietSessionStaysIdle(t *testing.T) {
	requireIntegrationEnv(t)
	useIsolatedSocket(t)
	defer KillServer()

	name := fmt.Sprintf("itest-idle-%d", time.Now().UnixNano())
	if err := CreateSession(name, "sleep 20"); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	s := NewSession(name, "sleep 20")

	// Prime baseline capture.
	for i := 0; i < 6; i++ {
		s.UpdateActivity()
		time.Sleep(200 * time.Millisecond)
	}

	activeSamples := 0
	totalSamples := 0
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		if s.UpdateActivity() {
			activeSamples++
		}
		totalSamples++
		time.Sleep(250 * time.Millisecond)
	}

	if activeSamples > 1 {
		t.Fatalf("quiet session had too many active samples: %d/%d", activeSamples, totalSamples)
	}
}

func TestIntegrationBurstTransitionsResponsive(t *testing.T) {
	requireIntegrationEnv(t)
	useIsolatedSocket(t)
	defer KillServer()

	name := fmt.Sprintf("itest-burst-%d", time.Now().UnixNano())
	// ~3 seconds of output, then quiet long enough to observe idle transition.
	command := "i=0; while [ $i -lt 12 ]; do echo tick-$i; i=$((i+1)); sleep 0.25; done; sleep 20"
	if err := CreateSession(name, command); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	s := NewSession(name, command)
	start := time.Now()

	activeAt, ok := waitForConsecutiveState(s, true, 2, 4*time.Second, 100*time.Millisecond)
	if !ok {
		t.Fatal("did not detect active state within 4s during burst output")
	}

	activeLatency := activeAt.Sub(start)
	if activeLatency > 2500*time.Millisecond {
		t.Fatalf("active detection too slow: %v", activeLatency)
	}
	t.Logf("active latency: %v", activeLatency)

	idleAt, ok := waitForConsecutiveState(s, false, 3, 12*time.Second, 200*time.Millisecond)
	if !ok {
		t.Fatal("did not return to idle state within 12s after burst")
	}

	// Burst emits for ~3s.
	idleLatencyFromBurstEnd := idleAt.Sub(start.Add(3 * time.Second))
	maxIdleLatency := IdleTimeout + 3*time.Second
	if idleLatencyFromBurstEnd > maxIdleLatency {
		t.Fatalf("idle detection too slow: %v (limit %v)", idleLatencyFromBurstEnd, maxIdleLatency)
	}
	t.Logf("idle latency from burst end: %v", idleLatencyFromBurstEnd)
}

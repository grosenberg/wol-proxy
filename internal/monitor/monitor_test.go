package monitor

import (
	"context"
	"testing"
	"time"

	"github.com/grosenberg/wol-proxy/internal/config"
	prober "github.com/prometheus-community/pro-bing"
)

func TestStateString(t *testing.T) {
	tests := []struct {
		state    State
		expected string
	}{
		{StateAwake, "Awake"},
		{StateAsleep, "Asleep"},
		{StateUnknown, "Unknown"},
		{State(99), "Unknown"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.expected {
			t.Errorf("State(%d).String() = %q, want %q", tt.state, got, tt.expected)
		}
	}
}

func TestNewMonitor(t *testing.T) {
	cfg := config.DefaultConfig()
	m := NewMonitor(cfg)

	if m == nil {
		t.Fatal("NewMonitor returned nil")
	}

	curr := m.GetCurrent()
	if curr.State != StateUnknown {
		t.Errorf("Initial current state = %v, want %v", curr.State, StateUnknown)
	}

	if curr.Target != cfg.ServerIP {
		t.Errorf("Initial target = %s, want %s", curr.Target, cfg.ServerIP)
	}

	prior := m.GetPrior()
	if prior.State != StateUnknown {
		t.Errorf("Initial prior state = %v, want %v", prior.State, StateUnknown)
	}

	if m.IsRemoteAwake() {
		t.Error("Expected IsRemoteAwake() to be false initially")
	}

	if m.IsRemoteAsleep() {
		t.Error("Expected IsRemoteAsleep() to be false initially")
	}

	if m.RemoteStateChanged() {
		t.Error("Expected RemoteStateChanged() to be false initially")
	}
}

func TestMonitor_Update(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.PingLogStatusChange = true
	m := NewMonitor(cfg)

	stats := &prober.Statistics{
		PacketsSent: 3,
		PacketsRecv: 3,
		PacketLoss:  0,
		AvgRtt:      15 * time.Millisecond,
	}

	// Transition 1: Unknown -> Awake
	m.update(StateAwake, "Awake now", stats)

	if !m.IsRemoteAwake() {
		t.Error("Expected IsRemoteAwake() to be true")
	}
	if m.IsRemoteAsleep() {
		t.Error("Expected IsRemoteAsleep() to be false")
	}
	if !m.RemoteStateChanged() {
		t.Error("Expected RemoteStateChanged() to be true (Unknown -> Awake)")
	}

	curr := m.GetCurrent()
	if curr.State != StateAwake || curr.PacketsRecv != 3 || curr.AvgRTT != 15*time.Millisecond {
		t.Errorf("GetCurrent() returned unexpected values: %+v", curr)
	}

	prior := m.GetPrior()
	if prior.State != StateUnknown {
		t.Errorf("GetPrior() state = %v, want %v", prior.State, StateUnknown)
	}

	// Transition 2: Awake -> Asleep
	asleepStats := &prober.Statistics{
		PacketsSent: 3,
		PacketsRecv: 0,
		PacketLoss:  100.0,
	}
	m.update(StateAsleep, "Host offline", asleepStats)

	if m.IsRemoteAwake() {
		t.Error("Expected IsRemoteAwake() to be false")
	}
	if !m.IsRemoteAsleep() {
		t.Error("Expected IsRemoteAsleep() to be true")
	}
	if !m.RemoteStateChanged() {
		t.Error("Expected RemoteStateChanged() to be true (Awake -> Asleep)")
	}

	curr = m.GetCurrent()
	if curr.State != StateAsleep || curr.PacketLoss != 100.0 {
		t.Errorf("GetCurrent() after sleep transition = %+v", curr)
	}

	prior = m.GetPrior()
	if prior.State != StateAwake {
		t.Errorf("GetPrior() state = %v, want %v", prior.State, StateAwake)
	}
}

func TestMonitor_Ping(t *testing.T) {
	t.Run("Loopback reachable", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.ServerIP = "127.0.0.1"
		// cfg.PingSetTimeout = 1 * time.Second
		// cfg.PingRetryInterval = 100 * time.Millisecond
		// cfg.PingSetCount = 1

		m := NewMonitor(cfg)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		awake := m.Ping(ctx)
		curr := m.GetCurrent()

		if awake {
			if !m.IsRemoteAwake() {
				t.Error("Expected IsRemoteAwake() to be true")
			}
			if m.IsRemoteAsleep() {
				t.Error("Expected IsRemoteAsleep() to be false")
			}
			if curr.State != StateAwake {
				t.Errorf("Current state = %v, want %v", curr.State, StateAwake)
			}
			if curr.PacketsRecv == 0 {
				t.Errorf("Expected PacketsRecv > 0, got %d", curr.PacketsRecv)
			}
		} else {
			// On systems where ICMP requires elevated privileges and fails to initialize/send
			t.Logf("Ping to loopback returned awake=false (may require elevated privileges): %s", curr.Explain)
		}
	})

	t.Run("Unreachable host returns asleep", func(t *testing.T) {
		cfg := config.DefaultConfig()
		// 192.0.2.1 is TEST-NET-1 (RFC 5737), guaranteed to not respond
		cfg.ServerIP = "192.0.2.1"
		// cfg.PingSetTimeout = 300 * time.Millisecond
		// cfg.PingRetryInterval = 50 * time.Millisecond
		// cfg.PingSetCount = 1

		m := NewMonitor(cfg)
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		awake := m.Ping(ctx)
		if awake {
			t.Error("Expected Ping to unreachable host to return false")
		}
		if !m.IsRemoteAsleep() {
			t.Errorf("Expected IsRemoteAsleep() to be true, got state %v", m.GetCurrent().State)
		}
		if m.IsRemoteAwake() {
			t.Error("Expected IsRemoteAwake() to be false")
		}
		curr := m.GetCurrent()
		if curr.State != StateAsleep {
			t.Errorf("Current state = %v, want %v", curr.State, StateAsleep)
		}
	})

	t.Run("Canceled context returns asleep or failure", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.ServerIP = "192.0.2.1"
		// cfg.PingSetTimeout = 5 * time.Second
		// cfg.PingRetryInterval = 500 * time.Millisecond
		// cfg.PingSetCount = 3

		m := NewMonitor(cfg)
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel immediately

		awake := m.Ping(ctx)
		if awake {
			t.Error("Expected Ping with canceled context to return false")
		}
		if m.IsRemoteAwake() {
			t.Error("Expected IsRemoteAwake() to be false")
		}
	})

	t.Run("Invalid target sets state", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.ServerIP = "invalid:::target::ip"
		// cfg.PingSetTimeout = 100 * time.Millisecond
		// cfg.PingRetryInterval = 50 * time.Millisecond
		// cfg.PingSetCount = 1

		m := NewMonitor(cfg)
		ctx := context.Background()

		awake := m.Ping(ctx)
		if awake {
			t.Error("Expected Ping with invalid target to return false")
		}
		curr := m.GetCurrent()
		if curr.State != StateUnknown && curr.State != StateAsleep {
			t.Errorf("Expected state to be StateUnknown or StateAsleep, got %v", curr.State)
		}
	})
}

func TestPercentLoss(t *testing.T) {
	tests := []struct {
		recv     int
		sent     int
		expected string
	}{
		{0, 0, "0.00%"},
		{5, 0, "0.00%"},
		{5, 5, "0.00%"},
		{2, 4, "50.00%"},
		{0, 3, "100.00%"},
		{1, 3, "66.67%"},
	}

	for _, tt := range tests {
		if got := percentLoss(tt.recv, tt.sent); got != tt.expected {
			t.Errorf("percentLoss(%d, %d) = %q, want %q", tt.recv, tt.sent, got, tt.expected)
		}
	}
}

func TestMonitor_LogCurrentState(t *testing.T) {
	cfg := config.DefaultConfig()
	m := NewMonitor(cfg)

	// Should not panic across different states
	m.LogCurrentState(-4) // Debug level / any slog level
	m.update(StateAwake, "Awake", nil)
	m.LogCurrentState(0) // Info level
	m.update(StateAsleep, "Asleep", nil)
	m.LogCurrentState(4) // Warn level
}

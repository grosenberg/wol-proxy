// Package monitor provides a functionally one-shot ping-based
// detector of the state of a remote computer system.
// Retains statistical records of the most current and
// last different states.
//
// Alternate library is "github.com/aanantaco/ping"
package monitor

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/grosenberg/wol-proxy/internal/config"
	prober "github.com/prometheus-community/pro-bing"
)

// State represents the result remote system state
type State int

const (
	StateAwake   State = iota // System is reachable
	StateAsleep               // System unreachable
	StateUnknown              // Initial/unknown state
)

// Result holds the details of a ping check
type Result struct {
	Timestamp   time.Time
	Target      string
	State       State
	PacketLoss  float64
	PacketsSent int
	PacketsRecv int
	AvgRTT      time.Duration
	Explain     string
}

// Monitor manages the monitoring state
type Monitor struct {
	cfg     *config.Config
	current *Result
	prior   *Result
	ctx     context.Context
	mutex   sync.RWMutex
}

// NewMonitor creates a new monitoring instance with the given configuration
func NewMonitor(cfg *config.Config) *Monitor {
	return &Monitor{
		cfg:     cfg,
		current: mkResult(cfg),
		prior:   mkResult(cfg),
	}
}

// Ping executes a single, on demand, ping check (no internal retries).
// Returns 'true' if the ping response was received.
func (m *Monitor) Ping(ctx context.Context) bool {
	var stats *prober.Statistics

	pinger, err := prober.NewPinger(m.current.Target)
	if err != nil {
		m.update(StateUnknown, fmt.Sprintf("Pinger initialization failed: %v", err), stats)
		return false
	}

	pinger.Timeout = 2 * time.Second  // total pinging time before exit
	pinger.Interval = 1 * time.Second // time between ping sends
	pinger.Count = 1                  // count of pings before exit; -1 for unbounded
	pinger.OnFinish = func(s *prober.Statistics) {
		stats = s
	}

	pinger.SetPrivileged(true)
	err = pinger.RunWithContext(ctx)
	m.updateResults(err, stats)
	return m.IsRemoteAwake()
}

func (m *Monitor) updateResults(err error, stats *prober.Statistics) {
	state := StateAwake
	explain := "Ping success"

	if err != nil || stats == nil || stats.PacketsRecv == 0 {
		explain = "No ping response received"
		if err != nil {
			explain = fmt.Sprintf("Ping failed: %v", err)
		}
		state = StateAsleep
	}

	m.update(state, explain, stats)
}

// if parameter state is different than current, push current to prior and update current
// if parameter state is the same as current, update current
func (m *Monitor) update(state State, explain string, stats *prober.Statistics) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// push current to prior
	if state != m.current.State {
		m.prior = m.current
	}

	// update current
	m.current = mkResult(m.cfg)
	m.current.State = state
	m.current.Explain = explain
	if stats != nil {
		m.current.PacketsSent = stats.PacketsSent
		m.current.PacketsRecv = stats.PacketsRecv
		m.current.PacketLoss = stats.PacketLoss // percentage loss
		m.current.AvgRTT = stats.AvgRtt         // round trip time
	}

	// // special case; init on first ping
	// if m.prior.State == StateUnknown {
	// 	m.prior = m.current
	// }

	if m.cfg.PingLogStatusChange && m.prior.State != m.current.State {
		m.logStateChange()
	}
}

func mkResult(cfg *config.Config) *Result {
	return &Result{
		Timestamp: time.Now(),
		Target:    cfg.ServerIP,
		State:     StateUnknown,
	}
}

// GetCurrent returns the current Result record (thread-safe)
func (m *Monitor) GetCurrent() Result {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return *m.current // dereference to make durable
}

// GetPrior returns the prior Result record (thread-safe)
func (m *Monitor) GetPrior() Result {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return *m.prior // dereference to make durable
}

func (m *Monitor) IsRemoteAwake() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.current.State == StateAwake
}

func (m *Monitor) IsRemoteAsleep() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.current.State == StateAsleep
}

// RemoteStateChanged checks whether the remote server state
// changed relative to that of the previous Ping execution
func (m *Monitor) RemoteStateChanged() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.prior.State != m.current.State
}

// String returns a string representation of status
func (s State) String() string {
	return toString(s)
}

// toString converts State to string
func toString(s State) string {
	switch s {
	case StateAwake:
		return "Awake"
	case StateAsleep:
		return "Asleep"
	default:
		return "Unknown"
	}
}

// logStateChange logs when the remote system state changes
func (m *Monitor) logStateChange() {
	if m.cfg.PingLogStatusChange && m.prior.State != m.current.State {
		args := []slog.Attr{
			slog.String("Ip", m.current.Target),
			slog.String("Now", m.current.State.String()),
			slog.String("Was", m.prior.State.String()),
			slog.Int("Packets sent", m.current.PacketsSent),
			slog.Int("Packets received", m.current.PacketsRecv),
			slog.Float64("Packet loss %", m.current.PacketLoss),
			slog.Duration("Ave RTT", m.current.AvgRTT),
		}

		// Choose log level based on state
		var lvl slog.Level
		if m.current.State == StateAwake {
			lvl = slog.LevelInfo
		} else {
			lvl = slog.LevelWarn
		}
		slog.LogAttrs(context.Background(), lvl, m.current.Explain, args...)
	}
}

func (m *Monitor) LogCurrentState(lvl slog.Level) {
	res := m.GetCurrent()
	loss := percentLoss(res.PacketsRecv, res.PacketsSent)
	args := []slog.Attr{
		slog.String("State", res.State.String()),
		slog.String("Ip", res.Target),
		slog.String("Loss", loss),
	}
	slog.LogAttrs(context.Background(), lvl, res.Explain, args...)
}

// percentLoss returns the packet loss formated as "x.xx%".
func percentLoss(received, sent int) string {
	if sent <= 0 {
		return "0.00%"
	}

	lost := sent - received
	val := (float64(lost) / float64(sent)) * 100
	return fmt.Sprintf("%.2f%%", val)
}

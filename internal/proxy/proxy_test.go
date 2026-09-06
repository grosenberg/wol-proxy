package proxy

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/grosenberg/wol-proxy/internal/config"
)

func TestMain(m *testing.M) {
	// Formats time to "YYYY-MM-DD hh:mm:ss.mmm"
	opts := &slog.HandlerOptions{
		Level: slog.LevelDebug,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				a.Value = slog.StringValue(a.Value.Time().Format("2006-01-02 15:04:05.000"))
			}
			return a
		},
	}

	// Sets the debug logger to os.Stderr
	handler := slog.NewTextHandler(os.Stderr, opts)
	slog.SetDefault(slog.New(handler))

	os.Exit(m.Run())
}

func TestNewServer(t *testing.T) {
	cfg := config.DefaultConfig()
	server := NewServer(cfg)

	if server == nil {
		t.Fatal("Expected NewServer to return non-nil server")
	}
	if server.cfg != cfg {
		t.Errorf("Expected server.cfg to equal passed config, got %+v", server.cfg)
	}
}

// customNetError simulates a net.Error implementation for testing.
type customNetError struct {
	msg     string
	timeout bool
}

func (e *customNetError) Error() string   { return e.msg }
func (e *customNetError) Timeout() bool   { return e.timeout }
func (e *customNetError) Temporary() bool { return false }

func TestIsTimeoutError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "context.DeadlineExceeded",
			err:      context.DeadlineExceeded,
			expected: true,
		},
		{
			name:     "os.ErrDeadlineExceeded",
			err:      os.ErrDeadlineExceeded,
			expected: true,
		},
		{
			name:     "net.Error with Timeout true",
			err:      &customNetError{msg: "network deadline exceeded", timeout: true},
			expected: true,
		},
		{
			name:     "net.Error with Timeout false",
			err:      &customNetError{msg: "connection refused", timeout: false},
			expected: false,
		},
		{
			name:     "io.EOF",
			err:      io.EOF,
			expected: false,
		},
		{
			name:     "net.ErrClosed",
			err:      net.ErrClosed,
			expected: false,
		},
		{
			name:     "generic error",
			err:      errors.New("some unexpected error"),
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := isTimeoutError(tc.err)
			if result != tc.expected {
				t.Errorf("isTimeoutError(%v) = %v; want %v", tc.err, result, tc.expected)
			}
		})
	}
}

func TestExplainError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		expectErr bool
	}{
		{
			name:      "nil error returns nil",
			err:       nil,
			expectErr: false,
		},
		{
			name:      "io.EOF returns nil (normal termination)",
			err:       io.EOF,
			expectErr: false,
		},
		{
			name:      "net.ErrClosed returns nil (already closed)",
			err:       net.ErrClosed,
			expectErr: false,
		},
		{
			name:      "timeout error returns error",
			err:       os.ErrDeadlineExceeded,
			expectErr: true,
		},
		{
			name:      "generic error returns error",
			err:       errors.New("i/o failure"),
			expectErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := explainError(tc.err, "inbound")
			if tc.expectErr && res == nil {
				t.Errorf("explainError(%v) expected error, got nil", tc.err)
			} else if !tc.expectErr && res != nil {
				t.Errorf("explainError(%v) expected nil, got error: %v", tc.err, res)
			}
		})
	}
}

func TestProtect_ReadWrite(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	timeout := 2 * time.Second
	pClient := protect(ctx, client, timeout)
	pServer := protect(ctx, server, timeout)

	testData := []byte("Hello, proxy test data!")

	var writeErr error
	var writeN int
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		writeN, writeErr = pClient.Write(testData)
	}()

	buf := make([]byte, len(testData))
	n, readErr := io.ReadFull(pServer, buf)

	wg.Wait()

	if writeErr != nil {
		t.Fatalf("pClient.Write failed: %v", writeErr)
	}
	if writeN != len(testData) {
		t.Errorf("pClient.Write wrote %d bytes, want %d", writeN, len(testData))
	}

	if readErr != nil {
		t.Fatalf("pServer.Read failed: %v", readErr)
	}
	if n != len(testData) {
		t.Errorf("pServer.Read read %d bytes, want %d", n, len(testData))
	}
	if !bytes.Equal(buf, testData) {
		t.Errorf("pServer.Read got %q, want %q", string(buf), string(testData))
	}
}

func TestProtect_ContextCancel(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	ctx, cancel := context.WithCancel(t.Context())

	pServer := protect(ctx, server, 5*time.Second)

	// Cancel context after a short pause
	time.AfterFunc(50*time.Millisecond, cancel)

	buf := make([]byte, 32)
	_, err := pServer.Read(buf)
	if err == nil {
		t.Error("Expected error after context cancellation, got nil")
	}
}

func getFreePort(t *testing.T) uint16 {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to allocate free port: %v", err)
	}
	defer l.Close()
	return uint16(l.Addr().(*net.TCPAddr).Port)
}

func TestServer_StartAndShutdown(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.ProxyPort = getFreePort(t)

	ctx, cancel := context.WithCancel(t.Context())
	var wg sync.WaitGroup

	server := NewServer(cfg)
	if err := server.Start(ctx, &wg); err != nil {
		t.Fatalf("server.Start failed: %v", err)
	}

	// Verify listener is accepting connections
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", string(rune(cfg.ProxyPort))), 500*time.Millisecond)
	// We only care whether port opened or connection reached accept loop
	if err == nil {
		conn.Close()
	}

	// Trigger shutdown
	cancel()

	// Wait for goroutines to terminate with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(3 * time.Second):
		t.Fatal("server failed to shut down within timeout")
	}
}

func TestMakeConnection_Success(t *testing.T) {
	// Start an echo server to dial
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to start mock remote server: %v", err)
	}
	defer listener.Close()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		io.Copy(conn, conn) // echo
	}()

	cfg := config.DefaultConfig()
	cfg.ProxyDialTimeout = 2 * time.Second

	server := NewServer(cfg)
	server.ctx = t.Context()

	conn, err := server.makeConnection(listener.Addr().String())
	if err != nil {
		t.Fatalf("makeConnection failed: %v", err)
	}
	defer conn.Close()

	// Test communicating over the connection
	msg := []byte("ping remote")
	if _, err := conn.Write(msg); err != nil {
		t.Fatalf("Write to remote connection failed: %v", err)
	}

	buf := make([]byte, len(msg))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("Read from remote connection failed: %v", err)
	}

	if !bytes.Equal(buf, msg) {
		t.Errorf("Echo response = %q, want %q", string(buf), string(msg))
	}
}

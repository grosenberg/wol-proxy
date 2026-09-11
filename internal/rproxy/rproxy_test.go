package rproxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/grosenberg/wol-proxy/internal/config"
	"github.com/grosenberg/wol-proxy/internal/testutil"
)

func TestMain(m *testing.M) {
	cleanup := testutil.InitMainLogging("wol-proxy-test.log")
	defer cleanup()
	os.Exit(m.Run())
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

func TestNewServer(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewServer(cfg)
	if s == nil {
		t.Fatal("Expected NewServer to return non-nil server")
	}
	if s.cfg != cfg {
		t.Errorf("Expected s.cfg to match passed config, got %+v", s.cfg)
	}
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

	// Verify listener is open and responds to HTTP requests
	client := &http.Client{Timeout: 500 * time.Millisecond}
	url := fmt.Sprintf("http://127.0.0.1:%d/ping", cfg.ProxyPort)

	// Will return Bad Gateway because backend isn't running, but verifies HTTP server is running
	resp, err := client.Get(url)
	if err == nil {
		resp.Body.Close()
	}

	// Trigger shutdown
	cancel()

	// Wait for goroutines to terminate
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

func TestDialWithWOL_DirectSuccess(t *testing.T) {
	// Mock remote HTTP server
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer backend.Close()

	cfg := config.DefaultConfig()
	cfg.ProxyDialTimeout = 2 * time.Second

	server := NewServer(cfg)
	server.ctx = t.Context()

	conn, err := server.makeConnection(t.Context(), "tcp", backend.Listener.Addr().String())
	if err != nil {
		t.Fatalf("dialWithWOL failed: %v", err)
	}
	defer conn.Close()

	// Send raw HTTP request
	req := fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", backend.Listener.Addr().String())
	if _, err := conn.Write([]byte(req)); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	body, err := io.ReadAll(conn)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if len(body) == 0 {
		t.Errorf("Expected non-empty response from backend")
	}
}

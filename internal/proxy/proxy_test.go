package proxy

import (
	"net"
	"testing"
	"time"

	"github.com/grosenberg/wol-proxy/internal/config"
)

func TestNewServer(t *testing.T) {
	cfg := config.DefaultConfig()
	server := NewServer(cfg)

	if server == nil {
		t.Fatal("NewServer() returned nil")
	}

	if server.config != cfg {
		t.Error("Server.config not set correctly")
	}

	if server.wol == nil {
		t.Error("Server.wol not initialized")
	}

	if server.pinger == nil {
		t.Error("Server.pinger not initialized")
	}
}

func TestServer_CheckServer(t *testing.T) {
	cfg := config.DefaultConfig()
	server := NewServer(cfg)

	// Test with unreachable IP
	cfg.ServerIP = "192.0.2.1" // Reserved for documentation, should not respond
	isAwake := server.checkServer()

	// This should be false since we're not running a real server
	if isAwake {
		t.Error("checkServer() with unreachable IP = true, want false")
	}
}

func TestServer_Forward(t *testing.T) {
	// Create two connected pipes to test forwarding
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	cfg := config.DefaultConfig()
	server := NewServer(cfg)

	testMessage := []byte("Hello, World!")

	// Write from client side
	go func() {
		_, err := clientConn.Write(testMessage)
		if err != nil {
			t.Errorf("Write failed: %v", err)
		}
		clientConn.Close()
	}()

	// Read from server side
	buf := make([]byte, 1024)
	n, err := serverConn.Read(buf)
	if err != nil {
		t.Errorf("Read failed: %v", err)
	}

	if string(buf[:n]) != string(testMessage) {
		t.Errorf("Received %q, want %q", string(buf[:n]), string(testMessage))
	}
}

func TestServer_WaitForServer(t *testing.T) {
	cfg := config.DefaultConfig()
	server := NewServer(cfg)

	// Set very short timeout for testing
	cfg.ServerInitialTimeout = 100 * time.Millisecond

	// Mock checkServer to return false quickly
	start := time.Now()
	// This will wait for timeout since server won't wake up
	woke := server.waitForServer()
	elapsed := time.Since(start)

	if woke {
		t.Error("waitForServer() with unreachable server = true, want false")
	}

	// Should have waited at least the timeout duration
	if elapsed < cfg.ServerInitialTimeout {
		t.Errorf("waitForServer() returned too quickly: %v, want at least %v",
			elapsed, cfg.ServerInitialTimeout)
	}
}

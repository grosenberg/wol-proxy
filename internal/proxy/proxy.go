package proxy

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/yourusername/go-wol-proxy/internal/config"
	"github.com/yourusername/go-wol-proxy/internal/netutil"
	"github.com/yourusername/go-wol-proxy/pkg/concurrency"
	"github.com/yourusername/go-wol-proxy/pkg/logger"
)

// Server represents the proxy server
type Server struct {
	config   *config.Config
	wol      *netutil.WOLSender
	pinger   *netutil.Pinger
	pool     *concurrency.Pool
	listener net.Listener
}

// NewServer creates a new proxy server
func NewServer(cfg *config.Config) *Server {
	return &Server{
		config: cfg,
		wol:    netutil.NewWOLSender(),
		pinger: netutil.NewPinger(cfg.PingTimeout),
		pool:   concurrency.NewPool(cfg.MaxConnections),
	}
}

// Run starts the proxy server
func (s *Server) Run() error {
	var err error
	s.listener, err = net.Listen("tcp", fmt.Sprintf(":%d", s.config.LocalProxyPort))
	if err != nil {
		return fmt.Errorf("failed to start listener: %v", err)
	}
	defer s.listener.Close()

	logger.Info("Proxy server started", "port", s.config.LocalProxyPort)

	return s.acceptConnections()
}

// acceptConnections handles incoming connections
func (s *Server) acceptConnections() error {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			logger.Error("Connection accept error", "error", err)
			continue
		}

		s.pool.Run(func() {
			s.handleConnection(conn)
		})
	}
}

// handleConnection manages a connection
func (s *Server) handleConnection(clientConn net.Conn) {
	defer clientConn.Close()

	remoteAddr := clientConn.RemoteAddr()
	logger.Info("New connection", "remote", remoteAddr)

	// Check if server is awake
	if !s.checkServer() {
		if !s.wakeServer() {
			return
		}
	}

	// Proxy the connection
	s.proxy(clientConn)
}

// checkServer checks if server is responsive
func (s *Server) checkServer() bool {
	return s.pinger.Ping(s.config.ServerIP)
}

// wakeServer attempts to wake the server via WOL
func (s *Server) wakeServer() bool {
	logger.Info("Attempting to wake server", "ip", s.config.ServerIP)

	for attempt := 1; attempt <= s.config.MaxWOLRetries; attempt++ {
		if err := s.wol.Send(s.config.ServerMAC, s.config.ServerIP, s.config.ServerMACPort); err == nil {
			logger.Info("WOL packet sent", "attempt", attempt)

			// Wait for server to boot
			if s.waitForServer() {
				return true
			}
		}

		logger.Warn("WOL attempt failed", "attempt", attempt)

		if attempt < s.config.MaxWOLRetries {
			time.Sleep(s.config.RetryInterval)
		}
	}

	logger.Error("Failed to wake server after all attempts")
	return false
}

// waitForServer waits for server to become responsive
func (s *Server) waitForServer() bool {
	deadline := time.Now().Add(s.config.ServerInitialTimeout)

	for time.Now().Before(deadline) {
		if s.checkServer() {
			logger.Info("Server woke up")
			return true
		}
		time.Sleep(1 * time.Second)
	}

	logger.Error("Server didn't respond within timeout")
	return false
}

// proxy forwards traffic between client and server
func (s *Server) proxy(clientConn net.Conn) {
	serverAddr := net.JoinHostPort(s.config.ServerIP, fmt.Sprintf("%d", s.config.LocalProxyPort))

	serverConn, err := net.DialTimeout("tcp", serverAddr, 10*time.Second)
	if err != nil {
		logger.Error("Failed to connect to server", "error", err)
		return
	}
	defer serverConn.Close()

	var wg sync.WaitGroup
	wg.Add(2)

	// Forward client -> server
	go func() {
		defer wg.Done()
		s.forward(clientConn, serverConn)
	}()

	// Forward server -> client
	go func() {
		defer wg.Done()
		s.forward(serverConn, clientConn)
	}()

	wg.Wait()
}

// forward copies data from src to dst
func (s *Server) forward(src, dst net.Conn) {
	buffer := make([]byte, 32*1024)

	for {
		n, err := src.Read(buffer)
		if n > 0 {
			dst.Write(buffer[:n])
		}
		if err != nil {
			break
		}
	}
}

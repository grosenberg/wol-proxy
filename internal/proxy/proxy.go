package proxy

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/grosenberg/wol-proxy/internal/config"
	"github.com/grosenberg/wol-proxy/internal/netutil"
	"golang.org/x/sync/errgroup"
)

// Server represents the proxy server
type Server struct {
	cfg    *config.Config
	wg     *sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc

	// local port packet listener
	listener net.Listener

	// Components
	wol *netutil.WOLSender
	// sysMon *monitor.Monitor
}

// NewServer creates a new proxy server
func NewServer(cfg *config.Config) *Server {
	return &Server{
		cfg: cfg,
	}
}

// thread #0: main thread (in "main.go")
// -- contains an ordinary function call to proxy.Start(...)
// -- waits for graceful shutdown
//
// thread #1: accept thread (in "proxy.go")
// -- accept go routine started from proxy.Start(...)
// -- loops on accepting local connection requests
// -- for each accept starts a connection handling thread
// -- accept thread lifetime is that of the main program
//
// thread #2: connection handling thread (in "proxy.go")
// -- composes a bidirectional proxy connection
// -- on remote connection dial error, attempts to wake remote server (stub the wake-up function call)
// -- on remote connection dial success, starts 2 unidirectional data transfer threads
// -- waits for shutdown of the data transfer threads using errorgroup.Wait()
// -- connection handling thread lifetime is that of its connection
//
// thread #3
// -- performs local <- remote data transfer copy
// -- terminates on connection close, context done, or transfer error
// -- thread lifetime is that of its unidirectional connection
//
// thread #4
// -- performs remote <- local data transfer copy
// -- terminates on connection close, context done, or transfer error
// -- thread lifetime is that of its unidirectional connection

// start the proxy server
func (s *Server) Start(ctx context.Context, wg *sync.WaitGroup) error {
	s.ctx = ctx
	s.wg = wg

	// Create and run the ping-based system sysMon
	// s.sysMon = monitor.NewMonitor(s.cfg)
	s.wol = netutil.NewWOLSender(s.cfg)

	var err error
	s.listener, err = net.Listen("tcp", fmt.Sprintf(":%d", s.cfg.ProxyPort))
	if err != nil {
		slog.Error("Failed to start listener", slog.Any("error", err))
		return err
	}
	slog.Info("Listener started", slog.Int("port", int(s.cfg.ProxyPort)))

	// to shutdown s.listener.Accept()
	cleanup := context.AfterFunc(ctx, func() {
		s.listener.Close()
	})

	wg.Go(func() {
		defer cleanup()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				slog.Debug("Proxy server waiting on Accept")
				localConn, err := s.listener.Accept()
				if err != nil {
					select {
					case <-ctx.Done(): // context cancelled: do cleanup
						return
					default:
						slog.Debug("Ignoring unknown listener accept error", slog.Any("Error", err))
						continue
					}
				}

				// Handle each connection in a separate, dedicated thread
				wg.Go(func() {
					s.handleConnection(localConn)
				})
			}
		}
	})

	slog.Info("Proxy server started", slog.Int("port", int(s.cfg.ProxyPort)))
	return nil
}

// handleConnection implements a single connection: starts two threads, each
// implementing a unidirectional data flow, and then waits for both threads
// to complete to perform connection cleanup
func (s *Server) handleConnection(localConn net.Conn) error {
	target := net.JoinHostPort(s.cfg.ServerIP, strconv.Itoa(int(s.cfg.ProxyPort)))
	slog.Info("Handle new connection", slog.String("server", target))

	srvrConn, err := s.makeConnection(target)
	if err != nil {
		slog.Warn("New server connection refused", slog.String("server", target))
		localConn.Close()
		return err
	}

	slog.Info("Server connection succeeded", slog.String("server", target))

	// Create a per-connection context derived from the server context
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	// use error wait group to block until both threads terminate
	eg, gctx := errgroup.WithContext(ctx)

	// inbound: localConn <-srvrConn
	eg.Go(func() error {
		_, err := io.Copy(
			protect(gctx, localConn, s.cfg.ProxyXferTimeout),
			protect(gctx, srvrConn, s.cfg.ProxyXferTimeout),
		)
		localConn.Close()
		return explainError(err, "inbound")
	})

	// outbound: srvrConn <- localConn
	eg.Go(func() error {
		_, err := io.Copy(
			protect(gctx, srvrConn, s.cfg.ProxyXferTimeout),
			protect(gctx, localConn, s.cfg.ProxyXferTimeout),
		)
		srvrConn.Close()
		return explainError(err, "outbound")
	})

	return eg.Wait()
}

// makeConnection attempts to create a connection to the remote server.
// On dial failure, executes WOL send.
// On send success, dial one last time.
// On send or dial failure, returns error.
func (s *Server) makeConnection(target string) (net.Conn, error) {
	dialer := net.Dialer{Timeout: s.cfg.ProxyDialTimeout}

	// attempt initial connection
	srvrConn, err := dialer.DialContext(s.ctx, "tcp", target)
	if err != nil {
		slog.Error("Failed dialing server (attempt 1)",
			slog.String("server", target),
			slog.Any("error", err),
		)

		// try waking server
		slog.Info("Remote is potentially asleep: attempting to wake...")
		if err := s.wol.Send(s.ctx); err != nil {
			slog.Error("Failed waking remote", slog.Any("error", err))
			return nil, err
		}

		// dial server one last time
		slog.Info("Retrying server connection...")
		srvrConn, err = dialer.DialContext(s.ctx, "tcp", target)
		if err != nil {
			slog.Error("Failed dialing server (attempt 2)",
				slog.String("server", target),
				slog.Any("error", err),
			)
			return nil, err
		}
	}

	return srvrConn, nil // success
}

// protect guards the read/write functions of the net.Conn parameter
func protect(ctx context.Context, conn net.Conn, timeout time.Duration) net.Conn {
	tc := &TimeoutConn{
		ctx:     ctx,
		Conn:    conn,
		Timeout: timeout,
	}

	// ensure immediate copy termination on context cancel
	context.AfterFunc(ctx, func() {
		conn.SetDeadline(time.Now())
	})

	return tc
}

// -------------------------
// ---- io.Copy wrapper ----

// represents a connection with timeout support
type TimeoutConn struct {
	net.Conn                 // embedded, resolves to the name Conn
	Timeout  time.Duration   // explicit, server read/write time limits
	ctx      context.Context // used to coordinate cancellation
}

func (tc *TimeoutConn) Read(b []byte) (n int, err error) {
	if tc.ctx.Err() != nil {
		tc.Conn.SetDeadline(time.Now())
		slog.Debug("Read connection cancelled", slog.Any("cause", tc.ctx.Err()))
		return 0, tc.ctx.Err()
	}

	tc.Conn.SetReadDeadline(time.Now().Add(tc.Timeout))

	// for debugging
	cnt, err := tc.Conn.Read(b) // use "tc.Conn.Read" to prevent recursion
	slog.Debug("Inbound data read",
		slog.Int("count", cnt),
		slog.Any("error", err),
	)
	return cnt, err

	// for production
	// return tc.Conn.Read(b) // use "tc.Conn.Read" to prevent recursion
}

func (tc *TimeoutConn) Write(b []byte) (n int, err error) {
	if tc.ctx.Err() != nil {
		tc.Conn.SetDeadline(time.Now())
		slog.Info("Write connection cancelled", slog.Any("cause", tc.ctx.Err()))
		return 0, tc.ctx.Err()
	}

	tc.Conn.SetWriteDeadline(time.Now().Add(tc.Timeout))

	// for debugging; has to be "tc.Conn.Write" to prevent recursion
	cnt, err := tc.Conn.Write(b)
	slog.Debug("Outbound data written",
		slog.Any("to", tc.Conn.RemoteAddr()),
		slog.Int("count", cnt),
		slog.Any("error", err),
	)
	return cnt, err

	// for production; has to be "tc.Conn.Write" to prevent recursion
	// return tc.Conn.Write(b)
}

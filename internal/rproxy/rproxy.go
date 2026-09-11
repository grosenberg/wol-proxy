package rproxy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"github.com/grosenberg/wol-proxy/internal/config"
	"github.com/grosenberg/wol-proxy/internal/netutil"
)

// Server represents the HTTP reverse proxy server
type Server struct {
	cfg        *config.Config
	wg         *sync.WaitGroup
	ctx        context.Context
	httpServer *http.Server

	// Components
	wol *netutil.WOLSender
}

// NewServer creates a new reverse proxy server
func NewServer(cfg *config.Config) *Server {
	return &Server{
		cfg: cfg,
	}
}

// Start starts the HTTP reverse proxy server
func (s *Server) Start(ctx context.Context, wg *sync.WaitGroup) error {
	s.ctx = ctx
	s.wg = wg
	s.wol = netutil.NewWOLSender(s.cfg)

	srvrAddr := fmt.Sprintf("http://%s:%d", s.cfg.ServerIP, s.cfg.ProxyPort)
	srvrURL, err := url.Parse(srvrAddr)
	if err != nil {
		slog.Error("Failed to parse target URL",
			slog.String("target", srvrAddr),
			slog.Any("error", err),
		)
		return err
	}

	// Custom transport with WOL-enabled dialing
	transport := &http.Transport{
		DialContext: func(dialCtx context.Context, network, addr string) (net.Conn, error) {
			return s.makeConnection(dialCtx, network, addr)
		},
		MaxIdleConns:          s.cfg.MaxIdleConnections,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	// Create reverse proxy targeting the remote server
	rp := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(srvrURL)
			r.SetXForwarded()
		},
		Transport:     transport,
		FlushInterval: -1, // flush each write immediately, crucial for streaming
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			slog.Error("Reverse proxy transfer error",
				slog.String("url", r.URL.String()),
				slog.String("method", r.Method),
				slog.Any("error", err),
			)
			w.WriteHeader(http.StatusBadGateway)
			fmt.Fprintf(w, "Bad Gateway: %v\n", err)
		},
	}

	clntAddr := fmt.Sprintf(":%d", s.cfg.ProxyPort)
	listener, err := net.Listen("tcp", clntAddr)
	if err != nil {
		slog.Error("Failed to start listener", slog.Any("error", err))
		return err
	}
	slog.Info("Listener started", slog.Int("port", int(s.cfg.ProxyPort)))

	s.httpServer = &http.Server{
		Addr:    clntAddr,
		Handler: rp,
		BaseContext: func(l net.Listener) context.Context {
			return ctx
		},
	}

	// Graceful shutdown on root context cancel
	cleanup := context.AfterFunc(ctx, func() {
		slog.Debug("Context cancelled: shutting down HTTP reverse proxy server")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), s.cfg.ShutdownTimeout)
		defer shutdownCancel()
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			slog.Warn("HTTP reverse proxy server shutdown error", slog.Any("error", err))
			s.httpServer.Close()
		}
	})

	wg.Go(func() {
		defer cleanup()
		slog.Debug("HTTP reverse proxy server waiting for requests")
		if err := s.httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("HTTP reverse proxy server error", slog.Any("error", err))
		}
	})

	slog.Info("Proxy server started", slog.Int("port", int(s.cfg.ProxyPort)))
	return nil
}

// makeConnection implements the dial-with-WOL procedure
// for connecting to the target server.
// On initial dial failure, executes WOL send.
// On WOL send success, dials one last time.
// Returns error on WOL send failure or second dial failure.
func (s *Server) makeConnection(ctx context.Context, network, target string) (net.Conn, error) {
	dialer := net.Dialer{Timeout: s.cfg.ProxyDialTimeout}

	// attempt initial connection
	srvrConn, err := dialer.DialContext(ctx, network, target)
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
		srvrConn, err = dialer.DialContext(ctx, network, target)
		if err != nil {
			slog.Error("Failed dialing server (attempt 2)",
				slog.String("server", target),
				slog.Any("error", err),
			)
			return nil, err
		}
	}

	slog.Info("Server connection succeeded", slog.String("server", target))
	return srvrConn, nil
}

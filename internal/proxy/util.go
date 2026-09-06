package proxy

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
)

func explainError(err error, dir string) error {
	switch {
	case err == nil:
		slog.Info("Connection terminated without error", slog.String("direction", dir))
		return nil
	case errors.Is(err, io.EOF):
		slog.Info("EOF received; normal termination", slog.String("direction", dir))
		return nil // EOF is normal termination
	case errors.Is(err, net.ErrClosed):
		slog.Info("Connection already closed; normal termination", slog.String("direction", dir))
		return nil // already closed; ignore error
	case isTimeoutError(err):
		slog.Warn("Connection timed out", slog.String("direction", dir), slog.Any("error", err))
		return err
	default:
		slog.Warn("Connection terminated", slog.String("direction", dir), slog.Any("error", err))
		return err
	}
}

// isTimeoutError returns true for network and context timeout errors.
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}

	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

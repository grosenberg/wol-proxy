package main

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/grosenberg/wol-proxy/internal/config"
)

var buf bytes.Buffer
var cfg *config.Config

func TestMain(m *testing.M) {

	cfg = config.DefaultConfig()
	lvl, err := toLogLevel(cfg.LogLevel)
	if err != nil {
		panic("Error converting log level in TestMain")
	}

	// dir := filepath.Dir(cfg.LogFile)
	// base := filepath.Base(cfg.LogFile)
	// ext := filepath.Ext(base)
	// name := strings.TrimSuffix(base, ext)

	opts := &slog.HandlerOptions{
		AddSource: true,
		Level:     lvl,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Formats time to "YYYY-MM-DD hh:mm:ss.mmm"
			if a.Key == slog.TimeKey {
				a.Value = slog.StringValue(a.Value.Time().Format("2006-01-02 15:04:05.000"))
			}
			// Reduces the source path to just the base name
			if a.Key == slog.SourceKey {
				source, _ := a.Value.Any().(*slog.Source)
				if source != nil {
					source.File = filepath.Base(source.File)
				}
			}
			return a
		},
	}

	// Point the logger output writer to a buffer
	handler := slog.NewTextHandler(&buf, opts)
	logger := slog.New(handler)
	slog.SetDefault(logger)
	os.Exit(m.Run())
}

func reset() {
	buf.Reset()
	cfg = config.DefaultConfig()
}

func TestSlogConfig(t *testing.T) {
	defer reset()

	want := " level=DEBUG msg=\"Initial Message\" \"Config log\"=./wol-proxy.log\n"

	slog.Debug("Initial Message", slog.String("Config log", cfg.LogFile))
	orig := buf.String()
	n := len(orig)
	t.Logf("len: %d, orig: %s\n", n, orig)

	skip := len("time=\"2006-01-02 15:04:05.000\"")
	if n < skip {
		t.Errorf("Input is shorter than expected: %d < %d", n, skip)
		return
	}

	have := orig[skip:]
	if have != want {
		t.Errorf("\nWant: %s\nHave: %s\n", want, have)
	}
}

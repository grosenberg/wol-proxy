package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/alchemy/rotoslog"
	"github.com/grosenberg/wol-proxy/internal/cflag"
	"github.com/grosenberg/wol-proxy/internal/config"
	"github.com/grosenberg/wol-proxy/internal/proxy"
)

// version can be overridden at build time via -ldflags "-X main.version=..."
var version = "dev"

func main() {

	// Command-line flags
	var (
		pathname    string
		logLevel    string
		saveConfig  bool
		saveForced  bool
		showVersion bool
	)

	flags := cflag.NewCFlagSet(config.AppName)
	flags.StringVar(&pathname, "c", "", "Pathname of YAML configuration file")
	flags.StringVar(&logLevel, "l", "INFO", "Set log level (DEBUG, INFO, WARN, ERROR)")
	flags.BoolVar(&saveConfig, "s", false, "Save default config and exit")
	flags.BoolVar(&saveForced, "S", false, "Force save to overwrite any existing config")
	flags.BoolVar(&showVersion, "v", false, "Print version information and exit")

	flags.Usage = func() {
		out := flags.Output()
		fmt.Fprintf(out, "wolproxy - Wake-on-LAN TCP Proxy Service\n\n")
		fmt.Fprintf(out, "Usage:\n")
		fmt.Fprintf(out, "  wolproxy [options]\n\n")
		fmt.Fprintf(out, "Options:\n")
		flags.PrintDefaults()
		fmt.Println()
	}

	cflag.Parse()

	// Interpret flags

	if showVersion {
		fmt.Printf(config.AppName+" %s\n", version)
		return
	}

	if saveConfig || saveForced {

		// -s or -s! & no -c: will save to <default user config path>/<appname>/config.yaml
		if pathname == "" {
			path, err := config.GetUserConfigPath()
			if err != nil {
				fmt.Printf("failed to determine user config directory: %v", err)
				os.Exit(1)
			}
			pathname = filepath.Join(path, config.ConfigName)
		}

		// -s & -c .: will save to <current working dir>/config.yaml
		if pathname == "." || pathname == ".." {
			cwd, err := os.Getwd()
			if err != nil {
				fmt.Printf("failed to determine user working directory: %v", err)
				os.Exit(1)
			}
			pathname = filepath.Join(cwd, pathname, config.ConfigName)
		}

		// -s & -c <pathname>: save to <pathname>
		// ensure a valid YAML ext; if no ext, add default config name
		base := filepath.Base(pathname)
		ext := filepath.Ext(base)
		if base == "." || base == ".." || ext == "" {
			pathname = filepath.Join(pathname, config.ConfigName)
		} else if ext != ".yaml" && ext != ".yml" {
			fmt.Printf("Expected a YAML config filename, not %s\n", base)
			os.Exit(1)
		}

		// now save
		if err := config.SaveConfig(pathname, config.DefaultConfig(), saveForced); err != nil {
			fmt.Printf("Failed to save configuration settings: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Configuration saved to %s\n", pathname)
		return
	}

	// Load configuration
	pathname, _, err := config.LocateConfig(pathname)
	if err != nil {
		fmt.Printf("Failed to find configuration at %s (%v)\n", pathname, err)
		os.Exit(1)
	}

	cfg, err := config.LoadConfig(pathname)
	if err != nil {
		fmt.Printf("Failed to load configuration from %s (%v)\n", pathname, err)
		os.Exit(1) // corrupt config file?
	}

	lvl, err := toLogLevel(logLevel)
	if err != nil {
		fmt.Printf("Bad log level: %s %v", lvl.String(), err)
	}
	cfg.LogLevel = lvl.String()

	// ==========================================
	// Configure slog for rotation
	dir := filepath.Dir(cfg.LogFile)
	name := filepath.Base(cfg.LogFile)
	ext := filepath.Ext(name)
	name = strings.TrimSuffix(name, ext)

	handler, err := rotoslog.NewHandler(
		rotoslog.LogDir(dir),
		rotoslog.FilePrefix(name+"-"),
		rotoslog.FileExt(ext),
		rotoslog.DateTimeLayout("20060102150405"),
		rotoslog.MaxFileSize(1024*1024), // default is 32MB
		rotoslog.MaxRotatedFiles(4),     // default is 8
		rotoslog.HandlerOptions(slog.HandlerOptions{Level: lvl}),
		rotoslog.LogHandlerBuilder(slog.NewTextHandler),
		// rotoslog.LogHandlerBuilder(slog.NewJSONHandler),
	)
	if err != nil {
		slog.Error("Failed to open log file", slog.Any("error", err))
		os.Exit(1)
	}
	defer handler.Close()
	logger := slog.New(handler)
	slog.SetDefault(logger)

	slog.Debug("Config loaded", slog.String("Config pathname", pathname))
	slog.Debug("Logging set", slog.String("level", cfg.LogLevel))

	// ==========================================
	// Graceful shutdown on SIGINT/SIGTERM/etc using
	// root context tied directly to OS shutdown signals
	ctx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT,
	)
	defer stop()

	// Create a common root WaitGroup
	var wg sync.WaitGroup

	// Create and run the port proxy server
	proxyServer := proxy.NewServer(cfg)
	if err := proxyServer.Start(ctx, &wg); err != nil {
		slog.Error("Failed to start the proxy server", slog.Any("error", err))
		os.Exit(1)
	}
	slog.Info("Proxy server started", slog.Int("port", int(cfg.ProxyPort)))

	// Block until a signal is received
	<-ctx.Done()
	slog.Info("Signal received: shutting down")
	stop() // restore default signal behavior: 2nd Ctrl+C immediately terminates

	// Wait for clean shutdown with timeout
	doneChan := waitToDone(&wg)
	timeout := time.After(cfg.ShutdownTimeout)

	select {
	case <-doneChan:
		slog.Info("Graceful shutdown complete")
		return

	case <-timeout:
		slog.Warn("Forced shutdown - timeout exceeded")
		os.Exit(1)
	}
}

func toLogLevel(lvl string) (slog.Level, error) {
	if lvl == "" {
		lvl = "INFO"
	}
	switch strings.ToUpper(lvl) {
	case "DEBUG":
		return slog.LevelDebug, nil
	case "INFO":
		return slog.LevelInfo, nil
	case "WARN":
		return slog.LevelWarn, nil
	case "ERROR":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("Unknown log level: %s; defaulting to INFO", lvl)
	}
}

func waitToDone(wg *sync.WaitGroup) chan struct{} {
	doneChan := make(chan struct{})
	go func() {
		wg.Wait() // Block until all other goroutines report done
		close(doneChan)
	}()
	return doneChan
}

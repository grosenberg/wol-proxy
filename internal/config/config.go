// Package config provides configuration for the application,
// including network parameters, proxy settings and server configurations.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	// Server details
	ServerMask int    `yaml:"server_mask"`
	ServerIP   string `yaml:"server_ip"`
	ServerMAC  string `yaml:"server_mac"`

	ProxyPort        uint16        `yaml:"proxy_port"`
	ProxyDialTimeout time.Duration `yaml:"proxy_dial_timeout"`
	ProxyXferTimeout time.Duration `yaml:"proxy_xfer_timeout"`

	// Ping settings
	PingLogStatusChange bool `yaml:"ping_log_status_change"` // log state change

	// WOL settings
	WOLServerPort     uint16        `yaml:"wol_port"`            // default is 9
	WOLDetectInterval time.Duration `yaml:"wol_detect_interval"` // max time for server to wake after WOL packet send
	WOLRetryInterval  time.Duration `yaml:"wol_retry_interval"`  // time between pings during wake detect
	WOLMaxRetries     int           `yaml:"wol_max_retries"`

	// Internal Ops settings
	MaxIdleConnections int           `yaml:"max_idle_connections"` // max concurrent keep-alive connections
	ShutdownTimeout    time.Duration `yaml:"shutdown_timeout"`     // wait time between graceful and forced shutdown

	// Logging
	LogMaxSize    int      `yaml:"log_maxsize"`
	LogMaxBackups int      `yaml:"log_maxbackups"`
	LogRotateAt   []string `yaml:"log_rotateat"`
	LogLevel      string   `yaml:"log_level"`
	LogFile       string   `yaml:"log_file"`
}

// DefaultConfig returns configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		ServerMAC:           "20:25:64:84:cf:96",    // server MAC
		ServerIP:            "192.168.1.140",        // server IP address
		ServerMask:          24,                     // class C mask
		ProxyPort:           11434,                  // port to proxy local <-> server
		ProxyDialTimeout:    30 * time.Second,       // server dial timeout
		ProxyXferTimeout:    5 * time.Second,        // internal data R/W xfer timeout
		PingLogStatusChange: true,                   // log awake <-> sleep changes
		WOLServerPort:       9,                      // dest port for WOL packet
		WOLDetectInterval:   10 * time.Second,       // max time for server to wake after WOL packet send
		WOLRetryInterval:    500 * time.Millisecond, // not used
		WOLMaxRetries:       3,                      // max number of WOL packet send retries
		MaxIdleConnections:  100,                    // max number of idle (keep-alive) connections
		ShutdownTimeout:     1 * time.Second,        // un-graceful shutdown limit
		LogMaxSize:          1,                      // max size in MB
		LogMaxBackups:       4,                      // max number of log files to keep
		LogRotateAt:         []string{},             // rotate at ["00:00", "12:00"] each day
		LogLevel:            "info",                 // min log level
		LogFile:             "./wol-proxy.log",      // log file location
	}
}

const (
	AppName    = "wolproxy"
	ConfigName = "config.yaml"
)

// LocateConfig finds a presumed existing configuration file.
//
// If a cli pathname is given, validate the ext is of type YAML.
// If only given a directory path, add the default config file name.
// If no pathname is given, look in the expected places to find a
// a configuration file:
// 1. the current working directory
// 2. the appname subdirectory of the current working directory
// 3. the hidden appname subdirectory of the current working directory
// 4. the user config directory for os.UserConfigDir()/<AppName>/config.yaml.
//
// Returns an error if retrieving the user config name is not a YAML type
// or if retriving the current working directory or default user config
// directory fails.
func LocateConfig(pathname string) (string, bool, error) {

	// evaluate the given pathname
	if pathname != "" {
		pathname = path.Clean(pathname)

		if path.IsAbs(pathname) {
			ext := filepath.Ext(pathname)
			if ext != "" && ext != ".yaml" && ext != ".yml" {
				return "", false, fmt.Errorf("Expected a YAML config filename, not %s\n", pathname)
			}
			if ext == "" {
				pathname = filepath.Join(pathname, ConfigName)
			}
			exists, err := PathExists(pathname)
			return pathname, exists, err
		}

		if !path.IsAbs(pathname) {
			cwd, err := os.Getwd()
			if err != nil {
				return "", false, fmt.Errorf("failed to determine current working directory: %+v", err)
			}
			pathname = filepath.Join(cwd, pathname)
		}

		pathname = path.Clean(pathname)
		ext := filepath.Ext(pathname)
		if ext != "" && ext != ".yaml" && ext != ".yml" {
			return "", false, fmt.Errorf("Expected a YAML config filename, not %s\n", pathname)
		}
		if ext == "" {
			pathname = filepath.Join(pathname, ConfigName)
		}

		exists, err := PathExists(pathname)
		return pathname, exists, err
	}

	// no pathname hint; look in the expected places
	// -- current working directory
	// -- the default user configuration directory

	cwd, err := os.Getwd()
	if err != nil {
		return "", false, fmt.Errorf("failed to determine current working directory: %+v", err)
	}

	// try direct
	pathname = filepath.Join(cwd, ConfigName)
	exists, err := PathExists(pathname)
	if exists {
		return pathname, exists, err
	}

	// try app name subdirectory
	pathname = filepath.Join(cwd, AppName, ConfigName)
	exists, err = PathExists(pathname)
	if exists {
		return pathname, exists, err
	}

	// try hidden app name subdirectory
	pathname = filepath.Join(cwd, "."+AppName, ConfigName)
	exists, err = PathExists(pathname)
	if exists {
		return pathname, exists, err
	}

	// try the default user config directory
	path, err := GetUserConfigPath()
	if err != nil {
		return "", false, fmt.Errorf("failed to locate the user config directory: %+v", err)
	}

	pathname = filepath.Join(path, ConfigName)
	exists, err = PathExists(pathname)
	return pathname, exists, err

}

// GetUserConfigPath returns the path to the default user
// config directory (os.UserConfigDir + AppName).
// Returns an error if the config dir is not accessible.
func GetUserConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine user config directory: %w", err)
	}

	path := filepath.Join(dir, AppName)
	return path, err
}

// LoadConfig loads configuration data from the named YAML file
// Missing configuration yaml key/value pairs receive their default values
// Invalid key names are silently ignored
// Valid keys with invalid values are reported as an error
// Returns the config structure, initially populated with the default values,
// updated consistent with YAML file configuration data. If the configuration
// file does not exist, the default valued configuration is returned.
func LoadConfig(pathname string) (*Config, error) {
	cfg := DefaultConfig()

	if exists, err := PathExists(pathname); exists {
		if err != nil { // path error, etc.
			slog.Error("Unable to access config file",
				slog.String("pathname", pathname),
				slog.Any("error", err),
			)
			return nil, err
		}

		data, err := os.ReadFile(pathname)
		if err != nil { // File exists, but is unreadable
			slog.Error("Unable to read config file",
				slog.String("pathname", pathname),
				slog.Any("error", err),
			)
			return nil, err
		}

		if err := yaml.Unmarshal(data, cfg); err != nil {
			slog.Error("Unmarshal error: config file possibly corrupt",
				slog.String("pathname", pathname),
				slog.Any("error", err),
			)
			return nil, err
		}
	}

	// return unmarshalled or default config
	return cfg, nil
}

// PathExists checks whether the given path is valid/exists.
// Returns false, no error if the error would be fs.ErrNotExist.
// Returns false, error on any unexpected condition: permission denied, invalid path, etc.
func PathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func SaveConfig(pathname string, cfg *Config, force bool) error {
	if len(strings.TrimSpace(pathname)) == 0 {
		return fmt.Errorf("Config not saved: no pathname")
	}

	autoGen := false
	if cfg == nil || reflect.DeepEqual(cfg, DefaultConfig()) {
		autoGen = true
		cfg = DefaultConfig()
	}

	exists, err := PathExists(pathname)
	if exists && !force {
		slog.Warn("Config file exists; will not overwrite",
			slog.String("pathname", pathname),
			slog.Bool("force", force),
		)
		return fmt.Errorf("Config not saved: will not overwrite")
	}

	// Create config file with defaults
	slog.Info("Saving config file", slog.String("pathname", pathname))

	// Create directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(pathname), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Convert config to YAML
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal default config: %w", err)
	}

	hdr := make([]byte, 0)
	if autoGen {
		// Add a auto gen comment header
		hdr = append(hdr, "# Application Configuration\n"...)
		hdr = append(hdr, "# This file was auto-generated with default values\n"...)
		hdr = append(hdr, "# Modify as needed\n"...)
	}
	data = append(hdr, data...)

	// Write to file
	if err := os.WriteFile(pathname, data, 0644); err != nil {
		return fmt.Errorf("failed to write default config: %w", err)
	}

	slog.Info("Config file created successfully.", slog.String("pathname", pathname))
	return nil
}

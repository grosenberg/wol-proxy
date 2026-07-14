package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	// Server details
	ServerMAC      string `yaml:"server_mac"`
	ServerIP       string `yaml:"server_ip"`
	ServerMACPort  int    `yaml:"server_mac_port"`
	LocalProxyPort int    `yaml:"local_proxy_port"`

	// Operational settings
	PingTimeout          time.Duration `yaml:"ping_timeout"`
	RetryInterval        time.Duration `yaml:"retry_interval"`
	MaxWOLRetries        int           `yaml:"max_wol_retries"`
	ServerInitialTimeout time.Duration `yaml:"server_initial_timeout"`
	MaxConnections       int           `yaml:"max_connections"`

	// Logging
	LogLevel string `yaml:"log_level"`
}

// DefaultConfig returns configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		ServerMAC:            "20:25:64:84:cf:96",
		ServerIP:             "192.168.1.66",
		ServerMACPort:        9,
		LocalProxyPort:       11434,
		PingTimeout:          2 * time.Second,
		RetryInterval:        250 * time.Millisecond,
		MaxWOLRetries:        3,
		ServerInitialTimeout: 60 * time.Second,
		MaxConnections:       100,
		LogLevel:             "info",
	}
}

// LoadConfig loads configuration from YAML file
func LoadConfig(filename string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(filename)
	if err != nil {
		// File doesn't exist, return defaults
		return cfg, nil
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

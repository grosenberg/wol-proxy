package config

import (
	"os"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.ServerMAC != "20:25:64:84:cf:96" {
		t.Errorf("DefaultConfig().ServerMAC = %s, want %s",
			cfg.ServerMAC, "20:25:64:84:cf:96")
	}

	if cfg.ServerIP != "192.168.1.66" {
		t.Errorf("DefaultConfig().ServerIP = %s, want %s",
			cfg.ServerIP, "192.168.1.66")
	}

	if cfg.LocalProxyPort != 11434 {
		t.Errorf("DefaultConfig().LocalProxyPort = %d, want %d",
			cfg.LocalProxyPort, 11434)
	}

	if cfg.MaxConnections != 100 {
		t.Errorf("DefaultConfig().MaxConnections = %d, want %d",
			cfg.MaxConnections, 100)
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	// Try to load non-existent file - should return defaults
	cfg, err := LoadConfig("non_existent_config.yaml")

	if err != nil {
		t.Errorf("LoadConfig() error = %v, want nil", err)
	}

	if cfg == nil {
		t.Fatal("LoadConfig() returned nil config")
	}

	// Should still have default values
	if cfg.ServerMAC == "" {
		t.Error("LoadConfig() returned empty ServerMAC")
	}
}

func TestLoadConfig_ValidYAML(t *testing.T) {
	// Create temporary config file
	yamlContent := `server_mac: "11:22:33:44:55:66"
server_ip: "10.0.0.1"
local_proxy_port: 12345
max_connections: 50
log_level: "debug"
ping_timeout: 5s
retry_interval: 500ms
max_wol_retries: 5
server_initial_timeout: 120s
server_mac_port: 7
`

	tmpfile, err := os.CreateTemp("", "test_config_*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(yamlContent)); err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()

	cfg, err := LoadConfig(tmpfile.Name())

	if err != nil {
		t.Errorf("LoadConfig() error = %v, want nil", err)
	}

	if cfg.ServerMAC != "11:22:33:44:55:66" {
		t.Errorf("ServerMAC = %s, want %s", cfg.ServerMAC, "11:22:33:44:55:66")
	}

	if cfg.LocalProxyPort != 12345 {
		t.Errorf("LocalProxyPort = %d, want %d", cfg.LocalProxyPort, 12345)
	}

	if cfg.MaxConnections != 50 {
		t.Errorf("MaxConnections = %d, want %d", cfg.MaxConnections, 50)
	}

	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %s, want %s", cfg.LogLevel, "debug")
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	// Create invalid YAML file
	invalidYAML := `server_mac: "11:22:33:44:55:66"
server_ip: "10.0.0.1"
local_proxy_port: notanumber  # This will cause parse error
`

	tmpfile, err := os.CreateTemp("", "test_invalid_*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(invalidYAML)); err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()

	cfg, err := LoadConfig(tmpfile.Name())

	if err == nil {
		t.Error("LoadConfig() with invalid YAML should return error")
	}

	if cfg != nil {
		t.Error("LoadConfig() with invalid YAML should return nil config")
	}
}

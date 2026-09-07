package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestFileExists(t *testing.T) {
	// Create a temporary file
	file, err := os.CreateTemp("", "testfile-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(file.Name()) // Clean up after test

	exists, err := PathExists(file.Name())
	if err != nil {
		t.Fatalf("Error checking file existence: %v", err)
	}

	if !exists {
		t.Errorf("Expected file to exist, but it does not.")
	}
}

// TestDefaultConfig tests that default configuration values are set properly
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.ServerMAC != "20:25:64:84:cf:96" {
		t.Errorf("DefaultConfig().ServerMAC = %s, want %s",
			cfg.ServerMAC, "20:25:64:84:cf:96")
	}

	if cfg.ServerIP != "192.168.1.140" {
		t.Errorf("DefaultConfig().ServerIP = %s, want %s",
			cfg.ServerIP, "192.168.1.140")
	}

	if cfg.ProxyPort != 11434 {
		t.Errorf("DefaultConfig().LocalProxyPort = %d, want %d",
			cfg.ProxyPort, 11434)
	}

	if cfg.PoolMaxConnections != 100 {
		t.Errorf("DefaultConfig().MaxConnections = %d, want %d",
			cfg.PoolMaxConnections, 100)
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

// TestLoadConfig tests configuration loading from file
func TestLoadConfig(t *testing.T) {

	// Create temporary config file
	file, err := os.CreateTemp("", "config-test-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(file.Name())

	// Convert config to YAML
	def := DefaultConfig()
	data, err := yaml.Marshal(def)
	if err != nil {
		t.Fatalf("Failed to marshal default config: %v", err)
	}

	// Write test configuration
	if err := os.WriteFile(file.Name(), []byte(data), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}
	file.Close()

	// Load configuration
	cfg, err := LoadConfig(file.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify values
	if !reflect.DeepEqual(def, cfg) {
		t.Errorf("Expected config as written and read to be equal")
	}
}

type TConfig struct {
	ServerIP         string        `yaml:"server_ip"`
	ProxyXferTimeout time.Duration `yaml:"proxy_xfer_timeout"`
}

func TDefaultConfig() *TConfig {
	return &TConfig{
		ServerIP:         "192.168.1.140",
		ProxyXferTimeout: 5 * time.Second,
	}
}

func TestUnMarshal(t *testing.T) {
	data := "proxy_xfer_timeout: 5"
	cfg := &TConfig{}
	err := yaml.Unmarshal([]byte(data), cfg)
	if err == nil {
		t.Fatalf("Unreported Unmarshal value error: %v", err)
	}

	data = "unknown_tag: 5s"
	cfg = &TConfig{}
	err = yaml.Unmarshal([]byte(data), cfg)
	if err != nil {
		t.Fatalf("Unmarshal tag error: %v", err)
	}
}

// TestInvalidConfig tests error handling for invalid configurations
func TestInvalidConfig(t *testing.T) {
	// Test missing required field
	file, err := os.CreateTemp("", "config-test-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(file.Name())

	// Write invalid configuration
	content := "proxy_xfer_timeout: 5" // valid tag w/invalid value
	if err := os.WriteFile(file.Name(), []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}
	file.Close()

	// Load configuration
	cfg, err := LoadConfig(file.Name())
	// fmt.Printf("Config loaded: %+v: %+v\n", cfg, err)
	if err == nil {
		t.Error("Expected error for invalid config, got nil")
	}
	if cfg != nil {
		t.Error("LoadConfig() with invalid YAML should return nil config")
	}
}

func TestLocateConfig_ExplicitPath(t *testing.T) {
	custom := "custom/path/config.yaml"
	got, _, err := LocateConfig(custom)
	if err != nil {
		t.Fatalf("LocateConfig(%q) unexpected error: %v", custom, err)
	}
	if got != custom {
		t.Errorf("LocateConfig(%q) = %q, want %q", custom, got, custom)
	}
}

func TestLocateConfig_EmptyPath(t *testing.T) {
	got, _, err := LocateConfig("")
	if err != nil {
		t.Fatalf("LocateConfig(\"\") unexpected error: %v", err)
	}
	if got == "" {
		t.Errorf("LocateConfig(\"\") returned empty string")
	}
}

func TestLocateConfig_UserConfigDirFound(t *testing.T) {

	// Execute with CWD set to repository root to avoid resource collision
	root := findRepoRoot(t)
	t.Chdir(root)

	tmpDir, err := os.MkdirTemp("", "userconfig-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create AppName/config.yaml inside tmpDir
	appConfigDir := filepath.Join(tmpDir, AppName)
	if err := os.MkdirAll(appConfigDir, 0755); err != nil {
		t.Fatal(err)
	}
	expectedPath := filepath.Join(appConfigDir, ConfigName)
	if err := os.WriteFile(expectedPath, []byte("# test config\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Temporarily override environment variables for UserConfigDir
	t.Setenv("AppData", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	got, _, err := LocateConfig("")
	if err != nil {
		t.Fatalf("LocateConfig(\"\") error: %v", err)
	}
	if got != expectedPath {
		t.Errorf("LocateConfig(\"\") = %q, want %q", got, expectedPath)
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repository root (go.mod)")
		}
		dir = parent
	}
}

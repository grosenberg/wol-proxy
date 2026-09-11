package netutil

import (
	"bytes"
	"log/slog"
	"net"
	"os"
	"testing"

	"github.com/grosenberg/wol-proxy/internal/config"
)

func TestMain(m *testing.M) {
	// Formats time to "YYYY-MM-DD hh:mm:ss.mmm"
	opts := &slog.HandlerOptions{
		Level: slog.LevelDebug,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				a.Value = slog.StringValue(a.Value.Time().Format("2006-01-02 15:04:05.000"))
			}
			return a
		},
	}

	// Sets the debug logger to os.Stderr. Change
	// to io.Discard to silence the output.
	handler := slog.NewTextHandler(os.Stderr, opts)
	slog.SetDefault(slog.New(handler))

	os.Exit(m.Run())
}

func TestNewWOLSender(t *testing.T) {
	cfg := config.DefaultConfig()
	sender := NewWOLSender(cfg)

	if sender == nil {
		t.Fatal("Expected NewWOLSender to return non-nil sender")
	}
	if sender.cfg != cfg {
		t.Errorf("Expected sender.cfg to equal passed config, got %+v", sender.cfg)
	}
}

func TestParseMAC(t *testing.T) {
	sender := NewWOLSender(config.DefaultConfig())

	tests := []struct {
		name      string
		macStr    string
		expectErr bool
		expected  []byte
	}{
		{
			name:      "Valid colon-separated MAC",
			macStr:    "00:11:22:33:44:55",
			expectErr: false,
			expected:  []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
		},
		{
			name:      "Valid hyphen-separated MAC",
			macStr:    "AA-BB-CC-DD-EE-FF",
			expectErr: false,
			expected:  []byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF},
		},
		{
			name:      "Valid dot-separated Cisco format",
			macStr:    "0011.2233.4455",
			expectErr: false,
			expected:  []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
		},
		{
			name:      "Invalid format",
			macStr:    "not-a-mac",
			expectErr: true,
			expected:  nil,
		},
		{
			name:      "Empty string",
			macStr:    "",
			expectErr: true,
			expected:  nil,
		},
		{
			name:      "Invalid length (EUI-64 8 bytes)",
			macStr:    "00:11:22:33:44:55:66:77",
			expectErr: true,
			expected:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sender.parseMAC(tt.macStr)
			if tt.expectErr {
				if err == nil {
					t.Errorf("parseMAC(%q) expected error, got nil", tt.macStr)
				}
			} else {
				if err != nil {
					t.Fatalf("parseMAC(%q) unexpected error: %v", tt.macStr, err)
				}
				if !bytes.Equal(got, tt.expected) {
					t.Errorf("parseMAC(%q) = %v, want %v", tt.macStr, got, tt.expected)
				}
			}
		})
	}
}

func TestCreateMagicPacket(t *testing.T) {
	mac := net.HardwareAddr{0x12, 0x34, 0x56, 0x78, 0x9A, 0xBC}
	packet := createMagicPacket(mac)

	// Magic packet must be exactly 102 bytes
	if len(packet) != 102 {
		t.Fatalf("Expected magic packet length of 102 bytes, got %d", len(packet))
	}

	// First 6 bytes must be 0xFF
	expectedPrefix := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
	if !bytes.Equal(packet[0:6], expectedPrefix) {
		t.Errorf("Expected first 6 bytes to be %v, got %v", expectedPrefix, packet[0:6])
	}

	// Following 16 chunks of 6 bytes must repeat the MAC address
	for idx := range 16 {
		start := 6 + idx*6
		end := start + 6
		chunk := packet[start:end]
		if !bytes.Equal(chunk, mac) {
			t.Errorf("Chunk %d (bytes %d-%d) = %v, want %v", idx, start, end, chunk, mac)
		}
	}
}

func TestWOLSender_Send(t *testing.T) {
	t.Run("Invalid MAC address in config returns error", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.ServerMAC = "invalid-mac"
		sender := NewWOLSender(cfg)

		err := sender.Send(t.Context())
		if err == nil {
			t.Error("Expected Send() to fail for invalid MAC address, but got nil error")
		}
	})

	t.Run("Valid MAC address successfully sends packet", func(t *testing.T) {
		cfg := config.DefaultConfig()
		// cfg.ServerMAC = "00:11:22:33:44:55"
		sender := NewWOLSender(cfg)

		err := sender.Send(t.Context())
		if err != nil {
			t.Errorf("Expected Send() to succeed, got error: %v", err)
		}
	})
}

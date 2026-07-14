package netutil

import (
	"testing"
)

func TestCalculateChecksum(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want uint16
	}{
		{"Empty data", []byte{}, 0xFFFF},
		{"Single byte zero", []byte{0x00}, 0xFF00},
		{"Two byte zero", []byte{0x00, 0x00}, 0xFFFF},
		{"Simple ICMP echo", []byte{0x08, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01}, 0xF7FE},
		{"Odd length data", []byte{0x01, 0x02, 0x03}, 0xF8FA},
		{"Known checksum case", []byte{0x45, 0x00, 0x00, 0x73, 0x00, 0x00, 0x40, 0x00, 0x40, 0x11, 0x00, 0x00, 0xc0, 0xa8, 0x00, 0x01, 0xc0, 0xa8, 0x00, 0xc7}, 0xb861},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateChecksum(tt.data)
			if got != tt.want {
				t.Errorf("calculateChecksum() = %04X, want %04X", got, tt.want)
			}
		})
	}
}

func TestCalculateICMPChecksum(t *testing.T) {
	// Test that ICMP checksum is just an alias for calculateChecksum
	data := []byte{0x08, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01}

	checksum1 := calculateChecksum(data)
	checksum2 := calculateICMPChecksum(data)

	if checksum1 != checksum2 {
		t.Errorf("calculateICMPChecksum() = %04X, calculateChecksum() = %04X, want equal",
			checksum2, checksum1)
	}
}

func TestChecksumWithLargeData(t *testing.T) {
	// Create a larger data set
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 256)
	}

	got := calculateChecksum(data)

	// Verify that checksum produces consistent results
	if got == 0xFFFF {
		t.Error("Checksum should not be all ones for non-empty data")
	}

	// Test idempotence
	got2 := calculateChecksum(data)
	if got != got2 {
		t.Errorf("calculateChecksum() not idempotent: %04X != %04X", got, got2)
	}
}

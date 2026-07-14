package netutil

import (
    "net"
    "testing"
    "time"
)

func TestWOLSender_ParseMAC(t *testing.T) {
    tests := []struct {
        name    string
        macStr  string
        wantErr bool
    }{
        {"Valid MAC with colons", "AA:BB:CC:DD:EE:FF", false},
        {"Valid MAC with dashes", "AA-BB-CC-DD-EE-FF", false},
        {"Invalid MAC", "AA:BB:CC:DD:EE:FG", true},
        {"Empty string", "", true},
        {"Too short", "AA:BB:CC:DD:EE", true},
        {"Too long", "AA:BB:CC:DD:EE:FF:11", true},
    }
    
    w := NewWOLSender()
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            _, err := w.parseMAC(tt.macStr)
            if (err != nil) != tt.wantErr {
                t.Errorf("parseMAC(%q) error = %v, wantErr %v", tt.macStr, err, tt.wantErr)
            }
        })
    }
}

func TestWOLSender_CreateMagicPacket(t *testing.T) {
    w := NewWOLSender()
    mac := []byte{0xDE, 0xAD, 0xBE, 0xEF, 0x00, 0xCC}
    
    packet := w.createMagicPacket(mac)
    
    // Check packet length: 6 bytes of FF + 16 repetitions of MAC
    expectedLength := 6 + 16*len(mac)
    if len(packet) != expectedLength {
        t.Errorf("Packet length = %d, want %d", len(packet), expectedLength)
    }
    
    // Check first 6 bytes are all 0xFF
    for i := 0; i < 6; i++ {
        if packet[i] != 0xFF {
            t.Errorf("Packet[%d] = %02X, want 0xFF", i, packet[i])
        }
    }
}

func TestPinger_CalculateICMPChecksum(t *testing.T) {
    p := NewPinger(2 * time.Second)
    
    tests := []struct {
        name string
        data []byte
        want uint 16
    }{
        {"Empty data", []byte{}, 0xFFFF},
        {"Single byte", []byte{0x00}, 0xFFFF},
        {"Simple data", []byte{0x08, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01}, 0xF 7FE},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := p.calculateICMPChecksum(tt.data)
            if got != tt.want {
                t.Errorf("calculateICMPChecksum() = %04X, want %04X", got, tt.want)
            }
        })
    }
}

func TestPinger_FallbackTCP(t *testing.T) {
    p := NewPinger(100 * time.Millisecond)
    
    // Test with localhost (should fail since we're not running servers)
    // This tests the failure path
    got := p.fallbackTCP("127.0.0.1")
    if got {
        t.Error("fallbackTCP() = true, want false (no servers running)")
    }
    
    // Test with invalid hostname
    got = p.fallbackTCP("invalid.hostname.local")
    if got {
        t.Error("fallbackTCP() with invalid host = true, want false")
    }
}

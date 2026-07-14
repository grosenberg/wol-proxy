package netutil

import (
	"bytes"
	"fmt"
	"log"
	"net"
)

// WOLSender handles Wake-on-LAN operations
type WOLSender struct{}

// NewWOLSender creates a new WOL sender
func NewWOLSender() *WOLSender {
	return &WOLSender{}
}

// parseMAC converts MAC address string to bytes
func (w *WOLSender) parseMAC(macStr string) ([]byte, error) {
	macBytes := make([]byte, 6)
	_, err := fmt.Sscanf(macStr, "%x:%x:%x:%x:%x:%x",
		&macBytes[0], &macBytes[1], &macBytes[2],
		&macBytes[3], &macBytes[4], &macBytes[5])

	if err != nil {
		_, err = fmt.Sscanf(macStr, "%x-%x-%x-%x-%x-%x",
			&macBytes[0], &macBytes[1], &macBytes[2],
			&macBytes[3], &macBytes[4], &macBytes[5])
	}

	if err != nil {
		return nil, fmt.Errorf("invalid MAC format: %v", err)
	}

	return macBytes, nil
}

// createMagicPacket constructs the WOL magic packet
func (w *WOLSender) createMagicPacket(mac []byte) []byte {
	packet := bytes.NewBuffer(nil)
	packet.Write([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF})
	for i := 0; i < 16; i++ {
		packet.Write(mac)
	}
	return packet.Bytes()
}

// Send sends a Wake-on-LAN magic packet
func (w *WOLSender) Send(macStr string, targetIP string, port int) error {
	mac, err := w.parseMAC(macStr)
	if err != nil {
		return fmt.Errorf("failed to parse MAC: %v", err)
	}

	packet := w.createMagicPacket(mac)

	// Use broadcast address
	broadcastIP := net.IPv4(255, 255, 255, 255)

	// Create UDP connection
	conn, err := net.DialUDP("udp", nil, &net.UDPAddr{
		IP:   broadcastIP,
		Port: port,
	})
	if err != nil {
		return fmt.Errorf("UDP connection failed: %v", err)
	}
	defer conn.Close()

	// Set write buffer
	if udpConn, ok := conn.(*net.UDPConn); ok {
		udpConn.SetWriteBuffer(len(packet))
	}

	// Send packet
	_, err = conn.Write(packet)
	if err != nil {
		return fmt.Errorf("failed to send WOL packet: %v", err)
	}

	log.Printf("WOL packet sent to %s for MAC %s", broadcastIP, macStr)
	return nil
}

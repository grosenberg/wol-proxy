package netutil

import (
	"fmt"
	"log"
	"net"
	"time"
)

// Pinger handles ICMP ping operations
type Pinger struct {
	timeout time.Duration
}

// NewPinger creates a new pinger with specified timeout
func NewPinger(timeout time.Duration) *Pinger {
	return &Pinger{timeout: timeout}
}

// calculateICMPChecksum calculates Internet checksum for ICMP packets
func (p *Pinger) calculateICMPChecksum(data []byte) uint16 {
	var sum uint32

	for i := 0; i < len(data); i += 2 {
		var word uint16
		if i+1 < len(data) {
			word = uint16(data[i])<<8 | uint16(data[i+1])
		} else {
			word = uint16(data[i]) << 8
		}
		sum += uint32(word)
	}

	for sum>>16 != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}

	return uint16(^sum)
}

// Ping sends ICMP echo request to check if host is alive
func (p *Pinger) Ping(ip string) bool {
	dstAddr := net.ParseIP(ip)
	if dstAddr == nil {
		log.Printf("Invalid IP address: %s", ip)
		return false
	}

	// Create ICMP socket
	conn, err := net.DialIP("ip 4:icmp", nil, &net.IPAddr{IP: dstAddr})
	if err != nil {
		log.Printf("ICMP socket failed: %v", err)
		return p.fallbackTCP(ip)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(p.timeout))

	// ICMP Echo Request
	icmpEchoRequest := []byte{
		8, 0, 0, 0, // Type, Code, Checksum
		0, 1, 0, 1, // Identifier, Sequence Number
	}

	// Add payload
	payload := []byte("WOLProxyPing")
	icmpEchoRequest = append(icmpEchoRequest, payload...)

	// Calculate checksum
	checksum := p.calculateICMPChecksum(icmpEchoRequest)
	icmpEchoRequest[2] = byte(checksum >> 8)
	icmpEchoRequest[3] = byte(checksum & 0xff)

	// Send request
	_, err = conn.Write(icmpEchoRequest)
	if err != nil {
		log.Printf("Failed to send ICMP: %v", err)
		return p.fallbackTCP(ip)
	}

	// Receive reply
	reply := make([]byte, 1500)
	n, err := conn.Read(reply)
	if err != nil {
		return false
	}

	return n >= 8 && reply[0] == 0 && reply[1] == 0
}

// fallbackTCP provides TCP fallback when ICMP fails
func (p *Pinger) fallbackTCP(ip string) bool {
	// Try common ports
	ports := []int{22, 80, 443}

	for _, port := range ports {
		addr := net.JoinHostPort(ip, fmt.Sprintf("%d", port))
		conn, err := net.DialTimeout("tcp", addr, p.timeout)
		if err == nil {
			conn.Close()
			return true
		}
	}

	return false
}

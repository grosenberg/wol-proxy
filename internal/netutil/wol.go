package netutil

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"time"

	"github.com/grosenberg/wol-proxy/internal/config"
	"github.com/grosenberg/wol-proxy/internal/monitor"
)

// WOLSender handles Wake-on-LAN operations
type WOLSender struct {
	cfg *config.Config
}

// NewWOLSender creates a new WOL sender
func NewWOLSender(cfg *config.Config) *WOLSender {
	w := &WOLSender{
		cfg: cfg,
	}

	slog.Info("WOL Sender created")
	return w
}

// Send sends a Wake-on-LAN magic packet
// -- loop up to WOLMaxRetries
// -- send packet
// -- start continuous ping to notice when server wakes
// -- if ping succeeds, return nil
// -- on ping timeout defined by WOLInitialInterval, continue
// -- after WOLMaxRetries, return err
func (w *WOLSender) Send(ctx context.Context) error {
	mac, err := w.parseMAC(w.cfg.ServerMAC)
	if err != nil {
		slog.Error("Failed to parse MAC", slog.String("MAC", w.cfg.ServerMAC))
		return fmt.Errorf("Failed to parse MAC: %v", err)
	}

	packet := createMagicPacket(mac)

	bcIP, err := calcBroadcastIP(w.cfg.ServerIP, w.cfg.ServerMask)
	target := netip.AddrPortFrom(bcIP, w.cfg.WOLServerPort)
	udpTgt := net.UDPAddrFromAddrPort(target)

	m := monitor.NewMonitor(w.cfg)

	// try up to WOLMaxRetries
	for idx := range w.cfg.WOLMaxRetries {
		slog.Debug("Trying to wake server", slog.Int("attempt", idx))

		// Create UDP connection
		conn, err := net.DialUDP("udp", nil, udpTgt)
		if err != nil {
			slog.Error("Dialing for WOL/UDP connection failed",
				slog.Any("target", target),
				slog.Any("error", err),
			)
			return fmt.Errorf("Dialing for WOL/UDP connection failed: %v", err)
		}

		// Send Wol packet and close conn
		_, err = conn.Write(packet)
		if err != nil {
			slog.Error("Failed to send WOL packet", slog.Int("attempt", idx), slog.Any("error", err))
			return fmt.Errorf("Failed to send WOL packet: %v", err)
		}
		slog.Info("WOL packet sent",
			slog.Int("attempt", idx),
			slog.Any("dest", target),
			slog.String("MAC", w.cfg.ServerMAC),
		)
		conn.Close()

		if awake := w.detectStateChange(ctx, m); !awake {
			continue
		}

		m.LogCurrentState(slog.LevelInfo)
		return nil
	}

	return fmt.Errorf("failed to wake server: %+v", err)
}

func (w *WOLSender) detectStateChange(ctx context.Context, m *monitor.Monitor) bool {
	timeout := time.After(w.cfg.WOLDetectInterval)
	ticker := time.NewTicker(w.cfg.WOLRetryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return false
		case <-timeout:
			return false // no state change detected
		case <-ticker.C:
			if m.Ping(ctx) { // one ping only
				return true
			}
		}
	}
}

// parseMAC converts MAC address string to bytes
func (w *WOLSender) parseMAC(str string) ([]byte, error) {
	// auto handles split on colon or dash, short or long hex parts
	mac, err := net.ParseMAC(str)
	if err != nil {
		return nil, fmt.Errorf("invalid MAC address %q: %w", str, err)
	}
	if len(mac) != 6 {
		return nil, fmt.Errorf("invalid MAC address length (%d bytes), expected 6: %q", len(mac), str)
	}
	return mac, nil
}

// createMagicPacket constructs the WOL magic packet.
// The packet is defined as 6 bytes of 0xFF followed by
// 16 repetitions of the MAC address
func createMagicPacket(mac net.HardwareAddr) []byte {
	var packet bytes.Buffer

	for range 6 {
		packet.WriteByte(0xFF)
	}

	for range 16 {
		packet.Write(mac)
	}
	return packet.Bytes()
}

// calcBroadcastIP calculates the broadcast IP for a given IPv4 host
// by network class type expressed as a mask size (0, 8, 16, or 24).
func calcBroadcastIP(srvrIP string, maskSize int) (netip.Addr, error) {

	// parse into an immutable netip.Addr
	addr, err := netip.ParseAddr(srvrIP)
	if err != nil {
		return netip.Addr{}, err
	}

	// validate IPv4 address
	if !addr.Is4() {
		return netip.Addr{}, fmt.Errorf("address must be an IPv4 address")
	}

	// convert address to byte array
	octets := addr.As4()

	// fill the host portion with 255 based on the standard byte boundary
	switch maskSize {
	case 0:
		octets[0], octets[1], octets[2], octets[3] = 255, 255, 255, 255
	case 8:
		octets[1], octets[2], octets[3] = 255, 255, 255
	case 16:
		octets[2], octets[3] = 255, 255
	case 24:
		octets[3] = 255
	default:
		return netip.Addr{}, fmt.Errorf("Unsupported mask size: must be 0, 8, 16, or 24")
	}

	return netip.AddrFrom4(octets), nil
}

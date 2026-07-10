/*
Sample Network Traffic Flow

Client Connects: A client (e.g., a web browser) connects to the local proxy
running on your Windows 11 machine at localhost:11434.
Data from Client: The client sends data to the proxy.

Server Check: The proxy checks if the server (192.168.1.166:11434) is awake.
If the server is sleeping, it holds the packet and sends a Wake-on-LAN magic
packet to wake up the server (assuming the server BIOS/UEFI settings allow WOL
magic packets on its network interface). If the server is awake, it proceeds
without holding any packets.

Server Response: After the server wakes up (or if it was already awake), it
starts processing requests.

Bi-Directional Data Flow:
A goroutine copies data from the client to the server on an ongoing basis.
The main execution thread within proxyData simultaneously copies data from
the server back to the client.
*/
package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/mdzmdz/wol"
)

// Global variables for configuration
const (
	ServerIP       = "192.168.1.166" // sage IP
	ServerPort     = 11434
	LocalProxyPort = 11434
	WOLMacAddress  = "20:25:64:84:cf:96" // sage MAC
	CheckInterval  = time.Second * 10
)

var (
	lastReceivedPacket []byte
)

func proxyData(conn net.Conn, dst string) error {
	dstConn, err := net.Dial("tcp", dst)
	if err != nil {
		log.Printf("Failed to connect to server: %v", err)
		return err
	}
	defer dstConn.Close()

	// Wait for the server to wake up if it is sleeping
	for !checkIfServerIsAwake(ServerIP, ServerPort) && lastReceivedPacket != nil {
		time.Sleep(CheckInterval)
	}

	if lastReceivedPacket != nil {
		_, err := dstConn.Write(lastReceivedPacket)
		if err != nil {
			log.Printf("Failed to forward held packet: %v", err)
			return err
		}
		lastReceivedPacket = nil
	}

	go func() {
		_, err := io.Copy(dstConn, conn)
		if err != nil && err != io.EOF {
			log.Printf("Error copying from client to server: %v", err)
		}
	}()

	_, err = io.Copy(conn, dstConn)
	if err != nil && err != io.EOF {
		log.Printf("Error copying from server to client: %v", err)
	}

	return nil
}

func checkIfServerIsAwake(ip string, port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", ip, port), time.Second*5)
	if err != nil {
		log.Printf("Failed to reach server %v, assuming it is asleap: %v", ip, err)
		return false
	}
	defer conn.Close()
	log.Printf("Server is awake.")
	return true
}

func sendMagicPacket(macAddress string) error {
	hardwareAddr, err := net.ParseMAC(strings.Replace(macAddress, "-", ":", -1))
	if err != nil {
		return err
	}

    // MagicPacket is a broadcast frame containing FF FF FF FF FF FF followed by 16 repetitions of the target MAC address
	magicPacket := wol.Build(hardwareAddr)
	packetBuf := bytes.NewBuffer(magicPacket)

	connection, err := net.DialUDP("udp", &net.UDPAddr{Port: 0}, &net.UDPAddr{
		IP:   net.IPv4(255, 255, 255, 255),
		Port: 9, // Discard Protocol Port
	})
	if err != nil {
		return err
	}
	defer connection.Close()

	_, err = io.Copy(connection, packetBuf)
	return err
}

func handleClientConnection(clientConn net.Conn) {
	dst := fmt.Sprintf("%s:%d", ServerIP, ServerPort)

	// Read data from the client before proxying to check server state
	buf := make([]byte, 4096)
	clientConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	bytesRead, err := clientConn.Read(buf)
	if err != nil {
		log.Printf("Error reading data from client: %v", err)
		return
	}

	// If server is asleep, hold the packet and send magic packet
	if !checkIfServerIsAwake(ServerIP, ServerPort) {
		lastReceivedPacket = buf[:bytesRead]
		log.Println("Holding packet for awake server.")
		_ = sendMagicPacket(WOLMacAddress)
	}

	// Reset deadline since we're going to proxy the connection regardless of whether the packet was held
	clientConn.SetReadDeadline(time.Time{})

	err = proxyData(clientConn, dst)
	if err != nil {
		log.Printf("Error during data proxying: %v", err)
	}
	clientConn.Close()
}

func startLocalListener() error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", LocalProxyPort))
	if err != nil {
		return err
	}
	defer listener.Close()

	log.Printf("Starting proxy on port %d...", LocalProxyPort)
	for {
		clientConn, err := listener.Accept()
		if err != nil {
			log.Printf("Error accepting connection: %v", err)
			continue
		}
		go handleClientConnection(clientConn)
	}
}

func main() {
	if len(os.Args) > 1 {
		WOLMacAddress = os.Args[1]
	}

	log.Println("Starting Wake-on-LAN Proxy...")
	if err := startLocalListener(); err != nil {
		log.Fatalf("Failed to start proxy: %v", err)
	}
}

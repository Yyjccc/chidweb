package client

import (
	"bytes"
	"chidweb/pkg/common"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

type Client struct {
	ServerURL           string          // HTTP communication server URL
	RemoteAddr          string          // TCP listening address of the server
	TargetAddr          string          // Target service address
	ClientID            string          // Client unique identifier
	HTTPClient          *http.Client    // HTTP client
	tcpConn             net.Conn        // TCP connection
	mutex               sync.RWMutex    // Mutex for protecting tcpConn
	sequence            uint32          // Sequence number counter
	connID              uint32          // Current connection ID
	sendBuffer          *bytes.Buffer   // Send buffer
	bufferMutex         sync.Mutex      // Buffer mutex
	done                chan struct{}   // Close signal
	autoConnect         bool            // Whether to automatically connect
	wg                  *sync.WaitGroup // Internal wait group
	isTunnelEstablished bool            // Whether the tunnel is established
	tunnelMutex         sync.Mutex      // Mutex for protecting tunnel status
}

// HeartbeatResponse Server returns heartbeat response
type HeartbeatResponse struct {
	Status string
	Data   []byte // Raw binary data
}

// HeartbeatRequest Sent to the server heartbeat request
type HeartbeatRequest struct {
	Data []byte // Raw binary data
}

// TCPAddress Represents the TCP connection address
type TCPAddress struct {
	Host string
	Port string
	Raw  string // Raw address string
}

func NewClient(serverURL, targetAddr, clientID string) *Client {
	return &Client{
		ServerURL:  serverURL,
		RemoteAddr: "127.0.0.1:0",
		TargetAddr: targetAddr,
		ClientID:   clientID,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		sendBuffer: &bytes.Buffer{},  // Initialize send buffer
		done:       make(chan struct{}),
		wg:         &sync.WaitGroup{},
	}
}

// Start Start the client
func (c *Client) Start() error {
	c.done = make(chan struct{})

	// Start heartbeat
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.heartbeatLoop()
	}()

	return nil
}

// Heartbeat loop
func (c *Client) heartbeatLoop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			if err := c.sendHeartbeat(); err != nil {
				common.Error("Heartbeat error: %v", err)
				continue
			}
		}
	}
}

// Monitor remote connection
func (c *Client) monitorRemoteConnection() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			if !c.IsTCPConnected() {
				if err := c.connectToRemote(); err != nil {
					common.Debug("Failed to connect to remote address: %v", err)
					continue
				}
			}
		}
	}
}

// Connect to remote address
func (c *Client) connectToRemote() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Ensure the previous connection is closed
	if c.tcpConn != nil {
		c.tcpConn.Close()
		c.tcpConn = nil
	}

	// Ensure HTTP tunnel is established
	if c.connID == 0 {
		return fmt.Errorf("HTTP tunnel not established")
	}

	conn, err := net.DialTimeout("tcp", c.TargetAddr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("Failed to connect to target address: %v", err)
	}

	c.tcpConn = conn
	common.Info("Connected to target: %s", c.TargetAddr)

	// Start data forwarding
	go c.handleRemoteConnection(conn)

	return nil
}

// Handle remote connection
func (c *Client) handleRemoteConnection(remoteConn net.Conn) {
	defer func() {
		remoteConn.Close()
		c.mutex.Lock()
		if c.tcpConn == remoteConn {
			c.tcpConn = nil
		}
		c.mutex.Unlock()
	}()

	buffer := make([]byte, 4096)
	for {
		select {
		case <-c.done:
			return
		default:
			n, err := remoteConn.Read(buffer)
			if err != nil {
				if err != io.EOF {
					common.Error("Failed to read remote data: %v", err)
				}
				return
			}

			if n > 0 {
				data := make([]byte, n)
				copy(data, buffer[:n])

				// Write data to send buffer
				c.bufferMutex.Lock()
				if c.sendBuffer != nil {
					_, err := c.sendBuffer.Write(data)
					if err != nil {
						common.Error("Failed to write to send buffer: %v", err)
					}
				}
				c.bufferMutex.Unlock()

				common.LogWithProbability(0.1, "debug", "Data written to send buffer [Length: %d]", n)
			}
		}
	}
}

// Send data packet
func (c *Client) sendPacket(packet *common.Packet) error {
	data, err := packet.Encode()
	if err != nil {
		return fmt.Errorf("Failed to encode data packet: %v", err)
	}

	resp, err := c.HTTPClient.Post(
		fmt.Sprintf("%s/heartbeat", c.ServerURL),
		"application/octet-stream",
		bytes.NewReader(data),
	)
	if err != nil {
		return fmt.Errorf("Failed to send data packet: %v", err)
	}
	defer resp.Body.Close()

	return nil
}

// Stop Stop the client
func (c *Client) Stop() {
	if c.done != nil {
		close(c.done)
	}

	// Close TCP connection
	c.mutex.Lock()
	if c.tcpConn != nil {
		c.tcpConn.Close()
		c.tcpConn = nil
	}
	c.mutex.Unlock()

	// Wait for all goroutines to complete
	c.Wait()
}

// Send heartbeat request
func (c *Client) sendHeartbeat() error {
	c.tunnelMutex.Lock()
	isTunnel := c.isTunnelEstablished
	c.tunnelMutex.Unlock()

	if isTunnel {
		return c.sendTunnelHeartbeat()
	}
	return c.sendInitialHeartbeat()
}

// Send initial heartbeat packet (when the tunnel is not established)
func (c *Client) sendInitialHeartbeat() error {
	heartbeatURL := fmt.Sprintf("%s/heartbeat", c.ServerURL)

	packet := &common.Packet{
		Type:     common.PacketTypeHeartbeat,
		ClientID: c.clientIDAsUint64(),
		ConnID:   c.connID,
		Sequence: c.nextSequence(),
	}

	data, err := packet.Encode()
	if err != nil {
		return fmt.Errorf("Failed to encode initial heartbeat: %v", err)
	}

	common.LogWithProbability(0.1, "debug", "Sending initial heartbeat [ClientID: %s]", c.ClientID)

	resp, err := c.HTTPClient.Post(
		heartbeatURL,
		"application/octet-stream",
		bytes.NewReader(data),
	)
	if err != nil {
		return fmt.Errorf("Initial heartbeat request failed: %v", err)
	}
	defer resp.Body.Close()

	// Handle response
	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("Failed to read initial heartbeat response: %v", err)
	}

	packets, err := common.DecodePackets(respData)
	if err != nil {
		return fmt.Errorf("Failed to decode initial heartbeat response: %v", err)
	}

	// Check if a request to establish a tunnel is received
	for _, packet := range packets {
		if packet.Type == common.PacketTypeConnect {
			if err := c.handleConnectPacket(packet); err != nil {
				return fmt.Errorf("Failed to handle connect request: %v", err)
			}
			// Update tunnel status
			c.tunnelMutex.Lock()
			c.isTunnelEstablished = true
			c.tunnelMutex.Unlock()
			
			common.Info("Tunnel established [ClientID: %s, ConnID: %d]", c.ClientID, c.connID)
			return nil
		}
	}

	return nil
}

// Send tunnel heartbeat packet (after the tunnel is established)
func (c *Client) sendTunnelHeartbeat() error {
	heartbeatURL := fmt.Sprintf("%s/heartbeat", c.ServerURL)

	// Check if there is pending data to send
	c.bufferMutex.Lock()
	hasData := c.sendBuffer != nil && c.sendBuffer.Len() > 0
	var pendingData []byte
	if hasData {
		pendingData = c.sendBuffer.Bytes()
		c.sendBuffer.Reset()
	}
	c.bufferMutex.Unlock()

	// Create heartbeat packet
	packet := &common.Packet{
		Type:     common.PacketTypeData,
		ClientID: c.clientIDAsUint64(),
		ConnID:   c.connID,
		Sequence: c.nextSequence(),
	}

	// If there is pending data, add it to the payload
	if hasData {
		packet.Payload = pendingData
		packet.Length = uint32(len(pendingData))
	}

	data, err := packet.Encode()
	if err != nil {
		return fmt.Errorf("Failed to encode tunnel heartbeat: %v", err)
	}

	common.LogWithProbability(0.1, "debug", "Sending tunnel heartbeat [ClientID: %s, ConnID: %d, DataLen: %d]",
		c.ClientID, c.connID, packet.Length)

	resp, err := c.HTTPClient.Post(
		heartbeatURL,
		"application/octet-stream",
		bytes.NewReader(data),
	)
	if err != nil {
		c.tunnelMutex.Lock()
		c.isTunnelEstablished = false
		c.tunnelMutex.Unlock()
		return fmt.Errorf("Tunnel heartbeat request failed: %v", err)
	}
	defer resp.Body.Close()

	// Handle response
	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("Failed to read tunnel heartbeat response: %v", err)
	}

	packets, err := common.DecodePackets(respData)
	if err != nil {
		return fmt.Errorf("Failed to decode tunnel heartbeat response: %v", err)
	}

	// Handle response data packets
	for _, packet := range packets {
		switch packet.Type {
		case common.PacketTypeData:
			if err := c.handleDataPacket(packet); err != nil {
				common.Error("Failed to handle data packet: %v", err)
			}
		case common.PacketTypeDisconnect:
			if err := c.handleDisconnectPacket(packet); err != nil {
				common.Error("Failed to handle disconnect request: %v", err)
			}
			return nil
		}
	}

	return nil
}

// Handle connect request packet
func (c *Client) handleConnectPacket(packet *common.Packet) error {
	c.mutex.Lock()
	// If a connection already exists, close it first
	if c.tcpConn != nil {
		c.tcpConn.Close()
		c.tcpConn = nil
	}
	c.mutex.Unlock()

	// Initialize send buffer
	c.bufferMutex.Lock()
	if c.sendBuffer == nil {
		c.sendBuffer = &bytes.Buffer{}
	} else {
		c.sendBuffer.Reset()
	}
	c.bufferMutex.Unlock()

	// Establish a new TCP connection
	if err := c.ConnectTCP(context.Background()); err != nil {
		return fmt.Errorf("Failed to establish TCP connection: %v", err)
	}

	// Start connection processing
	c.mutex.RLock()
	if c.tcpConn != nil {
		go c.handleRemoteConnection(c.tcpConn)
	}
	c.mutex.RUnlock()

	common.Info("Successfully established new TCP connection and data buffer")
	return nil
}

// Handle data packet
func (c *Client) handleDataPacket(packet *common.Packet) error {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if c.tcpConn == nil {
		return fmt.Errorf("TCP connection not established")
	}

	// Write data to local TCP connection
	if _, err := c.tcpConn.Write(packet.Payload); err != nil {
		return fmt.Errorf("Failed to write data to TCP connection: %v", err)
	}

	common.LogWithProbability(0.1, "debug", "Successfully forwarded data packet [Length: %d]", len(packet.Payload))
	return nil
}

// Handle disconnect request packet
func (c *Client) handleDisconnectPacket(packet *common.Packet) error {
	err := c.DisconnectTCP()
	
	// Clear send buffer
	c.bufferMutex.Lock()
	if c.sendBuffer != nil {
		c.sendBuffer.Reset()
	}
	c.bufferMutex.Unlock()

	// Update tunnel status
	c.tunnelMutex.Lock()
	c.isTunnelEstablished = false
	c.tunnelMutex.Unlock()
	
	return err
}

// Get sequence number
func (c *Client) nextSequence() uint32 {
	return atomic.AddUint32(&c.sequence, 1)
}

// Get connection ID
func (c *Client) nextConnID() uint32 {
	return atomic.AddUint32(&c.connID, 1)
}

// ClientIDAsUint64 Convert ClientID from string type to uint64
func (c *Client) clientIDAsUint64() uint64 {
	var hash uint64
	for _, b := range []byte(c.ClientID) {
		hash = hash*31 + uint64(b)
	}
	return hash
}

// GetServerAddress Get the server listening address
func (c *Client) GetServerAddress() (string, error) {
	resp, err := c.HTTPClient.Get(fmt.Sprintf("%s/address", c.ServerURL))
	if err != nil {
		return "", fmt.Errorf("Failed to get server address: %v", err)
	}
	defer resp.Body.Close()

	addr, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("Failed to read server address: %v", err)
	}

	return string(addr), nil
}

// GetTCPAddress Get the TCP address to connect to
func (c *Client) GetTCPAddress() (*TCPAddress, error) {
	//// Get server listening address
	//addrStr, err := c.GetServerAddress()
	//if err != nil {
	//	return nil, fmt.Errorf("Failed to get server address: %v", err)
	//}
	//
	//// Parse address
	//host, port, err := net.SplitHostPort(addrStr)
	//if err != nil {
	//	return nil, fmt.Errorf("Failed to parse address: %v", err)
	//}
	//
	//// Handle IPv6 address
	//if host == "::" {
	//	host = "localhost"
	//}
	//
	//return &TCPAddress{
	//	Host: host,
	//	Port: port,
	//	Raw:  addrStr,
	//}, nil
	return &TCPAddress{
		Host: "172.19.173.38",
		Port: "8088",
		Raw:  "172.19.173.38:8088",
	}, nil
}

// WaitForTCPAddress Wait for and get the TCP address
func (c *Client) WaitForTCPAddress(ctx context.Context) (*TCPAddress, error) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			addr, err := c.GetTCPAddress()
			if err != nil {
				common.Debug("Waiting for TCP address: %v", err)
				continue
			}
			return addr, nil
		}
	}
}

// ConnectTCP Connect to the TCP address of the server
func (c *Client) ConnectTCP(ctx context.Context) error {
	// Get TCP address
	addr, err := c.WaitForTCPAddress(ctx)
	if err != nil {
		return fmt.Errorf("Failed to get TCP address: %v", err)
	}

	// Create TCP connection
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
	}

	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(addr.Host, addr.Port))
	if err != nil {
		return fmt.Errorf("Failed to establish TCP connection: %v", err)
	}

	// Update connection status
	c.mutex.Lock()
	if c.tcpConn != nil {
		c.tcpConn.Close()
	}
	c.tcpConn = conn
	c.mutex.Unlock()

	common.Info("TCP connection established: %s", addr.Raw)
	return nil
}

// IsTCPConnected Check if TCP is connected
func (c *Client) IsTCPConnected() bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.tcpConn != nil
}

// DisconnectTCP Disconnect from the TCP connection
func (c *Client) DisconnectTCP() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.tcpConn != nil {
		err := c.tcpConn.Close()
		c.tcpConn = nil
		if err != nil {
			return fmt.Errorf("Failed to disconnect TCP connection: %v", err)
		}
		common.Info("TCP disconnected")
	}
	return nil
}

// Wait Wait for all goroutines to complete
func (c *Client) Wait() {
	if c.wg != nil {
		c.wg.Wait()
	}
}

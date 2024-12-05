package client

import (
	"chidweb/pkg/common"
	"fmt"
	"net"
	"sync"
	"time"
)

var (
	clientConfig *common.BasicConfig
)

func init() {
	clientConfig = common.DefaultConfig
}

type Client struct {
	ClientID  string
	manger    *TargetManger
	TargetMap map[uint32]uint32
	TunnelMap map[uint32]*TunnelClient
	dialer    net.Dialer
	done      chan struct{}   // Close signal
	wg        *sync.WaitGroup // Internal wait group
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

func NewClient(serverURL, clientID string, tcpAddrs []string, config *common.BasicConfig) *Client {
	if config != nil {
		clientConfig = config
	}
	client := &Client{
		ClientID: clientID,
		dialer: net.Dialer{
			Timeout: 30 * time.Second, // 设置超时时间为5秒
		},
		TunnelMap: make(map[uint32]*TunnelClient),
		TargetMap: make(map[uint32]uint32),
		done:      make(chan struct{}),
		wg:        &sync.WaitGroup{},
	}

	for _, tcpAddr := range tcpAddrs {
		target := NewTarget(tcpAddr)
		NewTunnelClient(client, serverURL, target)
	}

	return client
}

// Start Start the client
func (c *Client) Start() error {
	c.done = make(chan struct{})

	for _, cli := range c.TunnelMap {
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			common.Debug("start client main loop for target: %s", cli.target.addr.Raw)
			cli.heartbeatLoop()
		}()
	}

	return nil
}

// 连接目标
func (c *Client) connectToRemote(id uint32, channelID uint32) (*TunnelClient, error) {
	if tunnelID, ok := c.TargetMap[id]; ok {
		if tunnelClient, ok := c.TunnelMap[tunnelID]; ok {
			if !tunnelClient.isTunnelEstablished {
				tunnelClient.EstablishTunnel(channelID)
			}
			return tunnelClient, nil
		}
	}
	return nil, fmt.Errorf("target tunnel client not found")
}

func (c *Client) DisconnectCallback(tunnel *common.Tunnel) {

}

// Stop Stop the client
func (c *Client) Stop() {
	if c.done != nil {
		close(c.done)
	}
	// Wait for all goroutines to complete
	c.Wait()
}

// ClientIDAsUint64 Convert ClientID from string type to uint64
func (c *Client) clientIDAsUint64() uint64 {
	var hash uint64
	for _, b := range []byte(c.ClientID) {
		hash = hash*31 + uint64(b)
	}
	return hash
}

// Wait Wait for all goroutines to complete
func (c *Client) Wait() {
	if c.wg != nil {
		c.wg.Wait()
	}
}

func (c *Client) GetTunnelClient(id uint32) *TunnelClient {
	if tunnelClientID, ok := c.TargetMap[id]; ok {
		return c.TunnelMap[tunnelClientID]
	}
	return nil
}

func (c *Client) Dispatch(tunnelClient *TunnelClient, data []byte) {
	packets, err := common.DecodePackets(data)
	if err != nil {
		common.Error("Failed to decode packets: %v", err)
		return
	}
	for _, packet := range packets {
		if packet.ClientID != c.clientIDAsUint64() {
			return
		}
		switch packet.Type {
		case common.PacketTypeHeartbeat:
			common.LogWithProbability(0.1, "debug", "receive heartbeat response")
			break
		case common.PacketTypeData:
			err := tunnelClient.tunnel.Write(packet.Payload)
			if err != nil {
				common.Error("Failed to write packet: %v", err)
				return
			}
			common.LogWithProbability(0.1, "debug", "Successfully forwarded data packet [Length: %d]", len(packet.Payload))
			break
		case common.PacketTypeDisconnect:
			tunnelClient.CloseTunnel()
			break
		case common.PacketTypeConnect:
			_, err := c.connectToRemote(packet.TargetID, packet.ChannelID)
			if err != nil {
				common.Error("Failed to connect to remote: %v", err)
			}
			break
		default:
			common.Error("Unknown packet type: %v", packet.Type)
		}
	}
}

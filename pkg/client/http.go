package client

import (
	"bytes"
	"chidweb/pkg/common"
	"errors"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

var (
	heartbeatInterval = time.Second * 5
	heartbeatPath     = "/heartbeat"
	instance          *HttpClient
	once              sync.Once
	proxy             []string
)

const (
	POST_METHOD = "POST"
	GET_METHOD  = "GET"
)

type TunnelClient struct {
	client              *Client
	ServerAddr          string
	target              *Target
	ID                  uint32
	ticker              *time.Ticker
	sequence            uint32
	tunnel              *common.Tunnel
	done                chan struct{}
	isTunnelEstablished bool //隧道是否已经建立
}

func NewTunnelClient(client *Client, target *Target) *TunnelClient {
	tunnelClient := &TunnelClient{
		client:              client,
		ID:                  common.Generate32ID(),
		target:              target,
		ticker:              time.NewTicker(heartbeatInterval),
		done:                make(chan struct{}),
		isTunnelEstablished: false,
	}
	go tunnelClient.heartbeatLoop()
	client.TunnelMap[tunnelClient.ID] = tunnelClient
	client.TargetMap[target.ID] = tunnelClient.ID
	return tunnelClient
}

// 建立隧道
func (cli *TunnelClient) EstablishTunnel() {
	conn, err := cli.client.dialer.Dial("tcp", cli.target.addr.Raw)
	if err != nil {
		common.Error("tcp dial error : %v", err)
	}
	tunnel := common.NewTcpTunnel(conn, cli.client.DisconnectCallback)
	go tunnel.Listen()
	common.Info("tcp tunnel established to %s", conn.RemoteAddr().String())
	cli.tunnel = tunnel
	cli.isTunnelEstablished = true
}

// 维持心跳或者连接的主循环
func (cli *TunnelClient) heartbeatLoop() {
	defer cli.ticker.Stop()

	for {
		select {
		case <-cli.done:
			return
		case <-cli.ticker.C:
			if err := cli.MainLoop(); err != nil {
				common.Error("MainLoop error: %v", err)
				continue
			}
		}
	}
}

// 处理请求数据
func (http *TunnelClient) handlerBeforeSend(data []byte) []byte {
	return data
}

// 发送数据包
func (cli *TunnelClient) Send(data []byte) []byte {
	handleData := cli.handlerBeforeSend(data)
	resp, err := DoRequest(POST_METHOD, cli.ServerAddr+heartbeatPath, handleData)
	if err != nil {
		common.Error("Post error: %v", err)
	}
	defer resp.Body.Close()
	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		common.Error("Failed to read tunnel heartbeat response: %v", err)
		return nil
	}
	return cli.handleResponse(respData)
}

func (cli *TunnelClient) handleResponse(data []byte) []byte {

	return data
}

// 主循环
func (cli *TunnelClient) MainLoop() error {
	if cli.isTunnelEstablished {
		//已经建立隧道
		packet := &common.Packet{
			Type:      common.PacketTypeData,
			ClientID:  cli.client.clientIDAsUint64(),
			ChannelID: cli.tunnel.ID,
			Sequence:  cli.nextSequence(),
		}
		encode, err := packet.Encode()
		if err != nil {
			return err
		}
		cli.tunnel.BufferMutex.Lock()
		hasData := cli.tunnel.SendBuffer != nil && cli.tunnel.SendBuffer.Len() > 0
		var pendingData []byte
		if hasData {
			pendingData = cli.tunnel.SendBuffer.Bytes()
			cli.tunnel.SendBuffer.Reset()
		}
		cli.tunnel.BufferMutex.Unlock()

		if hasData {
			packet.Payload = pendingData
			packet.Length = uint32(len(pendingData))
		}
		respData := cli.Send(encode)
		common.LogWithProbability(0.1, "debug", "Sending tunnel heartbeat [ClientID: %s, tunnelID: %d, DataLen: %d]",
			cli.client.ClientID, cli.tunnel.ID, packet.Length)
		if respData == nil {
			return nil
		}
		cli.client.Dispatch(cli, respData)
	} else {
		//没有建立隧道
	}
	return nil
}

func (c *TunnelClient) nextSequence() uint32 {
	return atomic.AddUint32(&c.sequence, 1)
}

func (cli *TunnelClient) CloseTunnel() {
	//断开连接后继续发包
	cli.tunnel.Close()
	cli.isTunnelEstablished = false
	cli.tunnel = nil
}

// 关闭隧道和删除关联的数据
func (c *TunnelClient) Close() {
	c.CloseTunnel()
	close(c.done)
	delete(c.client.TunnelMap, c.ID)
	delete(c.client.TargetMap, c.target.ID)
}

// 全局 http 配置
type HttpClient struct {
	Proxy   []string //代理池
	Headers map[string]string
	client  *http.Client
	mu      sync.Mutex
}

func GetInstance() *HttpClient {
	once.Do(func() {
		header := make(map[string]string)
		header["Content-Type"] = "application/octet-stream"
		instance = &HttpClient{
			client: &http.Client{
				Timeout: 30 * time.Second,
			},
			Headers: make(map[string]string),
			Proxy:   proxy,
		}
	})
	return instance
}

// SetRandomProxy 随机选择代理并配置到 http.Client
func (hc *HttpClient) SetRandomProxy() error {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	if len(hc.Proxy) == 0 {
		return errors.New("proxy pool is empty")
	}

	// 随机选择代理
	rand.Seed(time.Now().UnixNano())
	randomProxy := hc.Proxy[rand.Intn(len(hc.Proxy))]

	// 解析代理 URL
	proxyURL, err := url.Parse(randomProxy)
	if err != nil {
		return err
	}

	// 设置 http.Client 的 Transport
	hc.client.Transport = &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
	}
	return nil
}

func (hc *HttpClient) addProxy(proxy string) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	hc.Proxy = append(hc.Proxy, proxy)
}

func DoRequest(method, url string, data []byte) (*http.Response, error) {
	cli := GetInstance()
	// 设置默认请求头
	req, err := http.NewRequest(method, url, bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}
	for key, value := range cli.Headers {
		req.Header.Set(key, value)
	}
	return cli.client.Do(req)
}

package client

import (
	"bytes"
	"chidweb/pkg/common"
	"errors"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	heartbeatInterval = time.Second * 5
	heartbeatPath     = "/heartbeat"
	instance          *HttpClient
	once              sync.Once
)

const (
	POST_METHOD = "POST"
	GET_METHOD  = "GET"
)

type TunnelClient struct {
	client              *Client
	ServerAddr          string
	ServerTunnelID      uint32 //服务端tunnel id
	target              *Target
	ID                  uint32
	ticker              *time.Ticker
	sequence            uint32
	tunnel              *common.Tunnel
	done                chan struct{}
	isTunnelEstablished bool //隧道是否已经建立
}

// 自动完成对Client的修改
func NewTunnelClient(client *Client, serverAddr string, target *Target) *TunnelClient {
	tunnelClient := &TunnelClient{
		client:              client,
		ServerAddr:          serverAddr,
		ID:                  common.Generate32ID(),
		target:              target,
		ticker:              time.NewTicker(heartbeatInterval),
		done:                make(chan struct{}),
		isTunnelEstablished: false,
	}
	//go tunnelClient.heartbeatLoop()
	client.TunnelMap[tunnelClient.ID] = tunnelClient
	client.TargetMap[target.ID] = tunnelClient.ID
	return tunnelClient
}

// SelectByProbability 从 []CustomConfig 中根据概率选择一个元素
func SelectByProbability() *common.CustomConfig {
	// 检查概率是否有效并计算总权重
	totalProbability := uint32(0)
	for _, config := range clientConfig.Custom {
		totalProbability += config.Probability
	}
	// 生成随机值
	rand.Seed(time.Now().UnixNano())
	randomValue := uint32(rand.Intn(int(totalProbability)))

	// 查找对应的配置
	currentSum := uint32(0)
	for _, config := range clientConfig.Custom {
		currentSum += config.Probability
		if randomValue < currentSum {
			return config
		}
	}
	return clientConfig.Custom[0]
}

// 建立隧道
func (cli *TunnelClient) EstablishTunnel(serverTunnelID uint32) {
	cli.ServerTunnelID = serverTunnelID
	conn, err := cli.client.dialer.Dial("tcp", cli.target.addr.Raw)
	if err != nil {
		common.Error("tcp dial error : %v", err)
		return
	}
	tunnel := common.NewTcpTunnel(conn, cli.client.DisconnectCallback)
	tunnel.ClientId = cli.ID
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

// 发送数据包
func (cli *TunnelClient) Send(data []byte) []byte {

	path := cli.ServerAddr + heartbeatPath
	var resp *http.Response
	var err error
	var config *common.CustomConfig
	//随机选取请求
	if clientConfig == nil {
		//没有自定义设置
		resp, err = DoRequest(POST_METHOD, path, data, nil)
		if err != nil {
			common.Error("Post error: %v", err)
		}
	} else {
		//随机选取一种加密方式
		config = SelectByProbability()
		TransformedData := data
		path = cli.ServerAddr + config.Path
		for e := config.RequestConfig.Transformers.Front(); e != nil; e = e.Next() {
			// 获取当前元素的值
			reqTransformer := e.Value.(common.Transformer)
			TransformedData = reqTransformer.Transform(TransformedData)
			if TransformedData == nil {
				common.Warn("will send empty data.")
				break
			}
		}
		var headers map[string]string = make(map[string]string)
		if config.RequestConfig.Headers != nil {
			for _, header := range config.RequestConfig.Headers {
				split := strings.Split(header, ":")
				headers[split[0]] = split[1]
			}
		}
		resp, err = DoRequest(config.Method, path, TransformedData, headers)
		if err != nil {
			common.Error("request error: %v", err)
		}
	}
	if resp == nil {
		return nil
	}
	defer resp.Body.Close()
	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		common.Error("Failed to read tunnel heartbeat response: %v", err)
		return nil
	}

	if path == cli.ServerAddr+heartbeatPath {
		return respData
	}
	var UnTransformedData = respData
	for e := config.ResponseConfig.Transformers.Back(); e != nil; e = e.Prev() {
		// 获取当前元素的值
		respTransformer := e.Value.(common.Transformer)
		UnTransformedData, err = respTransformer.UnTransform(UnTransformedData)
		if err != nil {
			common.Error("UnTransformedData error: %v", err)
			return nil
		}
	}
	return UnTransformedData
}

// 主循环
func (cli *TunnelClient) MainLoop() error {
	if cli.isTunnelEstablished {
		//已经建立隧道
		packet := &common.Packet{
			Type:      common.PacketTypeData,
			ClientID:  cli.client.clientIDAsUint64(),
			ChannelID: cli.ServerTunnelID,
			TargetID:  cli.target.ID,
			Sequence:  cli.nextSequence(),
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
		encode, err := packet.Encode()
		if err != nil {
			return err
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
		packet := &common.Packet{
			Type:     common.PacketTypeHeartbeat,
			ClientID: cli.client.clientIDAsUint64(),
			TargetID: cli.target.ID,
		}
		encode, err := packet.Encode()
		if err != nil {
			return err
		}
		respData := cli.Send(encode)
		common.LogWithProbability(0.1, "debug", "Sending  heartbeat [ClientID: %s]", cli.client.ClientID)
		if respData == nil {
			return nil
		}
		cli.client.Dispatch(cli, respData)
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
	Proxy       []string //代理池
	Headers     map[string]string
	client      *http.Client
	EnableProxy bool
	mu          sync.Mutex
}

func GetInstance() *HttpClient {
	once.Do(func() {
		header := make(map[string]string)
		header["Content-Type"] = "application/octet-stream"
		instance = &HttpClient{
			client: &http.Client{
				Timeout: 30 * time.Second,
			},
			Headers:     header,
			EnableProxy: clientConfig.EnableProxy,
			Proxy:       clientConfig.Proxies,
		}
	})
	return instance
}

// SetRandomProxy 设置代理到 http.Client，当只有一个代理时直接使用
func (hc *HttpClient) SetRandomProxy() error {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	if len(hc.Proxy) == 0 {
		return errors.New("proxy pool is empty")
	}

	var proxyStr string
	if len(hc.Proxy) == 1 {
		// 只有一个代理时直接使用
		proxyStr = hc.Proxy[0]
	} else {
		// 多个代理时随机选择
		rand.Seed(time.Now().UnixNano())
		proxyStr = hc.Proxy[rand.Intn(len(hc.Proxy))]
	}

	// 解析代理 URL
	proxyURL, err := url.Parse(proxyStr)
	if err != nil {
		return err
	}

	// 设置 http.Client 的 Transport
	hc.client.Transport = &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
	}
	return nil
}

func DoRequest(method, url string, data []byte, headers map[string]string) (*http.Response, error) {
	cli := GetInstance()

	// 如果启用了代理，在发送请求前设置随机代理
	if cli.EnableProxy {
		if err := cli.SetRandomProxy(); err != nil {
			common.Error("Failed to set random proxy: %v", err)
			// 即使设置代理失败，也继续发送请求
		}
	}

	// 设置默认请求头
	req, err := http.NewRequest(method, url, bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}
	for key, value := range cli.Headers {
		req.Header.Set(key, value)
	}
	if headers != nil {
		for key, value := range headers {
			req.Header.Set(key, value)
		}
	}
	return cli.client.Do(req)
}

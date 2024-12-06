package server

import (
	"chidweb/pkg/common"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	netMask       = ":"
	taskInterval  = time.Minute * 10
	cleanTimeout  = time.Minute * 5
	defaultPacket = common.Packet{
		Type:      common.PacketTypeHeartbeat,
		ClientID:  0,
		ChannelID: 0,
		TargetID:  0,
		Sequence:  0,
		Length:    0,
	}
)

type Server struct {
	config         *common.BasicConfig
	tunnels        map[uint32]*common.Tunnel
	listenerManger *TcpListenerManager
	httpPort       string
	HttpServer     *HttpServer
	done           chan struct{}
	wg             *sync.WaitGroup // 内部 WaitGroup
	clients        []*ActiveClient
	TunnelChan     chan *common.Tunnel
	tunnelClients  map[uint32]*ActiveClient
}

// 活跃的客户端
type ActiveClient struct {
	ClientID   uint64
	LastActive time.Time //上次活跃的时间
	tunnels    []uint32  //存在的隧道id
}

func (cli *ActiveClient) kill(server *Server) {
	for _, tunID := range cli.tunnels {
		delete(server.tunnelClients, tunID)
	}
}

func NewServer(config *common.BasicConfig, httpPort int, tcpPorts []string) *Server {
	var tcpAddrs []string
	if len(tcpPorts) == 0 {
		tcpAddrs = []string{":0"}
	} else {
		for _, tcpPort := range tcpPorts {
			tcpAddrs = append(tcpAddrs, netMask+tcpPort)
		}
	}
	manager := NewTcpListenerManager(tcpAddrs)
	return &Server{
		config:         config,
		tunnels:        make(map[uint32]*common.Tunnel),
		clients:        make([]*ActiveClient, 0),
		tunnelClients:  make(map[uint32]*ActiveClient),
		httpPort:       strconv.Itoa(httpPort),
		done:           make(chan struct{}),
		wg:             &sync.WaitGroup{},
		TunnelChan:     make(chan *common.Tunnel),
		listenerManger: manager,
	}
}

func (s *Server) Start() error {
	// 启动 HTTP 服务器
	s.HttpServer = NewHttpServer(s.httpPort)
	//默认路由
	s.HttpServer.RegisterHandler("/heartbeat", s.handleHeartbeat)
	if s.config != nil {
		s.addConfigHandler()
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.HttpServer.Start()
	}()

	s.listenerManger.StartAllListener(s)

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.clientCleaner()
	}()
	return nil
}

func (s *Server) Stop() {
	// 关闭信号通道
	close(s.done)

	// 关闭监听器
	s.listenerManger.StopAll()

	err := s.HttpServer.Stop()
	if err != nil {
		common.Error("%v", err)
	}

	// 等待所有协程完成
	s.wg.Wait()
}

// 客户端清理器
func (s *Server) clientCleaner() {
	ticker := time.NewTicker(taskInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			now := time.Now()
			for i, client := range s.clients {
				if now.Sub(client.LastActive) > cleanTimeout {
					s.clients = append(s.clients[:i], s.clients[i+1:]...)
					client.kill(s)
					common.Debug("client #%d has been cleaned up", client.ClientID)
				}
			}

		}
	}
}

// http 协议识别
func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	packets, err := common.DecodePackets(data)
	if err != nil {
		http.Error(w, "Failed to decode packets", http.StatusBadRequest)
		return
	}
	responsePackets := s.handlePackets(packets)
	// Respond with updated packets
	responseData, err := common.EncodePackets(responsePackets)
	if err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	_, err = w.Write(responseData)
	if err != nil {
		common.Error("%v", err)
	}
}

// 处理多个包
func (s *Server) handlePackets(packets []*common.Packet) []*common.Packet {
	var responsePackets []*common.Packet
	for _, packet := range packets {
		// Handle heartbeat and check existing connections
		responsePackets = append(responsePackets, s.parsePacket(packet)...)
	}
	return responsePackets
}

// 解析和处理数据包
func (s *Server) parsePacket(packet *common.Packet) []*common.Packet {
	var responsePackets []*common.Packet
	common.LogWithProbability(0.1, "debug", "server received packet,type: %v , clint id: %v", packet.Type, packet.ClientID)
	switch packet.Type {
	case common.PacketTypeHeartbeat:
		select {
		case tunnel := <-s.TunnelChan:
			pa := &common.Packet{
				Type:      common.PacketTypeConnect,
				ClientID:  packet.ClientID,
				ChannelID: tunnel.ID,
				TargetID:  packet.TargetID,
			}
			responsePackets = append(responsePackets, pa)
		default:
			p := &common.Packet{
				Type:      common.PacketTypeHeartbeat,
				ClientID:  packet.ClientID,
				ChannelID: packet.ChannelID,
				TargetID:  packet.TargetID,
				Sequence:  packet.Sequence,
				Length:    packet.Length,
			}
			responsePackets = append(responsePackets, p)
		}
		break
	//处理数据
	case common.PacketTypeData:
		//key := fmt.Sprintf("%d-%d", packet.ClientID, packet.ConnID)
		////如果已经存在连接
		if tunnel, ok := s.tunnels[packet.ChannelID]; ok {
			//tunnel := tunnelVal.(*Tunnel)
			err := tunnel.Write(packet.Payload)
			if err != nil {
				common.Error("Failed to write to tunnel ,%s", err.Error())
			}
			p := &common.Packet{
				Type:      common.PacketTypeData,
				ClientID:  packet.ClientID,
				ChannelID: tunnel.ID,
				TargetID:  packet.TargetID,
				Sequence:  packet.Sequence,
			}
			tunnel.BufferMutex.Lock()
			hasData := tunnel.SendBuffer != nil && tunnel.SendBuffer.Len() > 0
			var pendingData []byte
			if hasData {
				pendingData = tunnel.SendBuffer.Bytes()
				tunnel.SendBuffer.Reset()
			}
			tunnel.BufferMutex.Unlock()

			if hasData {
				p.Payload = pendingData
				p.Length = uint32(len(pendingData))
			}
			responsePackets = append(responsePackets, p)
		} else {
			common.Warn("not found tunnel [clintID %d] - [connID %d]", packet.ClientID, packet.ChannelID)
		}
		break
	case common.PacketTypeDisconnect:
		//客户端要求断开连接
		common.Info("receive disconnect packet, channelID: %d, targetID: %d", packet.ChannelID, packet.TargetID)
		if tunnel, ok := s.tunnels[packet.ChannelID]; ok {
			tunnel.Close()
		}
		c := &common.Packet{
			Type:      common.PacketTypeDisconnect,
			ClientID:  packet.ClientID,
			ChannelID: packet.ChannelID,
			TargetID:  packet.TargetID,
		}
		responsePackets = append(responsePackets, c)
		break
	default:
		common.Warn("Unknown packet type: %d", packet.Type)
	}
	if len(responsePackets) == 0 {
		//默认返回
		responsePackets = append(responsePackets, &defaultPacket)
	}
	return responsePackets
}

// 服务端主动断开连接时候执行的回调函数
func (s *Server) DisConnectCallBack(tunnel *common.Tunnel) {
	delete(s.tunnels, tunnel.ID)
	common.Info("server success close tunnel [tunnelID %v]", tunnel.ID)
}

func (s *Server) Wait() {
	s.wg.Wait()
}

// 添加配置中的路径处理器
func (s *Server) addConfigHandler() {
	for _, conf := range s.config.Custom {
		//为每一个配置注册路由
		s.HttpServer.RegisterHandler(conf.Path, func(w http.ResponseWriter, r *http.Request) {
			data, err := io.ReadAll(r.Body)
			if err != nil {
				common.Error("Failed to read request body")
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			unTransformData := data
			//解密请求数据
			for e := conf.RequestConfig.Transformers.Back(); e != nil; e = e.Prev() {
				// 获取当前元素的值
				reqTransformer := e.Value.(common.Transformer)
				unTransformData, err = reqTransformer.UnTransform(unTransformData)
				if unTransformData == nil || err != nil {
					common.Error("Failed to handle request data")
					w.WriteHeader(500)
					return
				}

			}

			packets, err := common.DecodePackets(unTransformData)
			if err != nil {
				common.Error("Failed to decode packets")
				w.WriteHeader(500)
				return
			}
			responsePackets := s.handlePackets(packets)
			// Respond with updated packets
			responseData, err := common.EncodePackets(responsePackets)
			if err != nil {
				http.Error(w, "Failed to encode response", http.StatusInternalServerError)
				return
			}

			//加密响应数据
			TransformData := responseData
			for e := conf.ResponseConfig.Transformers.Front(); e != nil; e = e.Next() {
				// 获取当前元素的值
				respTransformer := e.Value.(common.Transformer)
				TransformData = respTransformer.Transform(TransformData)
				if TransformData == nil {
					break
				}
			}

			//设置响应头
			if conf.ResponseConfig.Headers != nil {
				for _, header := range conf.ResponseConfig.Headers {
					split := strings.Split(header, ":")
					w.Header().Set(split[0], split[1])
				}
			}
			//设置状态码

			if conf.ResponseConfig.Codes != nil && len(conf.ResponseConfig.Codes) != 0 {
				// 设置随机种子
				rand.Seed(time.Now().UnixNano())

				// 从切片中随机选择一个元素
				randomIndex := rand.Intn(len(conf.ResponseConfig.Codes)) // 生成一个随机索引
				code := conf.ResponseConfig.Codes[randomIndex]           // 获取随机元素
				w.WriteHeader(code)
			}

			_, err = w.Write(TransformData)
			if err != nil {
				common.Error("%v", err)
			}

		})

	}
}

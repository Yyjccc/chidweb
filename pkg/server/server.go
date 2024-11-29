package server

import (
	"chidweb/pkg/common"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

var (
	netMask      = "127.0.0.1:"
	taskInterval = time.Minute * 10
	cleanTimeout = time.Minute * 5
)

type Server struct {
	tunnels        map[uint32]*common.Tunnel
	listenerManger *TcpListenerManager
	httpPort       string
	HttpServer     *HttpServer
	done           chan struct{}
	wg             *sync.WaitGroup // 内部 WaitGroup
	clients        []*ActiveClient
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

func NewServer(httpPort int, tcpPorts []string) *Server {
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
		tunnels:        make(map[uint32]*common.Tunnel),
		clients:        make([]*ActiveClient, 0),
		tunnelClients:  make(map[uint32]*ActiveClient),
		httpPort:       strconv.Itoa(httpPort),
		done:           make(chan struct{}),
		wg:             &sync.WaitGroup{},
		listenerManger: manager,
	}
}

func (s *Server) Start() error {
	// 启动 HTTP 服务器
	s.HttpServer = NewHttpServer(s.httpPort)
	s.HttpServer.RegisterHandler("/heartbeat", s.handleHeartbeat)
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

	var responsePackets []*common.Packet
	for _, packet := range packets {

		// Handle heartbeat and check existing connections
		responsePackets = append(responsePackets, s.parsePacket(packet)...)
	}
	if len(responsePackets) == 0 {
		defaultPacket := common.Packet{
			Type:      common.PacketTypeHeartbeat,
			ClientID:  0,
			ChannelID: 0,
			TargetID:  0,
			Sequence:  0,
			Length:    0,
		}
		responsePackets = append(responsePackets, &defaultPacket)
	}
	// Respond with updated packets
	responseData, err := common.EncodePackets(responsePackets)
	if err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(responseData)
}

// 解析和处理数据包
func (s *Server) parsePacket(packet *common.Packet) []*common.Packet {
	var responsePackets []*common.Packet
	switch packet.Type {
	case common.PacketTypeHeartbeat:
		p := &common.Packet{
			Type:      common.PacketTypeHeartbeat,
			ClientID:  packet.ClientID,
			ChannelID: packet.ChannelID,
			TargetID:  packet.TargetID,
			Sequence:  packet.Sequence,
			Length:    packet.Length,
		}
		responsePackets = append(responsePackets, p)
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
		} else {
			common.Warn("not found tunnel [clintID %d] - [connID %d]", packet.ClientID, packet.ChannelID)

		}
		break
	case common.PacketTypeDisconnect:
		//客户端要求断开连接

		break
	default:
		common.Warn("Unknown packet type: %d", packet.Type)
	}
	return responsePackets
}

// 服务端主动断开连接时候执行的回调函数
func (s *Server) DisConnectCallBack(tunnel *common.Tunnel) {

}

func (s *Server) Wait() {
	s.wg.Wait()
}

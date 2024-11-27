package server

import (
	"chidweb/pkg/common"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

type Connection struct {
	ClientID       uint64
	ConnID         uint32
	Conn           net.Conn
	PendingPackets chan *common.Packet // 待发送的数据包队列
	Done           chan struct{}       // 连接关闭信号
	LastActive     time.Time           // 最后活跃时间
}

type ServerConnectionManager struct {
	sync.Map
}

// Add these methods to ConnectionManager
func (cm *ServerConnectionManager) Store(key interface{}, value interface{}) {
	cm.Map.Store(key, value)
}

func (cm *ServerConnectionManager) Load(key interface{}) (interface{}, bool) {
	return cm.Map.Load(key)
}

func (cm *ServerConnectionManager) Delete(key interface{}) {
	cm.Map.Delete(key)
}

func (cm *ServerConnectionManager) Range(f func(key, value interface{}) bool) {
	cm.Map.Range(f)
}

type PendingConnection struct {
	conn       net.Conn
	clientChan chan *Connection
}

type Server struct {
	connections    *ServerConnectionManager
	httpHandler    *HTTPHandler
	httpPort       string
	listener       net.Listener
	done           chan struct{}
	wg             *sync.WaitGroup    // 内部 WaitGroup
	pendingConn    *PendingConnection // 新增：等待配对的TCP连接
	pendingConnMux sync.Mutex         // 新增：保护 pendingConn 的互斥锁
}

type HTTPHandler struct {
	server      *Server
	mux         *http.ServeMux
	middlewares []Middleware
}

type Middleware func(http.HandlerFunc) http.HandlerFunc

func NewServer(httpPort string) *Server {
	return &Server{
		connections: &ServerConnectionManager{},
		httpPort:    httpPort,
		done:        make(chan struct{}),
		wg:          &sync.WaitGroup{},
	}
}

func (s *Server) Start() error {
	// 创建 HTTP 服务器
	mux := http.NewServeMux()
	mux.HandleFunc("/heartbeat", s.handleHeartbeat)
	mux.HandleFunc("/address", func(w http.ResponseWriter, r *http.Request) {
		if s.listener != nil {
			fmt.Fprint(w, s.listener.Addr().String())
		}
	})

	// 启动 HTTP 服务器
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		server := &http.Server{
			Addr:    ":" + s.httpPort,
			Handler: mux,
		}
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			common.Error("HTTP server error: %v", err)
		}
	}()

	// 等待HTTP服务器动
	time.Sleep(time.Second)

	// 创建 TCP 监听器
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return fmt.Errorf("failed to create TCP listener: %v", err)
	}
	s.listener = listener
	common.Info("TCP listener started on %s", listener.Addr().String())

	// 启动 TCP 连接接收处理
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			select {
			case <-s.done:
				return
			default:
				conn, err := s.listener.Accept()
				if err != nil {
					if !errors.Is(err, net.ErrClosed) {
						common.Error("Accept error: %v", err)
					}
					continue
				}

				common.Debug("New TCP connection from %s", conn.RemoteAddr().String())

				// 保存新连接，等待客户端建立隧道
				s.pendingConnMux.Lock()
				if s.pendingConn != nil {
					// 如果已经有待处理的连接，关闭它
					s.pendingConn.conn.Close()
				}
				s.pendingConn = &PendingConnection{
					conn:       conn,
					clientChan: make(chan *Connection),
				}
				s.pendingConnMux.Unlock()
			}
		}
	}()

	// 启动连接清理器
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.connectionCleaner()
	}()

	return nil
}

// 新增 Wait 方法
func (s *Server) Wait() {
	if s.wg != nil {
		s.wg.Wait()
	}
}

// 改 Shutdown 方法
func (s *Server) Stop() {
	// 关闭信号通道
	if s.done != nil {
		close(s.done)
	}

	// 关闭监听器
	if s.listener != nil {
		s.listener.Close()
	}

	// 关闭所有连接
	s.connections.Range(func(key, value interface{}) bool {
		conn := value.(*Connection)
		s.handleDisconnect(&common.Packet{
			ClientID: conn.ClientID,
			ConnID:   conn.ConnID,
		})
		return true
	})

	// 等待所有协程完成
	s.Wait()
}

// 新增：连接清理器
func (s *Server) connectionCleaner() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			now := time.Now()
			s.connections.Range(func(key, value interface{}) bool {
				conn := value.(*Connection)
				if now.Sub(conn.LastActive) > 5*time.Minute {
					common.WithFields(logrus.Fields{
						"clientID": conn.ClientID,
						"connID":   conn.ConnID,
					}).Info("Cleaning expired connection")
					s.handleDisconnect(&common.Packet{
						ClientID: conn.ClientID,
						ConnID:   conn.ConnID,
					})
				}
				return true
			})
		}
	}
}

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
		// 接收心跳日志
		common.LogWithProbability(0.1, "debug", "Received heartbeat [ClientID: %d, ConnID: %d]",
			packet.ClientID, packet.ConnID)

		switch packet.Type {
		case common.PacketTypeHeartbeat:
			key := fmt.Sprintf("%d-%d", packet.ClientID, packet.ConnID)
			if connVal, ok := s.connections.Load(key); ok {
				conn := connVal.(*Connection)
				conn.LastActive = time.Now()

				// 检查是否有待发送的数据
				select {
				case packet := <-conn.PendingPackets:
					responsePackets = append(responsePackets, packet)
					common.LogWithProbability(0.1, "debug", "Sending pending packet [ClientID: %d, ConnID: %d, Length: %d]",
						packet.ClientID, packet.ConnID, packet.Length)
				default:
					// 没有待发送的数据，继续处理
				}

				 common.LogWithProbability(0.1, "debug", "Updated connection last active time [ClientID: %d, ConnID: %d]",
					packet.ClientID, packet.ConnID)
			} else {
				// 检查是否有待处理的连接
				s.pendingConnMux.Lock()
				if s.pendingConn != nil {
					// 创建新的连接对象
					conn := &Connection{
						ClientID:       packet.ClientID,
						ConnID:         packet.ConnID,
						Conn:           s.pendingConn.conn,
						PendingPackets: make(chan *common.Packet, 1000),
						Done:           make(chan struct{}),
						LastActive:     time.Now(),
					}

					// 存储连接
					s.connections.Store(key, conn)

					// 通知客户端建立隧道
					responsePackets = append(responsePackets, &common.Packet{
						Type:     common.PacketTypeConnect,
						ClientID: packet.ClientID,
						ConnID:   packet.ConnID,
					})

					// 启动数据处理
					go s.handleTCPConnection(conn)

					// 清除待处理连接
					s.pendingConn = nil
					common.Debug("New tunnel established [ClientID: %d, ConnID: %d]",
						packet.ClientID, packet.ConnID)
				}
				s.pendingConnMux.Unlock()
			}

		case common.PacketTypeData:
			if err := s.handleData(packet); err != nil {
				common.Error("Failed to handle data: %v", err)
			}

			// 检查是否有响应数据
			key := fmt.Sprintf("%d-%d", packet.ClientID, packet.ConnID)
			if connVal, ok := s.connections.Load(key); ok {
				conn := connVal.(*Connection)
				// 非阻塞方式检查待发送数据
				select {
				case packet := <-conn.PendingPackets:
					responsePackets = append(responsePackets, packet)
					common.LogWithProbability(0.1, "debug", "Sending response packet [ClientID: %d, ConnID: %d, Length: %d]",
						packet.ClientID, packet.ConnID, packet.Length)
				default:
					// 没有待发送的数据
				}
			}

		case common.PacketTypeDisconnect:
			s.handleDisconnect(packet)
		}
	}

	// 编码并发送响应
	responseData, err := common.EncodePackets(responsePackets)
	if err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}

	// 发送心跳响应日志
	if len(packets) > 0 {
		packet := packets[0]
		common.LogWithProbability(0.1, "debug", "Sent heartbeat response [ClientID: %d, ConnID: %d, ResponsePackets: %d]",
			packet.ClientID, packet.ConnID, len(responsePackets))
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(responseData)
}

func (s *Server) handleConnect(packet *common.Packet) (*Connection, error) {
	return s.handleTunnelConnection(packet)
}

func (s *Server) handleData(packet *common.Packet) error {
	key := fmt.Sprintf("%d-%d", packet.ClientID, packet.ConnID)
	if connVal, ok := s.connections.Load(key); ok {
		conn := connVal.(*Connection)
		conn.LastActive = time.Now()

		if conn.Conn != nil {
			_, err := conn.Conn.Write(packet.Payload)
			if err != nil {
				common.WithFields(logrus.Fields{
					"error":    err,
					"clientID": conn.ClientID,
					"connID":   conn.ConnID,
				}).Error("Failed to write data")
				return err
			}
			return nil
		}
	}
	return fmt.Errorf("connection not found")
}

func (s *Server) handleDisconnect(packet *common.Packet) {
	key := fmt.Sprintf("%d-%d", packet.ClientID, packet.ConnID)
	if connVal, ok := s.connections.Load(key); ok {
		conn := connVal.(*Connection)
		if conn.Conn != nil {
			conn.Conn.Close()
		}
		close(conn.Done)
		close(conn.PendingPackets)
		s.connections.Delete(key)

		common.WithFields(logrus.Fields{
			"clientID": conn.ClientID,
			"connID":   conn.ConnID,
		}).Info("Connection closed")
	}
}

func (s *Server) handleTCPConnection(conn *Connection) {
	buffer := make([]byte, 4096)
	sequence := uint32(0)

	defer func() {
		conn.Conn.Close()
		close(conn.Done)
		close(conn.PendingPackets)
		key := fmt.Sprintf("%d-%d", conn.ClientID, conn.ConnID)
		s.connections.Delete(key)
		common.Info("TCP connection closed [ClientID: %d, ConnID: %d]", conn.ClientID, conn.ConnID)
	}()

	// 启动数据读取goroutine
	go func() {
		for {
			select {
			case <-conn.Done:
				return
			default:
				n, err := conn.Conn.Read(buffer)
				if err != nil {
					if err != io.EOF {
						common.Error("Failed to read TCP data [ClientID: %d, ConnID: %d]: %v",
							conn.ClientID, conn.ConnID, err)
					}
					return
				}

				if n > 0 {
					data := make([]byte, n)
					copy(data, buffer[:n])

					packet := &common.Packet{
						Type:     common.PacketTypeData,
						ClientID: conn.ClientID,
						ConnID:   conn.ConnID,
						Sequence: sequence,
						Length:   uint32(n),
						Payload:  data,
					}
					sequence++

					select {
					case conn.PendingPackets <- packet:
						common.LogWithProbability(0.1, "debug", "Packet added to pending queue [ClientID: %d, ConnID: %d, Length: %d]",
							conn.ClientID, conn.ConnID, n)
					default:
						common.Warn("Packet buffer full [ClientID: %d, ConnID: %d]", conn.ClientID, conn.ConnID)
					}
				}
			}
		}
	}()

	// 等待连接关闭
	<-conn.Done
}

// 新增：TCP 连接接受处理
func (s *Server) acceptConnections() {
	// 只接受一个连接
	conn, err := s.listener.Accept()
	if err != nil {
		if !errors.Is(err, net.ErrClosed) {
			common.Error("Accept error: %v", err)
		}
		return
	}

	common.Info("Accepted TCP connection from %s", conn.RemoteAddr().String())

	// 创建等待配对的连接
	s.pendingConnMux.Lock()
	s.pendingConn = &PendingConnection{
		conn:       conn,
		clientChan: make(chan *Connection),
	}
	s.pendingConnMux.Unlock()

	// 等待连接关闭信号
	<-s.done
	conn.Close()
}

func (s *Server) handleTunnelConnection(packet *common.Packet) (*Connection, error) {
	// Create new connection object
	conn := &Connection{
		ClientID:       packet.ClientID,
		ConnID:         packet.ConnID,
		PendingPackets: make(chan *common.Packet, 1000),
		Done:           make(chan struct{}),
		LastActive:     time.Now(),
	}

	// Wait for TCP connection from client
	tcpConn, err := s.listener.Accept()
	if err != nil {
		return nil, fmt.Errorf("failed to accept TCP connection: %v", err)
	}

	conn.Conn = tcpConn
	key := fmt.Sprintf("%d-%d", packet.ClientID, packet.ConnID)
	s.connections.Store(key, conn)

	common.WithFields(logrus.Fields{
		"clientID": conn.ClientID,
		"connID":   conn.ConnID,
		"addr":     conn.Conn.RemoteAddr().String(),
	}).Info("Tunnel connection established")

	return conn, nil
}

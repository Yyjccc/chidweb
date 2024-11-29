package server

import (
	"chidweb/pkg/common"
	"errors"
	"net"
	"sync"
)

type TcpListenerManager struct {
	listeners map[string]net.Listener
	wg        *sync.WaitGroup
	addrs     []string
}

func NewTcpListenerManager(addrs []string) *TcpListenerManager {
	return &TcpListenerManager{
		listeners: make(map[string]net.Listener),
		wg:        &sync.WaitGroup{},
		addrs:     addrs,
	}
}

func (manager *TcpListenerManager) StopAll() {
	for _, listener := range manager.listeners {
		err := listener.Close()
		if err != nil {
			common.Error("Error closing listener %s", err.Error())
		}
	}
}

// 开启所有tcp端口监听
func (manager *TcpListenerManager) StartAllListener(server *Server) {
	for _, addr := range manager.addrs {
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			common.Error("failed to create TCP listener: %v", err)
		}
		common.Info("TCP listener started on %s", listener.Addr().String())
		manager.listeners[addr] = listener
		manager.wg.Add(1)
		go manager.acceptConnections(listener, server)
	}
}

func (manager *TcpListenerManager) acceptConnections(listener net.Listener, s *Server) {
	defer func() {
		manager.wg.Done()
		if r := recover(); r != nil {
			common.Error("Recovered from panic: %v", r)
		}
	}()
	for {
		conn, err := listener.Accept()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				common.Error("Accept error: %v", err)
			}
			return
		}
		common.Info("Accepted TCP connection from %s", conn.RemoteAddr().String())
		tunnel := common.NewTcpTunnel(conn, s.DisConnectCallBack)
		//开启隧道
		tunnel.Listen()
	}
}

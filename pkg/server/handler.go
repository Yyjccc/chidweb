package server

import (
	"chidweb/pkg/common"
	"net/http"
	"sync"
)

// http服务器
type HttpServer struct {
	Server       *http.Server
	port         string
	defaultHeart *common.Packet
	handlers     *http.ServeMux
	running      bool
	muxMutex     sync.Mutex // 保护 mux 的修改
}

func NewHttpServer(port string) *HttpServer {
	mux := http.NewServeMux()
	return &HttpServer{
		Server: &http.Server{
			Addr:    ":" + port,
			Handler: mux,
		},
		port:     port,
		handlers: mux,
		running:  false,
	}

}

func (s *HttpServer) Start() {
	s.running = true
	common.Info("[http] server on port %v", s.port)
	if err := s.Server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		common.Error("HTTP server error:", err)
	}

}

func (s *HttpServer) RegisterHandler(path string, handler func(http.ResponseWriter, *http.Request)) {
	// 加锁，确保线程安全
	s.muxMutex.Lock()
	defer s.muxMutex.Unlock()
	s.handlers.HandleFunc(path, handler)
	common.Info("http server register path: %s", path)
}

// 停止 HTTP 服务器
func (s *HttpServer) Stop() error {
	s.running = false
	common.Info("[http] server close")
	// 等待服务器关闭，释放资源
	if err := s.Server.Close(); err != nil {
		return err
	}
	return nil
}

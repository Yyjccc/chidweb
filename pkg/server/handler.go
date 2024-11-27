package server

import (
	"net/http"
)

type HTTPAPIHandler struct {
	server      *Server
	mux         *http.ServeMux
	middlewares []Middleware
}

func NewHTTPAPIHandler(server *Server) *HTTPAPIHandler {
	h := &HTTPAPIHandler{
		server: server,
		mux:    http.NewServeMux(),
	}
	h.setupRoutes()
	return h
}

func (h *HTTPAPIHandler) setupRoutes() {
	// 基础路由
	h.mux.HandleFunc("/heartbeat", h.withMiddlewares(h.handleHeartbeat))
	h.mux.HandleFunc("/address", h.withMiddlewares(h.handleAddress))

	// 伪装路由 (为后续扩展准备)
	h.mux.HandleFunc("/", h.withMiddlewares(h.handleFakeStatic))
}

func (h *HTTPAPIHandler) withMiddlewares(handler http.HandlerFunc) http.HandlerFunc {
	for i := len(h.middlewares) - 1; i >= 0; i-- {
		handler = h.middlewares[i](handler)
	}
	return handler
}

// 添加中间件
func (h *HTTPAPIHandler) Use(middleware Middleware) {
	h.middlewares = append(h.middlewares, middleware)
}

// handleFakeStatic handles requests to the root path
func (h *HTTPAPIHandler) handleFakeStatic(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}

// handleAddress handles address-related requests
func (h *HTTPAPIHandler) handleAddress(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// handleHeartbeat handles heartbeat requests
func (h *HTTPAPIHandler) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

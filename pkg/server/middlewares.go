package server

import (
	"chidweb/pkg/common"
	"net/http"
	"time"
)

// LoggingMiddleware 日志中间件
func LoggingMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next(w, r)
		common.Info("HTTP %s %s %v", r.Method, r.URL.Path, time.Since(start))
	}
}

// AuthMiddleware 认证中间件
func AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// TODO: 实现认证逻辑
		next(w, r)
	}
}

// FakeHeadersMiddleware 伪装头部中间件
func FakeHeadersMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "nginx/1.18.0")
		w.Header().Set("X-Powered-By", "PHP/7.4.3")
		next(w, r)
	}
}

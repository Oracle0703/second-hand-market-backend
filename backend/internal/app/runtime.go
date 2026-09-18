package app

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"second-hand-market-backend/backend/internal/common"
)

// Liveness stays cheap. Readiness checks dependencies without disclosing DSNs.
func (s *Server) readiness(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	if s.DB != nil {
		db, err := s.DB.DB()
		if err == nil && db.PingContext(ctx) == nil {
			common.Success(c, gin.H{"status": "ready"})
			return
		}
	}
	c.JSON(http.StatusServiceUnavailable, common.APIResponse{Code: common.CodeInternal, Message: "service not ready", RequestID: common.RequestIDFromContext(c)})
}

func requestLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		// Log route templates, not URLs/query strings, bodies, headers or credentials.
		slog.Info("http_request", "request_id", common.RequestIDFromContext(c), "method", c.Request.Method, "route", c.FullPath(), "status", c.Writer.Status(), "duration_ms", time.Since(start).Milliseconds())
	}
}

func (s *Server) RunContext(ctx context.Context) error {
	listener, err := net.Listen("tcp", s.cfg.Addr)
	if err != nil {
		return err
	}
	defer closeDatabase(s.DB)
	server := &http.Server{Handler: s.Router, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	// No global short write deadline: image processing has its own bounded context.
	return serveWithShutdown(ctx, server, listener)
}

func serveWithShutdown(ctx context.Context, server *http.Server, listener net.Listener) error {
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := server.Shutdown(shutdown)
		if err != nil {
			_ = server.Close()
		}
		<-done
		return err
	}
}

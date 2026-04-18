package gateway

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/carlosmaranje/goclaw/internal/agent"
	"github.com/carlosmaranje/goclaw/internal/orchestrator"
)

type Server struct {
	socketPath string
	registry   *agent.Registry
	runners    map[string]*agent.Runner
	dispatcher *orchestrator.Dispatcher

	httpSrv *http.Server
	ln      net.Listener
}

func NewServer(socketPath string, reg *agent.Registry, runners map[string]*agent.Runner, d *orchestrator.Dispatcher) *Server {
	return &Server{
		socketPath: socketPath,
		registry:   reg,
		runners:    runners,
		dispatcher: d,
	}
}

func (s *Server) Start(ctx context.Context) error {
	if err := os.MkdirAll(filepath.Dir(s.socketPath), 0o755); err != nil {
		return fmt.Errorf("create socket dir: %w", err)
	}
	_ = os.Remove(s.socketPath)

	ln, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("listen unix %s: %w", s.socketPath, err)
	}
	if err := os.Chmod(s.socketPath, 0o660); err != nil {
		log.Printf("gateway: chmod socket: %v", err)
	}
	s.ln = ln

	mux := http.NewServeMux()
	s.registerRoutes(mux)
	s.httpSrv = &http.Server{
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	go func() {
		if err := s.httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("gateway: serve: %v", err)
		}
	}()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.httpSrv.Shutdown(shutdownCtx)
		_ = os.Remove(s.socketPath)
	}()

	return nil
}

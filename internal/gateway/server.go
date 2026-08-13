package gateway

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/carlosmaranje/mango/core"
)

type Server struct {
	socketPath string
	httpAddr   string
	engine     *core.Engine

	httpSrv *http.Server
	tcpSrv  *http.Server
	ln      net.Listener
	tcpLn   net.Listener
}

func NewServer(socketPath, httpAddr string, engine *core.Engine) *Server {
	return &Server{
		socketPath: socketPath,
		httpAddr:   httpAddr,
		engine:     engine,
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
		WriteTimeout: 0,
	}

	go func() {
		if err := s.httpSrv.Serve(ln); err != nil && !errors.Is(http.ErrServerClosed, err) {
			log.Printf("gateway: serve: %v", err)
		}
	}()

	if s.httpAddr != "" {
		tcpLn, err := net.Listen("tcp", s.httpAddr)
		if err != nil {
			return fmt.Errorf("listen tcp %s: %w", s.httpAddr, err)
		}
		s.tcpLn = tcpLn
		s.tcpSrv = &http.Server{
			Handler:      corsMiddleware(mux),
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 0,
		}
		go func() {
			if err := s.tcpSrv.Serve(tcpLn); err != nil && err != http.ErrServerClosed {
				log.Printf("gateway: tcp serve: %v", err)
			}
		}()
		log.Printf("gateway: also listening on http://%s", s.httpAddr)
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.httpSrv.Shutdown(shutdownCtx)
		if s.tcpSrv != nil {
			_ = s.tcpSrv.Shutdown(shutdownCtx)
		}
		_ = os.Remove(s.socketPath)
	}()

	return nil
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

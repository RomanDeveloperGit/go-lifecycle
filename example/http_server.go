package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"time"
)

// httpServer оборачивает стандартный http.Server в компонент lifecycle.
type httpServer struct {
	addr string
	srv  *http.Server
}

func (s *httpServer) Startup(context.Context) error {
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.addr, err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	})

	s.srv = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		if err := s.srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("http: serve: %v", err)
		}
	}()

	log.Printf("http: listening on http://%s/health", listener.Addr())

	return nil
}

func (s *httpServer) Shutdown(ctx context.Context) error {
	if s.srv == nil {
		return nil
	}

	log.Print("http: waiting for active requests to finish")

	return s.srv.Shutdown(ctx)
}

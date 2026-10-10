package observability

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

func Server(addr string, metrics *Metrics) *http.Server {
	return &http.Server{Addr: addr, Handler: metrics.Handler(), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second}
}

// Serve binds every listener before serving and shares one shutdown deadline.
func Serve(ctx context.Context, timeout time.Duration, servers ...*http.Server) error {
	listeners := make([]net.Listener, 0, len(servers))
	for _, server := range servers {
		listener, err := net.Listen("tcp", server.Addr)
		if err != nil {
			for _, open := range listeners {
				_ = open.Close()
			}
			return fmt.Errorf("listen %s: %w", server.Addr, err)
		}
		listeners = append(listeners, listener)
	}
	errorsCh := make(chan error, len(servers))
	for i, server := range servers {
		go func() { errorsCh <- server.Serve(listeners[i]) }()
	}
	var serveErr error
	select {
	case <-ctx.Done():
	case serveErr = <-errorsCh:
	}
	if errors.Is(serveErr, http.ErrServerClosed) {
		serveErr = nil
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var wg sync.WaitGroup
	shutdownErrors := make(chan error, len(servers))
	for _, server := range servers {
		wg.Go(func() {
			if err := server.Shutdown(shutdownCtx); err != nil {
				shutdownErrors <- err
				_ = server.Close()
			}
		})
	}
	wg.Wait()
	close(shutdownErrors)
	for err := range shutdownErrors {
		serveErr = errors.Join(serveErr, err)
	}
	return serveErr
}

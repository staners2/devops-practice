package observability

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestServeReturnsBindFailureAndClosesOtherListeners(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	free, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := free.Addr().String()
	_ = free.Close()
	err = Serve(context.Background(), time.Second,
		&http.Server{Addr: addr}, &http.Server{Addr: occupied.Addr().String()})
	if err == nil {
		t.Fatal("bind failure ignored")
	}
	check, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("listener leaked: %v", err)
	}
	_ = check.Close()
}

func TestServeShutsDownAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Serve(ctx, time.Second, &http.Server{Addr: "127.0.0.1:0", Handler: New("worker").Handler()}); err != nil {
		t.Fatalf("graceful shutdown: %v", err)
	}
}

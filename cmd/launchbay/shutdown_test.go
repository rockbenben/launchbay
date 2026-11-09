package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestServeWaitsForActiveRequestDuringShutdown(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	srv := &http.Server{Addr: addr, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		select {
		case <-release:
			_, _ = io.WriteString(w, "finished")
		case <-r.Context().Done():
		}
	})}
	t.Cleanup(func() { _ = srv.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	served := make(chan error, 1)
	go func() { served <- serveUntilStopped(ctx, srv, func() {}) }()
	response := make(chan string, 1)
	go func() {
		client := &http.Client{Timeout: 5 * time.Second}
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			resp, err := client.Get("http://" + addr)
			if err != nil {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			raw, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			response <- string(raw)
			return
		}
		response <- "request failed"
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("request never reached the server")
	}
	cancel()
	select {
	case err := <-served:
		t.Fatalf("serve returned before the active request finished: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	unblock()
	select {
	case got := <-response:
		if got != "finished" {
			t.Errorf("active response was lost: %q", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("active response did not finish")
	}
	select {
	case err := <-served:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not finish after the active request")
	}
}

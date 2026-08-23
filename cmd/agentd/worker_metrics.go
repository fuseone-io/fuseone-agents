package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/fuseone/agents/internal/worker"
)

func startWorkerMetrics(ctx context.Context, addr string, metrics *worker.MetricsRegistry) error {
	if addr == "" {
		return nil
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen for worker metrics: %w", err)
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           metrics,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Warn("worker metrics shutdown failed", "err", err)
		}
	}()
	go func() {
		slog.Info("worker metrics started", "addr", addr)
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("worker metrics stopped", "err", err)
		}
	}()
	return nil
}

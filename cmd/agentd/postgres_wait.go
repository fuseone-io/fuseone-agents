package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// The serve chart startupProbe gives the process 90 seconds (3s * 30).
	// Keep this comfortably below that budget: after Postgres answers, serve
	// still has to open the store, run migrations, build identity, and bind
	// port 8080 before Kubernetes can observe readiness.
	postgresStartupTimeout  = 45 * time.Second
	postgresStartupInterval = 3 * time.Second
)

func waitForStartupPostgres(ctx context.Context, dsn string) error {
	if dsn == "" {
		return nil
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("parse postgres dsn: %w", err)
	}
	return waitForReady(ctx, "postgres", postgresStartupTimeout,
		postgresStartupInterval, func(ctx context.Context) error {
			return pingPostgres(ctx, cfg)
		}, sleepContext)
}

func waitForReady(
	ctx context.Context, name string, timeout, interval time.Duration,
	check func(context.Context) error, wait func(context.Context, time.Duration) error,
) error {
	deadline, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var last error
	for attempt := 1; ; attempt++ {
		if err := check(deadline); err == nil {
			if attempt > 1 {
				slog.Info(name+" answered", "attempts", attempt)
			}
			return nil
		} else {
			last = err
		}
		slog.Warn(name+" is not ready yet", "attempt", attempt)
		if err := wait(deadline, interval); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return fmt.Errorf("%s did not become ready within %s: %w", name, timeout, last)
			}
			return fmt.Errorf("%s wait cancelled: %w", name, last)
		}
	}
}

func pingPostgres(ctx context.Context, cfg *pgxpool.Config) error {
	cfg = cfg.Copy()
	cfg.MaxConns = 1
	cfg.MinConns = 0
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()

	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return pool.Ping(pingCtx)
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

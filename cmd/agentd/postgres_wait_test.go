package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestWaitForReadyRetriesUntilTheServiceAnswers(t *testing.T) {
	attempts := 0
	waits := 0
	err := waitForReady(t.Context(), "postgres", time.Minute, time.Second,
		func(context.Context) error {
			attempts++
			if attempts < 3 {
				return errors.New("still starting")
			}
			return nil
		},
		func(_ context.Context, d time.Duration) error {
			waits++
			if d != time.Second {
				t.Fatalf("waited %s, want %s", d, time.Second)
			}
			return nil
		})
	if err != nil {
		t.Fatalf("waitForReady = %v", err)
	}
	if attempts != 3 || waits != 2 {
		t.Fatalf("attempts/waits = %d/%d, want 3/2", attempts, waits)
	}
}

func TestWaitForReadyReportsTheLastErrorWhenTheBudgetExpires(t *testing.T) {
	sentinel := errors.New("connection refused")
	err := waitForReady(t.Context(), "postgres", time.Minute, time.Second,
		func(context.Context) error { return sentinel },
		func(context.Context, time.Duration) error { return context.DeadlineExceeded })
	if err == nil {
		t.Fatal("waitForReady succeeded")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("waitForReady = %v, want the last readiness error", err)
	}
	if !strings.Contains(err.Error(), "within 1m0s") {
		t.Fatalf("waitForReady = %v, want the startup budget named", err)
	}
}

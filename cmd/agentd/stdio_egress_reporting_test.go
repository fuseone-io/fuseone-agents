package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/egress"
	"github.com/fuseone/agents/internal/egressmetrics"
)

func TestStdioEgressReporting_buffersAndFlushesWithAnIndependentContext(t *testing.T) {
	store := &fakeStdioEgressStore{}
	reporting := newStdioEgressReporting(store, nil)
	now := time.Date(2026, 8, 24, 15, 0, 0, 0, time.UTC)
	reporting.clock = func() time.Time {
		at := now
		now = now.Add(time.Minute)
		return at
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	reporting.StdioEgressDenied(ctx, "crm", "", 0, "crm.internal:443/secret")
	reporting.StdioEgressDenied(ctx, "crm", "", 0, "crm.internal:443/secret")
	if len(store.batches) != 0 {
		t.Fatalf("store wrote before flush: %+v", store.batches)
	}

	reporting.flush(ctx)

	if store.sawCanceled {
		t.Fatal("flush used the cancelled request context")
	}
	if len(store.batches) != 1 || len(store.batches[0]) != 1 {
		t.Fatalf("batches = %+v, want one aggregate row", store.batches)
	}
	got := store.batches[0][0]
	if got.Code != egressmetrics.CodeOther || got.Attempts != 2 {
		t.Fatalf("aggregate = %+v, want bounded code with two attempts", got)
	}
	if !got.FirstSeen.Equal(time.Date(2026, 8, 24, 15, 0, 0, 0, time.UTC)) ||
		!got.LastSeen.Equal(time.Date(2026, 8, 24, 15, 1, 0, 0, time.UTC)) {
		t.Fatalf("seen window = %s..%s, want first and last denial time", got.FirstSeen, got.LastSeen)
	}
}

func TestStdioEgressReporting_requeuesWhenTheStoreFails(t *testing.T) {
	store := &fakeStdioEgressStore{err: errors.New("database down")}
	reporting := newStdioEgressReporting(store, nil)

	reporting.StdioEgressDenied(t.Context(),
		"crm", "allowed.internal", 443, egressmetrics.CodeDestinationUnavailable)
	reporting.flush(t.Context())

	reporting.mu.Lock()
	defer reporting.mu.Unlock()
	if len(reporting.pending) != 1 {
		t.Fatalf("pending = %+v, want failed batch requeued", reporting.pending)
	}
}

type fakeStdioEgressStore struct {
	err         error
	sawCanceled bool
	batches     [][]egress.Denial
}

func (s *fakeStdioEgressStore) RecordDenials(ctx context.Context, denials []egress.Denial) error {
	if ctx.Err() != nil {
		s.sawCanceled = true
	}
	copied := make([]egress.Denial, len(denials))
	copy(copied, denials)
	s.batches = append(s.batches, copied)
	return s.err
}

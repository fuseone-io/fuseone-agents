package main

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/fuseone/agents/internal/egress"
	"github.com/fuseone/agents/internal/egressmetrics"
	"github.com/fuseone/agents/internal/worker"
)

type stdioEgressReporting struct {
	store   stdioEgressStore
	metrics *worker.MetricsRegistry
	clock   func() time.Time
	mu      sync.Mutex
	pending map[stdioEgressKey]egress.Denial
}

type stdioEgressStore interface {
	RecordDenials(context.Context, []egress.Denial) error
}

type stdioEgressKey struct {
	server string
	host   string
	port   int
	code   string
}

const stdioEgressFlush = 30 * time.Second
const stdioEgressFlushTimeout = 5 * time.Second

func newStdioEgressReporting(
	store stdioEgressStore, metrics *worker.MetricsRegistry,
) *stdioEgressReporting {
	return &stdioEgressReporting{
		store: store, metrics: metrics, clock: time.Now,
		pending: map[stdioEgressKey]egress.Denial{},
	}
}

func (r *stdioEgressReporting) start(ctx context.Context, every time.Duration) {
	if r == nil || r.store == nil {
		return
	}
	go r.watch(ctx, every)
}

func (r *stdioEgressReporting) watch(ctx context.Context, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			r.flush(ctx)
		case <-ctx.Done():
			r.flush(ctx)
			return
		}
	}
}

func (r *stdioEgressReporting) StdioEgressDenied(
	_ context.Context, server, host string, port int, code string,
) {
	code = egressmetrics.Code(code)
	if r.metrics != nil {
		r.metrics.StdioEgressDenial(code)
	}
	if r.store == nil {
		return
	}
	r.add(server, host, port, code)
}

func (r *stdioEgressReporting) add(server, host string, port int, code string) {
	now := r.clock().UTC()
	key := stdioEgressKey{server: server, host: host, port: port, code: code}
	r.mu.Lock()
	defer r.mu.Unlock()
	denial := r.pending[key]
	if denial.Attempts == 0 {
		denial = egress.Denial{
			Server: server, Host: host, Port: port, Code: code,
			FirstSeen: now, LastSeen: now,
		}
	}
	denial.Attempts++
	if now.Before(denial.FirstSeen) {
		denial.FirstSeen = now
	}
	if now.After(denial.LastSeen) {
		denial.LastSeen = now
	}
	r.pending[key] = denial
}

func (r *stdioEgressReporting) flush(ctx context.Context) {
	batch := r.drain()
	if len(batch) == 0 || r.store == nil {
		return
	}
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), stdioEgressFlushTimeout)
	defer cancel()
	err := r.store.RecordDenials(writeCtx, batch)
	if err != nil {
		r.requeue(batch)
		slog.Warn("could not record stdio egress denials", "count", len(batch), "err", err)
	}
}

func (r *stdioEgressReporting) drain() []egress.Denial {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.pending) == 0 {
		return nil
	}
	out := make([]egress.Denial, 0, len(r.pending))
	for _, denial := range r.pending {
		out = append(out, denial)
	}
	r.pending = map[stdioEgressKey]egress.Denial{}
	return out
}

func (r *stdioEgressReporting) requeue(batch []egress.Denial) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, denial := range batch {
		key := stdioEgressKey{
			server: denial.Server, host: denial.Host, port: denial.Port,
			code: egressmetrics.Code(denial.Code),
		}
		current := r.pending[key]
		if current.Attempts == 0 {
			r.pending[key] = denial
			continue
		}
		current.Attempts += denial.Attempts
		if denial.FirstSeen.Before(current.FirstSeen) {
			current.FirstSeen = denial.FirstSeen
		}
		if denial.LastSeen.After(current.LastSeen) {
			current.LastSeen = denial.LastSeen
		}
		r.pending[key] = current
	}
}

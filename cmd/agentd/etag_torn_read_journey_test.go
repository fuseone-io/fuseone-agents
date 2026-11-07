package main

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/testdb"
)

// afterFirstRead runs a write once, right after the first statement that reads settings
// ends, wherever in a request that statement is: in a handler, in the store, in a second
// read nobody meant to add.
type afterFirstRead struct {
	mu    sync.Mutex
	armed bool
	fired bool
	write func()
}

type tracedSQL struct{}

func (a *afterFirstRead) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, tracedSQL{}, data.SQL)
}

func (a *afterFirstRead) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	sql, _ := ctx.Value(tracedSQL{}).(string)
	a.mu.Lock()
	run := a.armed && strings.Contains(sql, "from settings")
	if run {
		a.armed, a.fired = false, true
	}
	a.mu.Unlock()
	if run {
		a.write()
	}
}

func tracedPool(t *testing.T, tracer pgx.QueryTracer) *pgxpool.Pool {
	t.Helper()
	testdb.Pool(t) // migrated
	config, err := pgxpool.ParseConfig(testdb.Required(t))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	config.ConnConfig.Tracer = tracer
	pool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

/*
A read answers a body and the revision of that same body. Read apart, a write landing
between them answers the old configuration under the new one's revision, and sending the
old one back with it is accepted and erases the write nobody saw.
*/
func TestSettingsETag_aWriteBetweenTheReads_doesNotTearTheAnswer(t *testing.T) {
	for _, r := range managedResources() {
		t.Run(r.name, func(t *testing.T) {
			seeding := newReadBackWorld(t)
			a := r.body("ETAG-SECRET-ONE")
			stored(t, seeding, r, "ETAG-SECRET-ONE")
			b := r.body("")
			r.change(b)

			hook := &afterFirstRead{write: func() {
				if put := seeding.call(t, etagCall{method: http.MethodPut, path: r.path, body: b}); put.Code != http.StatusNoContent {
					t.Errorf("the write between the reads = %d: %s", put.Code, put.Body)
				}
			}}
			reader := readBackWorldOn(t, tracedPool(t, hook))
			hook.mu.Lock()
			hook.armed = true
			hook.mu.Unlock()
			read := reader.call(t, etagCall{method: http.MethodGet, path: r.path})
			if !hook.fired || read.Code != http.StatusOK {
				t.Fatalf("GET = %d, write fired = %v", read.Code, hook.fired)
			}

			back := seeding.call(t, etagCall{method: http.MethodPut, path: r.path, ifMatch: []string{read.Header().Get("ETag")}, body: a})
			if back.Code != http.StatusPreconditionFailed {
				t.Fatalf("the answer read, sent back with its ETag = %d, want 412: %s", back.Code, read.Body)
			}
		})
	}
}

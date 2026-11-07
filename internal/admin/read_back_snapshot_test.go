package admin_test

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

/*
A read by name is one instant, whatever a writer does while it is being answered.

A client compares what it reads with what it wants and writes the difference. An
answer holding the old address with the new credential's fingerprint describes a
resource that never existed, and the client acts on it. So each write here lands
at the worst moment there is: straight after the read's first statement, committed
before its second one starts, from a pool the read does not use. Everything is
inline on the reader's connection, so nothing about it depends on timing.
*/

// writeAfterFirstRead runs a write the moment the first SELECT it sees finishes.
type writeAfterFirstRead struct {
	mu    sync.Mutex
	write func()
	fired bool
}

func (w *writeAfterFirstRead) arm(write func()) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.write, w.fired = write, false
}

func (w *writeAfterFirstRead) TraceQueryStart(
	ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData,
) context.Context {
	return ctx
}

func (w *writeAfterFirstRead) TraceQueryEnd(_ context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	if data.Err != nil || !strings.HasPrefix(data.CommandTag.String(), "SELECT") {
		return
	}
	w.mu.Lock()
	write := w.write
	w.write = nil
	if write != nil {
		w.fired = true
	}
	w.mu.Unlock()
	if write != nil {
		write()
	}
}

func (w *writeAfterFirstRead) landed(t *testing.T) {
	t.Helper()
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.fired {
		t.Fatal("the read issued no SELECT, so no write landed inside it")
	}
}

// bounded stops a write that a locking read would otherwise block for good. A
// read taking row locks is its own defect, and it should fail here, not hang.
func bounded(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestMCPServerNamed_aWriteLandingAfterItsFirstRead_answersTheStateItBeganOn(t *testing.T) {
	pool := freshPool(t)
	v := testVault(t)
	writer := newIntegrationsOn(t, pool, v)
	trace := &writeAfterFirstRead{}
	reader := newIntegrationsOn(t, tracedPool(t, trace), v)
	ctx := context.Background()
	const name = "snapshot-server"
	t.Cleanup(func() {
		_ = writer.DeleteMCPServer(context.Background(), "usr_ana", domain.Scope{}, name)
	})

	put := func(ctx context.Context, url, token string) error {
		return writer.PutMCPServer(ctx, "usr_ana", domain.Scope{}, domain.MCPServer{
			Name: name, Transport: domain.TransportHTTP, URL: url, Enabled: true,
		}, domain.MCPCredentialPatch{Token: &token})
	}
	if err := put(ctx, "https://old.example.test", "old-token"); err != nil {
		t.Fatalf("put: %v", err)
	}
	before, err := reader.MCPServerNamed(ctx, name)
	if err != nil {
		t.Fatalf("read before: %v", err)
	}

	trace.arm(func() {
		if err := put(bounded(t), "https://new.example.test", "new-token"); err != nil {
			t.Errorf("the write landing inside the read: %v", err)
		}
	})
	during, err := reader.MCPServerNamed(ctx, name)
	if err != nil {
		t.Fatalf("read during: %v", err)
	}
	trace.landed(t)
	if !reflect.DeepEqual(during, before) {
		t.Errorf("a torn answer:\n got %+v\nwant %+v", during, before)
	}

	after, err := reader.MCPServerNamed(ctx, name)
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if after.URL == before.URL || after.SecretFingerprint == before.SecretFingerprint {
		t.Errorf("the write did not change both halves, so this proves nothing: %+v", after)
	}
}

func TestProviderNamed_aWriteLandingAfterItsFirstRead_answersTheStateItBeganOn(t *testing.T) {
	pool := freshPool(t)
	v := testVault(t)
	writer := newIntegrationsOn(t, pool, v)
	trace := &writeAfterFirstRead{}
	reader := newIntegrationsOn(t, tracedPool(t, trace), v)
	ctx := context.Background()
	const name = "snapshot-provider"
	t.Cleanup(func() {
		_ = writer.DeleteProvider(context.Background(), "usr_ana", domain.Scope{}, name)
	})

	put := func(ctx context.Context, url, key string) error {
		return writer.PutProvider(ctx, "usr_ana", domain.Scope{}, domain.ModelProvider{
			Name: name, Kind: "openai_compatible", BaseURL: url, Enabled: true,
		}, key)
	}
	if err := put(ctx, "https://old.example.test", "old-key"); err != nil {
		t.Fatalf("put: %v", err)
	}
	before, err := reader.ProviderNamed(ctx, name)
	if err != nil {
		t.Fatalf("read before: %v", err)
	}

	trace.arm(func() {
		if err := put(bounded(t), "https://new.example.test", "new-key"); err != nil {
			t.Errorf("the write landing inside the read: %v", err)
		}
	})
	during, err := reader.ProviderNamed(ctx, name)
	if err != nil {
		t.Fatalf("read during: %v", err)
	}
	trace.landed(t)
	if !reflect.DeepEqual(during, before) {
		t.Errorf("a torn answer:\n got %+v\nwant %+v", during, before)
	}

	after, err := reader.ProviderNamed(ctx, name)
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if after.BaseURL == before.BaseURL || after.SecretFingerprint == before.SecretFingerprint {
		t.Errorf("the write did not change both halves, so this proves nothing: %+v", after)
	}
}

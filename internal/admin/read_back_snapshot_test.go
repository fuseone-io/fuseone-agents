package admin_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
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

type namedChannel struct {
	found admin.Channel
	bound []admin.ChannelIdentity
}

func TestChannelNamed_writesLandingAfterItsFirstRead_answersTheStateItBeganOn(t *testing.T) {
	const name = "snapshot_slack"
	cases := []struct {
		name string
		// write commits behind the read's back.
		write func(ctx context.Context, writer *admin.Channels) error
		// landed proves the write is there for the next read, every part of it.
		// Without it, a write that changed nothing would pass this test.
		landed func(t *testing.T, before, after namedChannel, err error)
	}{{
		name: "the connection, a conversation and a binding all change",
		write: func(ctx context.Context, writer *admin.Channels) error {
			if err := writer.PutChannel(ctx, admin.ChannelWrite{
				Channel: admin.Channel{
					Name: name, Kind: "slack", Workspace: "W-new",
					DeliveryMode: channel.DeliveryHTTP, Enabled: true,
				},
				Credentials: channel.Credentials{Token: "bot-new", Signing: "sign-new"},
				By:          "usr_ana", Governs: true,
			}); err != nil {
				return err
			}
			if err := writer.PutConversation(ctx, name, admin.Conversation{
				ID: "C-new", Scope: domain.Scope{Company: "acme"}, Enabled: true,
			}, "usr_ana"); err != nil {
				return err
			}
			return writer.BindIdentity(ctx, admin.ChannelIdentity{
				Channel: name, Account: "U-new", Principal: "usr_ana",
			}, "usr_ana")
		},
		landed: func(t *testing.T, before, after namedChannel, err error) {
			t.Helper()
			if err != nil {
				t.Fatalf("read after the write: %v", err)
			}
			if after.found.Workspace == before.found.Workspace {
				t.Error("the workspace did not change, so this proves nothing about it")
			}
			if after.found.SecretFingerprint == before.found.SecretFingerprint {
				t.Error("the fingerprint did not change, so this proves nothing about it")
			}
			if len(after.found.Conversations) == len(before.found.Conversations) {
				t.Error("no conversation arrived, so this proves nothing about them")
			}
			if len(after.bound) == len(before.bound) {
				t.Error("no binding arrived, so this proves nothing about them")
			}
		},
	}, {
		name: "the connection is removed",
		write: func(ctx context.Context, writer *admin.Channels) error {
			return writer.DeleteChannel(ctx, name, "usr_ana", true)
		},
		landed: func(t *testing.T, _, _ namedChannel, err error) {
			t.Helper()
			if !errors.Is(err, admin.ErrChannelUnknown) {
				t.Errorf("read after the removal: %v, want ErrChannelUnknown", err)
			}
		},
	}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pool := freshPool(t)
			v := testVault(t)
			writer := admin.NewChannels(pool, settings.NewStore(pool, v), onlySlack{})
			trace := &writeAfterFirstRead{}
			traced := tracedPool(t, trace)
			reader := admin.NewChannels(traced, settings.NewStore(traced, v), onlySlack{})
			ctx := context.Background()

			if err := writer.PutChannel(ctx, admin.ChannelWrite{
				Channel: admin.Channel{
					Name: name, Kind: "slack", Workspace: "W-old",
					DeliveryMode: channel.DeliveryHTTP, Enabled: true,
				},
				Credentials: channel.Credentials{Token: "bot-old", Signing: "sign-old"},
				By:          "usr_ana", Governs: true,
			}); err != nil {
				t.Fatalf("put: %v", err)
			}
			if err := writer.PutConversation(ctx, name, admin.Conversation{
				ID: "C-old", Scope: domain.Scope{Company: "acme"}, Enabled: true,
			}, "usr_ana"); err != nil {
				t.Fatalf("put conversation: %v", err)
			}
			if err := writer.BindIdentity(ctx, admin.ChannelIdentity{
				Channel: name, Account: "U-old", Principal: "usr_ana",
			}, "usr_ana"); err != nil {
				t.Fatalf("bind: %v", err)
			}
			read := func() (namedChannel, error) {
				found, bound, err := reader.Named(ctx, name)
				return namedChannel{found: found, bound: bound}, err
			}
			before, err := read()
			if err != nil {
				t.Fatalf("read before: %v", err)
			}

			trace.arm(func() {
				if err := tc.write(bounded(t), writer); err != nil {
					t.Errorf("the write landing inside the read: %v", err)
				}
			})
			during, err := read()
			if err != nil {
				t.Fatalf("read during: %v", err)
			}
			trace.landed(t)
			if !reflect.DeepEqual(during, before) {
				t.Errorf("a torn answer:\n got %+v\nwant %+v", during, before)
			}

			after, err := read()
			tc.landed(t, before, after, err)
		})
	}
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

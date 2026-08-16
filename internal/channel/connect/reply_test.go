package connect

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/vault"
)

/*
Answering in the workspace the ask came from.

The consumer holds asks from every connection at once and names the one each
refusal belongs to. Routed by anything else — the first connection, the only
one configured when the process started — a private sentence about somebody's
ask is posted into another company's Slack.

An in-package test with its own driver table: the table production uses is the
one `Kinds` is asserted against, and what this needs to see is which credential
the name resolved to, which no fake Slack over HTTP would show more honestly.
*/

type spoke struct {
	token, conversation, thread, text string
}

func (s *spoke) Post(context.Context, channel.Conversation, channel.Message) (string, error) {
	return "", nil
}

func (s *spoke) Say(_ context.Context, conversation, thread, text string) error {
	s.conversation, s.thread, s.text = conversation, thread, text
	return nil
}

func stored(t *testing.T) (*settings.Store, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("REQUIRE_DATABASE") != "" {
			t.Fatal("REQUIRE_DATABASE is set but TEST_DATABASE_URL is empty")
		}
		t.Skip("TEST_DATABASE_URL is unset; skipping the driver suite")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := ledger.Migrate(context.Background(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := pool.Exec(context.Background(),
		`delete from settings where kind = 'channel'`); err != nil {
		t.Fatalf("clean: %v", err)
	}
	v, err := vault.New(make([]byte, 32), "test")
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	return settings.NewStore(pool, v), pool
}

// configure writes a connection the way the console does.
func configure(t *testing.T, s *settings.Store, name string) {
	t.Helper()
	value, _ := json.Marshal(channel.Connection{Kind: "slack", Workspace: name})
	creds, _ := json.Marshal(channel.Credentials{Token: "xoxb-" + name, Signing: "s3cr3t"})
	if err := s.Put(context.Background(), settings.Setting{
		ScopeKind: settings.ScopeInstallation, Kind: channel.KindChannel,
		Name: name, Value: value, Secret: string(creds),
		Enabled: true, UpdatedBy: "test",
	}); err != nil {
		t.Fatalf("configure %s: %v", name, err)
	}
}

// spies builds the drivers under a table that records who was reached.
func spies(store *settings.Store) (*Drivers, map[string]*spoke) {
	reached := map[string]*spoke{}
	return newWith(store, map[string]func(channel.Credentials) Driver{
		"slack": func(creds channel.Credentials) Driver {
			one := &spoke{token: creds.Token}
			reached[creds.Token] = one
			return one
		},
	}), reached
}

func TestReply_twoWorkspaces_answersInTheOneTheAskCameFrom(t *testing.T) {
	store, _ := stored(t)
	configure(t, store, "acme")
	configure(t, store, "globex")
	drivers, reached := spies(store)

	if err := drivers.Reply(context.Background(),
		"globex", "C07", "1786.1", "triage will not start: it is paused."); err != nil {
		t.Fatalf("reply: %v", err)
	}

	if _, wrong := reached["xoxb-acme"]; wrong {
		t.Error("acme's bot was built; the refusal went to another company's Slack")
	}
	said, ok := reached["xoxb-globex"]
	if !ok {
		t.Fatalf("reached = %v, want the named connection's own bot", reached)
	}
	if said.conversation != "C07" || said.thread != "1786.1" {
		t.Errorf("said in %s/%s, want the conversation and thread it was asked in",
			said.conversation, said.thread)
	}
	if !strings.Contains(said.text, "will not start") {
		t.Errorf("text = %q", said.text)
	}
}

/*
A connection somebody switched off keeps the refusal owed.

Not answered-and-lost: a switch is a state that reverses, so the honest outcome
is the person hearing it late rather than never. The inbox retries what came
back as an error, which is exactly what this needs it to be.
*/
func TestReply_aConnectionSwitchedOff_isAnErrorSoTheRefusalStaysOwed(t *testing.T) {
	store, pool := stored(t)
	configure(t, store, "acme")
	if _, err := pool.Exec(context.Background(),
		`update settings set enabled = false where kind = 'channel' and name = 'acme'`); err != nil {
		t.Fatalf("switch off: %v", err)
	}
	drivers, _ := spies(store)

	if err := drivers.Reply(context.Background(),
		"acme", "C07", "1786.1", "triage will not start."); err == nil {
		t.Fatal("no error; the refusal would be marked said with nobody told")
	}
}

// A name nobody configured says which one. The alternative is a refusal that
// disappears with no line saying where it went.
func TestReply_aChannelNobodyConfigured_saysWhichOne(t *testing.T) {
	store, _ := stored(t)
	drivers, _ := spies(store)

	err := drivers.Reply(context.Background(),
		"ghost", "C07", "1786.1", "triage will not start.")
	if err == nil || !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("err = %v, want the name that could not be reached", err)
	}
}

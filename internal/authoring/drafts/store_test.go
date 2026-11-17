package drafts_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/authoring/drafts"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/publishable"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/testdb"
)

/*
The draft's store (NT-018 1b, part 3).

One draft per agent, shared by its authors, changed only by operations under
the row's lock and against the ETag the caller read. Who may reach a draft is
judged in the same transaction, on the draft's scope and on where the agent
lives now, before any precondition: a stale tag never tells a caller about a
draft or an agent it cannot reach.
*/

var (
	now = time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	cx  = domain.Scope{Company: "acme", Area: "cx"}
	ops = domain.Scope{Company: "acme", Area: "ops"}
)

func uniqueAgent(t *testing.T) domain.AgentID {
	t.Helper()
	return domain.AgentID(fmt.Sprintf("draft-%d", time.Now().UnixNano()))
}

func in(scopes ...domain.Scope) drafts.Caller {
	return drafts.Caller{By: "usr_ana", Allow: func(s domain.Scope) bool {
		for _, one := range scopes {
			if one == s {
				return true
			}
		}
		return false
	}}
}

func ifMatch(t *testing.T, tag domain.Revision) domain.IfMatch {
	t.Helper()
	header := string(tag)
	parsed, err := domain.ParseIfMatch(&header)
	if err != nil {
		t.Fatalf("ParseIfMatch: %v", err)
	}
	return parsed
}

var validator = draft.Validator{Checker: publishable.Checker{}}

func publish(t *testing.T, pool *pgxpool.Pool, agent domain.AgentID, scope domain.Scope, text string) spec.Spec {
	t.Helper()
	s, err := spec.Parse("test", []byte(fmt.Sprintf(
		"---\nid: %s\nname: Triagem\narea: %s\nprovider: openai\nmodel: m\ntools: [crm.lookup]\n"+
			"budget: {micros: 500000, steps: 60}\n---\n\n%s", agent, scope.Area, text)))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if _, err := spec.NewPublisher(pool, engine.SystemClock{}).Publish(context.Background(), spec.Publication{
		Spec: s, By: "usr_ops", Company: scope.Company,
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	// As it is stored: the publication writes the company the file left out.
	s.Company = scope.Company
	return s
}

func staleAt(t *testing.T, err error) domain.Revision {
	t.Helper()
	var stale *domain.StaleError
	if !errors.As(err, &stale) {
		t.Fatalf("err = %v, want a stale precondition", err)
	}
	return stale.Current
}

func trail(t *testing.T, pool *pgxpool.Pool, agent domain.AgentID) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		`select action from agent_draft_trail where agent_id = $1 order by at, seq`, string(agent))
	if err != nil {
		t.Fatalf("read the trail: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var action string
		if err := rows.Scan(&action); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, action)
	}
	return out
}

func TestCreate_aNewAgent_isADraftNothingElseSees(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)

	opened, err := store.Create(t.Context(), agent, &drafts.Identity{Name: "Triagem", Scope: cx}, in(cx))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if opened.Draft.Spec.ID != agent || opened.Revision == "" || opened.BaseVersion != "" {
		t.Fatalf("opened %+v", opened)
	}
	if got := trail(t, pool, agent); strings.Join(got, ",") != "create" {
		t.Errorf("trail = %v, want the creation", got)
	}
	// The previous binary reads agent_specs and agent_state; a draft writes
	// neither, so to it the agent does not exist yet.
	if versions, err := spec.NewRegistry(pool).Versions(t.Context(), agent); err != nil || len(versions) != 0 {
		t.Errorf("versions = %v (%v), want none", versions, err)
	}
	var states int
	if err := pool.QueryRow(t.Context(), `select count(*) from agent_state where agent_id = $1`,
		string(agent)).Scan(&states); err != nil || states != 0 {
		t.Errorf("agent_state rows = %d (%v), want none", states, err)
	}
}

func TestCreate_fromThePublishedAgent_opensItsCurrentVersionUntouched(t *testing.T) {
	pool := testdb.Pool(t)
	agent := uniqueAgent(t)
	published := publish(t, pool, agent, cx, "Objetivo\nTriar.\n\nNunca\nPrometer.")

	opened, err := drafts.NewStore(pool).Create(t.Context(), agent, nil, in(cx))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if opened.BaseVersion != published.Version {
		t.Errorf("base = %s, want %s", opened.BaseVersion, published.Version)
	}
	a, _ := spec.Render(published)
	b, _ := spec.Render(opened.Draft.Definition())
	if string(a) != string(b) {
		t.Fatalf("the untouched draft renders other bytes\n%s\n%s", a, b)
	}
}

func TestCreate_refusals(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	published := uniqueAgent(t)
	publish(t, pool, published, cx, "Triar.")
	drafted := uniqueAgent(t)
	first, err := store.Create(t.Context(), drafted, &drafts.Identity{Name: "Triagem", Scope: cx}, in(cx))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	cases := []struct {
		name     string
		agent    domain.AgentID
		identity *drafts.Identity
		caller   drafts.Caller
		want     error
	}{
		{"an identity for a published agent", published, &drafts.Identity{Name: "X", Scope: cx}, in(cx), drafts.ErrAgentExists},
		{"an identity for a published agent out of reach", published, &drafts.Identity{Name: "X", Scope: ops}, in(ops), drafts.ErrNotFound},
		{"no identity for an agent nobody published", uniqueAgent(t), nil, in(cx), drafts.ErrNotFound},
		{"a published agent out of reach", published, nil, in(ops), drafts.ErrNotFound},
		{"a scope the caller may not publish into", uniqueAgent(t), &drafts.Identity{Name: "X", Scope: ops}, in(cx), drafts.ErrForbidden},
		{"a scope that is not whole", uniqueAgent(t), &drafts.Identity{Name: "X", Scope: domain.Scope{Area: "cx"}}, in(cx), draft.ErrInvalidOperation},
		{"a draft out of reach", drafted, &drafts.Identity{Name: "X", Scope: ops}, in(ops), drafts.ErrNotFound},
	}
	for _, tc := range cases {
		if _, err := store.Create(t.Context(), tc.agent, tc.identity, tc.caller); !errors.Is(err, tc.want) {
			t.Errorf("%s: Create = %v, want %v", tc.name, err, tc.want)
		}
	}
	_, err = store.Create(t.Context(), drafted, &drafts.Identity{Name: "X", Scope: cx}, in(cx))
	if got := staleAt(t, err); got != first.Revision {
		t.Errorf("a second draft: current = %s, want the first's %s", got, first.Revision)
	}
}

// Two creations of the same new agent, released together: exactly one
// draft, and the other told the one that exists.
func TestCreate_twoAtOnce_makeOneDraft(t *testing.T) {
	testdb.Pool(t)
	c := testdb.NewContention(t, 2)
	store := drafts.NewStore(c.Pool)
	agent := uniqueAgent(t)
	create := func() error {
		_, err := store.Create(context.Background(), agent, &drafts.Identity{Name: "Triagem", Scope: cx}, in(cx))
		return err
	}

	results := c.Overlap(t, `select pg_advisory_xact_lock($1)`, []any{spec.EventGraphLockKey},
		[]func() error{create, create})

	created, stale := 0, 0
	for _, err := range results {
		var s *domain.StaleError
		switch {
		case err == nil:
			created++
		case errors.As(err, &s):
			stale++
		default:
			t.Errorf("Create = %v", err)
		}
	}
	if created != 1 || stale != 1 {
		t.Fatalf("created %d, stale %d; want one of each", created, stale)
	}
}

func TestApply_withTheCurrentTag_appliesAndLeavesATrail(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	opened, err := store.Create(t.Context(), agent, &drafts.Identity{Name: "Triagem", Scope: cx}, in(cx))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	applied, err := store.Apply(t.Context(), drafts.Application{
		Agent: agent, Op: draft.AddTool{Tool: "crm.lookup"}, Origin: draft.FromControl,
		Expect: ifMatch(t, opened.Revision), Caller: in(cx), Validator: validator, Now: now,
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if applied.Revision == opened.Revision || len(applied.Draft.Spec.Tools) != 1 {
		t.Fatalf("applied %+v", applied)
	}
	var digest, origin string
	if err := pool.QueryRow(t.Context(), `
		select digest, origin from agent_draft_trail where agent_id = $1 and action = 'add_tool'`,
		string(agent)).Scan(&digest, &origin); err != nil {
		t.Fatalf("read the trail: %v", err)
	}
	if digest != draft.Digest(draft.AddTool{Tool: "crm.lookup"}) || origin != "control" {
		t.Errorf("trail row = %s %s", digest, origin)
	}
	reread, err := store.Get(t.Context(), agent, in(cx))
	if err != nil || reread.Revision != applied.Revision {
		t.Errorf("Get = %v (%v), want the applied draft", reread.Revision, err)
	}
}

func TestApply_refusals_writeNothing(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	opened, err := store.Create(t.Context(), agent, &drafts.Identity{Name: "Triagem", Scope: cx}, in(cx))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	stale := ifMatch(t, `"v1-stale"`)
	cases := []struct {
		name   string
		op     draft.Operation
		expect domain.IfMatch
		caller drafts.Caller
		want   error
	}{
		{"no If-Match", draft.AddTool{Tool: "crm.lookup"}, domain.IfMatch{}, in(cx), drafts.ErrPreconditionRequired},
		{"a stale tag", draft.AddTool{Tool: "crm.lookup"}, stale, in(cx), nil},
		{"out of reach, stale tag", draft.AddTool{Tool: "crm.lookup"}, stale, in(ops), drafts.ErrNotFound},
		{"a destination not allowed", draft.SetIdentity{Name: "T", Scope: ops}, ifMatch(t, opened.Revision), in(cx), drafts.ErrForbidden},
		{"a block by hand", draft.RemoveBlock{ID: "b1"}, ifMatch(t, opened.Revision), in(cx), draft.ErrConversationOnly},
		{"refused by validation", draft.SetCachePrefixTTL{TTL: "2h"}, ifMatch(t, opened.Revision), in(cx), draft.ErrRefused},
	}
	for _, tc := range cases {
		_, err := store.Apply(t.Context(), drafts.Application{
			Agent: agent, Op: tc.op, Origin: draft.FromControl, Expect: tc.expect,
			Caller: tc.caller, Validator: validator, Now: now,
		})
		if tc.want == nil {
			if got := staleAt(t, err); got != opened.Revision {
				t.Errorf("%s: current = %s, want %s", tc.name, got, opened.Revision)
			}
		} else if !errors.Is(err, tc.want) {
			t.Errorf("%s: Apply = %v, want %v", tc.name, err, tc.want)
		}
	}
	reread, err := store.Get(t.Context(), agent, in(cx))
	if err != nil || reread.Revision != opened.Revision {
		t.Errorf("after the refusals the draft is at %s (%v), want %s", reread.Revision, err, opened.Revision)
	}
	if got := trail(t, pool, agent); strings.Join(got, ",") != "create" {
		t.Errorf("trail = %v, want only the creation", got)
	}
}

// Contract 5: a draft whose agent moved needs both scopes, the draft's and
// where the agent lives now.
func TestApplyAndGet_theAgentMovedWhileTheDraftWasOpen_needBothScopes(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	publish(t, pool, agent, cx, "Triar.")
	opened, err := store.Create(t.Context(), agent, nil, in(cx))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	publish(t, pool, agent, ops, "Triar em ops.")

	if _, err := store.Get(t.Context(), agent, in(cx)); !errors.Is(err, drafts.ErrNotFound) {
		t.Errorf("Get with the draft's scope alone = %v, want ErrNotFound", err)
	}
	if _, err := store.Apply(t.Context(), drafts.Application{
		Agent: agent, Op: draft.SetReplyLocale{Locale: "en-US"}, Origin: draft.FromControl,
		Expect: ifMatch(t, opened.Revision), Caller: in(cx), Validator: validator, Now: now,
	}); !errors.Is(err, drafts.ErrNotFound) {
		t.Errorf("Apply with the draft's scope alone = %v, want ErrNotFound", err)
	}
	if _, err := store.Apply(t.Context(), drafts.Application{
		Agent: agent, Op: draft.SetReplyLocale{Locale: "en-US"}, Origin: draft.FromControl,
		Expect: ifMatch(t, opened.Revision), Caller: in(cx, ops), Validator: validator, Now: now,
	}); err != nil {
		t.Errorf("Apply with both scopes = %v, want it applied", err)
	}
}

// set_identity moves the draft: it needs the scope it leaves and the one it
// enters.
func TestApply_setIdentity_movesTheDraftWithBothScopes(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	opened, err := store.Create(t.Context(), agent, &drafts.Identity{Name: "Triagem", Scope: cx}, in(cx))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	moved, err := store.Apply(t.Context(), drafts.Application{
		Agent: agent, Op: draft.SetIdentity{Name: "Triagem ops", Scope: ops}, Origin: draft.FromControl,
		Expect: ifMatch(t, opened.Revision), Caller: in(cx, ops), Validator: validator, Now: now,
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if moved.Draft.Spec.Area != "ops" {
		t.Fatalf("area = %s, want ops", moved.Draft.Spec.Area)
	}
	if _, err := store.Get(t.Context(), agent, in(cx)); !errors.Is(err, drafts.ErrNotFound) {
		t.Errorf("Get from the scope it left = %v, want ErrNotFound", err)
	}
}

// Two operations against the same tag, released together: one lands, the
// other is stale, and the trail holds one operation.
func TestApply_twoAgainstTheSameTag_oneLands(t *testing.T) {
	pool := testdb.Pool(t)
	agent := uniqueAgent(t)
	opened, err := drafts.NewStore(pool).Create(t.Context(), agent, &drafts.Identity{Name: "Triagem", Scope: cx}, in(cx))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	c := testdb.NewContention(t, 2)
	store := drafts.NewStore(c.Pool)
	apply := func(op draft.Operation) func() error {
		return func() error {
			_, err := store.Apply(context.Background(), drafts.Application{
				Agent: agent, Op: op, Origin: draft.FromControl, Expect: ifMatch(t, opened.Revision),
				Caller: in(cx), Validator: validator, Now: now,
			})
			return err
		}
	}

	results := c.Overlap(t, `select 1 from agent_drafts where agent_id = $1 for update`, []any{string(agent)},
		[]func() error{apply(draft.SetReplyLocale{Locale: "en-US"}), apply(draft.SetReplyLocale{Locale: "pt-BR"})})

	landed, stale := 0, 0
	for _, err := range results {
		var s *domain.StaleError
		switch {
		case err == nil:
			landed++
		case errors.As(err, &s):
			stale++
		default:
			t.Errorf("Apply = %v", err)
		}
	}
	if landed != 1 || stale != 1 {
		t.Fatalf("landed %d, stale %d; want one of each", landed, stale)
	}
	if got := trail(t, pool, agent); strings.Join(got, ",") != "create,set_reply_locale" {
		t.Errorf("trail = %v, want one operation", got)
	}
}

func TestDiscard_needsTheTagAndKeepsTheTrail(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	opened, err := store.Create(t.Context(), agent, &drafts.Identity{Name: "Triagem", Scope: cx}, in(cx))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.Discard(t.Context(), agent, domain.IfMatch{}, in(cx)); !errors.Is(err, drafts.ErrPreconditionRequired) {
		t.Errorf("Discard without If-Match = %v", err)
	}
	if err := store.Discard(t.Context(), agent, ifMatch(t, opened.Revision), in(ops)); !errors.Is(err, drafts.ErrNotFound) {
		t.Errorf("Discard out of reach = %v", err)
	}
	if got := staleAt(t, store.Discard(t.Context(), agent, ifMatch(t, `"v1-stale"`), in(cx))); got != opened.Revision {
		t.Errorf("Discard with a stale tag: current = %s, want %s", got, opened.Revision)
	}
	if _, err := store.Get(t.Context(), agent, in(cx)); err != nil {
		t.Fatalf("a refused discard removed the draft: %v", err)
	}
	if err := store.Discard(t.Context(), agent, ifMatch(t, opened.Revision), in(cx)); err != nil {
		t.Fatalf("Discard: %v", err)
	}
	if _, err := store.Get(t.Context(), agent, in(cx)); !errors.Is(err, drafts.ErrNotFound) {
		t.Errorf("Get after discard = %v", err)
	}
	again, err := store.Create(t.Context(), agent, &drafts.Identity{Name: "Triagem", Scope: cx}, in(cx))
	if err != nil {
		t.Fatalf("Create again: %v", err)
	}
	if again.DraftID == opened.DraftID || again.Revision == opened.Revision {
		t.Errorf("the draft opened again reused %s / %s", again.DraftID, again.Revision)
	}
	if got := trail(t, pool, agent); strings.Join(got, ",") != "create,discard,create" {
		t.Errorf("trail = %v", got)
	}
}

func TestTrail_isAppendOnly(t *testing.T) {
	pool := testdb.Pool(t)
	agent := uniqueAgent(t)
	if _, err := drafts.NewStore(pool).Create(t.Context(), agent, &drafts.Identity{Name: "Triagem", Scope: cx}, in(cx)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	for _, statement := range []string{
		`update agent_draft_trail set digest = 'x' where agent_id = $1`,
		`delete from agent_draft_trail where agent_id = $1`,
	} {
		_, err := pool.Exec(t.Context(), statement, string(agent))
		var pg *pgconn.PgError
		if !errors.As(err, &pg) || pg.Code != "23001" {
			t.Errorf("%s = %v, want restrict_violation", statement, err)
		}
	}
}

// The table refuses a draft with no whole scope, whatever binary writes it.
func TestDrafts_aRowWithoutAWholeScope_isRefusedByTheTable(t *testing.T) {
	pool := testdb.Pool(t)
	for _, scope := range []domain.Scope{{Company: "", Area: "cx"}, {Company: "*", Area: "cx"}, {Company: "acme", Area: ""}} {
		_, err := pool.Exec(t.Context(), `
			insert into agent_drafts (agent_id, company_id, area_id, definition, blocks, revision,
				created_by, created_at, updated_by, updated_at)
			values ($1, $2, $3, '', '[]', 'r', 'u', now(), 'u', now())`,
			string(uniqueAgent(t)), string(scope.Company), string(scope.Area))
		var pg *pgconn.PgError
		if !errors.As(err, &pg) || pg.Code != "23514" {
			t.Errorf("scope %v = %v, want check_violation", scope, err)
		}
	}
}

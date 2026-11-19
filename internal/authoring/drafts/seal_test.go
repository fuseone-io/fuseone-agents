package drafts_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/authoring/drafts"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/regression"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/testdb"
)

/*
Sealing a draft (NT-018 phase 2, release 2b).

A seal keeps the draft's definition as the exact bytes a publication would write, under the
version id a publication would compute, outside agent_specs. It is taken against the
revision the author read, in the draft's scope, under the content fence an erasure waits
for, and it leaves nothing behind when it fails.
*/

// sealable is a draft that would publish: opened from a published version and changed by
// hand, so its bytes are a version nobody published yet.
func sealable(t *testing.T, store *drafts.Store, pool *pgxpool.Pool) drafts.Opened {
	t.Helper()
	agent := uniqueAgent(t)
	publish(t, pool, agent, cx, "Objetivo\n\nAtender bem.")
	o, err := store.Create(t.Context(), agent, nil, as("usr_ana", cx))
	if err != nil {
		t.Fatalf("Create from published: %v", err)
	}
	return applyControl(t, store, agent, o.Revision, draft.AddTool{Tool: "crm.sealed"})
}

func sealOf(o drafts.Opened, by domain.UserID, scopes ...domain.Scope) drafts.Sealing {
	expect, _ := domain.ParseIfMatch(ptrTo(string(o.Revision)))
	return drafts.Sealing{Agent: o.Agent, Expect: expect, Caller: as(by, scopes...)}
}

func ptrTo[T any](v T) *T { return &v }

// publishCanonical publishes text the way every publication path stores a definition:
// rendered and parsed back, so a draft opened from it renders the same bytes.
func publishCanonical(t *testing.T, pool *pgxpool.Pool, agent domain.AgentID, text string) spec.Spec {
	t.Helper()
	raw := publish(t, pool, agent, cx, text)
	rendered, err := spec.Render(raw)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	canonical, err := spec.Parse("canonical", rendered)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if _, err := spec.NewPublisher(pool, engine.SystemClock{}).Publish(context.Background(), spec.Publication{
		Spec: canonical, By: "usr_ops", Company: cx.Company,
	}); err != nil {
		t.Fatalf("Publish canonical: %v", err)
	}
	return canonical
}

func renderedOf(t *testing.T, o drafts.Opened) []byte {
	t.Helper()
	rendered, err := spec.Render(o.Draft.Definition())
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	return rendered
}

type sealRow struct {
	version, company, area, ref, draftID, revision, by string
}

func readSeal(t *testing.T, pool *pgxpool.Pool, id string) sealRow {
	t.Helper()
	var r sealRow
	if err := pool.QueryRow(t.Context(), `
		select version_id, company_id, area_id, content_ref, draft_id::text, draft_revision, sealed_by
		from agent_seals where seal_id = $1::uuid`, id).Scan(
		&r.version, &r.company, &r.area, &r.ref, &r.draftID, &r.revision, &r.by); err != nil {
		t.Fatalf("read seal %s: %v", id, err)
	}
	return r
}

func sealsOf(t *testing.T, pool *pgxpool.Pool, agent domain.AgentID) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), `select count(*) from agent_seals where agent_id = $1`,
		string(agent)).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSeal_keepsTheDraftsExactBytesUnderTheVersionAPublicationWouldWrite(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	o := sealable(t, store, pool)
	rendered := renderedOf(t, o)
	want, err := spec.Parse("x", rendered)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	got, err := store.Seal(t.Context(), sealOf(o, "usr_ana", cx))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	if got.SealID == "" || got.Published || got.Version != want.Version || got.Scope != cx {
		t.Fatalf("Seal = %+v, want a new seal of %s in %s", got, want.Version, cx)
	}
	row := readSeal(t, pool, got.SealID)
	if row.version != string(want.Version) || row.company != "acme" || row.area != "cx" ||
		row.draftID != o.DraftID || row.revision != string(o.Revision) || row.by != "usr_ana" {
		t.Errorf("seal row = %+v", row)
	}
	bytes, err := ledger.NewContent(pool).Get(t.Context(), row.ref)
	if err != nil || string(bytes) != string(rendered) {
		t.Errorf("sealed bytes (%v) differ from the rendered draft", err)
	}
	var claimed string
	if err := pool.QueryRow(t.Context(), `select digest from agent_version_digests
		where agent_id = $1 and version_id = $2`, string(o.Agent), string(want.Version)).Scan(&claimed); err != nil ||
		claimed != want.Digest {
		t.Errorf("claim = %q (%v), want the digest %s", claimed, err, want.Digest)
	}
}

func TestSeal_copiesTheDraftsSourceMap(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	o := sealable(t, store, pool)

	got, err := store.Seal(t.Context(), sealOf(o, "usr_ana", cx))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	var diff int
	if err := pool.QueryRow(t.Context(), `
		select count(*) from (
			(select part, trail_seq from agent_draft_sources where draft_id = $1::uuid
			 except select part, trail_seq from agent_seal_sources where seal_id = $2::uuid)
			union all
			(select part, trail_seq from agent_seal_sources where seal_id = $2::uuid
			 except select part, trail_seq from agent_draft_sources where draft_id = $1::uuid)
		) d`, o.DraftID, got.SealID).Scan(&diff); err != nil {
		t.Fatal(err)
	}
	if diff != 0 {
		t.Errorf("the seal's sources differ from the draft's in %d rows", diff)
	}
}

// A map an older writer left stale cannot vouch for a value that may have come from a
// conversation: those parts are sealed as unknown, so any erasure of the draft's threads
// reaches the seal. The three parts no proposal can write stay manual.
func TestSeal_aStaleSourceMap_sealsConversationPartsAsUnknown(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	o := sealable(t, store, pool)
	if _, err := pool.Exec(t.Context(), `update agent_drafts set sources_revision = '', sources_seq = 0
		where draft_id = $1::uuid`, o.DraftID); err != nil {
		t.Fatal(err)
	}

	got, err := store.Seal(t.Context(), sealOf(o, "usr_ana", cx))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	rows, err := pool.Query(t.Context(), `select part, trail_seq is null from agent_seal_sources
		where seal_id = $1::uuid order by part`, got.SealID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := 0
	for rows.Next() {
		var part string
		var unknown bool
		if err := rows.Scan(&part, &unknown); err != nil {
			t.Fatal(err)
		}
		seen++
		human := part == "name" || part == "scope" || part == "standingApprovals"
		if unknown == human {
			t.Errorf("part %s sealed unknown=%v, want unknown only for what a conversation can write", part, unknown)
		}
	}
	if seen == 0 {
		t.Fatal("the seal has no sources")
	}
}

func TestSeal_aStaleRevision_writesNothing(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	o := sealable(t, store, pool)
	read := o
	_ = applyControl(t, store, o.Agent, o.Revision, draft.AddTool{Tool: "crm.moved"})

	_, err := store.Seal(t.Context(), sealOf(read, "usr_ana", cx))
	_ = staleAt(t, err)
	if n := sealsOf(t, pool, o.Agent); n != 0 {
		t.Errorf("%d seals written against a stale revision", n)
	}
}

func TestSeal_outsideTheCallersScope_isNotFound(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	o := sealable(t, store, pool)
	finance := domain.Scope{Company: "acme", Area: "finance"}

	if _, err := store.Seal(t.Context(), sealOf(o, "usr_bia", finance)); !errors.Is(err, drafts.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if n := sealsOf(t, pool, o.Agent); n != 0 {
		t.Errorf("%d seals written for a caller outside the scope", n)
	}
}

// A draft whose bytes are already a published version is that version: nothing is
// sealed, and the answer names the published version.
func TestSeal_aDraftAlreadyPublished_sealsNothing(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	published := publishCanonical(t, pool, agent, "Objetivo\n\nAtender bem.")
	o, err := store.Create(t.Context(), agent, nil, as("usr_ana", cx))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := store.Seal(t.Context(), sealOf(o, "usr_ana", cx))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if !got.Published || got.SealID != "" || got.Version != published.Version {
		t.Errorf("Seal = %+v, want the published version %s and no seal", got, published.Version)
	}
	if n := sealsOf(t, pool, agent); n != 0 {
		t.Errorf("%d seals written for a published version", n)
	}
}

func TestSeal_aVersionIdClaimedForOtherBytes_isACollisionAndWritesNothing(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	o := sealable(t, store, pool)
	want, err := spec.Parse("x", renderedOf(t, o))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `insert into agent_version_digests (agent_id, version_id, digest)
		values ($1, $2, repeat('b', 64))`, string(o.Agent), string(want.Version)); err != nil {
		t.Fatalf("claim other bytes: %v", err)
	}

	if _, err := store.Seal(t.Context(), sealOf(o, "usr_ana", cx)); !errors.Is(err, spec.ErrVersionCollision) {
		t.Fatalf("err = %v, want ErrVersionCollision", err)
	}
	if n := sealsOf(t, pool, o.Agent); n != 0 {
		t.Errorf("%d seals written over a collision", n)
	}
}

// A seal that fails after its row is written leaves no claim, no content and no seal.
func TestSeal_failingAfterItsRow_leavesNoClaimContentOrSeal(t *testing.T) {
	pool := testdb.Isolated(t)
	store := drafts.NewStore(pool)
	o := sealable(t, store, pool)
	rendered := renderedOf(t, o)
	sum := sha256.Sum256(rendered)
	if _, err := pool.Exec(t.Context(), `
		create function refuse_seal_sources() returns trigger language plpgsql as 'begin
			raise exception ''seal sources refused'';
		end';
		create trigger refuse_seal_sources before insert on agent_seal_sources
			for each row execute function refuse_seal_sources()`); err != nil {
		t.Fatalf("install the failure: %v", err)
	}

	if _, err := store.Seal(t.Context(), sealOf(o, "usr_ana", cx)); err == nil {
		t.Fatal("Seal succeeded although its sources were refused")
	}
	var claims, content, seals int
	if err := pool.QueryRow(t.Context(), `
		select (select count(*) from agent_version_digests where agent_id = $1 and digest = $2),
		       (select count(*) from run_content where owner_kind = 'authoring' and digest = $2),
		       (select count(*) from agent_seals where agent_id = $1)`,
		string(o.Agent), hex.EncodeToString(sum[:])).Scan(&claims, &content, &seals); err != nil {
		t.Fatal(err)
	}
	if claims != 0 || content != 0 || seals != 0 {
		t.Errorf("after the failure: %d claims, %d content, %d seals; want none", claims, content, seals)
	}
}

// Sealing keeps the draft's text, so it holds the agent's content fence shared: an
// erasure holding it exclusive is waited for, and its tombstone is what the seal meets.
func TestSeal_waitsForAnErasureHoldingTheContentFence(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	o := sealable(t, store, pool)
	ctx := context.Background()

	entered, release, erased := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		erased <- regression.NewStore(pool).Exclusive(ctx, []domain.AgentID{o.Agent}, func(context.Context) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	sealed := make(chan error, 1)
	go func() {
		_, err := store.Seal(ctx, sealOf(o, "usr_ana", cx))
		sealed <- err
	}()
	select {
	case err := <-sealed:
		t.Fatalf("the seal did not wait for the erasure's fence: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	if err := <-erased; err != nil {
		t.Fatalf("Exclusive: %v", err)
	}
	if err := <-sealed; err != nil {
		t.Fatalf("Seal: %v", err)
	}
}

func TestSeal_aDraftOfOneAndAHalfMebibytes_isSealedWhole(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	o := sealable(t, store, pool)
	p := propose(t, store, o.Agent, "usr_ana", o.Revision, draft.InsertBlock{
		At: 0, Type: "prose", Text: strings.Repeat("Texto longo do autor. ", (3<<19)/22+1), Locale: "pt-BR",
	})
	o, err := accept(t, store, o.Agent, p.ID, as("usr_ana", cx))
	if err != nil {
		t.Fatalf("accept the long block: %v", err)
	}
	rendered := renderedOf(t, o)
	if len(rendered) < 3<<19 {
		t.Fatalf("the draft renders %d bytes, want at least 1.5 MiB", len(rendered))
	}

	got, err := store.Seal(t.Context(), sealOf(o, "usr_ana", cx))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	bytes, err := ledger.NewContent(pool).Get(t.Context(), readSeal(t, pool, got.SealID).ref)
	if err != nil || len(bytes) != len(rendered) {
		t.Fatalf("sealed %d bytes (%v), want all %d", len(bytes), err, len(rendered))
	}
}

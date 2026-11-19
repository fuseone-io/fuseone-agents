package spec_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/testdb"
)

/*
Activating a version (NT-018 phase 2, release 2c).

Activation makes a version current through the publication's own locks and checks, in the
order the code runs: the agent's row and its guard (404 before 412), the sealed scope
against the current one, the stage, then the definition — the published row when the
version is published, else a readable seal whose bytes hash to the claim — judged and
accepted before anything is written. A candidate's bytes become its agent_specs row; every
activation records which seal it read, or none.
*/

// candidateOf is definition sealed with its company, under another instruction: the
// bytes a seal holds name their scope, so they are always another version than first.
func candidateOf(instruction string) string {
	return strings.NewReplacer(
		"area: cx", "company: acme\narea: cx",
		"Read the ticket and classify it.", instruction,
	).Replace(definition)
}

type activationWorld struct {
	pool      *pgxpool.Pool
	content   *ledger.Content
	publisher *spec.Publisher
	reader    *spec.CandidateReader
}

func newActivationWorld(t *testing.T) activationWorld {
	t.Helper()
	pool := openSpecPool(t)
	content := ledger.NewContent(pool)
	return activationWorld{
		pool: pool, content: content, publisher: spec.NewPublisher(pool, fixedClock{}),
		reader: spec.NewCandidateReader(pool, content),
	}
}

// seal claims text's version and writes one seal of it, as sealing a draft does.
func (w activationWorld) seal(t *testing.T, text string) (string, spec.Spec) {
	t.Helper()
	ctx := context.Background()
	parsed := published(t, text)
	sum := sha256.Sum256([]byte(text))
	if _, err := w.pool.Exec(ctx, `
		insert into agent_version_digests (agent_id, version_id, digest) values ($1, $2, $3)
		on conflict do nothing`, string(parsed.ID), string(parsed.Version), hex.EncodeToString(sum[:])); err != nil {
		t.Fatalf("claim: %v", err)
	}
	var id string
	if err := w.pool.QueryRow(ctx, `select gen_random_uuid()::text`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	ref, err := w.content.PutFor(ctx, "authoring", id, 0, []byte(text))
	if err != nil {
		t.Fatalf("PutFor: %v", err)
	}
	if _, err := w.pool.Exec(ctx, `
		insert into agent_seals (seal_id, agent_id, version_id, company_id, area_id, content_ref,
		  draft_id, draft_revision, sealed_by)
		values ($1::uuid, $2, $3, $4, $5, $6, gen_random_uuid(), 'rev', 'usr_ana')`,
		id, string(parsed.ID), string(parsed.Version), string(parsed.Company), string(parsed.Area), ref); err != nil {
		t.Fatalf("seal: %v", err)
	}
	return id, parsed
}

func (w activationWorld) erase(t *testing.T, seal string) {
	t.Helper()
	tx, err := w.pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := w.content.EraseOwnersTx(context.Background(), tx, "authoring", []string{seal}, "test"); err != nil {
		t.Fatalf("erase: %v", err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// publishFirst publishes definition as the agent's live version, and returns it.
func (w activationWorld) publishFirst(t *testing.T) spec.Spec {
	t.Helper()
	first := published(t, definition)
	if _, err := w.publisher.Publish(context.Background(),
		spec.Publication{Spec: first, By: "usr_ana", Company: "acme"}); err != nil {
		t.Fatalf("Publish(first): %v", err)
	}
	return first
}

// activation is an activation of version by a caller who may publish in cx, made with
// the tag of the version live now.
func (w activationWorld) activation(t *testing.T, version domain.VersionID, live domain.VersionID) spec.Activation {
	t.Helper()
	return spec.Activation{
		Agent: "triage", Version: version, Scope: cx, By: "usr_bea",
		Guard:  spec.Guard{Allow: inScope(cx), Expect: ifMatch(t, domain.AgentRevision(live, false))},
		Accept: func(spec.Spec) error { return nil },
	}
}

type activationRow struct {
	version, seal, path, by string
}

func (w activationWorld) activations(t *testing.T) []activationRow {
	t.Helper()
	rows, err := w.pool.Query(context.Background(), `
		select version_id, coalesce(seal_id::text, ''), path, activated_by
		from agent_activations where agent_id = 'triage' order by activated_at, activation_id`)
	if err != nil {
		t.Fatalf("read activations: %v", err)
	}
	defer rows.Close()
	var out []activationRow
	for rows.Next() {
		var r activationRow
		if err := rows.Scan(&r.version, &r.seal, &r.path, &r.by); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

// current is the agent's live version, as every reader chooses it.
func (w activationWorld) current(t *testing.T) domain.VersionID {
	t.Helper()
	tx, err := w.pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	live, found, err := spec.CurrentIn(context.Background(), tx, "triage")
	if err != nil || !found {
		t.Fatalf("CurrentIn = %v, found %v", err, found)
	}
	return live.Version
}

// unchanged asserts a refused activation left first live and recorded nothing.
func (w activationWorld) unchanged(t *testing.T, first spec.Spec) {
	t.Helper()
	if got := w.current(t); got != first.Version {
		t.Errorf("current = %s, want %s still live", got, first.Version)
	}
	if rows := w.activations(t); len(rows) != 0 {
		t.Errorf("activations = %+v, want none recorded", rows)
	}
}

func TestActivate_candidate_goesLiveAsItsSealedBytes_andRecordsTheSealRead(t *testing.T) {
	w := newActivationWorld(t)
	first := w.publishFirst(t)
	seal, candidate := w.seal(t, candidateOf("Read the sealed ticket and classify it."))

	got, err := w.publisher.Activate(t.Context(), w.activation(t, candidate.Version, first.Version))
	if err != nil {
		t.Fatalf("Activate = %v, want the candidate live", err)
	}
	if !got.Created || got.SealID != seal || w.current(t) != candidate.Version {
		t.Fatalf("Activate = %+v, current %s; want the candidate created and live from %s", got, w.current(t), seal)
	}
	stored, err := spec.NewRegistry(w.pool).Get(t.Context(), "triage", candidate.Version)
	if err != nil || stored.Instructions != candidate.Instructions || stored.Company != "acme" {
		t.Errorf("stored = %+v (%v), want the sealed definition", stored, err)
	}
	want := []activationRow{{string(candidate.Version), seal, "candidate", "usr_bea"}}
	if rows := w.activations(t); len(rows) != 1 || rows[0] != want[0] {
		t.Errorf("activations = %+v, want %+v", rows, want)
	}
}

func TestActivate_publishedVersion_goesLiveFromItsRow_evenWithEverySealErased(t *testing.T) {
	w := newActivationWorld(t)
	first := w.publishFirst(t)
	seal, candidate := w.seal(t, candidateOf("Read the sealed ticket and classify it."))
	if _, err := w.publisher.Activate(t.Context(), w.activation(t, candidate.Version, first.Version)); err != nil {
		t.Fatalf("Activate(candidate) = %v", err)
	}
	w.erase(t, seal)

	got, err := w.publisher.Activate(t.Context(), w.activation(t, first.Version, candidate.Version))
	if err != nil || got.Created || got.SealID != "" || w.current(t) != first.Version {
		t.Fatalf("Activate(first) = %+v, %v; current %s; want first live again from its row", got, err, w.current(t))
	}
	rows := w.activations(t)
	if len(rows) != 2 || rows[1] != (activationRow{string(first.Version), "", "published", "usr_bea"}) {
		t.Errorf("activations = %+v, want the published path recorded with no seal", rows)
	}
}

func TestActivate_staleTag_refusedWithTheCurrentAndWritesNothing(t *testing.T) {
	w := newActivationWorld(t)
	first := w.publishFirst(t)
	_, candidate := w.seal(t, candidateOf("Read the sealed ticket and classify it."))
	a := w.activation(t, candidate.Version, first.Version)
	a.Guard.Expect = ifMatch(t, domain.AgentRevision(first.Version, true))

	_, err := w.publisher.Activate(t.Context(), a)
	if got := staleAt(t, err); got != domain.AgentRevision(first.Version, false) {
		t.Errorf("current = %s, want the live tag", got)
	}
	w.unchanged(t, first)
}

func TestActivate_callerOutsideTheCurrentScope_isNotFoundBeforeTheStaleTag(t *testing.T) {
	w := newActivationWorld(t)
	first := w.publishFirst(t)
	_, candidate := w.seal(t, candidateOf("Read the sealed ticket and classify it."))
	a := w.activation(t, candidate.Version, first.Version)
	a.Guard.Allow = inScope(domain.Scope{Company: "acme", Area: "finance"})
	a.Guard.Expect = ifMatch(t, domain.AgentRevision(first.Version, true))

	if _, err := w.publisher.Activate(t.Context(), a); !errors.Is(err, spec.ErrAgentNotFound) {
		t.Fatalf("Activate = %v, want not found, never the stale tag", err)
	}
	w.unchanged(t, first)
}

func TestActivate_agentMovedOutOfTheSealedScope_refused(t *testing.T) {
	w := newActivationWorld(t)
	first := w.publishFirst(t)
	_, candidate := w.seal(t, candidateOf("Read the sealed ticket and classify it."))
	a := w.activation(t, candidate.Version, first.Version)
	a.Scope = domain.Scope{Company: "acme", Area: "finance"}

	if _, err := w.publisher.Activate(t.Context(), a); !errors.Is(err, spec.ErrCandidateScopeMoved) {
		t.Fatalf("Activate = %v, want the scope move refused", err)
	}
	w.unchanged(t, first)
}

func TestActivate_autonomousAgent_refused(t *testing.T) {
	w := newActivationWorld(t)
	first := w.publishFirst(t)
	_, candidate := w.seal(t, candidateOf("Read the sealed ticket and classify it."))
	if _, err := w.pool.Exec(t.Context(),
		`update agent_state set stage = 'autonomous' where agent_id = 'triage'`); err != nil {
		t.Fatal(err)
	}

	_, err := w.publisher.Activate(t.Context(), w.activation(t, candidate.Version, first.Version))
	if !errors.Is(err, spec.ErrStageRefused) {
		t.Fatalf("Activate = %v, want the autonomous stage refused", err)
	}
	w.unchanged(t, first)
}

func TestActivate_versionClaimedWithoutADigest_refused(t *testing.T) {
	legacy := published(t, definition)
	pool := withLegacyVersion(t, legacy)
	content := ledger.NewContent(pool)
	w := activationWorld{pool: pool, content: content, publisher: spec.NewPublisher(pool, fixedClock{}),
		reader: spec.NewCandidateReader(pool, content)}
	a := w.activation(t, legacy.Version, legacy.Version)
	a.Guard.Expect = domain.IfMatch{}

	if _, err := w.publisher.Activate(t.Context(), a); !errors.Is(err, spec.ErrDigestUnknown) {
		t.Fatalf("Activate = %v, want the unknown digest refused", err)
	}
	if rows := w.activations(t); len(rows) != 0 {
		t.Errorf("activations = %+v, want none", rows)
	}
}

func TestActivate_candidateWithNoReadableSeal_unreadable(t *testing.T) {
	w := newActivationWorld(t)
	first := w.publishFirst(t)
	seal, candidate := w.seal(t, candidateOf("Read the sealed ticket and classify it."))
	w.erase(t, seal)

	_, err := w.publisher.Activate(t.Context(), w.activation(t, candidate.Version, first.Version))
	if !errors.Is(err, domain.ErrCandidateUnreadable) {
		t.Fatalf("Activate = %v, want the candidate unreadable", err)
	}
	w.unchanged(t, first)
}

// A seal row whose kept bytes are not the claimed ones: written around every writer, which
// claims make unreachable otherwise.
func TestActivate_sealBytesThatDoNotHashToTheClaim_collide(t *testing.T) {
	w := newActivationWorld(t)
	first := w.publishFirst(t)
	seal, candidate := w.seal(t, candidateOf("Read the sealed ticket and classify it."))
	if _, err := w.pool.Exec(t.Context(), `
		update run_content set bytes = bytes || 'And more.'::bytea
		where ref = (select content_ref from agent_seals where seal_id = $1::uuid)`, seal); err != nil {
		t.Fatalf("plant other bytes: %v", err)
	}

	_, err := w.publisher.Activate(t.Context(), w.activation(t, candidate.Version, first.Version))
	if !errors.Is(err, spec.ErrVersionCollision) {
		t.Fatalf("Activate = %v, want a collision", err)
	}
	w.unchanged(t, first)
}

func TestActivate_definitionNotAccepted_writesNothing(t *testing.T) {
	w := newActivationWorld(t)
	first := w.publishFirst(t)
	_, candidate := w.seal(t, candidateOf("Read the sealed ticket and classify it."))
	a := w.activation(t, candidate.Version, first.Version)
	refused := errors.New("no never block")
	var judged string
	a.Accept = func(s spec.Spec) error { judged = s.Instructions; return refused }

	if _, err := w.publisher.Activate(t.Context(), a); !errors.Is(err, refused) {
		t.Fatalf("Activate = %v, want the refusal", err)
	}
	if judged != candidate.Instructions {
		t.Errorf("accepted %q, want the sealed definition judged", judged)
	}
	w.unchanged(t, first)
}

func TestActivate_firstVersion_ifNoneMatch_goesLivePausedInTheSealedScope(t *testing.T) {
	w := newActivationWorld(t)
	seal, candidate := w.seal(t, candidateOf("Read the sealed ticket and classify it."))
	a := w.activation(t, candidate.Version, "")
	a.Guard = spec.Guard{Allow: inScope(cx), Absent: true}

	got, err := w.publisher.Activate(t.Context(), a)
	if err != nil || got.SealID != seal || w.current(t) != candidate.Version {
		t.Fatalf("Activate = %+v, %v; want the first version live", got, err)
	}
	if !got.Paused {
		t.Errorf("paused = %v, want a first version born paused", got.Paused)
	}
}

func TestActivate_ifNoneMatchOnAnAgentThatExists_refused(t *testing.T) {
	w := newActivationWorld(t)
	first := w.publishFirst(t)
	_, candidate := w.seal(t, candidateOf("Read the sealed ticket and classify it."))
	a := w.activation(t, candidate.Version, first.Version)
	a.Guard = spec.Guard{Allow: inScope(cx), Absent: true}

	_, err := w.publisher.Activate(t.Context(), a)
	if got := staleAt(t, err); got != domain.AgentRevision(first.Version, false) {
		t.Errorf("current = %s, want the live tag", got)
	}
	w.unchanged(t, first)
}

// A retry after a failure past the commit: activating the version already live, with the
// tag it has now, succeeds again, so the caller reaches whatever follows the commit.
func TestActivate_theLiveVersionAgainWithItsTag_succeeds(t *testing.T) {
	w := newActivationWorld(t)
	first := w.publishFirst(t)
	_, candidate := w.seal(t, candidateOf("Read the sealed ticket and classify it."))
	if _, err := w.publisher.Activate(t.Context(), w.activation(t, candidate.Version, first.Version)); err != nil {
		t.Fatalf("Activate = %v", err)
	}

	got, err := w.publisher.Activate(t.Context(), w.activation(t, candidate.Version, candidate.Version))
	if err != nil || got.Created || w.current(t) != candidate.Version {
		t.Fatalf("Activate again = %+v, %v; want it live, nothing created", got, err)
	}
}

/*
Under its locks an activation holds one pool connection, the event graph and the agent's
row. Every other activation waits for those locks holding a connection of its own, so a
read of the seal through the pool could wait for a connection nobody returns. The seal is
read in the activation's own transaction: with one connection in the pool, it still goes
through.
*/
func TestActivate_withOnePoolConnection_readsTheSealInItsOwnTransaction(t *testing.T) {
	w := newActivationWorld(t)
	first := w.publishFirst(t)
	seal, candidate := w.seal(t, candidateOf("Read the sealed ticket and classify it."))
	config, err := pgxpool.ParseConfig(testdb.Required(t))
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	content := ledger.NewContent(pool)
	one := activationWorld{pool: pool, content: content, publisher: spec.NewPublisher(pool, fixedClock{}),
		reader: spec.NewCandidateReader(pool, content)}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	got, err := one.publisher.Activate(ctx, one.activation(t, candidate.Version, first.Version))
	if err != nil || got.SealID != seal {
		t.Fatalf("Activate on one connection = %+v, %v; want the candidate live from %s", got, err, seal)
	}
}

/*
No window of seals is fixed. With more seals of one version than any listing answers, the
newest all erased and only the oldest kept, the activation still finds the oldest: the
search walks the seals by key, one row a query, the erased ones skipped by the query
itself, in the activation's transaction.
*/
func TestActivate_theOnlyReadableSealIsOlderThanTwoHundredErasedOnes_isFound(t *testing.T) {
	w := newActivationWorld(t)
	first := w.publishFirst(t)
	text := candidateOf("Read the sealed ticket and classify it.")
	oldest, candidate := w.seal(t, text)
	var erased []string
	for i := 0; i < spec.CandidateListLimit; i++ {
		id, _ := w.seal(t, text)
		erased = append(erased, id)
	}
	tx, err := w.pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := w.content.EraseOwnersTx(t.Context(), tx, "authoring", erased, "test"); err != nil {
		t.Fatalf("erase: %v", err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}

	got, err := w.publisher.Activate(t.Context(), w.activation(t, candidate.Version, first.Version))
	if err != nil || got.SealID != oldest {
		t.Fatalf("Activate = seal %q, %v; want the oldest seal %s, the only one kept", got.SealID, err, oldest)
	}
}

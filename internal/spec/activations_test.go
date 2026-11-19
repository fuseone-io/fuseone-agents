package spec_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/trigger"
)

/*
What the activation route reads before and after the locks (NT-018 phase 2, release 2c).

Before any lock, the version's scope — from its published row and its seals, which name
the same bytes — so the caller is authorised on it, and the definition the publication's
checks run over: the published row, else a readable seal. After the commit, the tag the
agent has now, judged under the lock that wrote it.
*/

func TestActivate_answersTheTagTheAgentHasAfterIt(t *testing.T) {
	for _, retired := range []bool{false, true} {
		w := newActivationWorld(t)
		first := w.publishFirst(t)
		_, candidate := w.seal(t, candidateOf("Read the sealed ticket and classify it."))
		if retired {
			if _, err := w.pool.Exec(t.Context(),
				`update agent_state set retired_at = now() where agent_id = 'triage'`); err != nil {
				t.Fatal(err)
			}
		}
		a := w.activation(t, candidate.Version, first.Version)
		a.Guard.Expect = ifMatch(t, domain.AgentRevision(first.Version, retired))

		got, err := w.publisher.Activate(t.Context(), a)
		if err != nil || got.Revision != domain.AgentRevision(candidate.Version, retired) {
			t.Errorf("retired %v: Activate = %+v, %v; want the candidate's tag", retired, got.Revision, err)
		}
	}
}

func TestActivations_scopes_areThoseOfThePublishedRowAndTheSeals(t *testing.T) {
	w := newActivationWorld(t)
	first := w.publishFirst(t)
	_, candidate := w.seal(t, candidateOf("Read the sealed ticket and classify it."))
	activations := spec.NewActivations(w.publisher, w.reader)

	for _, c := range []struct {
		version domain.VersionID
		want    int
	}{{first.Version, 1}, {candidate.Version, 1}, {"v000000000000", 0}} {
		scopes, err := activations.Scopes(t.Context(), "triage", c.version)
		if err != nil || len(scopes) != c.want || (c.want == 1 && scopes[0] != cx) {
			t.Errorf("Scopes(%s) = %v, %v; want %d scope(s) in cx", c.version, scopes, err, c.want)
		}
	}
}

func TestActivations_definition_isThePublishedRow_elseAReadableSeal_elseNone(t *testing.T) {
	w := newActivationWorld(t)
	first := w.publishFirst(t)
	seal, candidate := w.seal(t, candidateOf("Read the sealed ticket and classify it."))
	activations := spec.NewActivations(w.publisher, w.reader)

	row, found, err := activations.Definition(t.Context(), "triage", first.Version, cx)
	if err != nil || !found || row.Instructions != first.Instructions {
		t.Errorf("Definition(published) = %q, %v, %v; want its row", row.Instructions, found, err)
	}
	sealed, found, err := activations.Definition(t.Context(), "triage", candidate.Version, cx)
	if err != nil || !found || sealed.Digest != candidate.Digest {
		t.Errorf("Definition(candidate) = %q, %v, %v; want the sealed bytes", sealed.Digest, found, err)
	}
	w.erase(t, seal)
	if _, found, err := activations.Definition(t.Context(), "triage", candidate.Version, cx); err != nil || found {
		t.Errorf("Definition(erased) found %v, %v; want none, for the activation to refuse", found, err)
	}
}

/*
What must move with the version — its schedules and webhooks — moves in the activation's
transaction, under the agent's row: a reconciliation run after the commit could land after
a later activation's and put the earlier version's triggers back. Within sees the version
current, and its refusal undoes the activation.
*/
func TestActivate_within_seesTheVersionCurrentAndItsRefusalUndoesTheActivation(t *testing.T) {
	w := newActivationWorld(t)
	first := w.publishFirst(t)
	_, candidate := w.seal(t, candidateOf("Read the sealed ticket and classify it."))
	a := w.activation(t, candidate.Version, first.Version)
	refused := errors.New("the path belongs to another agent")
	var seen domain.VersionID
	a.Within = func(ctx context.Context, tx pgx.Tx, s spec.Spec) error {
		current, _, err := spec.CurrentIn(ctx, tx, "triage")
		seen = current.Version
		return errors.Join(err, refused)
	}

	if _, err := w.publisher.Activate(t.Context(), a); !errors.Is(err, refused) {
		t.Fatalf("Activate = %v, want the refusal", err)
	}
	if seen != candidate.Version {
		t.Errorf("Within saw %s current, want the candidate", seen)
	}
	w.unchanged(t, first)
}

func TestActivate_answersWhetherTheAgentIsPaused_asItsTransactionLeftIt(t *testing.T) {
	w := newActivationWorld(t)
	first := w.publishFirst(t)
	_, candidate := w.seal(t, candidateOf("Read the sealed ticket and classify it."))
	if err := w.publisher.SetPaused(t.Context(), "triage", false, "usr_ops"); err != nil {
		t.Fatal(err)
	}

	got, err := w.publisher.Activate(t.Context(), w.activation(t, candidate.Version, first.Version))
	if err != nil || got.Paused {
		t.Fatalf("Activate = paused %v, %v; want the running agent left running", got.Paused, err)
	}
}

func reconciling(t *testing.T, w activationWorld) *spec.Activations {
	t.Helper()
	if _, err := w.pool.Exec(t.Context(), `truncate trigger_schedules, webhook_triggers`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return spec.NewActivations(w.publisher, w.reader).
		WithReconciliation(trigger.NewPostgresSchedules(w.pool), trigger.NewPostgresWebhooks(w.pool))
}

func schedulesOf(t *testing.T, w activationWorld) []string {
	t.Helper()
	rows, err := w.pool.Query(t.Context(), `select schedule from trigger_schedules where agent_id = 'triage'`)
	if err != nil {
		t.Fatal(err)
	}
	out, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestActivations_activate_reconcilesTheVersionsSchedulesInItsTransaction(t *testing.T) {
	w := newActivationWorld(t)
	first := w.publishFirst(t)
	_, candidate := w.seal(t, candidateOf("Read the sealed ticket and classify it."))
	activations := reconciling(t, w)

	if _, err := activations.Activate(t.Context(), w.activation(t, candidate.Version, first.Version)); err != nil {
		t.Fatalf("Activate = %v", err)
	}
	if got := schedulesOf(t, w); len(got) != 1 || got[0] != "*/15 * * * *" {
		t.Errorf("schedules = %v, want the activated version's cron", got)
	}
}

func TestActivations_activate_aWebhookPathTakenByAnotherAgent_undoesEverything(t *testing.T) {
	w := newActivationWorld(t)
	first := w.publishFirst(t)
	text := strings.Replace(candidateOf("Read the sealed ticket and classify it."),
		`triggers:`, "triggers:\n  - { type: webhook, path: crm/ticket }", 1)
	_, hooked := w.seal(t, text)
	activations := reconciling(t, w)
	if err := trigger.NewPostgresWebhooks(w.pool).Sync(t.Context(), "someone-else", cx,
		[]domain.WebhookTrigger{{Path: "crm/ticket"}}); err != nil {
		t.Fatalf("take the path: %v", err)
	}

	_, err := activations.Activate(t.Context(), w.activation(t, hooked.Version, first.Version))
	if !errors.Is(err, trigger.ErrPathTaken) {
		t.Fatalf("Activate = %v, want the taken path refused", err)
	}
	w.unchanged(t, first)
	if got := schedulesOf(t, w); len(got) != 0 {
		t.Errorf("schedules = %v, want none written by an undone activation", got)
	}
}

/*
The publication's checks run before the locks, on the bytes the route could read then. A
candidate unreadable then is not checked, and a seal of the same version written in
between must not go live unchecked. That refusal is the store's, under the locks and after
everything NT-018 judges first: a caller outside the agent's scope still gets not found, a
stale tag still the current one, and a seal holding other bytes still collides.
*/
func TestActivate_aDefinitionNotCheckedBeforeTheLocks_isRefusedAfterWhatComesFirst(t *testing.T) {
	cases := map[string]struct {
		change func(*spec.Activation)
		want   func(error) bool
	}{
		"checked by nobody": {func(*spec.Activation) {}, func(err error) bool { return errors.Is(err, spec.ErrNotChecked) }},
		"outside the scope": {func(a *spec.Activation) { a.Guard.Allow = inScope(domain.Scope{Company: "acme", Area: "finance"}) },
			func(err error) bool { return errors.Is(err, spec.ErrAgentNotFound) }},
		"a stale tag": {func(a *spec.Activation) { a.Guard.Expect = ifMatch(t, domain.AgentRevision("vstale000001", false)) },
			func(err error) bool { var stale *domain.StaleError; return errors.As(err, &stale) }},
	}
	for name, c := range cases {
		w := newActivationWorld(t)
		first := w.publishFirst(t)
		_, candidate := w.seal(t, candidateOf("Read the sealed ticket and classify it."))
		a := w.activation(t, candidate.Version, first.Version)
		a.Checked = false
		c.change(&a)

		if _, err := w.publisher.Activate(t.Context(), a); !c.want(err) {
			t.Errorf("%s: Activate = %v", name, err)
		}
		w.unchanged(t, first)
	}
}

func TestActivate_aCollisionIsNotMaskedByTheMissingCheck(t *testing.T) {
	w := newActivationWorld(t)
	first := w.publishFirst(t)
	seal, candidate := w.seal(t, candidateOf("Read the sealed ticket and classify it."))
	if _, err := w.pool.Exec(t.Context(), `
		update run_content set bytes = bytes || 'And more.'::bytea
		where ref = (select content_ref from agent_seals where seal_id = $1::uuid)`, seal); err != nil {
		t.Fatal(err)
	}
	a := w.activation(t, candidate.Version, first.Version)
	a.Checked = false

	if _, err := w.publisher.Activate(t.Context(), a); !errors.Is(err, spec.ErrVersionCollision) {
		t.Fatalf("Activate = %v, want the collision", err)
	}
}

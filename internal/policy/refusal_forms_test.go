package policy_test

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/policy"
)

func refusalPoolFor(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("REQUIRE_DATABASE") != "" {
			t.Fatal("REQUIRE_DATABASE is set but TEST_DATABASE_URL is empty")
		}
		t.Skip("TEST_DATABASE_URL is unset; skipping the policy suite")
	}
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := ledger.Migrate(t.Context(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `
		truncate run_steps, runs, gate_refusal_forms, gate_refusal_alert_cursor`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return pool
}

func TestRefusalForms_importsOnlyNewProductionBlocksOnce(t *testing.T) {
	pool := refusalPoolFor(t)
	store := ledger.NewPostgres(pool)
	forms := policy.NewRefusalForms(pool)
	base := time.Now().Add(-time.Hour).UTC().Truncate(time.Microsecond)
	seedRefusalCursor(t, pool, base)

	appendRun(t, store, "run-block", base.Add(time.Second), false, domain.GateDecidedPayload{
		Tool: "crm.delete_account", Effect: domain.EffectDestructive,
		Verdict: domain.VerdictBlock, Rule: "policy", PolicyCode: "POL-100",
	})
	appendRun(t, store, "run-approval", base.Add(2*time.Second), false, domain.GateDecidedPayload{
		Tool: "crm.delete_account", Effect: domain.EffectDestructive,
		Verdict: domain.VerdictRequireApproval, Rule: "approval",
	})
	appendRun(t, store, "run-sim", base.Add(3*time.Second), true, domain.GateDecidedPayload{
		Tool: "crm.delete_account", Effect: domain.EffectDestructive,
		Verdict: domain.VerdictBlock, Rule: "policy", PolicyCode: "POL-100",
	})

	if n, err := forms.Import(t.Context(), base.Add(10*time.Second), 100); err != nil || n != 1 {
		t.Fatalf("Import = %d, %v; want one production block", n, err)
	}
	claimed, err := forms.Claim(t.Context(), "worker-a", time.Minute, 10)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("claimed %d, want one new form", len(claimed))
	}
	first := claimed[0]
	if first.PolicyCode != "POL-100" || first.RuleKey != "POL-100" ||
		first.Tool != "crm.delete_account" || first.Effect != domain.EffectDestructive ||
		first.FirstRunID != "run-block" {
		t.Fatalf("form = %+v, want the first concrete production block", first)
	}
	if err := forms.MarkAnnounced(t.Context(), first, "worker-a", base.Add(11*time.Second)); err != nil {
		t.Fatalf("MarkAnnounced: %v", err)
	}

	appendRun(t, store, "run-same-form", base.Add(20*time.Second), false, domain.GateDecidedPayload{
		Tool: "crm.delete_account", Effect: domain.EffectDestructive,
		Verdict: domain.VerdictBlock, Rule: "policy", PolicyCode: "POL-100",
	})
	if _, err := forms.Import(t.Context(), base.Add(30*time.Second), 100); err != nil {
		t.Fatalf("Import same form: %v", err)
	}
	if again, err := forms.Claim(t.Context(), "worker-a", time.Minute, 10); err != nil || len(again) != 0 {
		t.Fatalf("Claim after same form = %+v, %v; want silence", again, err)
	}

	appendRun(t, store, "run-new-form", base.Add(40*time.Second), false, domain.GateDecidedPayload{
		Tool: "crm.delete_account", Effect: domain.EffectDestructive,
		Verdict: domain.VerdictBlock, Rule: "policy", PolicyCode: "POL-200",
	})
	if _, err := forms.Import(t.Context(), base.Add(50*time.Second), 100); err != nil {
		t.Fatalf("Import new form: %v", err)
	}
	next, err := forms.Claim(t.Context(), "worker-a", time.Minute, 10)
	if err != nil {
		t.Fatalf("Claim new form: %v", err)
	}
	if len(next) != 1 || next[0].PolicyCode != "POL-200" {
		t.Fatalf("next = %+v, want the changed policy code to alert", next)
	}
}

func seedRefusalCursor(t *testing.T, pool *pgxpool.Pool, at time.Time) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), `
		insert into gate_refusal_alert_cursor (id, scanned_at, scanned_run_id, scanned_seq)
		values (true, $1, '', 0)
		on conflict (id) do update set
			scanned_at = excluded.scanned_at,
			scanned_run_id = excluded.scanned_run_id,
			scanned_seq = excluded.scanned_seq`, at.UTC()); err != nil {
		t.Fatalf("seed cursor: %v", err)
	}
}

func appendRun(
	t *testing.T, store *ledger.Postgres, id domain.RunID, at time.Time,
	simulated bool, decision domain.GateDecidedPayload,
) {
	t.Helper()
	started := domain.Step{
		RunID: id, Kind: domain.StepRunStarted,
		Scope:   domain.Scope{Company: "acme", Area: "platform"},
		AgentID: "triage", VersionID: "v1", At: at.UTC(),
	}
	if simulated {
		started.Payload = mustJSON(t, domain.RunStartedPayload{Simulated: true})
	}
	if _, err := store.Append(t.Context(), started); err != nil {
		t.Fatalf("Append start %s: %v", id, err)
	}
	if _, err := store.Append(t.Context(), domain.Step{
		RunID: id, Kind: domain.StepGateDecided,
		Scope:   domain.Scope{Company: "acme", Area: "platform"},
		AgentID: "triage", VersionID: "v1", At: at.Add(time.Second).UTC(),
		Payload: mustJSON(t, decision),
	}); err != nil {
		t.Fatalf("Append decision %s: %v", id, err)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

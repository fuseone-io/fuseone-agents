package admin_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/regression"
)

/*
Retention reaches what a candidate's seal was used for (NT-018 phase 2, release 2a).

A run opened on a candidate read the seal's bytes, and its output may repeat them. When the
seal ages out of the window its derived runs' content goes with it, however recent, and the
sweep fences the agent so nothing copies that content while it goes.
*/

// sealOf writes a seal of agent the way release 2b will, its bytes stored age ago.
func sealOf(t *testing.T, pool *pgxpool.Pool, content *ledger.Content, agent string, age time.Duration) string {
	t.Helper()
	ctx := context.Background()
	var id string
	if err := pool.QueryRow(ctx, `select gen_random_uuid()::text`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	bytes := []byte("sealed definition " + id)
	sum := sha256.Sum256(bytes)
	version := "vseal" + hex.EncodeToString(sum[:])[:7]
	if _, err := pool.Exec(ctx, `
		insert into agent_version_digests (agent_id, version_id, digest) values ($1, $2, $3)`,
		agent, version, hex.EncodeToString(sum[:])); err != nil {
		t.Fatalf("claim: %v", err)
	}
	ref, err := content.PutFor(ctx, "authoring", id, 0, bytes)
	if err != nil {
		t.Fatalf("keep the sealed bytes: %v", err)
	}
	if _, err := pool.Exec(ctx, `update run_content set created_at = now() - $2::interval where ref = $1`,
		ref, age.String()); err != nil {
		t.Fatalf("age the seal: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		insert into agent_seals (seal_id, agent_id, version_id, company_id, area_id, content_ref,
		  draft_id, draft_revision, sealed_by)
		values ($1::uuid, $2, $3, 'acme', 'suporte', $4, gen_random_uuid(), 'rev-1', 'usr_ana')`,
		id, agent, version, ref); err != nil {
		t.Fatalf("seal: %v", err)
	}
	return id
}

// runOnSeal opens a simulated experiment run on seal and keeps fresh content for it.
func runOnSeal(t *testing.T, pool *pgxpool.Pool, content *ledger.Content, agent, seal string) string {
	t.Helper()
	run := fresh("run-on-seal-")
	open(t, pool, agent, run, domain.RunStartedPayload{
		Trigger: "simulation", Simulated: true, Simulation: "exp-1", Experiment: "exp-1", SealID: seal,
	})
	ref, err := content.Put(context.Background(), run, 1, []byte("output echoing the sealed text"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	return ref
}

const sixYears = 6 * 365 * 24 * time.Hour

func TestSweep_anExpiredSeal_takesTheRecentContentOfTheRunsDerivedFromIt(t *testing.T) {
	erasures, content := erasuresFor(t, time.Now)
	pool := openPool(t)
	ctx := context.Background()
	agent := string(fresh("agent-seal-"))

	expired := sealOf(t, pool, content, agent, sixYears)
	derived := runOnSeal(t, pool, content, agent, expired)
	current := sealOf(t, pool, content, agent, time.Hour)
	keptRun := runOnSeal(t, pool, content, agent, current)
	plain := fresh("run-plain-")
	open(t, pool, agent, plain, domain.RunStartedPayload{Trigger: "manual"})
	plainContent, _ := content.Put(ctx, plain, 1, []byte("an ordinary run"))

	if _, err := erasures.Sweep(ctx); err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	if _, err := content.Get(ctx, derived); !errors.Is(err, ledger.ErrErased) {
		t.Errorf("a recent run on the expired seal: err = %v, want erased", err)
	}
	var reason string
	if err := pool.QueryRow(ctx, `select erased_reason from run_content where ref = $1`, derived).Scan(&reason); err != nil || reason != "retention" {
		t.Errorf("reason = %q, %v; want retention", reason, err)
	}
	for name, ref := range map[string]string{"a run on a seal inside the window": keptRun, "an ordinary run": plainContent} {
		if _, err := content.Get(ctx, ref); err != nil {
			t.Errorf("%s: %v, want it kept", name, err)
		}
	}
}

// The agent of an expiring seal is fenced like the agent of an expiring copy: a battery
// copying its runs' content must not race the sweep that takes it.
func TestSweep_waitsForACopyInProgressOnTheAgentOfAnExpiringSeal(t *testing.T) {
	erasures, content := erasuresFor(t, time.Now)
	pool := openPool(t)
	cleanCorpus(t, pool)
	ctx := context.Background()
	agent := string(fresh("agent-fenced-"))
	derived := runOnSeal(t, pool, content, agent, sealOf(t, pool, content, agent, sixYears))

	entered, release, swept := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	held := make(chan error, 1)
	go func() {
		held <- regression.NewStore(pool).Hold(ctx, domain.AgentID(agent), func(context.Context) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	go func() {
		_, err := erasures.Sweep(ctx)
		swept <- err
	}()

	select {
	case err := <-swept:
		t.Fatalf("the sweep did not wait for the agent's fence: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	if err := <-held; err != nil {
		t.Fatalf("Hold: %v", err)
	}
	if err := <-swept; err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if _, err := content.Get(ctx, derived); !errors.Is(err, ledger.ErrErased) {
		t.Fatalf("derived run after the sweep: err = %v, want erased", err)
	}
}

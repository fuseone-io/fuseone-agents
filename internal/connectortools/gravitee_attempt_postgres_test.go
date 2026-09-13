package connectortools

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ticket"
)

func TestPostgresGraviteeAttempts_oneSafeRecordSurvivesRetryAndWorkerHandoff(t *testing.T) {
	pool := graviteeTestPool(t)
	if _, err := pool.Exec(t.Context(), `delete from governed_external_attempts`); err != nil {
		t.Fatalf("clean attempts: %v", err)
	}
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	first := graviteeAttempt(t, pool, now)
	journal := NewPostgresGraviteeAttempts(pool)
	if stored, err := journal.Get(t.Context(), first.IdemKey); err != nil || stored != first {
		t.Fatalf("Get = (%+v, %v), want the atomically claimed attempt", stored, err)
	}

	if due, err := journal.ClaimDue(t.Context(), "worker-a", now, time.Minute, 10); err != nil || len(due) != 0 {
		t.Fatalf("early ClaimDue = (%+v, %v)", due, err)
	}
	due, err := journal.ClaimDue(t.Context(), "worker-a", first.ClaimedUntil, time.Minute, 10)
	if err != nil || len(due) != 1 || due[0].ClaimedBy != "worker-a" {
		t.Fatalf("ClaimDue = (%+v, %v)", due, err)
	}
	if _, err := journal.Resolve(t.Context(), GraviteeAttemptResolution{
		IdemKey: first.IdemKey, ClaimedBy: "worker-b", Status: GraviteeAttemptPending,
		NextCheckAt: now.Add(2 * time.Minute), At: now.Add(time.Minute),
	}); !errors.Is(err, ErrGraviteeAttemptClaim) {
		t.Fatalf("wrong-owner Resolve = %v, want ErrGraviteeAttemptClaim", err)
	}
	pending, err := journal.Arm(t.Context(), first.IdemKey, "worker-a",
		now.Add(2*time.Minute), now.Add(time.Minute))
	if err != nil || pending.Status != GraviteeAttemptPending ||
		pending.Checks != 0 || !pending.ClaimedUntil.IsZero() {
		t.Fatalf("Arm = (%+v, %v)", pending, err)
	}

	due, err = journal.ClaimDue(t.Context(), "worker-b", now.Add(2*time.Minute), time.Minute, 10)
	if err != nil || len(due) != 1 {
		t.Fatalf("second ClaimDue = (%+v, %v)", due, err)
	}
	result := ticket.ContentRef{Ref: "run://result", Digest: "sha256:result"}
	confirmed, err := journal.Resolve(t.Context(), GraviteeAttemptResolution{
		IdemKey: first.IdemKey, ClaimedBy: "worker-b", Status: GraviteeAttemptConfirmed,
		Result: result, OutcomeCode: "accepted", NextCheckAt: now.Add(3 * time.Minute),
		At: now.Add(2 * time.Minute),
	})
	if err != nil || confirmed.Status != GraviteeAttemptConfirmed || confirmed.Result != result {
		t.Fatalf("confirmed Resolve = (%+v, %v)", confirmed, err)
	}
	if err := journal.Settle(t.Context(), first.IdemKey, now.Add(3*time.Minute)); err != nil {
		t.Fatalf("Settle: %v", err)
	}
	settled, err := journal.Get(t.Context(), first.IdemKey)
	if err != nil || !settled.Settled || !settled.NextCheckAt.IsZero() {
		t.Fatalf("settled attempt = (%+v, %v)", settled, err)
	}
}

func TestPostgresGraviteeAttempts_manualAttentionRemainsScheduled(t *testing.T) {
	pool := graviteeTestPool(t)
	if _, err := pool.Exec(t.Context(), `delete from governed_external_attempts`); err != nil {
		t.Fatalf("clean attempts: %v", err)
	}
	now := time.Date(2026, 9, 12, 14, 0, 0, 0, time.UTC)
	first := graviteeAttempt(t, pool, now)
	journal := NewPostgresGraviteeAttempts(pool)
	due, err := journal.ClaimDue(t.Context(), "worker-a", first.ClaimedUntil, time.Minute, 10)
	if err != nil || len(due) != 1 {
		t.Fatalf("ClaimDue = (%+v, %v)", due, err)
	}
	next := now.Add(15 * time.Minute)
	manual, err := journal.Resolve(t.Context(), GraviteeAttemptResolution{
		IdemKey: first.IdemKey, ClaimedBy: "worker-a", Status: GraviteeAttemptManual,
		Result:      ticket.ContentRef{Ref: "run://attention", Digest: "sha256:attention"},
		OutcomeCode: CodeConnectorNeedsAttention, NextCheckAt: next, At: first.ClaimedUntil,
	})
	if err != nil || manual.Status != GraviteeAttemptManual || !manual.NextCheckAt.Equal(next) {
		t.Fatalf("manual Resolve = (%+v, %v), want scheduled at %s", manual, err, next)
	}
	if early, err := journal.ClaimDue(t.Context(), "worker-b", next.Add(-time.Second), time.Minute, 10); err != nil || len(early) != 0 {
		t.Fatalf("early ClaimDue = (%+v, %v)", early, err)
	}
	due, err = journal.ClaimDue(t.Context(), "worker-b", next, time.Minute, 10)
	if err != nil || len(due) != 1 || due[0].Status != GraviteeAttemptManual {
		t.Fatalf("manual ClaimDue = (%+v, %v)", due, err)
	}
}

func graviteeAttempt(t *testing.T, pool *pgxpool.Pool, now time.Time) GraviteeAttempt {
	t.Helper()
	root := fmt.Sprint(now.UnixNano())
	origin := ticket.Origin{
		Connection: "slack-gravitee-attempt", Conversation: "C-support", Root: root,
	}
	key, err := ticket.Key(origin.Connection, origin.Conversation, origin.Root)
	if err != nil {
		t.Fatalf("ticket key: %v", err)
	}
	if _, err := pool.Exec(t.Context(),
		`delete from governed_tickets where ticket_key = $1`, string(key)); err != nil {
		t.Fatalf("clean ticket: %v", err)
	}
	store := ticket.NewPostgres(pool)
	opened, _, err := store.Open(t.Context(), ticket.OpenInput{
		Key: key, Origin: origin,
		Scope:       domain.Scope{Company: "gravitee-attempt-test", Area: "runtime"},
		Agent:       "gateway-support",
		RunAs:       "usr_gateway",
		RequestedBy: "usr_requester", AddressedBy: "app:support",
		EventID: "event-attempt-" + root,
		Draft:   ticket.ContentRef{Ref: "ticket://draft", Digest: "sha256:draft"}, At: now,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	snapshot := ticket.ContentRef{Ref: "run://snapshot", Digest: "sha256:snapshot"}
	if _, _, err := store.RecordInspection(t.Context(), ticket.InspectionInput{
		Ref: opened.Current.Ref, Snapshot: snapshot, At: now,
	}); err != nil {
		t.Fatalf("RecordInspection: %v", err)
	}
	if _, _, err := store.AwaitApproval(t.Context(), ticket.ApprovalInput{
		Ref: opened.Current.Ref, RunID: "run-attempt", AtSeq: 5,
		Snapshot: snapshot, At: now,
	}); err != nil {
		t.Fatalf("AwaitApproval: %v", err)
	}
	_, seeded, _, err := store.ClaimExecutionWithAttempt(t.Context(), ticket.ClaimAttemptInput{
		Claim: ticket.ClaimInput{
			Ref: opened.Current.Ref, RunID: "run-attempt", ApprovalAtSeq: 5, At: now,
		},
		Attempt: ticket.ExternalAttemptInput{
			IdemKey: "gravitee-accept-once", Kind: graviteeAttemptKind, CallSeq: 7,
			Instance: "apim", Scope: area("gravitee-attempt-test", "runtime"),
			ContractDigest: "gravitee-contract/v1:sha256:fixed", TargetID: "sub-42",
			DecidedBy: "usr_approver", NextCheckAt: now.Add(time.Minute),
			DeadlineAt: now.Add(10 * time.Minute), ClaimedBy: "invoke:run-attempt:7",
			ClaimedUntil: now.Add(time.Minute),
		},
	})
	if err != nil {
		t.Fatalf("ClaimExecutionWithAttempt: %v", err)
	}
	attempt, err := graviteeAttemptFromExternal(seeded)
	if err != nil {
		t.Fatalf("decode attempt: %v", err)
	}
	return attempt
}

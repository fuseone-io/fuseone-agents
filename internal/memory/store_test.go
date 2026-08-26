package memory_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/memory"
)

var (
	platformScope = domain.Scope{Company: "acme", Area: "platform"}
	financeScope  = domain.Scope{Company: "acme", Area: "finance"}
)

func TestLayer_findStoresAClaimCheckAndPropagatesLabels(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := memory.NewMemory()
	content := engine.NewMemoryContent()
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	_, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
		a.Labels = domain.NewLabels(domain.LabelUntrusted)
		a.Claim = "restart the api deployment after the datasource error clears"
	}), "usr_ana", "reviewed incident", now)
	if err != nil {
		t.Fatalf("Assert: %v", err)
	}

	metrics := &recordingMemoryMetrics{}
	result, err := memory.NewLayer(nil, nil, content, store).WithMetrics(metrics).Invoke(ctx, engine.Call{
		RunID: "run-memory-1", Seq: 4, Scope: platformScope, AgentID: "triage",
		Tool: domain.ToolMemoryFind, Args: []byte(`{"search":"datasource"}`),
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if result.Failed || result.ResultRef == "" {
		t.Fatalf("result = %+v, want stored memory result", result)
	}
	if !result.Labels.Has(domain.LabelUntrusted) {
		t.Fatalf("labels = %v, want source taint propagated", result.Labels)
	}
	body, err := content.Get(ctx, result.ResultRef)
	if err != nil {
		t.Fatalf("Get result: %v", err)
	}
	var payload struct {
		Assertions []struct {
			Claim string `json:"claim"`
		} `json:"assertions"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if len(payload.Assertions) != 1 || payload.Assertions[0].Claim == "" {
		t.Fatalf("payload = %s, want one structured assertion", body)
	}
	if len(metrics.finds) != 1 || metrics.finds[0].returned != 1 ||
		metrics.finds[0].omitted != 0 || metrics.finds[0].failed {
		t.Fatalf("metrics = %+v, want successful memory find", metrics.finds)
	}
}

func TestLayer_findNamesAssertionsOmittedByTheResponseBudget(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := memory.NewMemory()
	content := engine.NewMemoryContent()
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	if _, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
		a.Subject, a.Confirmed, a.Observations = "budget first", 3, 3
		a.Claim = "small remembered fact"
	}), "usr_ana", "reviewed", now); err != nil {
		t.Fatalf("Assert first: %v", err)
	}
	if _, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
		a.Subject, a.Confirmed, a.Observations = "budget large", 2, 2
		a.Labels = domain.NewLabels(domain.LabelPersonal, domain.LabelUntrusted)
		a.Evidence[0].Artifact = strings.Repeat("large-artifact-name", 1600)
	}), "usr_ana", "reviewed", now); err != nil {
		t.Fatalf("Assert large: %v", err)
	}
	if _, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
		a.Subject, a.Confirmed, a.Observations = "budget later", 1, 1
	}), "usr_ana", "reviewed", now); err != nil {
		t.Fatalf("Assert later: %v", err)
	}

	result, err := memory.NewLayer(nil, nil, content, store).Invoke(ctx, engine.Call{
		RunID: "run-memory-1", Seq: 4, Scope: platformScope, AgentID: "triage",
		Tool: domain.ToolMemoryFind, Args: []byte(`{"search":"budget","limit":10}`),
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if result.Failed || !result.Labels.Has(domain.LabelUntrusted) {
		t.Fatalf("result = %+v, want success carrying labels from matched memory", result)
	}
	if !result.Labels.Has(domain.LabelPersonal) {
		t.Fatalf("labels = %v, want labels from omitted memory to keep tainting the run", result.Labels)
	}
	body, err := content.Get(ctx, result.ResultRef)
	if err != nil {
		t.Fatalf("Get result: %v", err)
	}
	var payload struct {
		Assertions []struct {
			Subject string `json:"subject"`
		} `json:"assertions"`
		Omitted       int    `json:"omitted"`
		OmittedReason string `json:"omitted_reason"`
		ByteBudget    int    `json:"byte_budget"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if len(payload.Assertions) != 1 || payload.Assertions[0].Subject != "budget first" ||
		payload.Omitted != 2 || payload.OmittedReason != "result_byte_budget" ||
		payload.ByteBudget <= 0 {
		t.Fatalf("payload = %s, want explicit memory budget omission", body)
	}
}

func TestLayer_rejectsMalformedFindArguments(t *testing.T) {
	t.Parallel()
	result, err := memory.NewLayer(nil, nil, engine.NewMemoryContent(), memory.NewMemory()).
		Invoke(context.Background(), engine.Call{
			RunID: "run-memory-1", Seq: 4, Scope: platformScope, AgentID: "triage",
			Tool: domain.ToolMemoryFind, Args: []byte(`{"search":"x"} 1`),
		})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !result.Failed || result.ErrorCode != memory.CodeMemoryArgumentsInvalid {
		t.Fatalf("result = %+v, want invalid-argument tool failure", result)
	}
}

func TestToolList_exposesMemoryAsANativeReadSource(t *testing.T) {
	t.Parallel()
	got, err := memory.NewToolList(toolSource([]domain.ToolEntry{
		{ID: "crm.lookup", Effect: domain.EffectRead, OnSurface: true},
	})).Tools(context.Background())
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	found := entryNamed(got, domain.ToolMemoryFind)
	if found.ID == "" || !found.Native || found.Effect != domain.EffectRead ||
		!found.Untrusted || !found.Scope.Contains(platformScope) {
		t.Fatalf("memory tool = %+v, want native read source", found)
	}
}

func TestMemoryAssert_correctionKeepsOriginalCreationStamp(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := memory.NewMemory()
	firstAt := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	secondAt := firstAt.Add(time.Hour)
	created, err := store.Assert(ctx, assertion(nil), "usr_ana", "created", firstAt)
	if err != nil {
		t.Fatalf("Assert create: %v", err)
	}
	updated, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
		a.Claim = "refresh the datasource token before restarting the worker"
	}), "usr_beto", "corrected", secondAt)
	if err != nil {
		t.Fatalf("Assert update: %v", err)
	}

	if updated.CreatedAt != created.CreatedAt || updated.CreatedBy != created.CreatedBy {
		t.Fatalf("created = (%s,%s), want original (%s,%s)",
			updated.CreatedAt, updated.CreatedBy, created.CreatedAt, created.CreatedBy)
	}
	if updated.UpdatedAt != secondAt || updated.UpdatedBy != "usr_beto" {
		t.Fatalf("updated = (%s,%s), want correction stamp", updated.UpdatedAt, updated.UpdatedBy)
	}
}

func TestPostgresList_respectsScopeHierarchyAndDeniesEmptyScope(t *testing.T) {
	ctx, store := postgresStore(t)
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	assertAt(t, store, assertion(func(a *domain.MemoryAssertion) {
		a.Scope, a.Subject = platformScope, "platform outage"
	}), now)
	assertAt(t, store, assertion(func(a *domain.MemoryAssertion) {
		a.Scope, a.Subject = financeScope, "finance outage"
	}), now)
	assertAt(t, store, assertion(func(a *domain.MemoryAssertion) {
		a.Scope = domain.Scope{Company: "globex", Area: "platform"}
	}), now)

	if got := list(t, ctx, store, nil); len(got) != 0 {
		t.Fatalf("empty scopes returned %d assertions", len(got))
	}
	if got := list(t, ctx, store, []domain.Scope{platformScope}); len(got) != 1 {
		t.Fatalf("area grant returned %d assertions, want 1", len(got))
	}
	if got := list(t, ctx, store, []domain.Scope{{Company: "acme"}}); len(got) != 2 {
		t.Fatalf("company grant returned %d assertions, want 2", len(got))
	}
	if got := list(t, ctx, store, []domain.Scope{{Company: domain.Installation}}); len(got) != 3 {
		t.Fatalf("installation grant returned %d assertions, want 3", len(got))
	}
}

func TestPostgresFind_returnsOnlyTheRunScopeAndAgentOrSharedMemory(t *testing.T) {
	ctx, store := postgresStore(t)
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	assertAt(t, store, assertion(func(a *domain.MemoryAssertion) {
		a.AgentID, a.Subject = "triage", "needle for triage"
	}), now)
	assertAt(t, store, assertion(func(a *domain.MemoryAssertion) {
		a.AgentID, a.Subject = "", "needle shared"
	}), now)
	assertAt(t, store, assertion(func(a *domain.MemoryAssertion) {
		a.AgentID, a.Subject = "billing", "needle billing"
	}), now)
	assertAt(t, store, assertion(func(a *domain.MemoryAssertion) {
		a.Scope, a.AgentID, a.Subject = financeScope, "triage", "needle finance"
	}), now)

	found, err := store.Find(ctx, domain.MemoryQuery{
		Scope: platformScope, AgentID: "triage", Search: "needle", Limit: 10, Now: now,
	})
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	got := subjects(found)
	want := []string{"needle shared", "needle for triage"}
	if !sameStrings(got, want) {
		t.Fatalf("subjects = %v, want %v", got, want)
	}
}

func TestPostgresList_statusFilterUnderstandsVirtualExpiry(t *testing.T) {
	ctx, store := postgresStore(t)
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)
	assertAt(t, store, assertion(func(a *domain.MemoryAssertion) {
		a.Subject, a.ExpiresAt = "expired by time", &past
	}), now)
	assertAt(t, store, assertion(func(a *domain.MemoryAssertion) {
		a.Subject = "still active"
	}), now)

	expired, err := store.List(ctx, memory.Filter{
		Scopes: []domain.Scope{platformScope}, Status: domain.MemoryExpired, Now: now,
	})
	if err != nil {
		t.Fatalf("List expired: %v", err)
	}
	if len(expired) != 1 || expired[0].Subject != "expired by time" ||
		expired[0].Status != domain.MemoryExpired {
		t.Fatalf("expired = %+v, want only the virtually expired assertion", expired)
	}
}

func TestPostgresMemoryEvents_refuseUpdateButAllowRetentionDelete(t *testing.T) {
	ctx, pool := postgresPool(t)
	store := memory.NewPostgres(pool)
	created, err := store.Assert(ctx, assertion(nil), "usr_ana", "reviewed", time.Unix(0, 0))
	if err != nil {
		t.Fatalf("Assert: %v", err)
	}

	_, err = pool.Exec(ctx,
		`update memory_assertion_events set reason = 'edited' where assertion_id = $1`,
		created.ID)
	if !isRestrictViolation(err) {
		t.Fatalf("update event err = %v, want restrict_violation", err)
	}
	if _, err := pool.Exec(ctx,
		`delete from memory_assertion_events where assertion_id = $1`,
		created.ID); err != nil {
		t.Fatalf("delete event: %v", err)
	}
}

func TestPostgresMigration_addsTrigramIndexesForMemorySearch(t *testing.T) {
	ctx, pool := postgresPool(t)
	for index, column := range map[string]string{
		"memory_assertions_subject_trgm_idx":   "subject gin_trgm_ops",
		"memory_assertions_signature_trgm_idx": "signature gin_trgm_ops",
		"memory_assertions_claim_trgm_idx":     "claim gin_trgm_ops",
	} {
		var definition string
		err := pool.QueryRow(ctx, `
			select indexdef
			from pg_indexes
			where schemaname = current_schema() and indexname = $1`, index).Scan(&definition)
		if err != nil {
			t.Fatalf("check %s: %v", index, err)
		}
		if !strings.Contains(definition, "USING gin") ||
			!strings.Contains(definition, column) ||
			!strings.Contains(definition, "WHERE (status = 'active'::text)") {
			t.Fatalf("%s = %s, want active trigram GIN index on %s", index, definition, column)
		}
	}
}

type toolSource []domain.ToolEntry

func (t toolSource) Tools(context.Context) ([]domain.ToolEntry, error) {
	return append([]domain.ToolEntry(nil), t...), nil
}

func entryNamed(entries []domain.ToolEntry, id domain.ToolID) domain.ToolEntry {
	for _, entry := range entries {
		if entry.ID == id {
			return entry
		}
	}
	return domain.ToolEntry{}
}

func assertion(edit func(*domain.MemoryAssertion)) domain.MemoryAssertion {
	a := domain.MemoryAssertion{
		Scope: platformScope, AgentID: "triage", Kind: "incident",
		Subject: "grafana datasource", Signature: "grafana.datasource.down",
		Claim: "datasource errors clear after refreshing the datasource token",
		Evidence: []domain.MemoryEvidence{{
			RunID: "run-evidence-1", Artifact: "final_answer", Digest: "sha256:abcd",
		}},
		Observations: 2, Confirmed: 1, Status: domain.MemoryActive,
	}
	if edit != nil {
		edit(&a)
	}
	return a
}

func postgresStore(t *testing.T) (context.Context, *memory.Postgres) {
	t.Helper()
	ctx, pool := postgresPool(t)
	return ctx, memory.NewPostgres(pool)
}

func postgresPool(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("REQUIRE_DATABASE") != "" {
			t.Fatal("REQUIRE_DATABASE is set but TEST_DATABASE_URL is empty")
		}
		t.Skip("TEST_DATABASE_URL is unset; skipping memory Postgres tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := ledger.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := pool.Exec(ctx, `truncate memory_assertion_events, memory_assertions`); err != nil {
		t.Fatalf("clean: %v", err)
	}
	return ctx, pool
}

func assertAt(t *testing.T, store *memory.Postgres, a domain.MemoryAssertion, now time.Time) {
	t.Helper()
	if _, err := store.Assert(context.Background(), a, "usr_ana", "reviewed", now); err != nil {
		t.Fatalf("Assert %s: %v", a.Subject, err)
	}
}

func list(t *testing.T, ctx context.Context, store *memory.Postgres, scopes []domain.Scope) []domain.MemoryAssertion {
	t.Helper()
	got, err := store.List(ctx, memory.Filter{Scopes: scopes, Limit: 50})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	return got
}

func subjects(assertions []domain.MemoryAssertion) []string {
	out := make([]string, 0, len(assertions))
	for _, a := range assertions {
		out = append(out, a.Subject)
	}
	return out
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	seen := map[string]int{}
	for _, v := range got {
		seen[v]++
	}
	for _, v := range want {
		if seen[v] == 0 {
			return false
		}
		seen[v]--
	}
	return true
}

func isRestrictViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23001"
}

type recordingMemoryMetrics struct{ finds []memoryFindMetric }

type memoryFindMetric struct {
	returned int
	omitted  int
	failed   bool
}

func (r *recordingMemoryMetrics) MemoryFind(_ time.Duration, returned int, omitted int, failed bool) {
	r.finds = append(r.finds, memoryFindMetric{returned: returned, omitted: omitted, failed: failed})
}

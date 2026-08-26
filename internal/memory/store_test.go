package memory_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/gate"
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

func TestLayer_suggestRecordsPendingMemoryWithoutServingIt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := memory.NewMemory()
	content := engine.NewMemoryContent()
	labels := domain.NewLabels(domain.LabelUntrusted).Union(domain.ScopeLabels(platformScope))

	result, err := memory.NewLayer(nil, nil, content, store).Invoke(ctx, engine.Call{
		RunID: "run-suggest-1", Seq: 3, Scope: platformScope, AgentID: "triage",
		Tool: domain.ToolMemorySuggest, Args: suggestionArgs("grafana.datasource.down"),
		Labels: labels,
		MemoryLearning: domain.MemoryLearningPolicy{
			Mode: domain.MemoryLearningReview,
		},
		At: time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if result.Failed || result.ResultRef == "" {
		t.Fatalf("result = %+v, want stored suggestion result", result)
	}
	suggestions, err := store.ListSuggestions(ctx, memory.SuggestionFilter{
		Scopes: []domain.Scope{platformScope}, Status: domain.MemorySuggestionPending,
	})
	if err != nil {
		t.Fatalf("ListSuggestions: %v", err)
	}
	if len(suggestions) != 1 || suggestions[0].Observations != 1 ||
		!suggestions[0].Labels.Has(domain.LabelUntrusted) {
		t.Fatalf("suggestions = %+v, want one labelled pending suggestion", suggestions)
	}
	found, err := store.Find(ctx, domain.MemoryQuery{
		Scope: platformScope, AgentID: "triage", Search: "grafana",
		Now: time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if len(found) != 0 {
		t.Fatalf("Find returned %+v, want pending suggestion invisible to recall", found)
	}
}

func TestLayer_suggestAutoConfirmsOnlyAfterRepeatedObservations(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := memory.NewMemory()
	content := engine.NewMemoryContent()
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	policy := domain.MemoryLearningPolicy{
		Mode: domain.MemoryLearningAutoConfirm, MinObservations: 3, TTLDays: 7,
	}

	for i := range 3 {
		result, err := memory.NewLayer(nil, nil, content, store).Invoke(ctx, engine.Call{
			RunID: domain.RunID("run-suggest-" + string(rune('a'+i))), Seq: int64(i + 1),
			Scope: platformScope, AgentID: "triage", Tool: domain.ToolMemorySuggest,
			Args: suggestionArgs("grafana.datasource.down"), Labels: domain.NewLabels(domain.LabelPersonal),
			MemoryLearning: policy, At: now.Add(time.Duration(i) * time.Minute),
		})
		if err != nil {
			t.Fatalf("Invoke %d: %v", i, err)
		}
		if result.Failed {
			t.Fatalf("result %d = %+v, want successful suggestion", i, result)
		}
	}

	found, err := store.Find(ctx, domain.MemoryQuery{
		Scope: platformScope, AgentID: "triage", Search: "datasource", Now: now,
	})
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if len(found) != 1 || found[0].Confirmed != 3 ||
		!found[0].Labels.Has(domain.LabelPersonal) ||
		found[0].ExpiresAt == nil ||
		!found[0].ExpiresAt.Equal(now.Add(2*time.Minute).AddDate(0, 0, 7)) {
		t.Fatalf("found = %+v, want auto-confirmed labelled memory with TTL", found)
	}
}

func TestLayer_suggestCountsOneObservationPerRun(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := memory.NewMemory()
	content := engine.NewMemoryContent()
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	policy := domain.MemoryLearningPolicy{
		Mode: domain.MemoryLearningAutoConfirm, MinObservations: 2, TTLDays: 7,
	}
	call := engine.Call{
		RunID: "run-suggest-1", Seq: 1, Scope: platformScope, AgentID: "triage",
		Tool: domain.ToolMemorySuggest, Args: suggestionArgs("grafana.datasource.down"),
		MemoryLearning: policy, At: now,
	}

	layer := memory.NewLayer(nil, nil, content, store)
	for i := range 2 {
		call.Seq = int64(i + 1)
		result, err := layer.Invoke(ctx, call)
		if err != nil {
			t.Fatalf("Invoke duplicate %d: %v", i, err)
		}
		if result.Failed {
			t.Fatalf("result duplicate %d = %+v, want successful suggestion", i, result)
		}
	}
	if found := findMemory(t, store, now); len(found) != 0 {
		t.Fatalf("found = %+v, want repeated suggestion in the same run not auto-confirmed", found)
	}

	call.RunID, call.Seq, call.At = "run-suggest-2", 1, now.Add(time.Minute)
	result, err := layer.Invoke(ctx, call)
	if err != nil {
		t.Fatalf("Invoke second run: %v", err)
	}
	if result.Failed {
		t.Fatalf("result second run = %+v, want successful suggestion", result)
	}
	found := findMemory(t, store, now.Add(time.Minute))
	if len(found) != 1 || found[0].Confirmed != 2 {
		t.Fatalf("found = %+v, want auto-confirm after two distinct runs", found)
	}
}

func TestLayer_suggestReportsStoreErrorsAsMemoryUnavailable(t *testing.T) {
	t.Parallel()

	result, err := memory.NewLayer(nil, nil, engine.NewMemoryContent(), failingStore{}).
		Invoke(context.Background(), engine.Call{
			RunID: "run-suggest-1", Seq: 3, Scope: platformScope, AgentID: "triage",
			Tool: domain.ToolMemorySuggest, Args: suggestionArgs("grafana.datasource.down"),
			MemoryLearning: domain.MemoryLearningPolicy{Mode: domain.MemoryLearningReview},
			At:             time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC),
		})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !result.Failed || result.ErrorCode != memory.CodeMemoryUnavailable {
		t.Fatalf("result = %+v, want memory unavailable tool failure", result)
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

func TestToolList_exposesMemorySuggestAsANativeWrite(t *testing.T) {
	t.Parallel()
	got, err := memory.NewToolList(nil).Tools(context.Background())
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	found := entryNamed(got, domain.ToolMemorySuggest)
	if found.ID == "" || !found.Native || found.Effect != domain.EffectWrite ||
		found.Untrusted || !found.Scope.Contains(platformScope) {
		t.Fatalf("memory suggest tool = %+v, want native state-writing suggestion tool", found)
	}
	layer := memory.NewLayer(nil, nil, nil, nil)
	effect, ok := layer.Effect(domain.ToolMemorySuggest)
	if !ok || effect != domain.EffectWrite {
		t.Fatalf("Effect(%s) = %v/%v, want write/true", domain.ToolMemorySuggest, effect, ok)
	}
}

func TestMemorySuggest_taintedSuggestionGoesThroughTheGateAsAWrite(t *testing.T) {
	t.Parallel()
	entry := memory.MemorySuggestToolEntry()
	decision, err := gate.New().Evaluate(context.Background(), gate.Request{
		Scope: platformScope, RunID: "run-suggest-gate", AgentID: "triage", Seq: 1,
		Tool: entry.ID, Effect: entry.Effect, Args: suggestionArgs("grafana.datasource.down"),
		ArgLabels: domain.NewLabels(domain.LabelUntrusted).Union(domain.ScopeLabels(platformScope)),
		Pack:      gate.NewPack(entry.ID), Stage: domain.StageAutonomous,
		Budget: domain.Budget{ToolCalls: 10}, Estimate: domain.Consumption{ToolCalls: 1},
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if decision.Verdict != domain.VerdictRequireApproval || decision.Rule != gate.RuleTaint {
		t.Fatalf("decision = %+v, want tainted write to require approval", decision)
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

func TestPostgresSuggest_acceptancePromotesOnlyPendingMemory(t *testing.T) {
	ctx, store := postgresStore(t)
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	first, err := store.Suggest(ctx, suggestion(func(s *domain.MemorySuggestion) {
		s.Labels = domain.NewLabels(domain.LabelUntrusted)
	}), domain.MemoryLearningPolicy{Mode: domain.MemoryLearningReview}, "agent:triage", now)
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	second, err := store.Suggest(ctx, suggestion(func(s *domain.MemorySuggestion) {
		s.Labels = domain.NewLabels(domain.LabelPersonal)
		s.Evidence[0].RunID = "run-evidence-2"
	}), domain.MemoryLearningPolicy{Mode: domain.MemoryLearningReview}, "agent:triage", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("Suggest again: %v", err)
	}
	if second.Suggestion.ID != first.Suggestion.ID ||
		second.Suggestion.Observations != 2 ||
		!second.Suggestion.Labels.Has(domain.LabelUntrusted) ||
		!second.Suggestion.Labels.Has(domain.LabelPersonal) {
		t.Fatalf("suggestion = %+v, want repeated pending suggestion merged with labels", second.Suggestion)
	}

	accepted, err := store.AcceptSuggestion(ctx, first.Suggestion.ID, platformScope,
		"usr_ana", "reviewed", now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("AcceptSuggestion: %v", err)
	}
	if accepted.Confirmed != 2 || !accepted.Labels.Has(domain.LabelUntrusted) ||
		!accepted.Labels.Has(domain.LabelPersonal) {
		t.Fatalf("accepted = %+v, want merged labelled assertion", accepted)
	}
	pending, err := store.ListSuggestions(ctx, memory.SuggestionFilter{
		Scopes: []domain.Scope{platformScope}, Status: domain.MemorySuggestionPending,
	})
	if err != nil {
		t.Fatalf("ListSuggestions pending: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending = %+v, want accepted suggestion removed from review queue", pending)
	}
}

func TestPostgresSuggest_concurrentEquivalentSuggestionsMerge(t *testing.T) {
	ctx, store := postgresStore(t)
	const writers = 12
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	policy := domain.MemoryLearningPolicy{Mode: domain.MemoryLearningReview}

	for i := range writers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, err := store.Suggest(ctx, suggestion(func(s *domain.MemorySuggestion) {
				s.Evidence[0].RunID = domain.RunID("run-concurrent-" + string(rune('a'+i)))
			}), policy, "agent:triage", now.Add(time.Duration(i)*time.Second))
			errs <- err
		}(i)
	}

	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Suggest concurrent: %v", err)
		}
	}
	got, err := store.ListSuggestions(ctx, memory.SuggestionFilter{
		Scopes: []domain.Scope{platformScope}, Status: domain.MemorySuggestionPending,
	})
	if err != nil {
		t.Fatalf("ListSuggestions: %v", err)
	}
	if len(got) != 1 || got[0].Observations != writers ||
		len(got[0].Evidence) != domain.MaxMemoryEvidence {
		t.Fatalf("suggestions = %+v, want one merged bounded suggestion", got)
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

type failingStore struct{}

func (failingStore) Find(context.Context, domain.MemoryQuery) ([]domain.MemoryAssertion, error) {
	return nil, errors.New("down")
}

func (failingStore) List(context.Context, memory.Filter) ([]domain.MemoryAssertion, error) {
	return nil, errors.New("down")
}

func (failingStore) Assert(
	context.Context, domain.MemoryAssertion, domain.UserID, string, time.Time,
) (domain.MemoryAssertion, error) {
	return domain.MemoryAssertion{}, errors.New("down")
}

func (failingStore) Disable(context.Context, string, domain.Scope, domain.UserID, string, time.Time) error {
	return errors.New("down")
}

func (failingStore) Suggest(
	context.Context, domain.MemorySuggestion, domain.MemoryLearningPolicy, domain.UserID, time.Time,
) (domain.MemorySuggestionOutcome, error) {
	return domain.MemorySuggestionOutcome{}, errors.New("down")
}

func (failingStore) ListSuggestions(context.Context, memory.SuggestionFilter) ([]domain.MemorySuggestion, error) {
	return nil, errors.New("down")
}

func (failingStore) AcceptSuggestion(
	context.Context, string, domain.Scope, domain.UserID, string, time.Time,
) (domain.MemoryAssertion, error) {
	return domain.MemoryAssertion{}, errors.New("down")
}

func (failingStore) DismissSuggestion(context.Context, string, domain.Scope, domain.UserID, string, time.Time) error {
	return errors.New("down")
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

func suggestion(edit func(*domain.MemorySuggestion)) domain.MemorySuggestion {
	s := domain.MemorySuggestion{
		Scope: platformScope, AgentID: "triage", Kind: "incident",
		Subject: "grafana datasource", Signature: "grafana.datasource.down",
		Claim: "datasource errors clear after refreshing the datasource token",
		Evidence: []domain.MemoryEvidence{{
			RunID: "run-evidence-1", Artifact: domain.ArtifactMemorySuggestion, Digest: "sha256:abcd",
		}},
	}
	if edit != nil {
		edit(&s)
	}
	return s
}

func suggestionArgs(signature string) []byte {
	return []byte(`{"kind":"incident","subject":"grafana datasource","signature":"` +
		signature + `","claim":"datasource errors clear after refreshing the datasource token"}`)
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
	if _, err := pool.Exec(ctx, `truncate memory_assertion_events, memory_suggestions, memory_assertions`); err != nil {
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

func findMemory(t *testing.T, store *memory.Memory, now time.Time) []domain.MemoryAssertion {
	t.Helper()
	found, err := store.Find(context.Background(), domain.MemoryQuery{
		Scope: platformScope, AgentID: "triage", Search: "datasource", Now: now,
	})
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	return found
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

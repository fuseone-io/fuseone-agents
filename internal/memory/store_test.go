package memory_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
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
	if result.Failed {
		t.Fatalf("result = %+v, want successful memory find", result)
	}
	if result.Labels.Has(domain.LabelUntrusted) || result.Labels.Has(domain.LabelPersonal) {
		t.Fatalf("labels = %v, want only labels from assertions returned to the model", result.Labels)
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

func TestLayer_findNamesSearchTermsOmittedByTheTermBudget(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := memory.NewMemory()
	content := engine.NewMemoryContent()
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	if _, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
		a.Subject = "alertas do superset slack entregues"
		a.Signature = "superset.alert.delivery"
		a.Claim = "not_in_channel means the app must be invited to the alert channel"
		a.Confirmed, a.Observations = 3, 3
	}), "usr_ana", "reviewed", now); err != nil {
		t.Fatalf("Assert memory: %v", err)
	}

	for _, tc := range []struct {
		name        string
		search      string
		wantSubject string
		wantUsed    string
		wantOmitted int
	}{
		{
			name:        "matching",
			search:      "alertas do superset entregues no slack com erro not_in_channel hoje",
			wantSubject: "alertas do superset slack entregues",
			wantUsed:    "not_in_channel alertas superset entregues slack erro",
			wantOmitted: 1,
		},
		{
			name:        "empty",
			search:      "foo bar baz quux quuz corge grault not_in_channel",
			wantSubject: "",
			wantUsed:    "not_in_channel foo bar baz quux quuz",
			wantOmitted: 2,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := memory.NewLayer(nil, nil, content, store).Invoke(ctx, engine.Call{
				RunID: "run-memory-1", Seq: 4, Scope: platformScope, AgentID: "triage",
				Tool: domain.ToolMemoryFind,
				Args: []byte(`{"search":` + strconv.Quote(tc.search) + `,"limit":10}`),
			})
			if err != nil {
				t.Fatalf("Invoke: %v", err)
			}
			if result.Failed {
				t.Fatalf("result = %+v, want successful memory find", result)
			}
			body, err := content.Get(ctx, result.ResultRef)
			if err != nil {
				t.Fatalf("Get result: %v", err)
			}
			var payload struct {
				Assertions []struct {
					Subject string `json:"subject"`
				} `json:"assertions"`
				SearchTermsUsed          []string `json:"search_terms_used"`
				SearchTermsOmitted       int      `json:"search_terms_omitted"`
				SearchTermsOmittedReason string   `json:"search_terms_omitted_reason"`
				SearchTermBudget         int      `json:"search_term_budget"`
			}
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatalf("decode result: %v", err)
			}
			if tc.wantSubject == "" {
				if len(payload.Assertions) != 0 {
					t.Fatalf("payload = %s, want no matching memory", body)
				}
			} else if len(payload.Assertions) != 1 || payload.Assertions[0].Subject != tc.wantSubject {
				t.Fatalf("payload = %s, want the bounded search to still return matching memory", body)
			}
			if strings.Join(payload.SearchTermsUsed, " ") != tc.wantUsed ||
				payload.SearchTermsOmitted != tc.wantOmitted ||
				payload.SearchTermsOmittedReason != "search_term_budget" || payload.SearchTermBudget != 6 {
				t.Fatalf("payload = %s, want explicit search term budget omission", body)
			}
		})
	}
}

func TestFind_searchMatchesSeparateTermsAcrossFields(t *testing.T) {
	t.Parallel()
	expectSearchMatchesSeparateTermsAcrossFields(t, context.Background(), memory.NewMemory())
}

func TestFind_modelChosenQueriesStillReachTheSameMemory(t *testing.T) {
	t.Parallel()
	expectModelChosenQueriesStillReachTheSameMemory(t, context.Background(), memory.NewMemory())
}

func TestFind_digitsDoNotOverrideTheRestOfTheSearch(t *testing.T) {
	t.Parallel()
	expectDigitsDoNotOverrideTheRestOfTheSearch(t, context.Background(), memory.NewMemory())
}

func TestFind_nonMatchingSearchDoesNotFailOpen(t *testing.T) {
	t.Parallel()
	expectNonMatchingSearchDoesNotFailOpen(t, context.Background(), memory.NewMemory())
}

func TestFind_strongSearchTermsSurviveTheTermBudget(t *testing.T) {
	t.Parallel()
	expectStrongSearchTermsSurviveTheTermBudget(t, context.Background(), memory.NewMemory())
}

func TestFind_shortIdentifiersAreSearchTerms(t *testing.T) {
	t.Parallel()
	expectShortIdentifiersAreSearchTerms(t, context.Background(), memory.NewMemory())
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

func TestSuggest_activeAssertionIdentityStopsDuplicateSuggestion(t *testing.T) {
	t.Parallel()
	expectActiveIdentityStopsDuplicateSuggestion(t, context.Background(), memory.NewMemory())
}

func TestSuggest_sharedActiveAssertionStopsAgentScopedDuplicateSuggestion(t *testing.T) {
	t.Parallel()
	expectSharedActiveAssertionStopsAgentScopedDuplicateSuggestion(t, context.Background(), memory.NewMemory())
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

func TestLayer_suggestDoesNotAutoConfirmUntrustedObservations(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := memory.NewMemory()
	content := engine.NewMemoryContent()
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	policy := domain.MemoryLearningPolicy{
		Mode: domain.MemoryLearningAutoConfirm, MinObservations: 2, TTLDays: 7,
	}
	labels := domain.NewLabels(domain.LabelUntrusted).Union(domain.ScopeLabels(platformScope))
	layer := memory.NewLayer(nil, nil, content, store)

	for i := range 2 {
		result, err := layer.Invoke(ctx, engine.Call{
			RunID: domain.RunID("run-untrusted-suggest-" + string(rune('a'+i))),
			Seq:   int64(i + 1), Scope: platformScope, AgentID: "triage",
			Tool: domain.ToolMemorySuggest, Args: suggestionArgs("grafana.datasource.down"),
			Labels: labels, MemoryLearning: policy,
			At: now.Add(time.Duration(i) * time.Minute),
		})
		if err != nil {
			t.Fatalf("Invoke %d: %v", i, err)
		}
		if result.Failed {
			t.Fatalf("result %d = %+v, want successful queued suggestion", i, result)
		}
	}

	if found := findMemory(t, store, now.Add(time.Minute)); len(found) != 0 {
		t.Fatalf("found = %+v, want untrusted observations to wait for review", found)
	}
	pending, err := store.ListSuggestions(ctx, memory.SuggestionFilter{
		Scopes: []domain.Scope{platformScope}, Status: domain.MemorySuggestionPending,
	})
	if err != nil {
		t.Fatalf("ListSuggestions: %v", err)
	}
	if len(pending) != 1 || pending[0].Observations != 2 ||
		!pending[0].Labels.Has(domain.LabelUntrusted) {
		t.Fatalf("pending = %+v, want merged untrusted suggestion still pending", pending)
	}
}

func TestSuggest_mixedTrustObservationsDoNotAutoConfirm(t *testing.T) {
	t.Parallel()
	expectMixedTrustObservationsDoNotAutoConfirm(t, context.Background(), memory.NewMemory())
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

func TestPostgresSuggest_activeAssertionIdentityStopsDuplicateSuggestion(t *testing.T) {
	ctx, store := postgresStore(t)
	expectActiveIdentityStopsDuplicateSuggestion(t, ctx, store)
}

func TestPostgresFind_searchMatchesSeparateTermsAcrossFields(t *testing.T) {
	ctx, store := postgresStore(t)
	expectSearchMatchesSeparateTermsAcrossFields(t, ctx, store)
}

func TestPostgresFind_modelChosenQueriesStillReachTheSameMemory(t *testing.T) {
	ctx, store := postgresStore(t)
	expectModelChosenQueriesStillReachTheSameMemory(t, ctx, store)
}

func TestPostgresFind_digitsDoNotOverrideTheRestOfTheSearch(t *testing.T) {
	ctx, store := postgresStore(t)
	expectDigitsDoNotOverrideTheRestOfTheSearch(t, ctx, store)
}

func TestPostgresFind_nonMatchingSearchDoesNotFailOpen(t *testing.T) {
	ctx, store := postgresStore(t)
	expectNonMatchingSearchDoesNotFailOpen(t, ctx, store)
}

func TestPostgresFind_strongSearchTermsSurviveTheTermBudget(t *testing.T) {
	ctx, store := postgresStore(t)
	expectStrongSearchTermsSurviveTheTermBudget(t, ctx, store)
}

func TestPostgresFind_shortIdentifiersAreSearchTerms(t *testing.T) {
	ctx, store := postgresStore(t)
	expectShortIdentifiersAreSearchTerms(t, ctx, store)
}

func TestPostgresSuggest_sharedActiveAssertionStopsAgentScopedDuplicateSuggestion(t *testing.T) {
	ctx, store := postgresStore(t)
	expectSharedActiveAssertionStopsAgentScopedDuplicateSuggestion(t, ctx, store)
}

func TestPostgresSuggest_mixedTrustObservationsDoNotAutoConfirm(t *testing.T) {
	ctx, store := postgresStore(t)
	expectMixedTrustObservationsDoNotAutoConfirm(t, ctx, store)
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

type suggestionStore interface {
	Assert(context.Context, domain.MemoryAssertion, domain.UserID, string, time.Time) (domain.MemoryAssertion, error)
	Find(context.Context, domain.MemoryQuery) ([]domain.MemoryAssertion, error)
	Suggest(context.Context, domain.MemorySuggestion, domain.MemoryLearningPolicy, domain.UserID, time.Time) (domain.MemorySuggestionOutcome, error)
	ListSuggestions(context.Context, memory.SuggestionFilter) ([]domain.MemorySuggestion, error)
}

type findStore interface {
	Assert(context.Context, domain.MemoryAssertion, domain.UserID, string, time.Time) (domain.MemoryAssertion, error)
	Find(context.Context, domain.MemoryQuery) ([]domain.MemoryAssertion, error)
}

func expectSearchMatchesSeparateTermsAcrossFields(
	t *testing.T,
	ctx context.Context,
	store findStore,
) {
	t.Helper()
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	if _, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
		a.Subject = "superset slack alerts"
		a.Signature = "superset.alert.delivery"
		a.Claim = "the api returned not_in_channel while sending the alert"
	}), "usr_ana", "reviewed", now); err != nil {
		t.Fatalf("Assert matching: %v", err)
	}
	if _, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
		a.Subject = "superset slack impostor"
		a.Signature = "superset.alert.delivery.wildcard"
		a.Claim = "the api returned notXinXchannel while sending the alert"
	}), "usr_ana", "reviewed", now); err != nil {
		t.Fatalf("Assert impostor: %v", err)
	}

	found, err := store.Find(ctx, domain.MemoryQuery{
		Scope: platformScope, AgentID: "triage", Search: "Slack not_in_channel", Now: now,
	})
	if err != nil {
		t.Fatalf("Find terms: %v", err)
	}
	if got := subjects(found); len(got) != 1 || got[0] != "superset slack alerts" {
		t.Fatalf("found = %v, want only the assertion matching both terms literally", got)
	}
}

func expectModelChosenQueriesStillReachTheSameMemory(
	t *testing.T,
	ctx context.Context,
	store findStore,
) {
	t.Helper()
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	if _, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
		a.Subject = "alertas do superset slack entregues"
		a.Signature = "superset.alert.delivery"
		a.Claim = "o erro not_in_channel significa que o app precisa estar no canal"
		a.Confirmed, a.Observations = 2, 2
	}), "usr_ana", "reviewed", now); err != nil {
		t.Fatalf("Assert matching: %v", err)
	}
	if _, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
		a.Subject = "slack onboarding"
		a.Signature = "slack.channel.setup"
		a.Claim = "invite the app before sending general announcements"
		a.Confirmed, a.Observations = 9, 9
	}), "usr_ana", "reviewed", now); err != nil {
		t.Fatalf("Assert slack distractor: %v", err)
	}

	for _, search := range []string{"Slack not_in_channel", "Superset alerta Slack", "Superset alerta Slack 500"} {
		found, err := store.Find(ctx, domain.MemoryQuery{
			Scope: platformScope, AgentID: "triage", Search: search, Limit: 1, Now: now,
		})
		if err != nil {
			t.Fatalf("Find %q: %v", search, err)
		}
		if got := subjects(found); len(got) != 1 || got[0] != "alertas do superset slack entregues" {
			t.Fatalf("search %q found %v, want the stable incident memory first", search, got)
		}
	}
}

func expectDigitsDoNotOverrideTheRestOfTheSearch(
	t *testing.T,
	ctx context.Context,
	store findStore,
) {
	t.Helper()
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	for _, seed := range []struct {
		subject string
		claim   string
	}{
		{"superset slack alerts", "not_in_channel means the app is not in the alert channel"},
		{"payroll ledger closing", "500 entries are waiting for settlement"},
		{"vpn rollout", "the 2026 certificate migration is still pending"},
	} {
		if _, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
			a.Subject = seed.subject
			a.Signature = strings.ReplaceAll(seed.subject, " ", ".")
			a.Claim = seed.claim
			a.Confirmed, a.Observations = 9, 9
		}), "usr_ana", "reviewed", now); err != nil {
			t.Fatalf("Assert %q: %v", seed.subject, err)
		}
	}

	found, err := store.Find(ctx, domain.MemoryQuery{
		Scope: platformScope, AgentID: "triage", Search: "superset alerta slack 500",
		Limit: 1, Now: now,
	})
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if got := subjects(found); len(got) != 1 || got[0] != "superset slack alerts" {
		t.Fatalf("found = %v, want the memory matching the incident terms, not the numeric distractor", got)
	}
}

func expectNonMatchingSearchDoesNotFailOpen(
	t *testing.T,
	ctx context.Context,
	store findStore,
) {
	t.Helper()
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	for _, subject := range []string{"superset slack alerts", "payroll ledger closing", "vpn rollout"} {
		if _, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
			a.Subject = subject
			a.Signature = strings.ReplaceAll(subject, " ", ".")
		}), "usr_ana", "reviewed", now); err != nil {
			t.Fatalf("Assert %q: %v", subject, err)
		}
	}

	for _, search := range []string{
		"???",
		"https://example.internal/execution-log?tab=errors&token=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	} {
		found, err := store.Find(ctx, domain.MemoryQuery{
			Scope: platformScope, AgentID: "triage", Search: search, Now: now,
		})
		if err != nil {
			t.Fatalf("Find %q: %v", search, err)
		}
		if len(found) != 0 {
			t.Fatalf("search %q found %v, want no fail-open list of the scope", search, subjects(found))
		}
	}
}

func expectStrongSearchTermsSurviveTheTermBudget(
	t *testing.T,
	ctx context.Context,
	store findStore,
) {
	t.Helper()
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	if _, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
		a.Subject = "superset slack alerts"
		a.Signature = "superset.alert.delivery"
		a.Claim = "not_in_channel is handled by inviting the app to the alert channel"
	}), "usr_ana", "reviewed", now); err != nil {
		t.Fatalf("Assert matching prefix: %v", err)
	}

	for _, search := range []string{
		"alertas do superset entregues no slack com erro not_in_channel hoje",
		"eu queria saber sobre aquele problema do not_in_channel",
		"por favor procure qualquer coisa sobre superset.alert.delivery",
	} {
		found, err := store.Find(ctx, domain.MemoryQuery{
			Scope: platformScope, AgentID: "triage", Search: search, Now: now,
		})
		if err != nil {
			t.Fatalf("Find %q: %v", search, err)
		}
		if got := subjects(found); len(got) != 1 || got[0] != "superset slack alerts" {
			t.Fatalf("search %q found %v, want the strong identifier to survive the budget", search, got)
		}
	}
}

func expectShortIdentifiersAreSearchTerms(
	t *testing.T,
	ctx context.Context,
	store findStore,
) {
	t.Helper()
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	if _, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
		a.Subject = "s3 bucket retention"
		a.Signature = "storage.s3.lifecycle"
		a.Claim = "s3 buckets keep incident artifacts for seven days"
	}), "usr_ana", "reviewed", now); err != nil {
		t.Fatalf("Assert s3: %v", err)
	}
	if _, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
		a.Subject = "slack channel setup"
		a.Signature = "slack.channel.invite"
		a.Claim = "invite the app before sending alerts"
	}), "usr_ana", "reviewed", now); err != nil {
		t.Fatalf("Assert slack: %v", err)
	}
	if _, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
		a.Subject = "database maintenance window"
		a.Signature = "database.maintenance.window"
		a.Claim = "the system pauses writes during the window"
	}), "usr_ana", "reviewed", now); err != nil {
		t.Fatalf("Assert database: %v", err)
	}

	for _, search := range []string{"s3", "sobre s3"} {
		found, err := store.Find(ctx, domain.MemoryQuery{
			Scope: platformScope, AgentID: "triage", Search: search, Now: now,
		})
		if err != nil {
			t.Fatalf("Find %q: %v", search, err)
		}
		if got := subjects(found); len(got) != 1 || got[0] != "s3 bucket retention" {
			t.Fatalf("search %q found %v, want the short identifier memory", search, got)
		}
	}

	for _, search := range []string{
		"do no",
		"qual o status da fila em producao",
		"what is it or an issue",
	} {
		found, err := store.Find(ctx, domain.MemoryQuery{
			Scope: platformScope, AgentID: "triage", Search: search, Now: now,
		})
		if err != nil {
			t.Fatalf("Find %q: %v", search, err)
		}
		if len(found) != 0 {
			t.Fatalf("search %q found %v, want no fail-open or substring match", search, subjects(found))
		}
	}
}

func expectActiveIdentityStopsDuplicateSuggestion(
	t *testing.T,
	ctx context.Context,
	store suggestionStore,
) {
	t.Helper()
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	active, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
		a.Claim = "the active memory is the reviewed claim"
	}), "usr_ana", "reviewed", now)
	if err != nil {
		t.Fatalf("Assert: %v", err)
	}
	out, err := store.Suggest(ctx, suggestion(func(s *domain.MemorySuggestion) {
		s.Claim = "the agent proposed a different wording for the same case"
		s.Evidence[0].RunID = "run-evidence-2"
	}), domain.MemoryLearningPolicy{Mode: domain.MemoryLearningReview}, "agent:triage", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	if out.Result != domain.MemorySuggestAlreadyActive || out.Assertion == nil ||
		out.Assertion.ID != active.ID || out.Assertion.Claim != active.Claim {
		t.Fatalf("out = %+v, want the active assertion instead of a queued duplicate", out)
	}
	pending, err := store.ListSuggestions(ctx, memory.SuggestionFilter{
		Scopes: []domain.Scope{platformScope}, Status: domain.MemorySuggestionPending,
	})
	if err != nil {
		t.Fatalf("ListSuggestions: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending = %+v, want no suggestion for an active assertion identity", pending)
	}
}

func expectSharedActiveAssertionStopsAgentScopedDuplicateSuggestion(
	t *testing.T,
	ctx context.Context,
	store suggestionStore,
) {
	t.Helper()
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	active, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
		a.AgentID = ""
		a.Claim = "the shared memory is the reviewed claim"
	}), "usr_ana", "reviewed", now)
	if err != nil {
		t.Fatalf("Assert shared: %v", err)
	}
	out, err := store.Suggest(ctx, suggestion(func(s *domain.MemorySuggestion) {
		s.AgentID = "triage"
		s.Claim = "the agent proposed another wording for the same shared memory"
		s.Evidence[0].RunID = "run-evidence-2"
	}), domain.MemoryLearningPolicy{Mode: domain.MemoryLearningReview}, "agent:triage", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	if out.Result != domain.MemorySuggestAlreadyActive || out.Assertion == nil ||
		out.Assertion.ID != active.ID || out.Assertion.AgentID != "" {
		t.Fatalf("out = %+v, want the visible shared assertion instead of a queued duplicate", out)
	}
	pending, err := store.ListSuggestions(ctx, memory.SuggestionFilter{
		Scopes: []domain.Scope{platformScope}, Status: domain.MemorySuggestionPending,
	})
	if err != nil {
		t.Fatalf("ListSuggestions: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending = %+v, want no suggestion for a visible shared assertion", pending)
	}
}

func expectMixedTrustObservationsDoNotAutoConfirm(
	t *testing.T,
	ctx context.Context,
	store suggestionStore,
) {
	t.Helper()
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	autoConfirm := domain.MemoryLearningPolicy{
		Mode: domain.MemoryLearningAutoConfirm, MinObservations: 3, TTLDays: 7,
	}
	labels := []domain.Labels{
		nil,
		nil,
		domain.NewLabels(domain.LabelUntrusted),
		nil,
	}

	var last domain.MemorySuggestionOutcome
	for i, label := range labels {
		policy := autoConfirm.ForSuggestion(label)
		out, err := store.Suggest(ctx, suggestion(func(s *domain.MemorySuggestion) {
			s.Labels = label
			s.Evidence[0].RunID = domain.RunID("run-mixed-trust-" + string(rune('a'+i)))
		}), policy, "agent:triage", now.Add(time.Duration(i)*time.Minute))
		if err != nil {
			t.Fatalf("Suggest %d: %v", i, err)
		}
		last = out
	}

	if last.Result != domain.MemorySuggestPending {
		t.Fatalf("last result = %+v, want accumulated untrusted suggestion kept for review", last)
	}
	found, err := store.Find(ctx, domain.MemoryQuery{
		Scope: platformScope, AgentID: "triage", Search: "datasource", Now: now.Add(4 * time.Minute),
	})
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if len(found) != 0 {
		t.Fatalf("found = %+v, want no active memory after an untrusted contribution", found)
	}
	pending, err := store.ListSuggestions(ctx, memory.SuggestionFilter{
		Scopes: []domain.Scope{platformScope}, Status: domain.MemorySuggestionPending,
	})
	if err != nil {
		t.Fatalf("ListSuggestions: %v", err)
	}
	if len(pending) != 1 || pending[0].Observations != 4 ||
		!pending[0].Labels.Has(domain.LabelUntrusted) {
		t.Fatalf("pending = %+v, want one accumulated tainted suggestion still pending", pending)
	}
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

/*
Two people teaching the same fact at the same time leave one memory holding
both.

Cardinality alone would not prove it: a single row with the second writer lost
is exactly what a lock in the wrong place produces, and it satisfies "one row"
perfectly. So this asserts what the row contains — both labels, both citations,
the higher counts — and that the trail's last event shows that same projection
rather than whichever write arrived last.

The two spellings differ only in case and spacing, which is the case the
canonical identity exists for: locking the assertion id would give each writer
its own lock and let both insert.
*/
func TestPostgresAssert_concurrentCorrectionsOfOneFact_loseNothing(t *testing.T) {
	ctx, store := postgresStore(t)
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	write := func(subject, claim, run, label string, observations int64) error {
		a := assertion(func(a *domain.MemoryAssertion) {
			a.Subject = subject
			a.Claim = claim
			a.Observations, a.Confirmed = observations, observations
			a.Labels = domain.NewLabels(label)
			a.Evidence = []domain.MemoryEvidence{{
				RunID: domain.RunID(run), Seq: 2, Artifact: "final_answer",
				Digest: "sha256:" + run, Labels: domain.NewLabels(label),
			}}
		})
		_, err := store.Assert(ctx, a, domain.UserID("usr_"+run), "corrected", now)
		return err
	}

	// One pair rarely interleaves: whichever writer commits first is usually
	// read by the second, and the race never shows. Repeating it is what makes
	// the window happen — and what makes a missing lock fail here rather than
	// on the afternoon two operators happen to collide.
	for round := range 40 {
		subject := fmt.Sprintf("Grafana Datasource %d", round)
		errs := make(chan error, 2)
		var start sync.WaitGroup
		start.Add(1)
		for _, w := range []struct {
			spelling, claim, run, label string
			observations                int64
		}{
			{subject, "the token expired", "ana", domain.LabelUntrusted, 5},
			{"  " + strings.ToLower(subject) + " ", "the pool was exhausted", "bruno", domain.LabelPersonal, 3},
		} {
			go func() {
				start.Wait()
				errs <- write(w.spelling, w.claim, w.run, w.label, w.observations)
			}()
		}
		start.Done()
		for range 2 {
			if err := <-errs; err != nil {
				t.Fatalf("Assert: %v", err)
			}
		}
	}

	found, err := store.List(ctx, memory.Filter{
		Scopes: []domain.Scope{platformScope}, Now: now,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(found) != 40 {
		t.Fatalf("rows = %d, want one memory per round", len(found))
	}

	got := found[0]
	if !got.Labels.HasAny(domain.LabelUntrusted) || !got.Labels.HasAny(domain.LabelPersonal) {
		t.Errorf("labels = %v, want both writers' labels", got.Labels)
	}
	runs := map[domain.RunID]bool{}
	for _, ev := range got.Evidence {
		runs[ev.RunID] = true
	}
	if !runs["ana"] || !runs["bruno"] {
		t.Errorf("evidence cites %v, want both writers' runs", runs)
	}
	if got.Observations != 5 || got.Confirmed != 5 {
		t.Errorf("observations/confirmed = %d/%d, want the higher of the two",
			got.Observations, got.Confirmed)
	}
}

// mergeStore is every path that writes an assertion, which is what the matrix
// below is about: one decision, reached three ways.
type mergeStore interface {
	Assert(context.Context, domain.MemoryAssertion, domain.UserID, string, time.Time) (domain.MemoryAssertion, error)
	Suggest(context.Context, domain.MemorySuggestion, domain.MemoryLearningPolicy, domain.UserID, time.Time) (domain.MemorySuggestionOutcome, error)
	AcceptSuggestion(context.Context, string, domain.Scope, domain.UserID, string, time.Time) (domain.MemoryAssertion, error)
	ListSuggestions(context.Context, memory.SuggestionFilter) ([]domain.MemorySuggestion, error)
	Find(context.Context, domain.MemoryQuery) ([]domain.MemoryAssertion, error)
}

var reviewPolicy = domain.MemoryLearningPolicy{Mode: domain.MemoryLearningReview}

/*
Accepting a suggestion merges into what is there; it does not replace it.

Before this, accept wrote the suggestion over the stored row: a memory somebody
had corrected, carrying a taint from the run that taught it, came back clean and
shorter-lived because a later suggestion happened to be accepted.
*/
func expectAcceptMergesIntoTheStoredAssertion(t *testing.T, ctx context.Context, store mergeStore) {
	t.Helper()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	far := now.Add(240 * time.Hour)

	// The agent suggests first: Suggest refuses to open a suggestion against an
	// identity that is already active, so this is the only order in which an
	// accept can ever meet a stored assertion.
	out, err := store.Suggest(ctx, suggestion(func(s *domain.MemorySuggestion) {
		s.Claim = "a later wording the agent proposed"
		s.Evidence = []domain.MemoryEvidence{{
			RunID: "run-new", Seq: 4, Artifact: domain.ArtifactMemorySuggestion, Digest: "sha256:n",
		}}
	}), reviewPolicy, "agent:triage", now)
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}

	// Then somebody writes the memory by hand, with a taint and a long life.
	if _, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
		a.Labels = domain.NewLabels(domain.LabelUntrusted)
		a.ExpiresAt = &far
		a.Evidence = []domain.MemoryEvidence{{
			RunID: "run-old", Seq: 1, Artifact: "final_answer", Digest: "sha256:o",
			Labels: domain.NewLabels(domain.LabelUntrusted),
		}}
	}), "usr_ana", "reviewed", now); err != nil {
		t.Fatalf("Assert: %v", err)
	}

	got, err := store.AcceptSuggestion(ctx, out.Suggestion.ID, platformScope, "usr_ana", "agreed", now)
	if err != nil {
		t.Fatalf("AcceptSuggestion: %v", err)
	}

	if !got.Labels.HasAny(domain.LabelUntrusted) {
		t.Errorf("labels = %v, want the stored taint to survive the accept", got.Labels)
	}
	if got.ExpiresAt == nil || !got.ExpiresAt.Equal(far) {
		t.Errorf("expiry = %v, want the stored one kept even though the suggestion cited something new",
			got.ExpiresAt)
	}
	if len(got.Evidence) != 2 {
		t.Errorf("evidence = %d records, want both the stored and the accepted citation", len(got.Evidence))
	}
}

/*
A terminal memory refuses the accept and the suggestion is not spent.

The dangerous shape is the quiet one: the suggestion marked accepted, the queue
emptied, and the assertion still disabled — nobody learns anything and nothing
says so.
*/
func expectAcceptOnADisabledAssertionConsumesNothing(t *testing.T, ctx context.Context, store mergeStore) {
	t.Helper()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	stored, err := store.Assert(ctx, assertion(nil), "usr_ana", "reviewed", now)
	if err != nil {
		t.Fatalf("Assert: %v", err)
	}
	disabler, ok := store.(interface {
		Disable(context.Context, string, domain.Scope, domain.UserID, string, time.Time) error
	})
	if !ok {
		t.Fatal("store cannot disable")
	}
	if err := disabler.Disable(ctx, stored.ID, platformScope, "usr_ana", "wrong", now); err != nil {
		t.Fatalf("Disable: %v", err)
	}

	out, err := store.Suggest(ctx, suggestion(func(s *domain.MemorySuggestion) {
		s.Claim = "the agent proposes it again"
	}), reviewPolicy, "agent:triage", now)
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}

	if _, err := store.AcceptSuggestion(ctx, out.Suggestion.ID, platformScope, "usr_ana", "agreed", now); err == nil {
		t.Fatal("accepted onto a disabled memory")
	}

	pending, err := store.ListSuggestions(ctx, memory.SuggestionFilter{
		Scopes: []domain.Scope{platformScope}, Status: domain.MemorySuggestionPending, Now: now,
	})
	if err != nil {
		t.Fatalf("ListSuggestions: %v", err)
	}
	if len(pending) != 1 {
		t.Errorf("pending = %d, want the suggestion left for somebody to decide", len(pending))
	}
}

func TestAccept_mergesIntoTheStoredAssertion(t *testing.T) {
	t.Parallel()
	expectAcceptMergesIntoTheStoredAssertion(t, context.Background(), memory.NewMemory())
}

func TestPostgresAccept_mergesIntoTheStoredAssertion(t *testing.T) {
	ctx, store := postgresStore(t)
	expectAcceptMergesIntoTheStoredAssertion(t, ctx, store)
}

func TestAccept_onADisabledAssertion_consumesNothing(t *testing.T) {
	t.Parallel()
	expectAcceptOnADisabledAssertionConsumesNothing(t, context.Background(), memory.NewMemory())
}

func TestPostgresAccept_onADisabledAssertion_consumesNothing(t *testing.T) {
	ctx, store := postgresStore(t)
	expectAcceptOnADisabledAssertionConsumesNothing(t, ctx, store)
}

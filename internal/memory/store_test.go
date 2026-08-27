package memory_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
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

/*
A row that carries the key beside the same row that does not is one identity
under two names, and a write refuses both.

This is the shape an upgrade actually produces, and the one the old query hid
best: the legacy row answers to its assertion id, the newer row answers to the
canonical key, and ordering the keyed one first made the query return an answer
that was true about half the table. The correction then landed on the newer row
while the legacy one stayed active, saying something else, in the same scope,
for the same agent.

Nothing is written on the way out — not the projection, not an event, not
updated_at. A refusal that still moved a timestamp would be a change nobody
asked for, on a row nobody chose.
*/
func TestPostgresAssert_aKeyedRowBesideItsLegacyTwin_refusesToChoose(t *testing.T) {
	ctx, pool := postgresPool(t)
	store := memory.NewPostgres(pool)
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	keyed, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
		a.Subject = "Grafana Datasource"
	}), "usr_ana", "reviewed", now)
	if err != nil {
		t.Fatalf("Assert the keyed row: %v", err)
	}
	// Its twin, from before any key existed to connect the two.
	legacy, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
		a.Subject = "an unrelated subject"
	}), "usr_bruno", "reviewed", now)
	if err != nil {
		t.Fatalf("Assert the row that becomes the twin: %v", err)
	}
	legacyTwin(t, ctx, pool, legacy.ID, "grafana  datasource")

	before := snapshotRows(t, ctx, pool, legacy.ID, keyed.ID)
	_, err = store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
		a.Subject = "Grafana Datasource"
		a.Claim = "a correction that must not land on half the fact"
	}), "usr_ana", "correcting the claim", now.Add(time.Hour))
	if !errors.Is(err, memory.ErrCanonicalConflict) {
		t.Fatalf("Assert = %v, want the write refused rather than one row chosen", err)
	}
	if after := snapshotRows(t, ctx, pool, legacy.ID, keyed.ID); !slices.Equal(after, before) {
		t.Errorf("rows went %v -> %v, want nothing changed by a refusal", before, after)
	}
	if n := countEvents(t, ctx, pool, legacy.ID) + countEvents(t, ctx, pool, keyed.ID); n != 2 {
		t.Errorf("events = %d, want only the two that created the rows", n)
	}
}

// Two rows that both carry the key are the same refusal. Unreachable through
// the write path once the lock and this rule are in place, which is exactly why
// it is planted: "nothing can produce it" is a property of today's code, and
// the row that outlives today's code is the one nobody checked.
func TestPostgresAssert_twoKeyedRowsOfOneIdentity_refusesToChoose(t *testing.T) {
	ctx, pool := postgresPool(t)
	store := memory.NewPostgres(pool)
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	first, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
		a.Subject = "Grafana Datasource"
	}), "usr_ana", "reviewed", now)
	if err != nil {
		t.Fatalf("Assert the first row: %v", err)
	}
	second, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
		a.Subject = "an unrelated subject"
	}), "usr_bruno", "reviewed", now)
	if err != nil {
		t.Fatalf("Assert the second row: %v", err)
	}
	// Spelled like the first and carrying its key, which is copied from the row
	// that has it: a test that wrote the hash itself would be a second
	// implementation of the key, and it would keep passing after the real one
	// changed.
	if _, err := pool.Exec(ctx, `
		update memory_assertions set subject = 'grafana  datasource',
			canonical_identity_key =
				(select canonical_identity_key from memory_assertions where assertion_id = $2)
		where assertion_id = $1`, second.ID, first.ID); err != nil {
		t.Fatalf("plant the second keyed row: %v", err)
	}

	before := snapshotRows(t, ctx, pool, first.ID, second.ID)
	if _, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
		a.Subject = "Grafana Datasource"
		a.Claim = "a correction that must not land on half the fact"
	}), "usr_ana", "correcting the claim", now.Add(time.Hour)); !errors.Is(
		err, memory.ErrCanonicalConflict) {
		t.Fatalf("Assert = %v, want the write refused rather than one row chosen", err)
	}
	if after := snapshotRows(t, ctx, pool, first.ID, second.ID); !slices.Equal(after, before) {
		t.Errorf("rows went %v -> %v, want nothing changed by a refusal", before, after)
	}
}

/*
A conflicted pair ends that row's repair and not the sweep.

The same defect the purged run had: hydration exists to walk rows written before
this work, and duplicate spellings are exactly what those rows are. Aborting on
the first pair would stop the repair on the population it was built for, and the
rows after it would stay unkeyed for ever while the job reported an error every
night.
*/
func TestPostgresHydrate_conflictedPair_endsThatRowAndNotTheSweep(t *testing.T) {
	ctx, pool := postgresPool(t)
	store := memory.NewPostgres(pool)
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	run, cited := legacyRun(t, "run-legacy")
	var ids []string
	for _, subject := range []string{"first subject", "second subject", "loki ingester"} {
		written, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
			a.Scope, a.Subject = run.scope, subject
			a.Evidence = []domain.MemoryEvidence{cited}
		}), "usr_ana", "reviewed", now)
		if err != nil {
			t.Fatalf("Assert %q: %v", subject, err)
		}
		ids = append(ids, written.ID)
	}
	legacyTwin(t, ctx, pool, ids[0], "Grafana Datasource")
	legacyTwin(t, ctx, pool, ids[1], "grafana  datasource")
	// A third row of its own identity, so the sweep has somewhere to get to once
	// the pair has been refused.
	unkey(t, ctx, pool, ids[2])

	out, err := store.Hydrate(ctx, memory.NewResolver(run.ledger, run.content),
		memory.HydratePage{Limit: 10, Now: now})
	if err != nil {
		t.Fatalf("Hydrate: %v", err)
	}
	if out.Scanned != 3 {
		t.Fatalf("scanned = %d, want the sweep to have reached every row", out.Scanned)
	}
	if out.Conflicted != 1 || out.Repaired != 2 {
		t.Errorf("conflicted = %d and repaired = %d, want the pair refused once and the other two done",
			out.Conflicted, out.Repaired)
	}
	// One of the pair is keyed — whichever the sweep reached first, since until
	// it is keyed the other is invisible to the lookup. What must not happen is
	// the sweep stopping, so the row that has nothing to do with the pair is
	// repaired.
	if !hasKey(t, ctx, pool, ids[2]) {
		t.Error("the unrelated row was left unkeyed, so the pair stopped the sweep")
	}
}

/*
A row written before the key, met by somebody who spells it differently, is one
memory with both citations.

Without this the second spelling is a second row: the legacy row has no key to
match and an assertion id nobody will type again, so the lookup finds nothing
and inserts. Then the fact is remembered twice, each half carrying the citations
the other does not, and Find returns whichever ranks higher.

The repair belongs on the write path and not only in the sweep, because a pod
running the old image can write a keyless row after the job has passed. The job
shortens the queue; it cannot close it.
*/
func TestPostgresAssert_aLegacyRowMeetsANewSpelling_staysOneMemory(t *testing.T) {
	ctx, pool := postgresPool(t)
	store := memory.NewPostgres(pool)
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	legacy, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
		a.Subject = "Grafana Datasource"
	}), "usr_ana", "reviewed", now)
	if err != nil {
		t.Fatalf("Assert the legacy row: %v", err)
	}
	unkey(t, ctx, pool, legacy.ID)

	merged, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
		a.Subject = "grafana  datasource"
		a.Evidence = []domain.MemoryEvidence{{
			RunID: "run-evidence-2", Artifact: "final_answer", Digest: "sha256:beef",
		}}
	}), "usr_bruno", "same fact, spelled differently", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Assert the second spelling: %v", err)
	}
	if merged.ID != legacy.ID {
		t.Errorf("wrote %s, want the merge to land on the row that was already there (%s)",
			merged.ID, legacy.ID)
	}

	all := list(t, ctx, store, []domain.Scope{platformScope})
	if len(all) != 1 {
		t.Fatalf("rows = %v, want one memory for one fact", subjects(all))
	}
	var runs []domain.RunID
	for _, ev := range all[0].Evidence {
		runs = append(runs, ev.RunID)
	}
	if !slices.Contains(runs, "run-evidence-1") || !slices.Contains(runs, "run-evidence-2") {
		t.Errorf("evidence cites %v, want both citations kept", runs)
	}
	if !hasKey(t, ctx, pool, legacy.ID) {
		t.Error("the legacy row was merged into without gaining its key")
	}
}

/*
Active memory spelled differently still stops the proposal.

The duplicate check asked for the row by the id the suggestion's own spelling
hashes to, so a memory somebody wrote as "Grafana Datasource" was invisible to
an agent that said "grafana  datasource" — and the queue filled with proposals
for a fact the platform already knew, each of which a person had to read and
dismiss.
*/
func TestSuggest_activeMemorySpelledDifferently_opensNoProposal(t *testing.T) {
	t.Parallel()
	expectSuggestFindsTheOtherSpelling(t, context.Background(), memory.NewMemory(), nil)
}

func TestPostgresSuggest_activeMemorySpelledDifferently_opensNoProposal(t *testing.T) {
	ctx, pool := postgresPool(t)
	expectSuggestFindsTheOtherSpelling(t, ctx, memory.NewPostgres(pool),
		func(id string) { unkey(t, ctx, pool, id) })
}

// The same for shared memory, which an agent-scoped proposal must also find.
// A run reads its own memory and the shared memory, so a shared fact answers
// the need — and the platform saying otherwise is the console filling up with
// proposals nobody can accept without creating a second copy.
func TestSuggest_sharedMemorySpelledDifferently_opensNoProposal(t *testing.T) {
	t.Parallel()
	expectSharedSuggestFindsTheOtherSpelling(t, context.Background(), memory.NewMemory(), nil)
}

func TestPostgresSuggest_sharedMemorySpelledDifferently_opensNoProposal(t *testing.T) {
	ctx, pool := postgresPool(t)
	expectSharedSuggestFindsTheOtherSpelling(t, ctx, memory.NewPostgres(pool),
		func(id string) { unkey(t, ctx, pool, id) })
}

// makeLegacy is how a store that keeps the key on the row is put back in the
// state an upgrade inherits. The fake computes the key when it looks, so it has
// no such state and passes nil: what both must agree on is the answer, not the
// column.
func expectSuggestFindsTheOtherSpelling(
	t *testing.T, ctx context.Context, store mergeStore, makeLegacy func(string),
) {
	t.Helper()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	active, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
		a.Subject = "Grafana Datasource"
	}), "usr_ana", "reviewed", now)
	if err != nil {
		t.Fatalf("Assert: %v", err)
	}
	if makeLegacy != nil {
		makeLegacy(active.ID)
	}

	out, err := store.Suggest(ctx, suggestion(func(s *domain.MemorySuggestion) {
		s.Subject = "grafana  datasource"
	}), reviewPolicy, "agent:triage", now)
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	if out.Result != domain.MemorySuggestAlreadyActive {
		t.Errorf("result = %s, want the proposal answered by the memory already there", out.Result)
	}
	if out.Assertion == nil || out.Assertion.ID != active.ID {
		t.Errorf("assertion = %+v, want the row that was already active", out.Assertion)
	}

	pending, err := store.ListSuggestions(ctx, memory.SuggestionFilter{
		Scopes: []domain.Scope{platformScope}, Status: domain.MemorySuggestionPending, Now: now,
	})
	if err != nil {
		t.Fatalf("ListSuggestions: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("pending = %d, want nothing for somebody to decide", len(pending))
	}
}

func expectSharedSuggestFindsTheOtherSpelling(
	t *testing.T, ctx context.Context, store mergeStore, makeLegacy func(string),
) {
	t.Helper()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	shared, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
		a.AgentID, a.Subject = "", "Grafana Datasource"
	}), "usr_ana", "reviewed", now)
	if err != nil {
		t.Fatalf("Assert shared: %v", err)
	}
	if makeLegacy != nil {
		makeLegacy(shared.ID)
	}

	out, err := store.Suggest(ctx, suggestion(func(s *domain.MemorySuggestion) {
		s.Subject = "grafana  datasource"
	}), reviewPolicy, "agent:triage", now)
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	if out.Result != domain.MemorySuggestAlreadyActive {
		t.Errorf("result = %s, want the shared memory to answer the proposal", out.Result)
	}
	if out.Assertion == nil || out.Assertion.ID != shared.ID {
		t.Errorf("assertion = %+v, want the shared row", out.Assertion)
	}
}

/*
Two keyless rows of one identity fail closed once the write path can see them.

Before the key was filled they were invisible to each other and to the lookup,
so a write inserted a third. Filling the key is what reveals the pair — and
having revealed it, the write must refuse rather than merge into whichever the
ordering put first.
*/
func TestPostgresAssert_twoLegacyRowsOfOneIdentity_refusesToChoose(t *testing.T) {
	ctx, pool := postgresPool(t)
	store := memory.NewPostgres(pool)
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	for i, subject := range []string{"grafana  datasource", "GRAFANA DATASOURCE"} {
		written, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
			a.Subject = fmt.Sprintf("grafana datasource %d", i)
		}), "usr_ana", "reviewed", now)
		if err != nil {
			t.Fatalf("Assert %q: %v", subject, err)
		}
		legacyTwin(t, ctx, pool, written.ID, subject)
	}

	if _, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
		a.Subject = "Grafana Datasource"
	}), "usr_bruno", "a third spelling of the same fact", now.Add(time.Hour)); !errors.Is(
		err, memory.ErrCanonicalConflict) {
		t.Fatalf("Assert = %v, want the write refused rather than one row chosen", err)
	}
	if all := list(t, ctx, store, []domain.Scope{platformScope}); len(all) != 2 {
		t.Errorf("rows = %v, want the two that were there and no third", subjects(all))
	}
}

// legacyTwin puts a row back in the state an upgrade inherits: written under a
// spelling of its own, before any key existed to connect it to the other one.
// The write path can no longer produce this, which is exactly the point — it is
// what the table already holds.
func legacyTwin(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id, subject string) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		update memory_assertions set subject = $2, canonical_identity_key = null
		where assertion_id = $1`, id, subject); err != nil {
		t.Fatalf("plant the legacy twin of %s: %v", id, err)
	}
}

func unkey(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id string) {
	t.Helper()
	if _, err := pool.Exec(ctx,
		`update memory_assertions set canonical_identity_key = null where assertion_id = $1`,
		id); err != nil {
		t.Fatalf("clear the key of %s: %v", id, err)
	}
}

func hasKey(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id string) bool {
	t.Helper()
	var key *string
	if err := pool.QueryRow(ctx,
		`select canonical_identity_key from memory_assertions where assertion_id = $1`,
		id).Scan(&key); err != nil {
		t.Fatalf("read the key of %s: %v", id, err)
	}
	return key != nil
}

// snapshotRows is everything a merge would have moved, so "nothing changed" is
// asserted against the row rather than against one field somebody remembered.
func snapshotRows(
	t *testing.T, ctx context.Context, pool *pgxpool.Pool, ids ...string,
) []string {
	t.Helper()
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		var claim, updatedBy string
		var labels []string
		var updatedAt time.Time
		var key *string
		if err := pool.QueryRow(ctx, `
			select claim, labels, updated_by, updated_at, canonical_identity_key
			from memory_assertions where assertion_id = $1`, id).Scan(
			&claim, &labels, &updatedBy, &updatedAt, &key); err != nil {
			t.Fatalf("read %s: %v", id, err)
		}
		stored := "unkeyed"
		if key != nil {
			stored = *key
		}
		out = append(out, fmt.Sprintf("%s|%s|%v|%s|%s|%s",
			id, claim, labels, updatedBy, updatedAt.UTC(), stored))
	}
	return out
}

func countEvents(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx,
		`select count(*) from memory_assertion_events where assertion_id = $1`, id).Scan(&n); err != nil {
		t.Fatalf("count events of %s: %v", id, err)
	}
	return n
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

/*
A write that fails after projecting leaves nothing behind.

The proof has to come from inside the transaction, not before it: a failure that
happens before the first write proves only that nothing started. So the event
insert is refused by a trigger, which fires after the projection has already
been written — and if any of it survived, the suggestion would be accepted
beside a memory nobody agreed to.
*/
func TestPostgresAccept_failureAfterProjecting_leavesNothing(t *testing.T) {
	ctx, pool := postgresPool(t)
	store := memory.NewPostgres(pool)
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	out, err := store.Suggest(ctx, suggestion(nil), reviewPolicy, "agent:triage", now)
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		create or replace function refuse_memory_event() returns trigger as $$
		begin raise exception 'refused for the test'; end; $$ language plpgsql;
		create trigger refuse_memory_event before insert on memory_assertion_events
		for each row execute function refuse_memory_event();`); err != nil {
		t.Fatalf("install trigger: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`drop trigger if exists refuse_memory_event on memory_assertion_events`)
	})

	if _, err := store.AcceptSuggestion(ctx, out.Suggestion.ID, platformScope,
		"usr_ana", "agreed", now); err == nil {
		t.Fatal("accept succeeded while the event was being refused")
	}

	found, err := store.Find(ctx, domain.MemoryQuery{
		Scope: platformScope, AgentID: "triage", Now: now,
	})
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if len(found) != 0 {
		t.Errorf("assertions = %d, want the projection rolled back with the event", len(found))
	}

	pending, err := store.ListSuggestions(ctx, memory.SuggestionFilter{
		Scopes: []domain.Scope{platformScope}, Status: domain.MemorySuggestionPending, Now: now,
	})
	if err != nil {
		t.Fatalf("ListSuggestions: %v", err)
	}
	if len(pending) != 1 {
		t.Errorf("pending = %d, want the suggestion untouched", len(pending))
	}
}

/*
A person accepting while the agent reaches its auto-confirm threshold.

Both orders are correct and the test accepts either, because the platform has
not decided that active memory keeps accumulating observations. If the accept
lands first the suggestion is spent and the concurrent observation meets active
memory, which Suggest reports as already-active and does not record. If the
auto-confirm lands first the accept meets a memory that already holds the fact.

What must hold in both is the same: one assertion, the suggestion in a terminal
state that matches what happened, and neither writer waiting on the other.

Whether an already-active memory should still count a sighting is a real
question — Find ranks on observations, so a memory freezes its rank the moment
it becomes active — but it is a decision about what observations mean, not
something to settle inside a test.
*/
func TestPostgresAccept_racingAutoConfirm_leavesOneMemoryAndOneEnding(t *testing.T) {
	ctx, store := postgresStore(t)
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	policy := domain.MemoryLearningPolicy{
		Mode: domain.MemoryLearningAutoConfirm, MinObservations: 2,
	}

	out, err := store.Suggest(ctx, suggestion(func(s *domain.MemorySuggestion) {
		s.Evidence = []domain.MemoryEvidence{{
			RunID: "run-1", Seq: 1, Artifact: domain.ArtifactMemorySuggestion, Digest: "sha256:1",
		}}
	}), policy, "agent:triage", now)
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}

	errs := make(chan error, 2)
	var start sync.WaitGroup
	start.Add(1)
	go func() {
		start.Wait()
		_, err := store.AcceptSuggestion(ctx, out.Suggestion.ID, platformScope, "usr_ana", "agreed", now)
		errs <- err
	}()
	go func() {
		start.Wait()
		// The second observation, which takes the suggestion to its threshold.
		_, err := store.Suggest(ctx, suggestion(func(s *domain.MemorySuggestion) {
			s.Evidence = []domain.MemoryEvidence{{
				RunID: "run-2", Seq: 1, Artifact: domain.ArtifactMemorySuggestion, Digest: "sha256:2",
			}}
		}), policy, "agent:triage", now)
		errs <- err
	}()
	start.Done()
	for range 2 {
		// Either order is fine; a deadlock or a lost writer is not.
		if err := <-errs; err != nil && !errors.Is(err, memory.ErrNotFound) {
			t.Fatalf("racing writers: %v", err)
		}
	}

	found, err := store.Find(ctx, domain.MemoryQuery{
		Scope: platformScope, AgentID: "triage", Now: now,
	})
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("assertions = %d, want exactly one memory of one fact", len(found))
	}

	pending, err := store.ListSuggestions(ctx, memory.SuggestionFilter{
		Scopes: []domain.Scope{platformScope}, Status: domain.MemorySuggestionPending, Now: now,
	})
	if err != nil {
		t.Fatalf("ListSuggestions: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("pending = %d, want the suggestion to have an ending", len(pending))
	}
}

// hydrateStore is the repair door: it may complete what the platform can derive
// and nothing else.
type hydrateStore interface {
	Assert(context.Context, domain.MemoryAssertion, domain.UserID, string, time.Time) (domain.MemoryAssertion, error)
	Hydrate(context.Context, *memory.Resolver, memory.HydratePage) (memory.HydrateResult, error)
	Find(context.Context, domain.MemoryQuery) ([]domain.MemoryAssertion, error)
}

/*
A citation goes in legacy and comes out complete, still one record.

This is the whole of hydration in one assertion. The old shape named a run and
an artifact; the new one names the step, the run that produced the bytes, the
whole digest and the labels the run had accumulated. Replacing rather than
adding is what keeps it one citation — the two forms have different keys, so a
merge would have kept both and doubled every record it repaired.

And a second pass changes nothing. A repair that is not idempotent is a repair
that cannot be run twice, which means it cannot be run at all on a schedule.
*/
func expectHydrationCompletesACitationInPlace(
	t *testing.T, ctx context.Context, store hydrateStore, resolver *memory.Resolver,
	seeded domain.MemoryAssertion, now time.Time,
) {
	t.Helper()

	first, err := store.Hydrate(ctx, resolver, memory.HydratePage{Limit: 10, Now: now})
	if err != nil {
		t.Fatalf("Hydrate: %v", err)
	}
	if first.Repaired != 1 {
		t.Fatalf("repaired = %d, want the one legacy row", first.Repaired)
	}

	found, err := store.Find(ctx, domain.MemoryQuery{
		Scope: seeded.Scope, AgentID: seeded.AgentID, Now: now,
	})
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("assertions = %d, want the one that was there", len(found))
	}
	got := found[0]

	if len(got.Evidence) != 1 {
		t.Fatalf("evidence = %d records, want the legacy citation replaced and not joined",
			len(got.Evidence))
	}
	ev := got.Evidence[0]
	if ev.Seq == 0 {
		t.Error("the citation still does not say which step it names")
	}
	if len(ev.Digest) != 64 {
		t.Errorf("digest = %q, want the whole one the store recorded", ev.Digest)
	}
	if !ev.Labels.Has(domain.LabelUntrusted) || !got.Labels.Has(domain.LabelUntrusted) {
		t.Errorf("labels = %v / %v, want the taint the run carried", ev.Labels, got.Labels)
	}

	// Nothing a person decided may move.
	if got.Claim != seeded.Claim || got.Observations != seeded.Observations ||
		got.Confirmed != seeded.Confirmed || got.CreatedBy != seeded.CreatedBy {
		t.Errorf("hydration changed what somebody decided: %+v", got)
	}

	second, err := store.Hydrate(ctx, resolver, memory.HydratePage{Limit: 10, Now: now})
	if err != nil {
		t.Fatalf("Hydrate again: %v", err)
	}
	if second.Repaired != 0 {
		t.Errorf("second pass repaired %d, want a no-op", second.Repaired)
	}

	after, err := store.Find(ctx, domain.MemoryQuery{
		Scope: seeded.Scope, AgentID: seeded.AgentID, Now: now,
	})
	if err != nil {
		t.Fatalf("Find after: %v", err)
	}
	// Compared against what was seeded, not against the first pass: a repair
	// that moved the timestamp on the way in would move it for both reads and
	// look stationary.
	if !got.UpdatedAt.Equal(seeded.UpdatedAt) || !after[0].UpdatedAt.Equal(seeded.UpdatedAt) {
		t.Errorf("updated_at moved for a repair nobody made: %v then %v, want %v",
			got.UpdatedAt, after[0].UpdatedAt, seeded.UpdatedAt)
	}
}

func TestHydrate_completesALegacyCitationInPlace(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := memory.NewMemory()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	run, cited := legacyRun(t, "run-legacy")
	seed := assertion(func(a *domain.MemoryAssertion) {
		a.Scope = run.scope
		a.Evidence = []domain.MemoryEvidence{cited}
	})
	seeded, err := store.Assert(ctx, seed, "usr_ana", "reviewed", now)
	if err != nil {
		t.Fatalf("Assert: %v", err)
	}

	expectHydrationCompletesACitationInPlace(t, ctx, store, run.resolver(), seeded, now)
}

func TestPostgresHydrate_completesALegacyCitationInPlace(t *testing.T) {
	ctx, store := postgresStore(t)
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	run, cited := legacyRun(t, "run-legacy")
	seed := assertion(func(a *domain.MemoryAssertion) {
		a.Scope = run.scope
		a.Evidence = []domain.MemoryEvidence{cited}
	})
	seeded, err := store.Assert(ctx, seed, "usr_ana", "reviewed", now)
	if err != nil {
		t.Fatalf("Assert: %v", err)
	}

	expectHydrationCompletesACitationInPlace(t, ctx, store, run.resolver(), seeded, now)
}

// legacyRun seeds a tainted run and the citation a row written before this work
// would have carried: a run and an artifact, no step, and the truncated digest
// the reference happens to hold.
func legacyRun(t *testing.T, id domain.RunID) (*run, domain.MemoryEvidence) {
	t.Helper()
	r, cited := finished(t, id)
	full := cited
	resolved, err := r.resolver().Resolve(context.Background(), r.scope,
		[]domain.MemoryEvidence{full})
	if err != nil {
		t.Fatalf("resolve for the fixture: %v", err)
	}
	// What a row written before this work carries: the run and the artifact,
	// the whole digest the handler checked against the payload, and no step.
	return r, domain.MemoryEvidence{
		RunID: id, Artifact: domain.ArtifactFinalAnswer, Digest: resolved[0].Digest,
	}
}

// blockingContent lets a test stop a hydration in the middle of reading the
// ledger, which is the only moment where another writer can slip past it.
type blockingContent struct {
	inner   memory.EvidenceContent
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (c *blockingContent) Metadata(
	ctx context.Context, ref string,
) (domain.ContentMetadata, error) {
	c.once.Do(func() {
		close(c.entered)
		<-c.release
	})
	return c.inner.Metadata(ctx, ref)
}

/*
A hydration that was overtaken does not undo the writer that overtook it.

The repair reads the ledger outside the lock, so a correction can land between
the snapshot and the write. Its citations are newer than what this pass is
describing, and writing the snapshot's version over them would delete a citation
somebody just made — the repair silently losing a write is worse than the row
staying unrepaired for one more pass.
*/
func expectHydrationOvertakenDoesNotLoseTheNewCitation(
	t *testing.T, ctx context.Context, store hydrateStore,
	blocked *blockingContent, ledger memory.EvidenceLedger,
	seeded domain.MemoryAssertion, now time.Time,
) {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		_, err := store.Hydrate(ctx, memory.NewResolver(ledger, blocked),
			memory.HydratePage{Limit: 10, Now: now})
		done <- err
	}()

	<-blocked.entered
	// A second citation arrives while the repair is still reading.
	second := seeded
	second.Evidence = append(slices.Clone(seeded.Evidence), domain.MemoryEvidence{
		RunID: "run-later", Seq: 7, Artifact: "final_answer",
		Digest: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
	})
	if _, err := store.Assert(ctx, second, "usr_bruno", "another citation", now); err != nil {
		t.Fatalf("Assert while hydrating: %v", err)
	}
	close(blocked.release)

	if err := <-done; err != nil {
		t.Fatalf("Hydrate: %v", err)
	}

	found, err := store.Find(ctx, domain.MemoryQuery{
		Scope: seeded.Scope, AgentID: seeded.AgentID, Now: now,
	})
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	var runs []domain.RunID
	for _, ev := range found[0].Evidence {
		runs = append(runs, ev.RunID)
	}
	if !slices.Contains(runs, "run-later") {
		t.Errorf("evidence cites %v, want the citation that arrived mid-repair", runs)
	}
}

func TestHydrate_overtakenByAWriter_doesNotLoseTheNewCitation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := memory.NewMemory()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	run, cited := legacyRun(t, "run-legacy")
	seeded, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
		a.Scope = run.scope
		a.Evidence = []domain.MemoryEvidence{cited}
	}), "usr_ana", "reviewed", now)
	if err != nil {
		t.Fatalf("Assert: %v", err)
	}

	blocked := &blockingContent{
		inner: run.content, entered: make(chan struct{}), release: make(chan struct{}),
	}
	expectHydrationOvertakenDoesNotLoseTheNewCitation(
		t, ctx, store, blocked, run.ledger, seeded, now)
}

func TestPostgresHydrate_overtakenByAWriter_doesNotLoseTheNewCitation(t *testing.T) {
	ctx, store := postgresStore(t)
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	run, cited := legacyRun(t, "run-legacy")
	seeded, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
		a.Scope = run.scope
		a.Evidence = []domain.MemoryEvidence{cited}
	}), "usr_ana", "reviewed", now)
	if err != nil {
		t.Fatalf("Assert: %v", err)
	}

	blocked := &blockingContent{
		inner: run.content, entered: make(chan struct{}), release: make(chan struct{}),
	}
	expectHydrationOvertakenDoesNotLoseTheNewCitation(
		t, ctx, store, blocked, run.ledger, seeded, now)
}

/*
Filling only the canonical key writes no event, and it should not.

The key is recalculable from the row's own fields and does not appear in an
event's detail, so an event about it would say nothing a reader could act on —
it would be noise in the one log an auditor reads to reconstruct what a memory
rests on. Events are for provenance moving; identity is arithmetic.

The row is made legacy the only way this case exists: complete evidence, key
cleared, exactly what a row written before the key existed looks like once its
citations were already whole.
*/
func TestPostgresHydrate_fillingOnlyTheKey_recordsNoEvent(t *testing.T) {
	ctx, pool := postgresPool(t)
	store := memory.NewPostgres(pool)
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	run, cited := finished(t, "run-whole")
	resolved, err := memory.NewResolver(run.ledger, run.content).
		Resolve(ctx, run.scope, []domain.MemoryEvidence{cited})
	if err != nil {
		t.Fatalf("resolve for the fixture: %v", err)
	}
	created, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
		a.Scope = run.scope
		a.Evidence = resolved
		a.Labels = resolved[0].Labels
	}), "usr_ana", "reviewed", now)
	if err != nil {
		t.Fatalf("Assert: %v", err)
	}

	if _, err := pool.Exec(ctx,
		`update memory_assertions set canonical_identity_key = null where assertion_id = $1`,
		created.ID); err != nil {
		t.Fatalf("clear the key: %v", err)
	}

	before := countHydratedEvents(t, ctx, pool, created.ID)
	if _, err := store.Hydrate(ctx, memory.NewResolver(run.ledger, run.content),
		memory.HydratePage{Limit: 10, Now: now}); err != nil {
		t.Fatalf("Hydrate: %v", err)
	}

	var key *string
	if err := pool.QueryRow(ctx,
		`select canonical_identity_key from memory_assertions where assertion_id = $1`,
		created.ID).Scan(&key); err != nil {
		t.Fatalf("read the key: %v", err)
	}
	if key == nil {
		t.Fatal("the key was not filled")
	}
	if after := countHydratedEvents(t, ctx, pool, created.ID); after != before {
		t.Errorf("hydrated events went %d -> %d, want none for arithmetic", before, after)
	}
}

func countHydratedEvents(
	t *testing.T, ctx context.Context, pool *pgxpool.Pool, id string,
) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx,
		`select count(*) from memory_assertion_events where assertion_id = $1 and action = 'hydrated'`,
		id).Scan(&n); err != nil {
		t.Fatalf("count events: %v", err)
	}
	return n
}

/*
A pending proposal is repaired too, and nothing else about it moves.

A suggestion written before this work carries the older citation, and accepting
it after the rollout puts it through the merge with labels it cannot explain —
which the eviction rule refuses. So the queue has to be repaired as well, or the
first accept after an upgrade fails on a proposal somebody made weeks ago.

What must not move is everything a person or a policy decided: the status, the
claim, the count of observations, the expiry, who wrote it and when it was last
touched.
*/
func expectSuggestionHydrationCompletesTheCitation(
	t *testing.T, ctx context.Context, store suggestionHydrator,
	resolver *memory.Resolver, seeded domain.MemorySuggestion, now time.Time,
) {
	t.Helper()

	if _, err := store.HydrateSuggestions(ctx, resolver,
		memory.HydratePage{Limit: 10, Now: now}); err != nil {
		t.Fatalf("HydrateSuggestions: %v", err)
	}

	found, err := store.ListSuggestions(ctx, memory.SuggestionFilter{
		Scopes: []domain.Scope{seeded.Scope}, Now: now,
	})
	if err != nil {
		t.Fatalf("ListSuggestions: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("suggestions = %d, want the one that was there", len(found))
	}
	got := found[0]

	if len(got.Evidence) != 1 || got.Evidence[0].Seq == 0 {
		t.Errorf("evidence = %+v, want one citation that says which step it names", got.Evidence)
	}
	if !got.Labels.Has(domain.LabelUntrusted) {
		t.Errorf("labels = %v, want the taint the run carried", got.Labels)
	}
	if got.Status != seeded.Status || got.Claim != seeded.Claim ||
		got.Observations != seeded.Observations || got.CreatedBy != seeded.CreatedBy ||
		!got.UpdatedAt.Equal(seeded.UpdatedAt) {
		t.Errorf("hydration changed what somebody decided: %+v", got)
	}
}

type suggestionHydrator interface {
	HydrateSuggestions(context.Context, *memory.Resolver, memory.HydratePage) (memory.HydrateResult, error)
	ListSuggestions(context.Context, memory.SuggestionFilter) ([]domain.MemorySuggestion, error)
}

func legacySuggestion(t *testing.T) (*run, domain.MemorySuggestion) {
	t.Helper()
	r := newRun(t, "run-suggest")
	r.step(domain.StepRunStarted, domain.RunStartedPayload{Trigger: "channel"}, domain.LabelUntrusted)
	ref, digest := r.put(2, []byte(`{"kind":"incident","subject":"slack"}`))
	r.step(domain.StepToolCalled, domain.ToolCalledPayload{
		Tool: domain.ToolMemorySuggest, Effect: domain.EffectWrite,
		ArgsRef: ref, ArgsDigest: digest,
	})
	return r, suggestion(func(s *domain.MemorySuggestion) {
		s.Scope = r.scope
		// The older shape: run and artifact, no step.
		s.Evidence = []domain.MemoryEvidence{{
			RunID: "run-suggest", Artifact: domain.ArtifactMemorySuggestion, Digest: digest,
		}}
	})
}

func TestHydrateSuggestions_completesTheCitation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := memory.NewMemory()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	run, seed := legacySuggestion(t)
	out, err := store.Suggest(ctx, seed, reviewPolicy, "agent:triage", now)
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	expectSuggestionHydrationCompletesTheCitation(t, ctx, store, run.resolver(), out.Suggestion, now)
}

func TestPostgresHydrateSuggestions_completesTheCitation(t *testing.T) {
	ctx, store := postgresStore(t)
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	run, seed := legacySuggestion(t)
	out, err := store.Suggest(ctx, seed, reviewPolicy, "agent:triage", now)
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	expectSuggestionHydrationCompletesTheCitation(t, ctx, store, run.resolver(), out.Suggestion, now)
}

// hydrated in memory_assertion_events would be ambiguous: the row carries no
// suggestion id, no status and no covered_by, so a reader could not tell a
// repaired memory from a repaired proposal, nor rebuild either from it.
func TestPostgresHydrateSuggestions_recordNoEvent(t *testing.T) {
	ctx, pool := postgresPool(t)
	store := memory.NewPostgres(pool)
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	run, seed := legacySuggestion(t)
	out, err := store.Suggest(ctx, seed, reviewPolicy, "agent:triage", now)
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	before := countHydratedEvents(t, ctx, pool, out.Suggestion.AssertionID)

	if _, err := store.HydrateSuggestions(ctx, run.resolver(),
		memory.HydratePage{Limit: 10, Now: now}); err != nil {
		t.Fatalf("HydrateSuggestions: %v", err)
	}
	if after := countHydratedEvents(t, ctx, pool, out.Suggestion.AssertionID); after != before {
		t.Errorf("hydrated events went %d -> %d, want a repair to write none", before, after)
	}
}

/*
A repair of a proposal gives way to the observation that overtook it.

Same shape as the assertion case and the same stake: the ledger is read outside
the lock, so another run can merge a citation into the proposal while the repair
is still reading. Writing the snapshot's version over it would delete an
observation that had just been counted.
*/
func TestPostgresHydrateSuggestions_overtaken_doesNotLoseTheNewCitation(t *testing.T) {
	ctx, store := postgresStore(t)
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	run, seed := legacySuggestion(t)
	if _, err := store.Suggest(ctx, seed, reviewPolicy, "agent:triage", now); err != nil {
		t.Fatalf("Suggest: %v", err)
	}

	blocked := &blockingContent{
		inner: run.content, entered: make(chan struct{}), release: make(chan struct{}),
	}
	done := make(chan error, 1)
	go func() {
		_, err := store.HydrateSuggestions(ctx, memory.NewResolver(run.ledger, blocked),
			memory.HydratePage{Limit: 10, Now: now})
		done <- err
	}()

	<-blocked.entered
	// A second run observes the same fact while the repair is reading.
	second := seed
	second.Evidence = []domain.MemoryEvidence{{
		RunID: "run-second", Seq: 3, Artifact: domain.ArtifactMemorySuggestion,
		Digest: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
	}}
	if _, err := store.Suggest(ctx, second, reviewPolicy, "agent:triage", now); err != nil {
		t.Fatalf("Suggest while hydrating: %v", err)
	}
	close(blocked.release)

	if err := <-done; err != nil {
		t.Fatalf("HydrateSuggestions: %v", err)
	}

	found, err := store.ListSuggestions(ctx, memory.SuggestionFilter{
		Scopes: []domain.Scope{run.scope}, Now: now,
	})
	if err != nil {
		t.Fatalf("ListSuggestions: %v", err)
	}
	var runs []domain.RunID
	for _, ev := range found[0].Evidence {
		runs = append(runs, ev.RunID)
	}
	if !slices.Contains(runs, "run-second") {
		t.Errorf("evidence cites %v, want the observation that arrived mid-repair", runs)
	}
}

/*
A memory whose run is gone stops being readable, and the sweep carries on.

The population hydration exists to repair is the oldest one, so a run that
retention has taken is the likeliest thing it will meet. Treating that as a
failure stopped the sweep on the first such row — every row after it stayed
unrepaired for ever. Treating it as merely unprovable would have left active
memory whose source we know does not exist.

Two rows in one page: the first cites a run that is not there, the second is
whole. The first ends source_erased, the second is hydrated, and neither
prevents the other.
*/
func TestPostgresHydrate_runGoneEndsThatRowAndNotTheSweep(t *testing.T) {
	ctx, store := postgresStore(t)
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	run, cited := legacyRun(t, "run-legacy")
	gone := assertion(func(a *domain.MemoryAssertion) {
		a.Scope, a.Subject = run.scope, "aaa first by id"
		a.Evidence = []domain.MemoryEvidence{{
			RunID: "run-taken-by-retention", Artifact: domain.ArtifactFinalAnswer,
			Digest: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
		}}
	})
	whole := assertion(func(a *domain.MemoryAssertion) {
		a.Scope, a.Subject = run.scope, "zzz second by id"
		a.Evidence = []domain.MemoryEvidence{cited}
	})
	for _, a := range []domain.MemoryAssertion{gone, whole} {
		if _, err := store.Assert(ctx, a, "usr_ana", "reviewed", now); err != nil {
			t.Fatalf("Assert: %v", err)
		}
	}

	out, err := store.Hydrate(ctx, run.resolver(), memory.HydratePage{Limit: 10, Now: now})
	if err != nil {
		t.Fatalf("Hydrate stopped on a run that is gone: %v", err)
	}
	if out.SourceGone != 1 {
		t.Errorf("source gone = %d, want the one whose run was taken", out.SourceGone)
	}

	readable, err := store.Find(ctx, domain.MemoryQuery{
		Scope: run.scope, AgentID: "triage", Now: now,
	})
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if len(readable) != 1 || readable[0].Subject != "zzz second by id" {
		t.Fatalf("readable = %+v, want only the memory whose run is still there", readable)
	}
	if readable[0].Evidence[0].Seq == 0 {
		t.Error("the surviving memory was not hydrated")
	}
}

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/authoring"
	"github.com/fuseone/agents/internal/authoring/drafts"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/model"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/testdb"
)

/*
A turn of the workbench's conversation, from the route to the provider and
back (NT-018 phase 3). Nothing is injected between the route and the wire: the
installation's choice becomes the model asked and its output cap, the
configured price becomes the reservation, the usage the provider reports
becomes the cost, and the answer's operation becomes a proposal in Ana's
thread. A retry of the key calls nobody; Bia reads nothing of Ana's; Ana's
erasure takes what was said and leaves the figures.
*/

// answeringModel is an OpenAI-compatible provider that answers one turn and
// remembers what it was asked.
type answeringModel struct {
	mu    sync.Mutex
	asked []struct {
		Model     string `json:"model"`
		MaxTokens int64  `json:"max_tokens"`
	}
}

func (m *answeringModel) serve(t *testing.T) string {
	t.Helper()
	answer := `{"reply":"Sugiro responder em inglês.","proposals":[{"kind":"set_reply_locale","operation":{"Locale":"en-US"}}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model     string `json:"model"`
			MaxTokens int64  `json:"max_tokens"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		m.mu.Lock()
		m.asked = append(m.asked, body)
		m.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": answer}}},
			"usage":   map[string]any{"prompt_tokens": 1000, "completion_tokens": 100},
		})
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// fixedAuthoring is the installation's authoring choice; the store that keeps
// it has its own tests.
type fixedAuthoring struct{ choice authoring.Choice }

func (f fixedAuthoring) Current(context.Context) (authoring.Choice, error) { return f.choice, nil }
func (fixedAuthoring) Choose(context.Context, authoring.Choice, domain.UserID) error {
	return nil
}
func (fixedAuthoring) Disable(context.Context, domain.UserID) error { return nil }

func TestTurnRoute_aTurnReachesTheProviderItNamesAndSettlesAtWhatItCost(t *testing.T) {
	pool := testdb.Pool(t)
	m := &answeringModel{}
	providers := model.NewRegistry(nil)
	if err := providers.Register(model.Provider{
		Name: "assistant", Kind: model.KindOpenAICompatible, BaseURL: m.serve(t), APIKey: "k",
		Prices: map[string]model.Prices{"author-large": {InputMicros: 3_000_000, OutputMicros: 15_000_000}},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	api := httpapi.NewServer(ledger.NewMemory(), "test").
		WithAgents(spec.NewRegistry(pool)).
		WithPublisher(spec.NewPublisher(pool, engine.SystemClock{})).
		WithProviderNames(providers).
		WithAuthoring(fixedAuthoring{authoring.Choice{Provider: "assistant", Model: "author-large", DailyMicros: 50_000_000, Enabled: true}}).
		WithDrafts(drafts.NewStore(pool)).
		WithTurnModels(providers)
	route := draftJourney{etagJourney{handler: apiHandlerFor(api)}}
	agent := fmt.Sprintf("nt018-turn-%d", time.Now().UnixNano())
	ana := areaAuthor("factory")
	bia := domain.Principal{ID: "usr_bia", Kind: domain.PrincipalUser,
		Grants: []domain.Grant{{Scope: domain.Scope{Company: "acme", Area: "factory"}, Role: domain.RoleAuthor}}}
	wantStatus(t, "create", route.create(t, ana, agent, "factory"), http.StatusCreated)

	turnsPath := "/api/v1/agents/" + agent + "/draft/turns"
	send := func() openapi.DraftTurn {
		rec := route.do(t, ana, http.MethodPost, turnsPath, []byte(`{"key":"k1","message":"Responda os clientes."}`), nil)
		wantStatus(t, "send", rec, http.StatusOK)
		var turn openapi.DraftTurn
		decode(t, rec, &turn)
		return turn
	}
	first := send()
	// 1000 tokens in at 3 and 100 out at 15 per million: 4500 micros.
	if first.State != openapi.DraftTurnStateDone || first.CostMicros != 4500 || first.ReservedMicros <= first.CostMicros {
		t.Fatalf("turn = %+v, want it done at 4500 under a larger reservation", first)
	}
	if len(m.asked) != 1 || m.asked[0].Model != "author-large" || m.asked[0].MaxTokens != 4096 {
		t.Fatalf("provider asked %+v, want author-large capped at 4096 tokens", m.asked)
	}
	if again := send(); again.TurnId != first.TurnId || len(m.asked) != 1 {
		t.Errorf("the key again = %+v after %d calls, want the stored turn and no call", again, len(m.asked))
	}

	var proposals struct{ Items []openapi.DraftProposal }
	decode(t, route.do(t, ana, http.MethodGet, "/api/v1/agents/"+agent+"/draft/proposals", nil, nil), &proposals)
	if len(proposals.Items) != 1 || proposals.Items[0].Kind != "set_reply_locale" || valueOf(proposals.Items[0].TurnId) != first.TurnId {
		t.Errorf("Ana's proposals = %+v, want the turn's one, naming it", proposals.Items)
	}

	var thread struct {
		Items        []openapi.DraftTurn
		Conversation string
	}
	decode(t, route.do(t, bia, http.MethodGet, turnsPath, nil, nil), &thread)
	if len(thread.Items) != 0 || thread.Conversation != "open" {
		t.Errorf("Bia reads %d of Ana's turns, the conversation %q", len(thread.Items), thread.Conversation)
	}
	wantStatus(t, "erase", route.do(t, ana, http.MethodDelete, "/api/v1/agents/"+agent+"/draft/thread", nil, nil), http.StatusNoContent)
	decode(t, route.do(t, ana, http.MethodGet, turnsPath, nil, nil), &thread)
	if len(thread.Items) != 1 || !thread.Items[0].Erased || thread.Items[0].Message != nil || thread.Items[0].CostMicros != 4500 {
		t.Errorf("after erasure = %+v, want the words gone and the cost kept", thread.Items)
	}
	if strings.Contains(fmt.Sprint(thread.Items), "Responda os clientes") {
		t.Error("the message survived its erasure")
	}
}

func valueOf(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

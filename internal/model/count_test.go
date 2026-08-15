package model_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fuseone/agents/internal/model"
)

/*
What an instruction costs, answered by the model that will read it.

The console printed characters and said so, because a token count needs a
tokeniser and the console has none. Estimating one there would put a wrong
number exactly where somebody goes to size a prompt, and it would age with
every model released. So the count is the provider's own or it is not offered.
*/

func TestCount_returnsTheProvidersOwnNumber(t *testing.T) {
	t.Parallel()

	counter, _ := counterAgainst(t, `{"input_tokens": 412}`)

	got, err := counter.Count(context.Background(), "Answer only about refunds.")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if got != 412 {
		t.Errorf("tokens = %d, want the 412 the provider answered", got)
	}
}

// Counted where a run sends it. An instruction is the system prompt; counted
// as a user message it would be a different number, for a request the
// platform never makes.
func TestCount_sendsTheInstructionAsTheSystemPrompt(t *testing.T) {
	t.Parallel()

	counter, sent := counterAgainst(t, `{"input_tokens": 412}`)

	if _, err := counter.Count(context.Background(), "Answer only about refunds."); err != nil {
		t.Fatalf("count: %v", err)
	}
	if got := (*sent)["system"]; got != "Answer only about refunds." {
		t.Errorf("system = %v, want the instruction", got)
	}
	if got := (*sent)["model"]; got != "claude-opus-5" {
		t.Errorf("model = %v, want the agent's model", got)
	}
}

func TestCounter_aProviderThatCannotCount_saysSoRatherThanEstimating(t *testing.T) {
	t.Parallel()

	registry := model.NewRegistry(nil)
	if err := registry.Register(model.Provider{
		Name: "vllm", Kind: model.KindOpenAICompatible, BaseURL: "http://127.0.0.1:1/v1",
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	_, err := registry.Counter("vllm", model.Config{Model: "llama-3"})
	if !errors.Is(err, model.ErrNoTokeniser) {
		t.Fatalf("err = %v, want ErrNoTokeniser", err)
	}
}

// counterAgainst is a counter pointed at a stub provider, and the body it was
// sent.
func counterAgainst(t *testing.T, answer string) (model.Counter, *map[string]any) {
	t.Helper()

	sent := map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &sent)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(answer))
	}))
	t.Cleanup(server.Close)

	registry := model.NewRegistry(server.Client())
	if err := registry.Register(model.Provider{
		Name: "anthropic", Kind: model.KindAnthropic, BaseURL: server.URL, APIKey: "test",
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	counter, err := registry.Counter("anthropic", model.Config{Model: "claude-opus-5"})
	if err != nil {
		t.Fatalf("counter: %v", err)
	}
	return counter, &sent
}

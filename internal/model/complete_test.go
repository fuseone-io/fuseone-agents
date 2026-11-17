package model_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fuseone/agents/internal/model"
)

/*
A completion's output cap reaches the provider. Callers that reserve a call's
worst case before making it (NT-018 phase 3) price the cap: a provider never
told it could answer longer, and cost more, than what was set aside. With no
cap asked, the platform's default is sent, as a run's planner sends it.
*/
func TestComplete_openAICompatible_sendsTheOutputCap(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct{ cap, want int64 }{
		"a cap is sent":           {cap: 4096, want: 4096},
		"no cap, the default one": {cap: 0, want: 16000},
	} {
		var asked map[string]any
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&asked)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{}}`))
		}))
		registry := model.NewRegistry(nil)
		if err := registry.Register(model.Provider{Name: "p", Kind: model.KindOpenAICompatible, BaseURL: srv.URL, APIKey: "k"}); err != nil {
			t.Fatalf("Register: %v", err)
		}
		completer, err := registry.Completer("p", model.Config{Model: "m", MaxTokens: tc.cap})
		if err != nil {
			t.Fatalf("Completer: %v", err)
		}
		if _, err := completer.Complete(t.Context(), "hi"); err != nil {
			t.Fatalf("%s: Complete: %v", name, err)
		}
		srv.Close()
		if got := asked["max_tokens"]; got != float64(tc.want) {
			t.Errorf("%s: max_tokens = %v, want %d", name, got, tc.want)
		}
	}
}

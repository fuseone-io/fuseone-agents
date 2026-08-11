package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"strings"
	"sync"
)

// serveModel speaks chat-completions, so the platform's real client talks to
// it over real HTTP: request encoding, tool-call parsing and usage accounting
// all run the production path.
//
// What it does not simulate is judgement. It calls each tool the agent was
// granted, once, in the order it was offered them, and then reports what it
// found. That makes a local run deterministic, which is the point: a
// development stack whose behaviour changes between runs cannot tell you
// whether a change to the platform did.
func serveModel(args []string) error {
	fs := flag.NewFlagSet("model", flag.ContinueOnError)
	addr := fs.String("addr", "127.0.0.1:8091", "listen address")
	if err := fs.Parse(args); err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /chat/completions", complete)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	log.Printf("devstack model listening on http://%s", *addr)
	return http.ListenAndServe(*addr, mux)
}

// turns counts requests per conversation so the stub advances instead of
// looping. Keyed by the run's transcript length, which is the only thing that
// distinguishes one turn from the next on this endpoint.
var turns sync.Map

func complete(w http.ResponseWriter, r *http.Request) {
	var req request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(reply(req))
}

func reply(req request) map[string]any {
	// One tool call per tool offered, then an answer. Which turn this is falls
	// out of how many tool results are already in the transcript, so the stub
	// needs no session of its own and resumes correctly after a restart.
	used := 0
	for _, m := range req.Messages {
		if m.Role == "tool" {
			used++
		}
	}

	message := map[string]any{"role": "assistant"}
	if used < len(req.Tools) {
		tool := req.Tools[used].Function
		message["tool_calls"] = []map[string]any{{
			"id": "call_" + tool.Name, "type": "function",
			"function": map[string]any{
				"name":      tool.Name,
				"arguments": argumentsFor(tool),
			},
		}}
	} else {
		message["content"] = "Consultei " + summarise(req.Tools) +
			". Cliente acct_4471 está ativo no plano enterprise; nada exige ação."
	}

	return map[string]any{
		"choices": []map[string]any{{"finish_reason": "stop", "message": message}},
		// Plausible figures so the cost ledger has something real to add up.
		"usage": map[string]any{
			"prompt_tokens":         1200 + int64(used)*450,
			"completion_tokens":     90,
			"prompt_tokens_details": map[string]any{"cached_tokens": int64(used) * 400},
		},
	}
}

// argumentsFor fills the tool's schema with values that satisfy it. The Gate
// validates arguments against the contract, so a stub that sent {} would be
// testing the refusal path on every call.
func argumentsFor(tool function) string {
	args := map[string]any{}
	for name, raw := range tool.Parameters.Properties {
		switch {
		case strings.Contains(name, "email"):
			args[name] = "cliente@exemplo.com.br"
		case raw.Type == "number" || raw.Type == "integer":
			args[name] = 1
		case raw.Type == "boolean":
			args[name] = true
		default:
			args[name] = "segunda via de boleto"
		}
	}
	encoded, _ := json.Marshal(args)
	return string(encoded)
}

func summarise(tools []tool) string {
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		names = append(names, t.Function.Name)
	}
	if len(names) == 0 {
		return "nenhuma ferramenta"
	}
	return strings.Join(names, ", ")
}

type request struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
	Tools    []tool    `json:"tools"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type tool struct {
	Function function `json:"function"`
}

type function struct {
	Name       string `json:"name"`
	Parameters struct {
		Properties map[string]struct {
			Type string `json:"type"`
		} `json:"properties"`
	} `json:"parameters"`
}

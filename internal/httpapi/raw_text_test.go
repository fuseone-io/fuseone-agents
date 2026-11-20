package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

/*
A route that keeps what a person typed refuses text the JSON decoder would rewrite
(NT-018 phase 4). encoding/json replaces invalid UTF-8 and an unpaired surrogate escape with
U+FFFD without a word, so a handler only ever sees valid text: the check reads the bytes
before the decoder does.
*/

const rehearsalPattern = "POST /api/v1/agents/*/rehearsals"

type textReached struct {
	hit  bool
	body []byte
}

func (r *textReached) ServeHTTP(_ http.ResponseWriter, req *http.Request) {
	r.hit = true
	r.body, _ = io.ReadAll(req.Body)
}

func sendRaw(method, path string, body []byte) (*httptest.ResponseRecorder, *textReached) {
	next := &textReached{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	RefuseRewrittenText(next, rehearsalPattern).ServeHTTP(rec, req)
	return rec, next
}

func TestRefuseRewrittenText_textTheDecoderWouldRewrite_isRefusedBeforeTheHandler(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{
		"invalid UTF-8":                     "{\"case\":\"caso \xff\"}",
		"a lone high surrogate":             `{"case":"\ud800"}`,
		"a lone low surrogate":              `{"case":"\udc00"}`,
		"a high surrogate before a char":    `{"case":"\ud800A"}`,
		"a high surrogate before an escape": `{"case":"\ud800\u0041"}`,
		"a high surrogate at the end":       `{"case":"\uD800`,
		"a lone surrogate in another key":   `{"sealId":"\udfff","case":"ok"}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rec, next := sendRaw(http.MethodPost, "/api/v1/agents/triage/rehearsals", []byte(body))
			if rec.Code != http.StatusBadRequest || next.hit {
				t.Fatalf("status = %d, handler reached = %v; want 400 before the handler", rec.Code, next.hit)
			}
			var problem struct{ Type string }
			if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil || problem.Type != string(CodeInvalidInput) {
				t.Errorf("problem = %s (%v), want %s", rec.Body, err, CodeInvalidInput)
			}
		})
	}
}

func TestRefuseRewrittenText_textTheDecoderKeeps_reachesTheHandlerIntact(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{
		"plain text":                      `{"case":"O cliente pede reembolso"}`,
		"a surrogate pair":                `{"case":"\ud83d\ude00"}`,
		"a replacement character typed":   `{"case":"` + "�" + `"}`,
		"an escaped backslash before a u": `{"case":"\\ud800"}`,
		"accents as UTF-8":                `{"case":"ação"}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rec, next := sendRaw(http.MethodPost, "/api/v1/agents/triage/rehearsals", []byte(body))
			if !next.hit || string(next.body) != body || rec.Code != http.StatusOK {
				t.Fatalf("handler reached = %v with %q (status %d), want the body as sent", next.hit, next.body, rec.Code)
			}
		})
	}
}

func TestRefuseRewrittenText_otherRoutes_areNotJudged(t *testing.T) {
	t.Parallel()
	body := []byte("{\"x\":\"\xff\"}")
	for name, req := range map[string][2]string{
		"another method":     {http.MethodGet, "/api/v1/agents/triage/rehearsals"},
		"another path":       {http.MethodPost, "/api/v1/agents/triage/experiments"},
		"a deeper path":      {http.MethodPost, "/api/v1/agents/triage/rehearsals/x"},
		"outside the prefix": {http.MethodPost, "/agents/triage/rehearsals"},
	} {
		if _, next := sendRaw(req[0], req[1], body); !next.hit || !bytes.Equal(next.body, body) {
			t.Errorf("%s: handler reached = %v with %q, want it untouched", name, next.hit, next.body)
		}
	}
}

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

/*
An `.agent.md` published through the route an installation's pipeline calls
is the version the console's fields publish (NT-018, contract 9).

Through the router, the content type on the wire and the Postgres publisher,
because each could split the two formats: a router that decoded only JSON, a
handler that published the file's own bytes instead of their canonical form,
or a claim that saw two digests for one definition. The file carries no
company, as one kept for several installations does; the query supplies it.
*/
func TestPublishRoute_anAgentFile_isTheVersionTheFieldsPublish(t *testing.T) {
	route := cycleRoute(t)
	agent := fmt.Sprintf("nt018-file-%d", time.Now().UnixNano())

	fields := route.publishFields(t, agent)
	file := route.publishFile(t, agent, agentFileFor(agent, ""), "acme")

	if file.Code != http.StatusOK {
		t.Fatalf("PUT file = %d: %s", file.Code, file.Body)
	}
	var got openapi.AgentPublished
	if err := json.Unmarshal(file.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.VersionId != fields.VersionId || got.Created {
		t.Fatalf("file published %s (created %v), want the fields' %s and no new version",
			got.VersionId, got.Created, fields.VersionId)
	}
}

// The same file naming a company the query contradicts is refused on the
// wire as the JSON's invalid request is, before anything is written.
func TestPublishRoute_anAgentFileWhoseCompanyTheQueryContradicts_isRefused(t *testing.T) {
	route := cycleRoute(t)
	agent := fmt.Sprintf("nt018-file-%d", time.Now().UnixNano())

	rec := route.publishFile(t, agent, agentFileFor(agent, "acme"), "globex")

	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "fuseone:invalid-input") {
		t.Fatalf("PUT file = %d: %s; want the invalid-input problem", rec.Code, rec.Body)
	}
}

func agentFileFor(agent, company string) string {
	front := "---\nid: " + agent + "\nname: " + agent + "\narea: factory\n"
	if company != "" {
		front += "company: " + company + "\n"
	}
	return front + "provider: openai\nmodel: gpt-test\ntools: [crm.lookup]\n" +
		"budget: {micros: 500000, steps: 60}\n---\n\nDo the work.\n"
}

// publishFields is the console's PUT of the same definition as fields.
func (r publishRoute) publishFields(t *testing.T, agent string) openapi.AgentPublished {
	t.Helper()
	body, err := json.Marshal(openapi.AgentDefinition{
		Name: agent, Company: "acme", Area: "factory",
		Provider: "openai", Model: "gpt-test", Instructions: "Do the work.",
		Tools:  &[]string{"crm.lookup"},
		Budget: &openapi.Budget{Micros: ptrTo(int64(500_000)), Steps: ptrTo(int64(60))},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	rec := r.put(t, "/api/v1/agents/"+agent+"/versions", "application/json", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT fields = %d: %s", rec.Code, rec.Body)
	}
	var out openapi.AgentPublished
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

func (r publishRoute) publishFile(t *testing.T, agent, file, company string) *httptest.ResponseRecorder {
	t.Helper()
	path := "/api/v1/agents/" + agent + "/versions?company=" + url.QueryEscape(company)
	return r.put(t, path, "text/markdown; charset=utf-8", []byte(file))
}

func (r publishRoute) put(t *testing.T, path, contentType string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPut, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	req = req.WithContext(auth.WithPrincipal(req.Context(), domain.Principal{
		ID: "usr_ana", Kind: domain.PrincipalUser,
		Grants: []domain.Grant{{Scope: domain.Scope{Company: "acme"}, Role: domain.RoleAuthor}},
	}))
	rec := httptest.NewRecorder()
	r.handler.ServeHTTP(rec, req)
	return rec
}

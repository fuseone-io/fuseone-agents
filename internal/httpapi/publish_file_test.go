package httpapi

import (
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

/*
An agent published as the file it is kept in (NT-018, contract 9).

The file an installation keeps in git and the fields the console sends are one
format, so the same definition sent either way is one version: the file is
parsed, rendered and parsed again exactly as the fields are, and the company a
file kept for several installations leaves out comes from the query.
*/

// agentFile is the JSON fixture's definition as an author would keep it:
// keys in another order, a comment, a flow list, blank lines around the body.
// None of that may reach the version.
func agentFile(company string) string {
	front := "---\n# Kept in git.\nid: triage\narea: cx\nname: Atendimento\n"
	if company != "" {
		front += "company: " + company + "\n"
	}
	return front + "provider: openai\nmodel: gpt-test\ntools: [crm.lookup]\n" +
		"budget:\n  steps: 60\n  micros: 500000\n---\n\n\nAtenda o chamado e responda.\n\n"
}

func publishFile(
	t *testing.T, server *Server, id, file string, company *string,
) openapi.PublishAgentResponseObject {
	t.Helper()
	resp, err := server.PublishAgent(inArea("cx", domain.RoleAuthor), openapi.PublishAgentRequestObject{
		AgentId: id, Params: openapi.PublishAgentParams{Company: company},
		Body: strings.NewReader(file),
	})
	if err != nil {
		t.Fatalf("PublishAgent(file): %v", err)
	}
	return resp
}

func TestPublishAgentFile_theSameDefinitionAsTheJSON_isTheSameVersion(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		file    string
		company *string
	}{
		{name: "company in the frontmatter", file: agentFile("acme")},
		{name: "company only in the query", file: agentFile(""), company: ptr("acme")},
		{name: "company in both, agreeing", file: agentFile("acme"), company: ptr("acme")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pub := newPublisher()
			server := publishServer(t, pub)

			fields, err := server.PublishAgent(inArea("cx", domain.RoleAuthor),
				openapi.PublishAgentRequestObject{AgentId: "triage", JSONBody: definition(nil)})
			if err != nil {
				t.Fatalf("PublishAgent(JSON): %v", err)
			}
			file := publishFile(t, server, "triage", tc.file, tc.company)

			a, okA := fields.(openapi.PublishAgent200JSONResponse)
			b, okB := file.(openapi.PublishAgent200JSONResponse)
			if !okA || !okB {
				t.Fatalf("responses = %T, %T; want both published", fields, file)
			}
			if a.VersionId != b.VersionId || *a.Definition != *b.Definition {
				t.Fatalf("JSON is %s and the file is %s\n%s\n%s",
					a.VersionId, b.VersionId, *a.Definition, *b.Definition)
			}
			if len(pub.published) != 2 || pub.published[1].Version != pub.published[0].Version ||
				pub.published[1].Digest != pub.published[0].Digest {
				t.Fatalf("published %+v, want the same version and digest twice", pub.published)
			}
			if pub.companies[1] != "acme" || pub.published[1].Company != "acme" {
				t.Errorf("published into %q (spec %q), want acme", pub.companies[1], pub.published[1].Company)
			}
		})
	}
}

// What contract 9 refuses, each as the JSON's invalid request is refused:
// a 400 problem, and nothing written.
func TestPublishAgentFile_companyOrIdTheRequestCannotAgreeOn_isRefused(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		id      string
		file    string
		company *string
		want    string
	}{
		{name: "company in neither", id: "triage", file: agentFile(""), want: "company"},
		// Sent empty, the query names no company: absent in both is still a
		// 400 before authorisation, never a 403 for an empty scope.
		{name: "an empty query and none in the file", id: "triage", file: agentFile(""),
			company: ptr(""), want: "company"},
		{name: "a blank query and none in the file", id: "triage", file: agentFile(""),
			company: ptr("  "), want: "company"},
		{name: "an empty query beside the file's company", id: "triage", file: agentFile("acme"),
			company: ptr(""), want: "company"},
		{name: "frontmatter and query disagree", id: "triage", file: agentFile("acme"),
			company: ptr("globex"), want: "company"},
		{name: "the file names another agent", id: "billing", file: agentFile("acme"), want: "triage"},
		{name: "not an agent file", id: "triage", file: "Atenda o chamado.", want: "frontmatter"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pub := newPublisher()
			resp := publishFile(t, publishServer(t, pub), tc.id, tc.file, tc.company)

			refused, ok := resp.(openapi.PublishAgent400ApplicationProblemPlusJSONResponse)
			if !ok || len(pub.published) != 0 {
				t.Fatalf("response = %T (%d published), want 400 and nothing written", resp, len(pub.published))
			}
			if refused.Detail == nil || !strings.Contains(*refused.Detail, tc.want) {
				t.Errorf("detail = %v, want it to name %q", refused.Detail, tc.want)
			}
		})
	}
}

// The query names the file's company. Sent with the fields too, it has to say
// what the body says, or one request would name two companies; sent empty, it
// names none, whichever format it comes beside.
func TestPublishAgent_queryCompanyOtherThanTheBody_isRefused(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		query string
		body  string
	}{
		{name: "another company", query: "globex", body: "acme"},
		{name: "empty, beside an empty body", query: "", body: ""},
		{name: "blank, beside an empty body", query: " ", body: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pub := newPublisher()
			resp, err := publishServer(t, pub).PublishAgent(inArea("cx", domain.RoleAuthor),
				openapi.PublishAgentRequestObject{
					AgentId:  "triage",
					JSONBody: definition(func(d *openapi.AgentDefinition) { d.Company = tc.body }),
					Params:   openapi.PublishAgentParams{Company: ptr(tc.query)},
				})
			if err != nil {
				t.Fatalf("PublishAgent: %v", err)
			}
			if _, ok := resp.(openapi.PublishAgent400ApplicationProblemPlusJSONResponse); !ok || len(pub.published) != 0 {
				t.Fatalf("response = %T (%d published), want 400 and nothing written", resp, len(pub.published))
			}
		})
	}
}

// The installation's checks are the publication's, whatever the format: a
// tool this installation does not offer is refused with the JSON's own words.
func TestPublishAgentFile_aToolNotOffered_isRefusedAsTheJSONIs(t *testing.T) {
	t.Parallel()
	pub := newPublisher()
	server := publishServer(t, pub).WithAdministration(nil, publishFlowTools(), nil)
	misspelt := strings.Replace(agentFile("acme"), "[crm.lookup]", "[crm.lokup]", 1)

	fields, err := server.PublishAgent(inArea("cx", domain.RoleAuthor), openapi.PublishAgentRequestObject{
		AgentId: "triage", JSONBody: definition(func(d *openapi.AgentDefinition) {
			d.Tools = &[]string{"crm.lokup"}
		}),
	})
	if err != nil {
		t.Fatalf("PublishAgent(JSON): %v", err)
	}
	file := publishFile(t, server, "triage", misspelt, nil)

	a, okA := fields.(openapi.PublishAgent400ApplicationProblemPlusJSONResponse)
	b, okB := file.(openapi.PublishAgent400ApplicationProblemPlusJSONResponse)
	if !okA || !okB || len(pub.published) != 0 {
		t.Fatalf("responses = %T, %T (%d published), want both refused", fields, file, len(pub.published))
	}
	if a.Detail == nil || b.Detail == nil || *a.Detail != *b.Detail || !strings.Contains(*b.Detail, "crm.lokup") {
		t.Errorf("details = %v and %v, want the same refusal naming crm.lokup", a.Detail, b.Detail)
	}
}

// The scope a file declares is judged like the one the fields declare.
func TestPublishAgentFile_anAreaTheCallerCannotPublishInto_isForbidden(t *testing.T) {
	t.Parallel()
	pub := newPublisher()
	resp, err := publishServer(t, pub).PublishAgent(inArea("ops", domain.RoleAuthor),
		openapi.PublishAgentRequestObject{AgentId: "triage", Body: strings.NewReader(agentFile("acme"))})
	if err != nil {
		t.Fatalf("PublishAgent: %v", err)
	}
	if _, ok := resp.(openapi.PublishAgent403ApplicationProblemPlusJSONResponse); !ok || len(pub.published) != 0 {
		t.Fatalf("response = %T (%d published), want 403 and nothing written", resp, len(pub.published))
	}
}

func TestPublishAgentFile_overTheCeiling_isRefusedAsTooLarge(t *testing.T) {
	t.Parallel()
	pub := newPublisher()
	huge := agentFile("acme") + strings.Repeat("x", maxAgentFileBytes)

	resp := publishFile(t, publishServer(t, pub), "triage", huge, nil)

	if _, ok := resp.(openapi.PublishAgent413ApplicationProblemPlusJSONResponse); !ok || len(pub.published) != 0 {
		t.Fatalf("response = %T (%d published), want 413 and nothing written", resp, len(pub.published))
	}
}

// Neither the fields nor the file: a content type the route does not read.
func TestPublishAgent_noDefinitionAtAll_isRefused(t *testing.T) {
	t.Parallel()
	pub := newPublisher()
	resp, err := publishServer(t, pub).PublishAgent(inArea("cx", domain.RoleAuthor),
		openapi.PublishAgentRequestObject{AgentId: "triage"})
	if err != nil {
		t.Fatalf("PublishAgent: %v", err)
	}
	if _, ok := resp.(openapi.PublishAgent400ApplicationProblemPlusJSONResponse); !ok || len(pub.published) != 0 {
		t.Fatalf("response = %T (%d published), want 400 and nothing written", resp, len(pub.published))
	}
}

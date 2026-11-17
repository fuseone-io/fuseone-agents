package httpapi

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/spec"
)

/*
The agent's entity-tag, and the writes that honour it (NT-018 1a, NT-017
phase 1).

GET names the current state; publishing, retiring and restoring take If-Match,
and publishing If-None-Match: * for a creation. Both are optional in this
release: a client that sends neither publishes as before. Who may write is
judged where the agent lives now, under its lock, before the precondition.
*/

var triageAtV2 = spec.Spec{ID: "triage", Company: "acme", Area: "cx", Version: "v2"}

func tagOf(retired bool) string { return string(domain.AgentRevision("v2", retired)) }

func TestGetAgent_namesTheCurrentStateInItsETag(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		retired bool
		version *string
		want    *string
	}{
		{name: "live", want: ptr(tagOf(false))},
		{name: "retired", retired: true, want: ptr(tagOf(true))},
		// An older version is not the state the writes judge.
		{name: "a version asked for", version: ptr("v2")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := retirable(t, &retirementSpy{out: map[domain.AgentID]bool{"triage": tc.retired}})
			resp, err := server.GetAgent(inArea("cx", domain.RoleAuthor), openapi.GetAgentRequestObject{
				AgentId: "triage", Params: openapi.GetAgentParams{Version: tc.version},
			})
			if err != nil {
				t.Fatalf("GetAgent: %v", err)
			}
			got, ok := resp.(openapi.GetAgent200JSONResponse)
			if !ok {
				t.Fatalf("response = %T, want the agent", resp)
			}
			if (got.Headers.ETag == nil) != (tc.want == nil) ||
				(tc.want != nil && *got.Headers.ETag != *tc.want) {
				t.Fatalf("ETag = %v, want %v", got.Headers.ETag, tc.want)
			}
		})
	}
}

func publishWith(
	t *testing.T, pub *publisher, ctxArea string, params openapi.PublishAgentParams,
	over func(*openapi.AgentDefinition),
) openapi.PublishAgentResponseObject {
	t.Helper()
	resp, err := publishServer(t, pub).PublishAgent(inArea(ctxArea, domain.RoleAuthor),
		openapi.PublishAgentRequestObject{AgentId: "triage", Params: params, JSONBody: definition(over)})
	if err != nil {
		t.Fatalf("PublishAgent: %v", err)
	}
	return resp
}

func TestPublishAgent_preconditions_areJudgedAgainstTheCurrentTag(t *testing.T) {
	t.Parallel()
	star, stale := "*", `"v1-stale"`
	cases := []struct {
		name     string
		existing bool
		params   openapi.PublishAgentParams
		want     string // "published", "stale"
	}{
		{name: "no header", existing: true, want: "published"},
		{name: "If-Match the current tag", existing: true,
			params: openapi.PublishAgentParams{IfMatch: ptr(tagOf(false))}, want: "published"},
		{name: "If-Match an older tag", existing: true,
			params: openapi.PublishAgentParams{IfMatch: &stale}, want: "stale"},
		{name: "If-None-Match on an agent that exists", existing: true,
			params: openapi.PublishAgentParams{IfNoneMatch: &star}, want: "stale"},
		{name: "If-None-Match on a new agent",
			params: openapi.PublishAgentParams{IfNoneMatch: &star}, want: "published"},
		{name: "If-Match on a new agent",
			params: openapi.PublishAgentParams{IfMatch: &star}, want: "stale"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pub := newPublisher()
			if tc.existing {
				pub = newPublisher(triageAtV2)
			}
			resp := publishWith(t, pub, "cx", tc.params, nil)

			switch tc.want {
			case "published":
				if _, ok := resp.(openapi.PublishAgent200JSONResponse); !ok || len(pub.published) != 1 {
					t.Fatalf("response = %T (%d published), want published", resp, len(pub.published))
				}
			case "stale":
				refused, ok := resp.(openapi.PublishAgent412ApplicationProblemPlusJSONResponse)
				if !ok || len(pub.published) != 0 {
					t.Fatalf("response = %T (%d published), want 412 and nothing written", resp, len(pub.published))
				}
				if tc.existing && (refused.Headers.ETag == nil || *refused.Headers.ETag != tagOf(false)) {
					t.Errorf("ETag = %v, want the current tag %s", refused.Headers.ETag, tagOf(false))
				}
			}
		})
	}
}

// Headers are judged by syntax before anything stored: a 400 that depends
// on nothing an unauthorised caller could learn from.
func TestPublishAgent_preconditionsThatDoNotParse_areRefused(t *testing.T) {
	t.Parallel()
	star, junk, tag := "*", "not-a-tag", `"v1-x"`
	for name, params := range map[string]openapi.PublishAgentParams{
		"both headers":             {IfMatch: &tag, IfNoneMatch: &star},
		"If-Match unparseable":     {IfMatch: &junk},
		"If-None-Match not a star": {IfNoneMatch: &tag},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pub := newPublisher(triageAtV2)
			resp := publishWith(t, pub, "cx", params, nil)
			if _, ok := resp.(openapi.PublishAgent400ApplicationProblemPlusJSONResponse); !ok || len(pub.published) != 0 {
				t.Fatalf("response = %T (%d published), want 400 and nothing written", resp, len(pub.published))
			}
		})
	}
}

/*
Moving an agent needs the scope it leaves (contract 5). An author of ops alone
names ops as the destination of triage, which lives in cx: before, that moved
the agent; now it is the 404 of an agent the caller cannot reach, whatever
precondition it sent, so a stale tag never says triage changed.
*/
func TestPublishAgent_moveOutOfAScopeTheCallerCannotPublish_isNotFound(t *testing.T) {
	t.Parallel()
	stale := `"v1-stale"`
	for name, params := range map[string]openapi.PublishAgentParams{
		"no header": {}, "a stale tag": {IfMatch: &stale},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pub := newPublisher(triageAtV2)
			resp := publishWith(t, pub, "ops", params, func(d *openapi.AgentDefinition) { d.Area = "ops" })
			if _, ok := resp.(openapi.PublishAgent404ApplicationProblemPlusJSONResponse); !ok || len(pub.published) != 0 {
				t.Fatalf("response = %T (%d published), want 404 and nothing written", resp, len(pub.published))
			}
		})
	}
}

func TestSetAgentRetired_preconditions_areJudgedAgainstTheCurrentTag(t *testing.T) {
	t.Parallel()
	stale, junk := `"v1-stale"`, "junk"
	cases := []struct {
		name    string
		retired bool
		ifMatch *string
		ctx     string
		role    domain.Role
		want    string
	}{
		{name: "the current tag", ifMatch: ptr(tagOf(false)), role: domain.RoleAuthor, want: "done"},
		{name: "restoring at the retired tag", retired: true, ifMatch: ptr(tagOf(true)),
			role: domain.RoleAuthor, want: "done"},
		{name: "an older tag", ifMatch: &stale, role: domain.RoleAuthor, want: "stale"},
		{name: "a tag that does not parse", ifMatch: &junk, role: domain.RoleAuthor, want: "bad"},
		{name: "an auditor with a stale tag", ifMatch: &stale, role: domain.RoleAuditor, want: "absent"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			spy := &retirementSpy{out: map[domain.AgentID]bool{"triage": tc.retired}}
			resp, err := retirable(t, spy).SetAgentRetired(inArea("cx", tc.role),
				openapi.SetAgentRetiredRequestObject{
					AgentId: "triage", Params: openapi.SetAgentRetiredParams{IfMatch: tc.ifMatch},
					Body: &openapi.SetAgentRetiredJSONRequestBody{Retired: !tc.retired},
				})
			if err != nil {
				t.Fatalf("SetAgentRetired: %v", err)
			}
			wrote := spy.retired + spy.restored
			var ok bool
			switch tc.want {
			case "done":
				_, ok = resp.(openapi.SetAgentRetired204Response)
				ok = ok && wrote == 1
			case "stale":
				var refused openapi.SetAgentRetired412ApplicationProblemPlusJSONResponse
				refused, ok = resp.(openapi.SetAgentRetired412ApplicationProblemPlusJSONResponse)
				ok = ok && wrote == 0 && refused.Headers.ETag != nil && *refused.Headers.ETag == tagOf(false)
			case "bad":
				_, ok = resp.(openapi.SetAgentRetired400ApplicationProblemPlusJSONResponse)
				ok = ok && wrote == 0
			case "absent":
				_, ok = resp.(openapi.SetAgentRetired404ApplicationProblemPlusJSONResponse)
				ok = ok && wrote == 0
			}
			if !ok {
				t.Fatalf("response = %T (%d written), want %s", resp, wrote, tc.want)
			}
		})
	}
}

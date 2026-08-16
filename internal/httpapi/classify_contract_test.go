package httpapi

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

/*
The door a ruling arrives at, and the two ways it is refused.

The catalogue enforces this properly on its own — a ruling only reaches a tool
whose definition it names. But the catalogue is reached through here, and this
is the surface an old console, a script or a curl actually calls. A rule the
store keeps and the door does not is a rule that holds until somebody stops
using the screen.
*/

// judging is a Curator that records what it was asked to store, over a
// catalogue holding one tool with a known definition.
type judging struct {
	stored domain.ToolClassification
	tools  []domain.ToolEntry
}

func (j *judging) Classify(_ context.Context, _ domain.Scope, r domain.ToolClassification) error {
	j.stored = r
	return nil
}

func (j *judging) List(context.Context, domain.Scope) ([]domain.ToolClassification, error) {
	return nil, nil
}

func (j *judging) Events(context.Context, string, int) ([]domain.AdminEvent, error) {
	return nil, nil
}

func (j *judging) Tools(context.Context) ([]domain.ToolEntry, error) { return j.tools, nil }

func judgingOne(t *testing.T, digest string) (*Server, *judging) {
	t.Helper()
	one := &judging{tools: []domain.ToolEntry{{
		ID: "crm.lookup", Server: "crm", Description: "Look a customer up",
		Digest: digest,
	}}}
	return NewServer(ledger.NewMemory(), "test").WithAdministration(one, one, nil), one
}

func ruling(digest *string) openapi.ClassifyToolRequestObject {
	return openapi.ClassifyToolRequestObject{
		ToolId: "crm.lookup",
		Body:   &openapi.ClassifyToolJSONRequestBody{Effect: "read", Digest: digest},
	}
}

/*
A ruling that names no definition, for a tool whose definition we hold.

An empty digest is how a ruling recorded before any of this existed keeps
working, so it cannot also mean "I did not check". Accepted here, any caller
that omits the field is back to classification by name — which is the model
this replaced, reachable by leaving a field out.
*/
func TestClassifyTool_withNoDigestForAToolWeKnow_isRefused(t *testing.T) {
	t.Parallel()
	server, curator := judgingOne(t, "sha-current")

	resp, err := server.ClassifyTool(as(domain.RoleCurator), ruling(nil))
	if err != nil {
		t.Fatalf("ClassifyTool: %v", err)
	}
	if _, refused := resp.(openapi.ClassifyTool400ApplicationProblemPlusJSONResponse); !refused {
		t.Fatalf("response = %T, want 400", resp)
	}
	if curator.stored.Tool != "" {
		t.Error("the ruling was stored anyway; the check is a message, not a control")
	}
}

// A ruling about the definition that was read is recorded, or the check is an
// outage rather than a control.
func TestClassifyTool_namingTheDefinitionOnOffer_isRecorded(t *testing.T) {
	t.Parallel()
	server, curator := judgingOne(t, "sha-current")

	resp, err := server.ClassifyTool(as(domain.RoleCurator), ruling(ptr("sha-current")))
	if err != nil {
		t.Fatalf("ClassifyTool: %v", err)
	}
	if _, ok := resp.(openapi.ClassifyTool204Response); !ok {
		t.Fatalf("response = %T, want 204", resp)
	}
	if curator.stored.Digest != "sha-current" {
		t.Errorf("stored digest = %q, want the definition that was judged", curator.stored.Digest)
	}
}

/*
A ruling about a definition the server has since replaced.

Refused rather than stored against what is there now: the Curator read a
description and a schema, and recording their judgement over a different one
would put their name on a decision they never made.
*/
func TestClassifyTool_namingADefinitionAlreadyReplaced_isRefused(t *testing.T) {
	t.Parallel()
	server, curator := judgingOne(t, "sha-current")

	resp, err := server.ClassifyTool(as(domain.RoleCurator), ruling(ptr("sha-from-this-morning")))
	if err != nil {
		t.Fatalf("ClassifyTool: %v", err)
	}
	if _, refused := resp.(openapi.ClassifyTool409ApplicationProblemPlusJSONResponse); !refused {
		t.Fatalf("response = %T, want 409", resp)
	}
	if curator.stored.Tool != "" {
		t.Error("stored anyway; somebody's name is now on a judgement of another definition")
	}
}

/*
A tool the catalogue holds without a digest of its own.

Published before this existed and not rediscovered since. Nothing to compare
against is not a mismatch, and refusing here would make an upgrade look like a
platform that had forgotten every tool it has.
*/
func TestClassifyTool_forAToolPublishedBeforeDigests_isRecorded(t *testing.T) {
	t.Parallel()
	server, curator := judgingOne(t, "")

	resp, err := server.ClassifyTool(as(domain.RoleCurator), ruling(nil))
	if err != nil {
		t.Fatalf("ClassifyTool: %v", err)
	}
	if _, ok := resp.(openapi.ClassifyTool204Response); !ok {
		t.Fatalf("response = %T, want 204", resp)
	}
	if curator.stored.Effect != domain.EffectRead {
		t.Errorf("stored = %+v, want the ruling recorded", curator.stored)
	}
}

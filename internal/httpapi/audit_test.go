package httpapi

import (
	"context"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/audit"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

// The audit trail is the one screen whose whole purpose is to show what
// happened, which makes it the most attractive way to see an area you were
// never granted.

type trailSpy struct {
	asked   audit.Filter
	entries []audit.Entry
}

func (t *trailSpy) Read(_ context.Context, filter audit.Filter, _ int) ([]audit.Entry, error) {
	t.asked = filter
	return t.entries, nil
}

func TestListAudit_narrowsToTheScopesTheCallerHolds(t *testing.T) {
	t.Parallel()
	spy := &trailSpy{}

	if _, err := NewServer(ledger.NewMemory(), "test").WithAudit(spy).
		ListAudit(inArea("cx", domain.RoleAuditor), openapi.ListAuditRequestObject{}); err != nil {
		t.Fatalf("ListAudit: %v", err)
	}

	// Never an unscoped read. The store would happily return everything.
	if len(spy.asked.Scopes) == 0 {
		t.Fatal("the trail was read with no scope at all")
	}
	if spy.asked.Scopes[0].Area != "cx" {
		t.Errorf("scopes = %+v, want the caller's own", spy.asked.Scopes)
	}
}

func TestListAudit_askingForAnotherArea_isRefusedNotEmptied(t *testing.T) {
	t.Parallel()

	// An empty page reads as "nothing happened there", which is a different
	// and more dangerous statement than "you cannot see that".
	resp, err := NewServer(ledger.NewMemory(), "test").WithAudit(&trailSpy{}).
		ListAudit(inArea("cx", domain.RoleAuditor), openapi.ListAuditRequestObject{
			Params: openapi.ListAuditParams{Company: ptr("acme"), Area: ptr("marketing")},
		})
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	if _, refused := resp.(openapi.ListAudit403ApplicationProblemPlusJSONResponse); !refused {
		t.Fatalf("response = %T, want 403", resp)
	}
}

func TestListAudit_withoutThePermissionAnywhere_isRefused(t *testing.T) {
	t.Parallel()

	resp, err := NewServer(ledger.NewMemory(), "test").WithAudit(&trailSpy{}).
		ListAudit(context.Background(), openapi.ListAuditRequestObject{})
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	if _, refused := resp.(openapi.ListAudit403ApplicationProblemPlusJSONResponse); !refused {
		t.Fatalf("response = %T, want 403", resp)
	}
}

func TestListAudit_reportsASealOnlyWhereOneExists(t *testing.T) {
	t.Parallel()
	spy := &trailSpy{entries: []audit.Entry{
		{At: time.Now(), Source: audit.SourceLedger, Actor: "triage",
			Verb: "gate.blocked", Target: "crm.reply", Hash: "abc123"},
		{At: time.Now(), Source: audit.SourceAdmin, Actor: "usr_ana",
			Verb: "tool.classified", Target: "crm.reply"},
	}}

	resp, err := NewServer(ledger.NewMemory(), "test").WithAudit(spy).
		ListAudit(inArea("cx", domain.RoleAuditor), openapi.ListAuditRequestObject{})
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	page := resp.(openapi.ListAudit200JSONResponse)

	if page.Items[0].Hash == nil {
		t.Error("a ledger entry reported no seal")
	}
	// Absent rather than empty: an administrative entry is append-only but not
	// chained, and an empty string reads as a seal that failed.
	if page.Items[1].Hash != nil {
		t.Error("an administrative entry reported a seal it cannot have")
	}
}

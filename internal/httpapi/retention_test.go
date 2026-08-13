package httpapi

import (
	gocontext "context"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

// The three endpoints that decide whether data survives. Every assertion here
// is about them being harder to reach than the rest of administration.

type fakeRetention struct {
	window time.Duration
	set    time.Duration
	err    error
}

func (f *fakeRetention) Window(gocontext.Context) (time.Duration, error) {
	return f.window, nil
}

func (f *fakeRetention) SetWindow(
	_ gocontext.Context, _ domain.UserID, _ domain.Scope, window time.Duration,
) error {
	if f.err != nil {
		return f.err
	}
	f.set = window
	return nil
}

type fakeErasures struct {
	runs   []domain.RunID
	reason string
	by     domain.UserID
}

func (f *fakeErasures) ForSubject(
	_ gocontext.Context, by domain.UserID, _ domain.Scope, runs []domain.RunID, reason string,
) (int, error) {
	f.runs, f.reason, f.by = runs, reason, by
	return len(runs), nil
}

func erasing(retention *fakeRetention, erasures *fakeErasures) *Server {
	return NewServer(ledger.NewMemory(), "test").WithRetention(retention, erasures)
}

func TestGetRetention_saysWhenTheDefaultIsInForce(t *testing.T) {
	t.Parallel()

	resp, err := erasing(&fakeRetention{window: admin.DefaultRetention}, &fakeErasures{}).
		GetRetention(as(domain.RoleCurator), openapi.GetRetentionRequestObject{})
	if err != nil {
		t.Fatalf("GetRetention: %v", err)
	}
	got := resp.(openapi.GetRetention200JSONResponse)

	// A default is a promise the installation has not made deliberately, and
	// a screen showing it as one reports a decision nobody took.
	if got.Configured {
		t.Error("the default is reported as configured")
	}
	if got.Days != 5*365 {
		t.Errorf("days = %d", got.Days)
	}
}

func TestSetRetention_shorterThanADay_readsAsABadRequest(t *testing.T) {
	t.Parallel()

	retention := &fakeRetention{err: admin.ErrRetentionTooShort}
	resp, err := erasing(retention, &fakeErasures{}).SetRetention(as(domain.RoleCurator),
		openapi.SetRetentionRequestObject{Body: &openapi.SetRetentionJSONRequestBody{Days: 0}})
	if err != nil {
		t.Fatalf("SetRetention: %v", err)
	}
	// The operator's mistake, and telling them beats a 500 that reads as the
	// platform being broken.
	if _, ok := resp.(openapi.SetRetention400ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want a refusal", resp)
	}
}

func TestEraseContent_recordsWhoAskedAndWhy(t *testing.T) {
	t.Parallel()

	erasures := &fakeErasures{}
	resp, err := erasing(&fakeRetention{window: admin.DefaultRetention}, erasures).
		EraseContent(as(domain.RoleCurator), openapi.EraseContentRequestObject{
			Body: &openapi.EraseContentJSONRequestBody{
				Runs: []string{"run-1", "run-2"}, Reason: "titular pediu",
			},
		})
	if err != nil {
		t.Fatalf("EraseContent: %v", err)
	}
	if got := resp.(openapi.EraseContent200JSONResponse); got.Objects != 2 {
		t.Errorf("objects = %d", got.Objects)
	}
	// An erasure nobody can account for is indistinguishable from data loss.
	if erasures.by == "" || erasures.reason != "titular pediu" {
		t.Errorf("recorded by %q for %q", erasures.by, erasures.reason)
	}
}

func TestErasing_withoutTheEraseAuthority_isRefused(t *testing.T) {
	t.Parallel()

	retention, erasures := &fakeRetention{window: admin.DefaultRetention}, &fakeErasures{}
	server := erasing(retention, erasures)

	// An approver administers approvals and nothing here. Destroying content
	// is the one act in this product nobody can undo, so it does not come
	// with administration in general.
	set, _ := server.SetRetention(as(domain.RoleApprover),
		openapi.SetRetentionRequestObject{Body: &openapi.SetRetentionJSONRequestBody{Days: 1}})
	if _, ok := set.(openapi.SetRetention403ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("set = %T, want it refused", set)
	}

	erase, _ := server.EraseContent(as(domain.RoleApprover),
		openapi.EraseContentRequestObject{Body: &openapi.EraseContentJSONRequestBody{
			Runs: []string{"run-1"}, Reason: "x",
		}})
	if _, ok := erase.(openapi.EraseContent403ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("erase = %T, want it refused", erase)
	}
	if retention.set != 0 || len(erasures.runs) != 0 {
		t.Error("a refused request still reached the store")
	}
}

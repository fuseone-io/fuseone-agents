package spec_test

import (
	"context"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

func withRegistryStanding(t *testing.T) string {
	t.Helper()
	return strings.Replace(definition, "tools:", "standing_approvals: forbidden\ntools:", 1)
}

func TestPublish_standingApprovalsForbidden_survivesTheRegistry(t *testing.T) {
	r := openRegistry(t)
	ctx := context.Background()
	forbidding := published(t, withRegistryStanding(t))

	if err := r.Publish(ctx, forbidding, "usr_ana", "acme"); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	// The worker resolves from here. A column the read forgets is a grant
	// that releases a write the definition said only a person may release.
	got, err := r.Get(ctx, "triage", forbidding.Version)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.StandingApprovals != domain.StandingForbidden {
		t.Errorf("StandingApprovals = %q, want forbidden", got.StandingApprovals)
	}
}

func TestList_standingApprovalsForbidden_isHandedBackToTheEditor(t *testing.T) {
	r := openRegistry(t)
	ctx := context.Background()
	if err := r.Publish(ctx, published(t, withRegistryStanding(t)), "usr_ana", "acme"); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	// What a read omits an editor cannot put back, and the next publish from
	// the console would allow grants again on an edit made for another reason.
	listed, err := r.List(ctx, domain.Scope{}, false)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listed) != 1 || listed[0].StandingApprovals != domain.StandingForbidden {
		t.Fatalf("listed = %+v, want standing approvals forbidden", listed)
	}
	versions, err := r.Versions(ctx, "triage")
	if err != nil {
		t.Fatalf("Versions: %v", err)
	}
	if len(versions) != 1 || versions[0].StandingApprovals != domain.StandingForbidden {
		t.Fatalf("versions = %+v, want standing approvals forbidden", versions)
	}
}

func TestGet_aVersionPublishedWithoutTheLine_allowsStandingApprovals(t *testing.T) {
	r := openRegistry(t)
	ctx := context.Background()
	plain := published(t, definition)
	if err := r.Publish(ctx, plain, "usr_ana", "acme"); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	got, err := r.Get(ctx, "triage", plain.Version)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.StandingApprovals.Forbidden() {
		t.Errorf("StandingApprovals = %q, want allowed", got.StandingApprovals)
	}
}

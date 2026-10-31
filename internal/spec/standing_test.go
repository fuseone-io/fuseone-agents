package spec_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/spec"
)

// withStanding declares a standing-approvals line on the shared valid definition.
func withStanding(line string) string {
	return strings.Replace(valid, "effort: medium\n", "effort: medium\n"+line, 1)
}

func TestParse_standingApprovalsForbidden_reachesTheSpec(t *testing.T) {
	t.Parallel()

	s := parse(t, withStanding("standing_approvals: forbidden\n"))
	if s.StandingApprovals != domain.StandingForbidden {
		t.Errorf("StandingApprovals = %q, want forbidden", s.StandingApprovals)
	}
}

func TestParse_noStandingApprovalsLine_keepsTodaysBehaviour(t *testing.T) {
	t.Parallel()

	if s := parse(t, valid); s.StandingApprovals.Forbidden() {
		t.Errorf("StandingApprovals = %q, want allowed", s.StandingApprovals)
	}
}

func TestParse_standingApprovalsAnyOtherValue_refused(t *testing.T) {
	t.Parallel()

	// "allowed" too: it is the absence of the line, and a second spelling of
	// it would give one definition two digests.
	for _, value := range []string{"allowed", "forbid", "true", "Forbidden"} {
		t.Run(value, func(t *testing.T) {
			t.Parallel()
			_, err := spec.Parse("test.agent.md", []byte(withStanding("standing_approvals: "+value+"\n")))
			if !errors.Is(err, spec.ErrInvalid) || !strings.Contains(err.Error(), "standing_approvals") {
				t.Fatalf("Parse = %v, want a standing_approvals refusal", err)
			}
		})
	}
}

func TestRender_standingApprovalsForbidden_survivesTheRoundTrip(t *testing.T) {
	t.Parallel()

	original := parse(t, withStanding("standing_approvals: forbidden\n"))
	rendered, err := spec.Render(original)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	back, err := spec.Parse("console", rendered)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if back.StandingApprovals != domain.StandingForbidden {
		t.Errorf("round trip StandingApprovals = %q, want forbidden", back.StandingApprovals)
	}
}

func TestRender_standingApprovalsAllowed_writesNoLine(t *testing.T) {
	t.Parallel()

	rendered, err := spec.Render(parse(t, valid))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	// A written-out default would change the digest of every agent published
	// before the setting existed.
	if strings.Contains(string(rendered), "standing_approvals") {
		t.Errorf("rendered = %s, want no standing_approvals line", rendered)
	}
}

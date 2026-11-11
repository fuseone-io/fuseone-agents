package httpapi

import (
	"fmt"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/spec"
)

/*
A publication through the API carries the digest of the bytes it rendered, and
a version id that names other bytes is a conflict the publisher can act on
(NT-018).
*/

func TestPublishAgent_theVersionCarriesTheDigestOfItsRenderedBytes(t *testing.T) {
	t.Parallel()
	pub := newPublisher()
	if _, err := publishServer(t, pub).PublishAgent(inArea("cx", domain.RoleAuthor),
		openapi.PublishAgentRequestObject{AgentId: "triage", Body: definition(nil)}); err != nil {
		t.Fatalf("PublishAgent: %v", err)
	}
	if len(pub.published) != 1 {
		t.Fatalf("published %d versions, want one", len(pub.published))
	}
	got := pub.published[0]
	if len(got.Digest) != 64 || string(got.Version) != "v"+got.Digest[:12] {
		t.Errorf("version %q with digest %q, want the version cut from the digest", got.Version, got.Digest)
	}
}

func TestPublishAgent_aVersionIdThatCannotBeClaimed_answersConflictWithItsCode(t *testing.T) {
	t.Parallel()
	for _, one := range []struct {
		refusal error
		code    Code
	}{
		{spec.ErrVersionCollision, CodeVersionCollision},
		{spec.ErrDigestUnknown, CodeVersionDigestUnknown},
	} {
		t.Run(string(one.code), func(t *testing.T) {
			t.Parallel()
			pub := newPublisher()
			pub.claim = fmt.Errorf("%w: triage@v1", one.refusal)

			resp, err := publishServer(t, pub).PublishAgent(inArea("cx", domain.RoleAuthor),
				openapi.PublishAgentRequestObject{AgentId: "triage", Body: definition(nil)})
			if err != nil {
				t.Fatalf("PublishAgent: %v", err)
			}
			got, ok := resp.(openapi.PublishAgent409ApplicationProblemPlusJSONResponse)
			if !ok || got.Type == nil || *got.Type != string(one.code) {
				t.Fatalf("response = %#v, want 409 %s", resp, one.code)
			}
			if len(pub.ensured) != 0 {
				t.Errorf("ensured %v: a refused publication must leave nothing", pub.ensured)
			}
		})
	}
}

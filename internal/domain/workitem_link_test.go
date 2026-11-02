package domain_test

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

/*
The work-item link, as every reader of the ledger knows it (#204, slice 2a).

Nothing writes a work_item_linked step yet. The kind and its payload ship one
release ahead of the writer, so that a worker still on this release meets a
link a newer worker sealed and reads it rather than failing the run.
*/

const (
	linkRoot  = domain.WorkItemRef("github:fuseone-io/fuseone-agents#204")
	linkAlias = domain.WorkItemRef("github:fuseone-io/fuseone-agents#240")
)

func validLink() domain.WorkItemLinkedPayload {
	return domain.WorkItemLinkedPayload{
		Alias: linkAlias,
		Root:  linkRoot,
		Proof: domain.WorkItemLinkProof{Kind: domain.StepToolReturned, Seq: 12},
	}
}

func TestStepKind_workItemLinked_isInTheClosedSet(t *testing.T) {
	t.Parallel()

	if domain.StepWorkItemLinked != "work_item_linked" {
		t.Fatalf("link kind = %q, want the name the ledger stores", domain.StepWorkItemLinked)
	}
	if !domain.StepWorkItemLinked.Valid() {
		t.Fatalf("%s: Valid = false; a worker on this release would refuse a newer worker's link", domain.StepWorkItemLinked)
	}
	if domain.StepWorkItemLinked.Terminal() {
		t.Fatalf("%s: Terminal = true; a link ends nothing", domain.StepWorkItemLinked)
	}
	if !slices.Contains(domain.StepKinds(), domain.StepWorkItemLinked) {
		t.Fatalf("StepKinds() = %v, want it to list the link", domain.StepKinds())
	}
}

// StepKinds is the closed set itself, so a projection can be held to it.
func TestStepKinds_listsEveryValidKindOnce(t *testing.T) {
	t.Parallel()

	kinds := domain.StepKinds()
	seen := map[domain.StepKind]bool{}
	for _, k := range kinds {
		if !k.Valid() || seen[k] {
			t.Fatalf("StepKinds() = %v: %q is invalid or repeated", kinds, k)
		}
		seen[k] = true
	}
	if !slices.IsSorted(kinds) {
		t.Fatalf("StepKinds() = %v, want a stable order", kinds)
	}
	if len(kinds) != 20 {
		t.Fatalf("StepKinds() lists %d kinds, want the 20 the ledger knows", len(kinds))
	}
}

func TestWorkItemLinkedPayload_valid_accepted(t *testing.T) {
	t.Parallel()

	for name, proof := range map[string]domain.WorkItemLinkProof{
		"proved by the pull request the run created": {Kind: domain.StepToolReturned, Seq: 12},
		"proved by a person who approved the link":   {Kind: domain.StepApprovalDecided, Seq: 9},
	} {
		p := validLink()
		p.Proof = proof
		if err := p.Validate(); err != nil {
			t.Errorf("%s: Validate = %v, want nil", name, err)
		}
	}
}

func TestWorkItemLinkedPayload_broken_refused(t *testing.T) {
	t.Parallel()

	cases := map[string]func(*domain.WorkItemLinkedPayload){
		"missing alias":          func(p *domain.WorkItemLinkedPayload) { p.Alias = "" },
		"missing root":           func(p *domain.WorkItemLinkedPayload) { p.Root = "" },
		"alias is the root":      func(p *domain.WorkItemLinkedPayload) { p.Alias = p.Root },
		"alias in another repo":  func(p *domain.WorkItemLinkedPayload) { p.Alias = "github:fuseone-io/fuseone-site#240" },
		"alias of another owner": func(p *domain.WorkItemLinkedPayload) { p.Alias = "github:someone-else/fuseone-agents#240" },
		"root is a board item":   func(p *domain.WorkItemLinkedPayload) { p.Root = "ghproject:PVTI_lADOabc" },
		"alias is a board item":  func(p *domain.WorkItemLinkedPayload) { p.Alias = "ghproject:PVTI_lADOabc" },
		// The ledger records what it judged: a spelling it would fold to
		// another one is not the reference it judged.
		"alias not canonical":  func(p *domain.WorkItemLinkedPayload) { p.Alias = "github:FuseOne-io/fuseone-agents#240" },
		"root not canonical":   func(p *domain.WorkItemLinkedPayload) { p.Root = " github:fuseone-io/fuseone-agents#204" },
		"alias does not parse": func(p *domain.WorkItemLinkedPayload) { p.Alias = "the pull request I opened" },
		"missing proof seq":    func(p *domain.WorkItemLinkedPayload) { p.Proof.Seq = 0 },
		"negative proof seq":   func(p *domain.WorkItemLinkedPayload) { p.Proof.Seq = -3 },
		"missing proof kind":   func(p *domain.WorkItemLinkedPayload) { p.Proof.Kind = "" },
		"proof of the wrong kind": func(p *domain.WorkItemLinkedPayload) {
			p.Proof.Kind = domain.StepToolCalled
		},
	}
	for name, breakIt := range cases {
		p := validLink()
		breakIt(&p)
		if err := p.Validate(); !errors.Is(err, domain.ErrWorkItemLinkInvalid) {
			t.Errorf("%s: Validate = %v, want ErrWorkItemLinkInvalid", name, err)
		}
	}
}

// The wire names are what the ledger hashes and every reader decodes: they
// are fixed the day the first link is sealed.
func TestWorkItemLinkedPayload_wireShape(t *testing.T) {
	t.Parallel()

	got, err := json.Marshal(validLink())
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	want := `{"alias":"github:fuseone-io/fuseone-agents#240","root":"github:fuseone-io/fuseone-agents#204",` +
		`"proof":{"kind":"tool_returned","seq":12}}`
	if string(got) != want {
		t.Fatalf("payload = %s\nwant      %s", got, want)
	}
}

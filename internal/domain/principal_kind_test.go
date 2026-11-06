package domain_test

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

// A role says what a job may do. Some things are only ever a person's, whatever the
// role says: releasing an effect the Gate held, and deciding who else may act. A
// token that could do those would turn one leaked secret into the human check it was
// supposed to stop at.
func TestPrincipal_onlyAPersonHoldsWhatMustBeAPersons(t *testing.T) {
	t.Parallel()

	grant := domain.Grant{Scope: domain.Scope{Company: domain.Installation}, Role: domain.RoleAdmin}
	scope := domain.Scope{Company: "acme", Area: "cx"}
	personal := []domain.Permission{
		domain.PermApprovalAct, domain.PermApprovalGrant, domain.PermIdentityWrite,
	}

	for _, kind := range []domain.PrincipalKind{domain.PrincipalService, domain.PrincipalAgent} {
		machine := domain.Principal{Kind: kind, Grants: []domain.Grant{grant}}
		for _, perm := range personal {
			if machine.Can(perm, scope) {
				t.Errorf("%s: Can(%s)", kind, perm)
			}
			if machine.CanAnywhere(perm) {
				t.Errorf("%s: CanAnywhere(%s)", kind, perm)
			}
			if got := machine.ScopesFor(perm); len(got) != 0 {
				t.Errorf("%s: ScopesFor(%s) = %v", kind, perm, got)
			}
		}
		// The role still means everything else, or a token could do nothing.
		if !machine.Can(domain.PermAgentPublish, scope) {
			t.Errorf("%s: lost %s", kind, domain.PermAgentPublish)
		}
	}

	person := domain.Principal{Kind: domain.PrincipalUser, Grants: []domain.Grant{grant}}
	for _, perm := range personal {
		if !person.Can(perm, scope) {
			t.Errorf("a person lost %s", perm)
		}
	}
}

package auth_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
)

func scope(company, area string) domain.Scope {
	return domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}
}

func person(kind domain.PrincipalKind, grants ...domain.Grant) domain.Principal {
	return domain.Principal{ID: "u1", Kind: kind, Grants: grants}
}

// --- the authorisation model -------------------------------------------------

func TestRoles_curatorIsTheOnlyOneThatCanWidenWhatAgentsMayDo(t *testing.T) {
	t.Parallel()

	// Classifying a tool's effect and writing a capability pack are the two
	// acts that grant agents new reach. Holding them anywhere else would put
	// the guardrail back in the hands of the person it constrains.
	for _, perm := range []domain.Permission{
		domain.PermToolClassify, domain.PermPackWrite,
		domain.PermBudgetWrite, domain.PermPolicyWrite,
		domain.PermProviderWrite, domain.PermIdentityWrite,
	} {
		for _, role := range domain.Roles() {
			allowed := role.Allows(perm)
			if role == domain.RoleCurator && !allowed {
				t.Errorf("curator cannot %s", perm)
			}
			if role != domain.RoleCurator && allowed {
				t.Errorf("%s can %s — only the curator should", role, perm)
			}
		}
	}
}

func TestRoles_auditorCanReadEverythingAndChangeNothing(t *testing.T) {
	t.Parallel()

	if !domain.RoleAuditor.Allows(domain.PermAuditExport) {
		t.Error("auditor cannot export the trail")
	}
	// An auditor who can alter what they audit is not an auditor.
	for _, perm := range []domain.Permission{
		domain.PermRunTrigger, domain.PermRunCancel, domain.PermAgentPublish,
		domain.PermApprovalAct, domain.PermToolClassify,
	} {
		if domain.RoleAuditor.Allows(perm) {
			t.Errorf("auditor can %s", perm)
		}
	}
}

func TestRoles_authorNeverTouchesAGuardrail(t *testing.T) {
	t.Parallel()

	// The whole premise of open authoring: the domain author describes the
	// process and owns the outcome; the envelope is drawn by someone else.
	for _, perm := range []domain.Permission{
		domain.PermToolClassify, domain.PermPackWrite, domain.PermBudgetWrite,
		domain.PermPolicyWrite, domain.PermScopeWrite,
	} {
		if domain.RoleAuthor.Allows(perm) {
			t.Errorf("author can %s", perm)
		}
	}
}

func TestCan_grantInOneAreaDoesNotReachAnother(t *testing.T) {
	t.Parallel()

	p := person(domain.PrincipalUser, domain.Grant{Scope: scope("acme", "cx"), Role: domain.RoleCurator})

	if !p.Can(domain.PermToolClassify, scope("acme", "cx")) {
		t.Error("the granted scope was refused")
	}
	if p.Can(domain.PermToolClassify, scope("acme", "marketing")) {
		t.Error("a grant leaked into a neighbouring area")
	}
	// This is the check the company boundary rests on once a group runs more
	// than one of them.
	if p.Can(domain.PermToolClassify, scope("outra", "cx")) {
		t.Error("a grant leaked across the company boundary")
	}
}

func TestScopesFor_listsWhereAListingShouldLook(t *testing.T) {
	t.Parallel()

	p := person(domain.PrincipalUser,
		domain.Grant{Scope: scope("acme", "cx"), Role: domain.RoleAuthor},
		domain.Grant{Scope: scope("acme", "marketing"), Role: domain.RoleApprover},
	)

	// Approving is granted in one area only; a listing filtered by this reads
	// one area at the query rather than reading both and discarding.
	got := p.ScopesFor(domain.PermApprovalAct)
	if len(got) != 1 || got[0] != scope("acme", "marketing") {
		t.Errorf("ScopesFor = %v, want only acme/marketing", got)
	}
}

func TestDelegate_agentNeverWidensTheHumansReach(t *testing.T) {
	t.Parallel()

	human := person(domain.PrincipalUser,
		domain.Grant{Scope: scope("acme", "cx"), Role: domain.RoleAuthor},
	)
	// The agent's envelope is broader than the person who triggered it.
	envelope := []domain.Grant{
		{Scope: scope("acme", "cx"), Role: domain.RoleAuthor},
		{Scope: scope("acme", "financeiro"), Role: domain.RoleCurator},
	}

	agent := domain.Delegate(human, "triage", envelope)

	// The effective reach is the intersection: an agent acting for someone
	// must not do what that person could not (PRD AU-06).
	if !agent.Can(domain.PermRunTrigger, scope("acme", "cx")) {
		t.Error("the delegated agent lost a grant both sides held")
	}
	if agent.Can(domain.PermToolClassify, scope("acme", "financeiro")) {
		t.Error("the agent kept reach its delegator never had")
	}
	// The trail must always be able to name the pair.
	if agent.OnBehalfOf != human.ID || agent.Kind != domain.PrincipalAgent {
		t.Errorf("delegated principal = %+v, want an agent acting for %s", agent, human.ID)
	}
}

// --- the request boundary ----------------------------------------------------

type directory struct {
	principal domain.Principal
	err       error
}

func (d directory) PrincipalBySession(context.Context, []byte, time.Time) (domain.Principal, auth.Session, error) {
	return d.principal, auth.Session{}, d.err
}

func (d directory) PrincipalByToken(context.Context, []byte, time.Time) (domain.Principal, error) {
	return d.principal, d.err
}

func serve(t *testing.T, dir directory, r *http.Request) *httptest.ResponseRecorder {
	t.Helper()

	handler := auth.NewAuthenticator(dir, false, nil).Middleware(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }),
	)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, r)
	return rec
}

func TestMiddleware_noCredential_isRejectedWithTheScheme(t *testing.T) {
	t.Parallel()

	rec := serve(t, directory{}, httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	// A bare 401 leaves a client guessing which scheme to use.
	if rec.Header().Get("WWW-Authenticate") == "" {
		t.Error("no WWW-Authenticate header on a 401")
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type = %q, want the contract's error shape", ct)
	}
}

func TestMiddleware_expiredSession_saysExpiredNotUnknown(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: "stale"})

	rec := serve(t, directory{err: auth.ErrExpired}, req)

	// "Signed out" and "never signed in" lead the console to different
	// screens; collapsing them makes a lapsed session look like a bug.
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	if body := rec.Body.String(); !contains(body, "expired") {
		t.Errorf("body = %s, want it to say the session expired", body)
	}
}

func TestMiddleware_cookieWrite_requiresTheCSRFEcho(t *testing.T) {
	t.Parallel()

	dir := directory{principal: person(domain.PrincipalUser)}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/runs/r1/approvals", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: "live"})
	req.AddCookie(&http.Cookie{Name: auth.CSRFCookieName, Value: "abc123"})

	// A cross-site page can make the browser send the cookies but cannot read
	// them to set the header. Without the echo, someone else's page could
	// approve a payment on the operator's behalf.
	if rec := serve(t, dir, req); rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 without the CSRF header", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/runs/r1/approvals", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: "live"})
	req.AddCookie(&http.Cookie{Name: auth.CSRFCookieName, Value: "abc123"})
	req.Header.Set(auth.CSRFHeader, "abc123")

	if rec := serve(t, dir, req); rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 with a matching echo", rec.Code)
	}
}

func TestMiddleware_cookieRead_needsNoCSRFEcho(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: "live"})

	rec := serve(t, directory{principal: person(domain.PrincipalUser)}, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 — a read is not a state change", rec.Code)
	}
}

func TestMiddleware_bearerWrite_isExemptFromCSRF(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/runs", nil)
	req.Header.Set("Authorization", "Bearer service-token")

	// Nothing attaches an Authorization header automatically, so there is no
	// confused deputy for CSRF to protect against.
	rec := serve(t, directory{principal: person(domain.PrincipalService)}, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for a bearer caller", rec.Code)
	}
}

func TestRequire_missingGrant_isForbiddenNotNotFound(t *testing.T) {
	t.Parallel()

	ctx := auth.WithPrincipal(context.Background(),
		person(domain.PrincipalUser, domain.Grant{Scope: scope("acme", "cx"), Role: domain.RoleAuthor}))

	if err := auth.Require(ctx, domain.PermRunRead, scope("acme", "cx")); err != nil {
		t.Errorf("Require refused a granted action: %v", err)
	}
	if err := auth.Require(ctx, domain.PermToolClassify, scope("acme", "cx")); !errors.Is(err, auth.ErrForbidden) {
		t.Errorf("Require = %v, want %v", err, auth.ErrForbidden)
	}
}

func TestRequire_noPrincipal_failsClosed(t *testing.T) {
	t.Parallel()

	// A handler reached without authentication must not operate as a
	// zero-valued principal that merely happens to hold no grants.
	err := auth.Require(context.Background(), domain.PermRunRead, scope("acme", "cx"))
	if !errors.Is(err, auth.ErrNoCredential) {
		t.Errorf("Require = %v, want %v", err, auth.ErrNoCredential)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle ||
		len(needle) == 0 || indexOf(haystack, needle) >= 0)
}

func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}

package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
)

/*
Writing an HTTP reply by hand.

These are for the handlers that predate the generated server — sign-in, the
installation gate — and they exist so those replies are shaped like every
other one: a problem document with a code a client can branch on, never a
sentence.
*/
// Me describes the signed-in caller to the console.
type Me struct {
	ID      string    `json:"id"`
	Display string    `json:"display"`
	Kind    string    `json:"kind"`
	Grants  []MeGrant `json:"grants"`
	Can     []string  `json:"can"`
	// Platform is what the caller holds at the platform's administration
	// scope. A permission checked in two places, such as tool:read for the
	// catalogue and for the integrations themselves, is in Can for the first
	// and only here for the second.
	Platform []string `json:"platform"`
	// Configure is where the caller holds provider:write: the scopes a form
	// for a connector instance may offer (NT-019). Held at the platform's
	// administration, it governs every scope, which platform already says.
	Configure []MeScope `json:"configure"`
	// Classify is where the caller holds tool:classify: an owner's ruling is
	// for a tool whose server is stored at a scope one of these contains
	// (NT-019). Held at the platform's administration, it governs every
	// scope, which platform already says.
	Classify []MeScope `json:"classify"`
}

type MeScope struct {
	Company string `json:"company"`
	Area    string `json:"area"`
}

type MeGrant struct {
	Company string `json:"company"`
	Area    string `json:"area"`
	Role    string `json:"role"`
}

// meFrom renders the caller, including the permissions they hold where the
// console can actually use them.
//
// The console uses `can` to decide which navigation to show. It is a hint for
// the interface, never the enforcement — every request is checked again on the
// server, where the scope of the specific resource is known.
func meFrom(p domain.Principal) Me {
	out := Me{
		ID: string(p.ID), Display: p.Display, Kind: string(p.Kind),
		Grants: []MeGrant{}, Can: []string{}, Platform: []string{}, Configure: []MeScope{}, Classify: []MeScope{},
	}
	for _, scope := range p.ScopesFor(permConfigure) {
		out.Configure = append(out.Configure, MeScope{Company: string(scope.Company), Area: string(scope.Area)})
	}
	for _, scope := range p.ScopesFor(domain.PermToolClassify) {
		out.Classify = append(out.Classify, MeScope{Company: string(scope.Company), Area: string(scope.Area)})
	}
	seen := map[domain.Permission]struct{}{}

	for _, g := range p.Grants {
		out.Grants = append(out.Grants, MeGrant{
			Company: string(g.Scope.Company), Area: string(g.Scope.Area), Role: string(g.Role),
		})
		for _, perm := range g.Role.Permissions() {
			if _, dup := seen[perm]; dup {
				continue
			}
			if !canShowInConsole(p, perm) {
				continue
			}
			seen[perm] = struct{}{}
			out.Can = append(out.Can, string(perm))
		}
	}
	for _, perm := range out.Can {
		if p.Can(domain.Permission(perm), adminScope) {
			out.Platform = append(out.Platform, perm)
		}
	}
	return out
}

/*
platformOnly are the permissions the server checks only at the platform's
administration scope (adminScope), whatever the caller holds elsewhere.

Advertised from any area, they put on an area administrator's screen menus and
buttons that answer 403 on every click. A permission that moves to a resource's
own scope leaves this list in the same change.
*/
var platformOnly = map[domain.Permission]bool{
	// provider:write left with NT-019 phase 1: connector instances are
	// configured at their own scope. The console reads MCP servers, providers
	// and channels, still the platform's, from Me.Platform. tool:classify
	// leaves with the console that rules per tool (NT-019, B2b): until then
	// the console's tools screen lists unscoped and rules at the platform's
	// level, which an area curator would meet as 403 on every click.
	domain.PermToolClassify: true,
	domain.PermBudgetWrite:  true, domain.PermPolicyWrite: true,
	domain.PermApprovalGrant: true, domain.PermAuditExport: true,
	domain.PermBrandWrite: true, domain.PermDataErase: true,
}

func canShowInConsole(p domain.Principal, perm domain.Permission) bool {
	switch {
	case perm == domain.PermCompanyWrite:
		return p.Can(perm, domain.Scope{Company: domain.Installation})
	case perm == domain.PermIdentityWrite:
		return p.Can(perm, identityScope)
	case platformOnly[perm]:
		return p.Can(perm, adminScope)
	default:
		return p.CanAnywhere(perm)
	}
}

// MeHandler answers "who am I", for the console's first request after load.
func MeHandler(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFrom(r.Context())
	if !ok {
		writeProblemJSON(w, http.StatusUnauthorized, CodeNotSignedIn, "Not signed in", "no session")
		return
	}
	writeJSON(w, http.StatusOK, meFrom(principal))
}

// clientIP prefers the proxy's forwarded address, which is what an operator
// sees in a session list when checking whether a sign-in was theirs.
func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		if first, _, found := strings.Cut(fwd, ","); found {
			return strings.TrimSpace(first)
		}
		return strings.TrimSpace(fwd)
	}
	host, _, found := strings.Cut(r.RemoteAddr, ":")
	if !found {
		return r.RemoteAddr
	}
	return host
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

/*
writeProblemJSON is the refusal written by the routes that are not generated
from the contract: signing in, setting up, and receiving a webhook.

It carries a code like every other refusal, so the console translates rather
than rendering whatever English the server holds. The webhook refusals are the
one place where the prose is the point: their reader is an integrator looking
at a raw response, and the detail tells them what header to send.
*/
func writeProblemJSON(w http.ResponseWriter, status int, code Code, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	body := map[string]any{"title": title, "status": status, "detail": detail}
	if code != "" {
		body["type"] = string(code)
	}
	_ = json.NewEncoder(w).Encode(body)
}

// OpenInstallation answers for a server running with no identity at all.
//
// It says so plainly rather than pretending: the console renders, and the
// process log has already warned that every caller has full access.
func OpenInstallation(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"providers":        []any{},
		"bootstrapPending": false,
		"authRequired":     false,
	})
}

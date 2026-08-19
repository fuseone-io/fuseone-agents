package httpapi

import (
	"fmt"
	"net/http"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

/*
Refusals, named by a code rather than by a sentence.

The server said "Classificação inválida" to one caller and "Invalid stop" to
the next: half the refusals were in Portuguese, half in English, and none of
them went through i18n. A console in either language showed whichever the
server happened to hold, and a client that was not the console had nothing to
branch on but prose.

So the server names the condition and the reader chooses the words. `type`
carries a stable code, `detail` carries the particulars — an identifier, a
permission, a scope — and `title` is an English fallback for a client that
knows no codes. The alternative was negotiating on Accept-Language, which puts
two copies of every sentence in a Go binary and still leaves a machine client
parsing them.
*/

// Code is a stable name for a refusal. It appears in problem+json and in the
// console's catalogue, and it never changes once shipped.
type Code string

const (
	CodeNotFound        Code = "fuseone:not-found"
	CodeForbidden       Code = "fuseone:forbidden"
	CodeInvalidInput    Code = "fuseone:invalid-input"
	CodeNotStored       Code = "fuseone:not-stored"
	CodeConflict        Code = "fuseone:conflict"
	CodeUpstreamRefused Code = "fuseone:upstream-refused"
	CodeUpstreamBusy    Code = "fuseone:upstream-busy"
	CodeUnavailable     Code = "fuseone:unavailable"
	// CodeSavedNotReachable is its own condition rather than a failure: the
	// configuration was stored and the far side did not answer. Saying only
	// "could not be reached" reads as nothing having been saved, beside a row
	// that just appeared on the screen.
	CodeSavedNotReachable Code = "fuseone:saved-not-reachable"
	// CodeNotSimulated is the one refusal the console answers with a route
	// rather than a sentence: an agent leaves Draft by being simulated, and
	// the useful reply is the way to do that.
	CodeNotSimulated Code = "fuseone:not-simulated"
	// The two the sign-in flow answers with. Separate from the generic
	// refusals because the console does something with them rather than
	// showing them: one offers the sign-in button again, the other is what
	// the session gate waits for before rendering anything at all.
	CodeSignInFailed Code = "fuseone:sign-in-failed"
	CodeNotSignedIn  Code = "fuseone:not-signed-in"
)

/*
refusal builds a problem body around a code.

The title is deliberately terse English and deliberately not a message: it
exists so a caller reading a raw response is not left guessing, and anything
that renders it to a person is doing the wrong thing.
*/
func refusal(status int, code Code, title, detail string) openapi.Problem {
	return openapi.Problem{
		Type: ptr(string(code)), Title: title, Status: status, Detail: ptr(detail),
	}
}

// notFound builds the shared problem body every 404 in the contract reuses.
//
// The identifier is the detail rather than part of a sentence: a console that
// wants to say "no run with id X" builds that around the code, and one that
// wants to say something else can.
func notFound(id string) openapi.NotFoundApplicationProblemPlusJSONResponse {
	return openapi.NotFoundApplicationProblemPlusJSONResponse(
		refusal(http.StatusNotFound, CodeNotFound, "Not found", id))
}

// forbidden names the permission and the scope, which is what somebody refused
// needs in order to ask the right person for it.
func forbidden(perm domain.Permission, scope domain.Scope) openapi.ForbiddenApplicationProblemPlusJSONResponse {
	return openapi.ForbiddenApplicationProblemPlusJSONResponse(
		refusal(http.StatusForbidden, CodeForbidden, "Forbidden",
			fmt.Sprintf("%s in %s", perm, scope)))
}

// invalid is a request the server understood and will not accept.
func invalid(detail string) openapi.Problem {
	return refusal(http.StatusBadRequest, CodeInvalidInput, "Invalid request", detail)
}

// notStored is a write the server accepted and could not complete.
func notStored(detail string) openapi.Problem {
	return refusal(http.StatusBadRequest, CodeNotStored, "Not stored", detail)
}

// conflicted is a request that contradicts the state it arrived at.
func conflicted(detail string) openapi.Problem {
	return refusal(http.StatusConflict, CodeConflict, "Conflict", detail)
}

// upstreamRefused is something outside this installation saying no.
func upstreamRefused(detail string) openapi.Problem {
	return refusal(http.StatusBadRequest, CodeUpstreamRefused, "Upstream refused", detail)
}

// upstreamRefusedLater is not a refusal of this request. The provider is
// overloaded, rate-limiting or unreachable, so a person should hear "try
// later" and not "the other side said no".
func upstreamRefusedLater(detail string) openapi.Problem {
	return refusal(http.StatusServiceUnavailable, CodeUpstreamBusy, "Upstream busy", detail)
}

// savedNotReachable is a configuration stored against something that did not
// answer. Both halves, because either alone is misleading.
func savedNotReachable(detail string) openapi.Problem {
	return refusal(http.StatusBadRequest, CodeSavedNotReachable,
		"Saved, but not reachable", detail)
}

// unavailable is something the installation depends on not answering.
func unavailable(detail string) openapi.Problem {
	return refusal(http.StatusBadGateway, CodeUnavailable, "Unavailable", detail)
}

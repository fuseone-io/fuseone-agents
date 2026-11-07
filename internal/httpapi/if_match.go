package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// staleFrom shapes a write its If-Match refused, with what the resource is now.
func staleFrom(err error) (openapi.PreconditionFailedApplicationProblemPlusJSONResponse, bool) {
	var stale *domain.StaleError
	if !errors.As(err, &stale) {
		return openapi.PreconditionFailedApplicationProblemPlusJSONResponse{}, false
	}
	out := openapi.PreconditionFailedApplicationProblemPlusJSONResponse{
		Body: refusal(http.StatusPreconditionFailed, CodePreconditionFailed, "Precondition failed", stale.Error()),
	}
	if stale.Current != "" {
		out.Headers.ETag = ptr(string(stale.Current))
	}
	return out, true
}

// FoldIfMatch joins If-Match lines into one, which HTTP says mean the same. The generated
// routes bind a header to a single value and refuse more, and they do that before any
// middleware of theirs runs, so this has to stand in front of them.
func FoldIfMatch(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if lines := r.Header.Values("If-Match"); len(lines) > 1 {
			r.Header.Set("If-Match", strings.Join(lines, ", "))
		}
		next.ServeHTTP(w, r)
	})
}

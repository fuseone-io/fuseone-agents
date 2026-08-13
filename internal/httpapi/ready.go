package httpapi

import (
	"context"
	"net/http"

	"github.com/fuseone/agents/internal/httpapi/openapi"
)

/*
Ready reports whether this process can serve, which is not the same question as
whether it is alive.

Liveness touches nothing outside the process: a probe that asked the database
would restart every pod during a database blip, turning a recoverable outage
into a crash loop across the installation. Readiness does ask, because a
process that cannot read the ledger has nothing to answer with — it should
leave the rotation and it should not be restarted for it.

The reason travels with the refusal so `kubectl describe` shows what is wrong
without anybody opening a shell.
*/
func (s *Server) Ready(ctx context.Context, _ openapi.ReadyRequestObject) (openapi.ReadyResponseObject, error) {
	// The cheapest question that proves the connection works and the schema is
	// there. Reading a run would be a better proof and a worse probe: an
	// installation with no runs yet is ready, and one with millions should not
	// pay for a scan every few seconds.
	if _, err := s.store.Runs(ctx); err != nil {
		return openapi.Ready503ApplicationProblemPlusJSONResponse(
			refusal(http.StatusServiceUnavailable, CodeUnavailable,
				"Not ready", err.Error())), nil
	}
	return openapi.Ready200JSONResponse{Status: openapi.Ok, Version: s.version}, nil
}

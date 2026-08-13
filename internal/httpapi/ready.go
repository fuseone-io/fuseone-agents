package httpapi

import (
	"context"
	"log/slog"
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

What the refusal says is deliberately thin. This is the one endpoint that
answers without a credential — the kubelet holds none — so anybody who can
reach the Service can read it, and the driver's own error carries the string it
dialled: user, database, host and port. The refusal names what is unavailable;
the log, which needs access to the cluster to read and is where an operator is
already looking, carries why.
*/
func (s *Server) Ready(ctx context.Context, _ openapi.ReadyRequestObject) (openapi.ReadyResponseObject, error) {
	// The cheapest question that proves the connection works and the schema is
	// there. Reading a run would be a better proof and a worse probe: an
	// installation with no runs yet is ready, and one with millions should not
	// pay for a scan every few seconds.
	if _, err := s.store.Runs(ctx); err != nil {
		slog.ErrorContext(ctx, "not ready", "err", err)
		return openapi.Ready503ApplicationProblemPlusJSONResponse(
			refusal(http.StatusServiceUnavailable, CodeUnavailable,
				"Not ready", "the ledger is not reachable")), nil
	}
	return openapi.Ready200JSONResponse{Status: openapi.Ok, Version: s.version}, nil
}

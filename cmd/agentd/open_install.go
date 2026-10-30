package main

import (
	"log/slog"
	"net/http"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// apiHandlerFor routes the OpenAPI contract to its implementation.
func apiHandlerFor(api *httpapi.Server) http.Handler {
	return openapi.HandlerWithOptions(
		openapi.NewStrictHandlerWithOptions(api, nil, openapi.StrictHTTPServerOptions{
			RequestErrorHandlerFunc: requestProblem,
			ResponseErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
				writeProblem(w, http.StatusInternalServerError, "Internal error", err.Error())
			},
		}),
		openapi.StdHTTPServerOptions{
			BaseURL:    "/api/v1",
			BaseRouter: http.NewServeMux(),
			ErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
				writeProblem(w, http.StatusBadRequest, "Invalid request", err.Error())
			},
		},
	)
}

// mountOpen serves an installation that has no database, and so nobody to
// sign in as. Reached only when there is no identity: an installation with a
// database always authenticates, and nothing here is mounted beside that.
func mountOpen(root *http.ServeMux, apiHandler http.Handler) {
	slog.Warn("running without authentication; every caller has full access")
	root.Handle("/api/", apiProblems(openAccess(apiHandler)))
	// The console asks who it should let in before it renders anything.
	// Without this it gets the SPA's own index.html back, fails to read it
	// as JSON, and shows an error instead of the console — an installation
	// with nothing to protect would look broken rather than open.
	root.Handle("GET /auth/providers", http.HandlerFunc(httpapi.OpenInstallation))
}

/*
openAccess acts every request as the open installation's one principal.

The handlers authorise each request against the principal in its context and
fail closed without one, which is right, so an installation with nobody to
sign in as served nothing: the warning above said "full access" while every
route answered 403 or 404. Granting it here, explicitly, keeps the handlers'
rule intact rather than teaching auth.Require an exception (#77).
*/
func openAccess(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), openPrincipal())))
	})
}

// openPrincipal is who acts on an installation without a database: an
// administrator of the whole installation, named so the trail says it plainly.
// A function rather than a variable: a principal carries a slice, and a
// shared one is mutable package state.
func openPrincipal() domain.Principal {
	return domain.Principal{
		ID:      "open-installation",
		Subject: "open-installation",
		Display: "Open installation (no database)",
		Kind:    domain.PrincipalService,
		Grants: []domain.Grant{{
			Scope: domain.Scope{Company: domain.Installation},
			Role:  domain.RoleAdmin,
		}},
	}
}

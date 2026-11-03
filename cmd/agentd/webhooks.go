package main

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/trigger"
	"github.com/fuseone/agents/internal/vault"
)

// webhooksWith is the webhook store, able to arm GitHub hooks when the master
// key is set.
//
// The nil check is the point: a nil *vault.Vault handed to WithVault would be
// a non-nil interface, and the first GitHub rotation would dereference it
// instead of being refused with a sentence.
func webhooksWith(pool *pgxpool.Pool, v *vault.Vault) *trigger.PostgresWebhooks {
	hooks := trigger.NewPostgresWebhooks(pool)
	if v != nil {
		hooks.WithVault(v)
	}
	return hooks
}

// webhookDoor is the /hooks door serve mounts, built once here so a journey
// test knocks on the same one.
func webhookDoor(pool *pgxpool.Pool, store Store, v *vault.Vault) *httpapi.Hooks {
	return httpapi.NewHooks(
		webhooksWith(pool, v),
		trigger.NewOpener(store, spec.NewRegistry(pool), engine.SystemClock{}).
			WithContent(ledger.NewContent(pool)).
			WithPauses(spec.NewState(pool)).
			// A webhook is a way a run starts, so the switches reach it
			// too. One honoured by the console and not by an inbound hook
			// is a stop that quietens the half nobody is watching.
			WithStops(admin.NewStops(pool)).
			WithStages(spec.NewState(pool)),
		slog.Default(),
	)
}

package main

import (
	"github.com/jackc/pgx/v5/pgxpool"

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

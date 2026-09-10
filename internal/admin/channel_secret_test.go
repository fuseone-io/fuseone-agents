package admin_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

/*
Changing how a connection is reached removes the secret the new way cannot use.

A signing secret on a Socket Mode connection is a credential nothing verifies
with, and an app token on an HTTP one is a credential nothing connects with.
Kept, it is a secret nobody is accounting for that comes back into force the day
somebody switches the mode again — and the trail, which records only which
credentials are now held, says it is gone.

Dropping it from the merged pair was not enough: an omitted secret means "keep
what is stored", which is what lets somebody change an unrelated field without
pasting a token back in. Emptied by the mode, the write has to say so.
*/
func TestPutChannel_changingDeliveryMode_removesTheSecretItCannotUse(t *testing.T) {
	for _, one := range []struct {
		name  string
		from  string
		to    string
		creds channel.Credentials
	}{
		{"signing survives a move to socket", channel.DeliveryHTTP, channel.DeliverySocket,
			channel.Credentials{Signing: "signing-must-go"}},
		{"an app token survives a move to http", channel.DeliverySocket, channel.DeliveryHTTP,
			channel.Credentials{AppToken: "xapp-must-go"}},
	} {
		t.Run(one.name, func(t *testing.T) {
			pool := freshPool(t)
			store := settings.NewStore(pool, testVault(t))
			channels := admin.NewChannels(pool, store, onlySlack{})
			ctx := context.Background()

			if err := channels.PutChannel(ctx, admin.ChannelWrite{
				Channel: admin.Channel{
					Name: "acme-slack", Kind: "slack",
					DeliveryMode: one.from, Enabled: true,
				},
				Credentials: one.creds, By: "usr_ana", Governs: true,
			}); err != nil {
				t.Fatalf("configure the connection: %v", err)
			}

			// Nothing supplied: the switch is the whole request.
			if err := channels.PutChannel(ctx, admin.ChannelWrite{
				Channel: admin.Channel{
					Name: "acme-slack", Kind: "slack",
					DeliveryMode: one.to, Enabled: true,
				},
				By: "usr_ana", Governs: true,
			}); err != nil {
				t.Fatalf("change the delivery mode: %v", err)
			}

			held, err := store.Reveal(ctx, settings.ScopeInstallation, domain.Scope{},
				channel.KindChannel, "acme-slack")
			if err != nil {
				t.Fatalf("read the credentials back: %v", err)
			}
			if got := channel.ReadCredentials(held.Secret); got != (channel.Credentials{}) {
				t.Fatalf("credentials = %+v, want the incompatible one gone", got)
			}
			// And the listing agrees.
			if listed := oneChannel(t, channels, "acme-slack"); listed.HasSigning || listed.HasAppToken {
				t.Errorf("listing still reports an inbound secret: %+v", listed)
			}
			// So does the trail, which is the record an auditor reads and the
			// only place that says what a change did to the credentials. It
			// saying "none held" while one was sealed is the half of this
			// defect that survives the fix to the other half.
			recorded := lastConfigured(t, pool, "acme-slack")
			for _, name := range []string{"signing", "appToken"} {
				if held, ok := recorded[name].(bool); !ok || held {
					t.Errorf("trail says %s = %v, want it recorded as gone", name, recorded[name])
				}
			}
		})
	}
}

// lastConfigured decodes what the most recent "channel.configured" event
// recorded. Decoded rather than matched as a substring: the column is jsonb and
// the database chooses its own spacing, so a string comparison would be
// asserting Postgres's formatting instead of the fact.
func lastConfigured(t *testing.T, pool *pgxpool.Pool, target string) map[string]any {
	t.Helper()
	action, detail := lastTrailDetail(t, pool, target)
	if action != "channel.configured" {
		t.Fatalf("last action = %q, want the connection being configured", action)
	}
	var recorded map[string]any
	if err := json.Unmarshal([]byte(detail), &recorded); err != nil {
		t.Fatalf("decode the trail detail: %v", err)
	}
	return recorded
}

func oneChannel(t *testing.T, channels *admin.Channels, name string) admin.Channel {
	t.Helper()
	listed, err := channels.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, one := range listed {
		if one.Name == name {
			return one
		}
	}
	t.Fatalf("no connection %s", name)
	return admin.Channel{}
}

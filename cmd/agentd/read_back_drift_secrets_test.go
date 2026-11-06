package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/settings"
)

// A provider's key and a channel's secrets are all secret, so the fingerprint is
// the only thing that may move when one changes, and it has to.
func TestReadBack_providerAndChannel_changingOneSecret_movesTheFingerprintAlone(t *testing.T) {
	world := newReadBackWorld(t)
	provider := "/admin/integrations/providers/readback-drift"
	channelPath := "/admin/channels/readback-drift"
	world.removeAtTheEnd(t, provider)
	world.removeAtTheEnd(t, channelPath)
	gateway := func(key string) map[string]any {
		return map[string]any{"kind": "openai_compatible", "baseUrl": "https://models.example.test", "apiKey": key}
	}
	slack := func(mode string, secrets map[string]any) map[string]any {
		out := map[string]any{"kind": "slack", "deliveryMode": mode}
		for field, value := range secrets {
			out[field] = value
		}
		return out
	}
	for _, c := range []struct {
		name, path    string
		before, after map[string]any
	}{
		{"a provider's key", provider, gateway("drift-key-a"), gateway("drift-key-b")},
		{
			"a channel's bot token", channelPath,
			slack("http", map[string]any{"token": "drift-bot-a"}),
			slack("http", map[string]any{"token": "drift-bot-b"}),
		},
		{
			"a channel's signing secret", channelPath,
			slack("http", map[string]any{"token": "drift-bot-a", "signingSecret": "drift-sign-a"}),
			slack("http", map[string]any{"token": "drift-bot-a", "signingSecret": "drift-sign-b"}),
		},
		{
			"a channel's app token", channelPath,
			slack("socket", map[string]any{"token": "drift-bot-a", "appToken": "drift-app-a"}),
			slack("socket", map[string]any{"token": "drift-bot-a", "appToken": "drift-app-b"}),
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			world.put(t, c.path, c.before)
			first, rawFirst := world.get(t, c.path)
			world.put(t, c.path, c.after)
			second, rawSecond := world.get(t, c.path)
			if first["secretFingerprint"] == nil || first["secretFingerprint"] == second["secretFingerprint"] {
				t.Errorf("fingerprint %v then %v, want it to move", first["secretFingerprint"], second["secretFingerprint"])
			}
			delete(first, "secretFingerprint")
			delete(second, "secretFingerprint")
			if !reflect.DeepEqual(withoutMeta(first), withoutMeta(second)) {
				t.Errorf("more than the fingerprint moved\n before: %v\n  after: %v", first, second)
			}
			if strings.Contains(string(rawFirst)+string(rawSecond), "drift-") {
				t.Errorf("an answer carries a secret: %s\n%s", rawFirst, rawSecond)
			}
		})
	}
}

// A connection stored before the pair existed holds the bare bot token.
func TestReadBack_channel_rowStoredBeforeTheDocument_readsAsItWouldAfter(t *testing.T) {
	world := newReadBackWorld(t)
	path := "/admin/channels/readback-legacy"
	world.removeAtTheEnd(t, path)
	if err := world.store.Put(t.Context(), settings.Setting{
		ScopeKind: settings.ScopeInstallation, Kind: channel.KindChannel, Name: "readback-legacy",
		Value: []byte(`{"kind":"slack"}`), Secret: "drift-bare-bot", Enabled: true,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	stored, raw := world.get(t, path)
	if stored["secretFingerprint"] == nil || stored["hasCredential"] != true ||
		strings.Contains(string(raw), "drift-bare-bot") {
		t.Fatalf("a stored token reads as %s", raw)
	}
	world.put(t, path, stored)
	after, _ := world.get(t, path)
	if !reflect.DeepEqual(withoutMeta(stored), withoutMeta(after)) {
		t.Errorf("re-sealing the row moved the answer\n before: %v\n  after: %v", stored, after)
	}
}

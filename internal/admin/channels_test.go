package admin

import (
	"testing"

	"github.com/fuseone/agents/internal/channel"
)

// A delivery mode has one inbound secret, and the other is not kept "in case".
// A signing secret on a Socket Mode connection is a credential nothing verifies
// with, sitting in the vault waiting for somebody to switch modes back and
// start trusting it again.
func TestMergedCredentials_dropsTheSecretTheModeCannotUse(t *testing.T) {
	t.Parallel()
	both := channel.Credentials{
		Token: "xoxb-1", Signing: "signing", AppToken: "xapp-1",
	}

	http := mergedCredentials(channel.Credentials{}, both, channel.DeliveryHTTP)
	if http.Token != "xoxb-1" || http.Signing != "signing" || http.AppToken != "" {
		t.Fatalf("http credentials = %+v", http)
	}

	socket := mergedCredentials(channel.Credentials{}, both, channel.DeliverySocket)
	if socket.Token != "xoxb-1" || socket.Signing != "" || socket.AppToken != "xapp-1" {
		t.Fatalf("socket credentials = %+v", socket)
	}
}

// And what a request leaves out is kept. "Change the token" must not mean
// "and forget how inbound calls are verified".
func TestMergedCredentials_whatIsLeftOut_isKept(t *testing.T) {
	t.Parallel()
	stored := channel.Credentials{Token: "xoxb-old", Signing: "signing-old"}

	got := mergedCredentials(stored, channel.Credentials{Token: "xoxb-new"},
		channel.DeliveryHTTP)
	if got.Token != "xoxb-new" || got.Signing != "signing-old" {
		t.Fatalf("merged = %+v", got)
	}
}

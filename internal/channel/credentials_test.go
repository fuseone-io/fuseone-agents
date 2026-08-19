package channel_test

import (
	"testing"

	"github.com/fuseone/agents/internal/channel"
)

/*
A channel's two secrets.

They are not interchangeable, and the reason the pair exists at all is that a
webhook trigger's hashed secret cannot do this job: HMAC needs the secret
itself, and there is no reading it back out of a hash.
*/
func TestCredentials_roundTrip_keepsBothApart(t *testing.T) {
	t.Parallel()
	sealed := channel.Credentials{
		Token: "xoxb-1", Signing: "s3cr3t", AppToken: "xapp-1",
	}.Sealed()

	back := channel.ReadCredentials(sealed)
	if back.Token != "xoxb-1" || back.Signing != "s3cr3t" || back.AppToken != "xapp-1" {
		t.Fatalf("read back %+v", back)
	}
}

// What an installation configured before there was anything to check
// signatures with. Refusing to read it would take their notifications away in
// order to add a feature they have not switched on.
func TestReadCredentials_aBareToken_isStillATokan(t *testing.T) {
	t.Parallel()
	back := channel.ReadCredentials("xoxb-from-before")

	if back.Token != "xoxb-from-before" {
		t.Errorf("token = %q, want the value read as one", back.Token)
	}
	if back.Signing != "" {
		t.Errorf("signing = %q, want none — the inbound surface stays closed", back.Signing)
	}
	if back.AppToken != "" {
		t.Errorf("app token = %q, want none — Socket Mode stays closed", back.AppToken)
	}
}

func TestReadCredentials_nothingStored_isNothing(t *testing.T) {
	t.Parallel()
	if back := channel.ReadCredentials(""); back != (channel.Credentials{}) {
		t.Errorf("read %+v from an empty vault entry", back)
	}
}

// Posting must keep working while the inbound half is unconfigured: stage one
// is outbound and does not need a signing secret at all.
func TestCredentials_tokenOnly_seals(t *testing.T) {
	t.Parallel()
	back := channel.ReadCredentials(channel.Credentials{Token: "xoxb-1"}.Sealed())

	if back.Token != "xoxb-1" || back.Signing != "" {
		t.Errorf("read back %+v", back)
	}
}

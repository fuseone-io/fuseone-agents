package admin

import (
	"testing"

	"github.com/fuseone/agents/internal/channel"
)

func TestChannelCredentialsForMode_dropsTheSecretTheModeCannotUse(t *testing.T) {
	t.Parallel()
	both := channel.Credentials{
		Token: "xoxb-1", Signing: "signing", AppToken: "xapp-1",
	}

	http := channelCredentialsForMode(both, channel.DeliveryHTTP)
	if http.Token != "xoxb-1" || http.Signing != "signing" || http.AppToken != "" {
		t.Fatalf("http credentials = %+v", http)
	}

	socket := channelCredentialsForMode(both, channel.DeliverySocket)
	if socket.Token != "xoxb-1" || socket.Signing != "" || socket.AppToken != "xapp-1" {
		t.Fatalf("socket credentials = %+v", socket)
	}
}

package domain

// WebhookFromGitHub is the one source a webhook may name besides none.
//
// GitHub does not send a shared secret: it signs each delivery with one, names
// the delivery in its own header and sends every event the hook subscribes to.
// A hook that says it comes from GitHub is verified and filtered that way.
const WebhookFromGitHub = "github"

// WebhookTrigger is one webhook a version declares, as the door needs it.
//
// From is empty for an ordinary hook — the caller sends the secret and names
// the delivery — or WebhookFromGitHub. Events is what a GitHub delivery must
// be to start a run: `event` or `event.action`, as GitHub names them.
type WebhookTrigger struct {
	Path   string
	From   string
	Events []string
}

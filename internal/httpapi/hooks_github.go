package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/fuseone/agents/internal/trigger"
)

// The headers GitHub sends with every delivery.
const (
	githubSignatureHeader = "X-Hub-Signature-256"
	githubDeliveryHeader  = "X-GitHub-Delivery"
	githubEventHeader     = "X-GitHub-Event"
)

// githubInput is what a GitHub delivery becomes as a run's input.
//
// GitHub's body does not carry its own event name — that is a header — so an
// agent woken by two kinds of event could not tell them apart from the body
// alone. The delivery id is there so the agent can cite it.
type githubInput struct {
	Event    string          `json:"event"`
	Action   string          `json:"action,omitempty"`
	Delivery string          `json:"delivery"`
	Payload  json.RawMessage `json:"payload"`
}

/*
serveGitHub serves a delivery whose signature already proved a hook that says
`from: github` (receive authenticates every kind the same way).

The caller is GitHub and may be told why a delivery did nothing: a ping is
answered, an event the trigger did not name is acknowledged with 2xx — GitHub
retries anything else and marks the hook failing — and a delivery with no id,
or a body that is not JSON, is refused with the reason.
*/
func (h *Hooks) serveGitHub(w http.ResponseWriter, r *http.Request, hook trigger.Hook, body []byte) {
	event := r.Header.Get(githubEventHeader)
	if event == "ping" {
		writeJSON(w, http.StatusOK, map[string]string{"status": "pong"})
		return
	}
	delivery := r.Header.Get(githubDeliveryHeader)
	if delivery == "" {
		writeProblemJSON(w, http.StatusBadRequest, CodeInvalidInput, "A delivery identifier is required",
			"GitHub names every delivery in the "+githubDeliveryHeader+" header; without it a "+
				"redelivery would open a second run.")
		return
	}
	if !json.Valid(body) {
		writeProblemJSON(w, http.StatusUnsupportedMediaType, CodeInvalidInput, "The body is not JSON",
			"Set the GitHub webhook's content type to application/json.")
		return
	}

	action := trigger.GitHubAction(body)
	if !trigger.GitHubWants(hook.Events, event, action) {
		h.log.Debug("GitHub delivery not named by the trigger", "path", hook.Path, "event", event, "action", action)
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}

	input, err := json.Marshal(githubInput{Event: event, Action: action, Delivery: delivery, Payload: body})
	if err != nil {
		h.log.Error("could not wrap a GitHub delivery", "path", hook.Path, "err", err)
		writeProblemJSON(w, http.StatusInternalServerError, CodeUnavailable, "Could not open the run", "")
		return
	}
	h.open(w, r, hook.Path, hook, delivery, input)
}

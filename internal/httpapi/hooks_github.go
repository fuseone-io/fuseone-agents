package httpapi

import (
	"encoding/json"
	"errors"
	"io"
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
receiveGitHub serves a path whose published trigger says `from: github`.

The body is read before anything else, because the signature is over it. A
body over the limit is never read whole, so it cannot be verified, and it is
refused the way every delivery nobody can vouch for is: the same 401 as a
wrong signature, an unrotated hook or a path that does not exist.

After the signature, the caller is GitHub and may be told why a delivery did
nothing: a ping is answered, an event the trigger did not name is acknowledged
with 2xx — GitHub retries anything else and marks the hook failing — and a
delivery with no id, or a body that is not JSON, is refused with the reason.
*/
func (h *Hooks) receiveGitHub(w http.ResponseWriter, r *http.Request, hook trigger.Hook) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxHookBody))
	if err != nil {
		h.log.Info("refused a GitHub delivery over the size limit", "path", hook.Path, "limit", maxHookBody)
		rejectHook(w)
		return
	}
	if !h.signedByGitHub(r, hook.Path, body) {
		rejectHook(w)
		return
	}

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

// signedByGitHub checks the delivery's signature against the hook's secret.
// The secret and the reason it could not be read are never logged.
func (h *Hooks) signedByGitHub(r *http.Request, path string, body []byte) bool {
	secret, err := h.webhooks.SigningSecret(r.Context(), path)
	if err != nil {
		if !errors.Is(err, trigger.ErrNotArmed) && !errors.Is(err, trigger.ErrNoHook) {
			h.log.Error("could not read a GitHub webhook's signing secret", "path", path, "err", err)
		}
		return false
	}
	return trigger.VerifyGitHubSignature(secret, body, r.Header.Get(githubSignatureHeader))
}

package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/trigger"
)

// maxHookBody bounds what a caller can push through the door. A webhook body
// becomes a run's input and reaches a model, so an unbounded one is both a
// memory question and a bill.
const maxHookBody = 1 << 20 // 1 MiB

// secretHeader is where the caller puts the secret the operator gave them.
const secretHeader = "X-FuseOne-Secret" //nolint:gosec // a header name, not a credential

// deliveryHeader names one delivery. Required: without it there is no way to
// tell a redelivery from a second event, and every webhook sender in existence
// redelivers.
const deliveryHeader = "Idempotency-Key"

// Hooks is the door external systems knock on.
//
// Deliberately outside the session middleware — the caller is an ERP or a CRM,
// not a person with a browser — and therefore authenticated by a secret that
// an operator generated and only this path accepts.
type Hooks struct {
	webhooks trigger.Webhooks
	opener   *trigger.Opener
	log      *slog.Logger
}

func NewHooks(webhooks trigger.Webhooks, opener *trigger.Opener, log *slog.Logger) *Hooks {
	return &Hooks{webhooks: webhooks, opener: opener, log: log}
}

// Mount registers the endpoint. One route, one method: a webhook that answered
// GET would be triggerable by anything that follows links.
func (h *Hooks) Mount(mux *http.ServeMux) {
	mux.Handle("POST /hooks/{path...}", http.HandlerFunc(h.receive))
}

func (h *Hooks) receive(w http.ResponseWriter, r *http.Request) {
	path := r.PathValue("path")

	// Everything that is not a correct secret answers the same way. A caller
	// probing paths must not learn which ones exist, which are configured, or
	// which belong to an agent — that is a map of the installation.
	ok, err := h.webhooks.Verify(r.Context(), path, r.Header.Get(secretHeader))
	if err != nil || !ok {
		if err != nil && !errors.Is(err, trigger.ErrNoHook) && !errors.Is(err, trigger.ErrNotArmed) {
			h.log.Error("could not verify a webhook", "path", path, "err", err)
		}
		writeProblemJSON(w, http.StatusUnauthorized, CodeForbidden, "Rejected", "The secret is missing or wrong.")
		return
	}

	delivery := r.Header.Get(deliveryHeader)
	if delivery == "" {
		// Stated plainly, because the sender's integrator is the only person
		// who can fix it and they are reading this response.
		writeProblemJSON(w, http.StatusBadRequest, CodeInvalidInput, "A delivery identifier is required",
			"Send a value unique to this delivery in the "+deliveryHeader+" header, and repeat "+
				"the same value on every retry of it. Without one a redelivery opens a second run.")
		return
	}

	hook, err := h.webhooks.Find(r.Context(), path)
	if err != nil {
		writeProblemJSON(w, http.StatusUnauthorized, CodeForbidden, "Rejected", "The secret is missing or wrong.")
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxHookBody))
	if err != nil {
		writeProblemJSON(w, http.StatusRequestEntityTooLarge, CodeInvalidInput, "The body is too large",
			"A webhook body becomes a run's input and reaches a model.")
		return
	}

	opened, err := h.opener.Open(r.Context(), trigger.Request{
		Agent:   hook.Agent,
		IdemKey: trigger.DeliveryKey(path, delivery),
		Trigger: "webhook",
		Input:   body,
		// A body somebody outside sent. On a good day it is an ERP's JSON and
		// on a bad one it is whatever they posted, and the difference is not
		// visible from here — the secret proves who sent it and says nothing
		// about what is inside.
		Labels: domain.NewLabels(domain.LabelUntrusted),
	})
	/*
		An agent that is not running is a state, not a failure of the platform.

		Answered 500 with an empty detail, the person reading it is the sender's
		integrator and what they conclude is that this platform is broken — while
		on our side it is logged as an error and pages somebody about a decision
		an operator made on purpose.

		The secret was correct, so there is nothing left to protect: a caller who
		authenticated is entitled to know why their delivery did nothing. The
		probing this handler guards against is answered identically whether a path
		exists or not, and that happened before any of this.
	*/
	if notRunning(err) {
		writeProblemJSON(w, http.StatusConflict, CodeConflict, "The agent is not running", err.Error())
		return
	}
	if err != nil {
		h.log.Error("could not open a run from a webhook", "path", path, "err", err)
		writeProblemJSON(w, http.StatusInternalServerError, CodeUnavailable, "Could not open the run", "")
		return
	}

	status := http.StatusAccepted
	if !opened.Created {
		// Already delivered. Not an error: a sender retrying is behaving
		// correctly, and it gets the run its delivery names.
		status = http.StatusOK
	}
	h.log.Info("webhook accepted", "path", path, "run", opened.RunID, "created", opened.Created)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"runId": string(opened.RunID)})
}

// notRunning reports whether the agent declined to start rather than failed to.
//
// Three states and one answer, because the sender's question is the same in
// all three: their delivery arrived, it was accepted as theirs, and nothing
// happened because of a decision on this side.
func notRunning(err error) bool {
	return errors.Is(err, trigger.ErrPaused) ||
		errors.Is(err, trigger.ErrStopped) ||
		errors.Is(err, trigger.ErrDraft)
}

package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/channel/slack"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

/*
What a conversation says back.

Stage 2 of NT-005, and the whole of the inbound surface: a signed interaction
payload, never free text. A person presses a button on the message a run posted
and the decision they take is the decision the console takes — not one that
looks like it.

That is the point of this file and the reason it is thin. It verifies, resolves
who pressed, puts that person in the context, and calls DecideApproval. Same
permission check against the run's own scope, same refusal when a later step
already superseded the ask, same step sealed into the same chain with the same
digest. A second approval path with weaker facts would give the record two
grades of approval, which is exactly what internal/audit refuses to do for the
two trails it already merges.

Mounted outside /api/v1 on purpose. This is a vendor's webhook and not part of
the contract clients are generated from: its shape is Slack's, it will differ
for Teams, and putting it in the API would make somebody else's payload format
part of ours.
*/

// Bindings answers who a channel account speaks for, declared here by the
// consumer.
type Bindings interface {
	PrincipalFor(ctx context.Context, channel, account string) (domain.UserID, bool)
	Secrets(ctx context.Context, channel string) (channel.Credentials, bool)
}

// Directory loads a person the platform has already decided is this person.
type Directory interface {
	PrincipalByID(ctx context.Context, id domain.UserID) (domain.Principal, error)
}

// ChannelHooks receives what conversations send back.
type ChannelHooks struct {
	api       *Server
	bindings  Bindings
	directory Directory
	now       func() time.Time
	log       *slog.Logger
}

func NewChannelHooks(
	api *Server, bindings Bindings, directory Directory,
	now func() time.Time, log *slog.Logger,
) *ChannelHooks {
	if log == nil {
		log = slog.Default()
	}
	return &ChannelHooks{api: api, bindings: bindings, directory: directory, now: now, log: log}
}

// Mount wires the one path a channel posts to.
func (h *ChannelHooks) Mount(mux *http.ServeMux) {
	mux.HandleFunc("POST /hooks/channel/{channel}/slack", h.slackInteraction)
}

// bodyLimit bounds what an unauthenticated caller can make this process read.
// Slack's interaction payloads are a few kilobytes; this is generous.
const bodyLimit = 128 << 10

func (h *ChannelHooks) slackInteraction(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("channel")

	// Read once and verify those exact bytes. A verifier that re-read the body
	// would check one set and let the handler act on another.
	body, err := io.ReadAll(io.LimitReader(r.Body, bodyLimit))
	if err != nil {
		http.Error(w, "unreadable", http.StatusBadRequest)
		return
	}

	creds, known := h.bindings.Secrets(r.Context(), name)
	if !known {
		// Not "unauthorised": there is no such channel, and saying so is not a
		// secret — the caller supplied the name.
		http.Error(w, "no such channel", http.StatusNotFound)
		return
	}
	if err := slack.Verify(r, body, creds.Signing, h.now()); err != nil {
		// Logged and refused flatly. The reason belongs in the log, not in a
		// reply to somebody who failed to prove who they are.
		h.log.Warn("a channel interaction was refused", "channel", name, "err", err)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	action, err := slack.ReadInteraction(body)
	if err != nil {
		http.Error(w, "unreadable payload", http.StatusBadRequest)
		return
	}

	answer := h.decide(r.Context(), name, action)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	// Slack replaces the message with whatever comes back, which is how the
	// buttons stop being pressable once somebody has answered.
	_ = json.NewEncoder(w).Encode(map[string]any{
		"replace_original": true,
		"text":             answer,
	})
}

// decide resolves who pressed and takes the decision as them.
func (h *ChannelHooks) decide(
	ctx context.Context, name string, action slack.Interaction,
) slack.Answer {
	principalID, bound := h.bindings.PrincipalFor(ctx, name, action.User)
	if !bound {
		// Refused by name. "Something went wrong" would send somebody to
		// debug a platform that is working exactly as intended.
		h.log.Warn("an unbound account tried to decide",
			"channel", name, "account", action.User, "run", action.RunID)
		return slack.AnswerUnbound
	}

	principal, err := h.directory.PrincipalByID(ctx, principalID)
	if err != nil {
		h.log.Warn("a bound account resolved to nobody",
			"principal", principalID, "err", err)
		return slack.AnswerUnknown
	}

	// From here it is the console's path, with the console's checks. Nothing
	// below knows the decision arrived through a conversation.
	as := auth.WithPrincipal(ctx, principal)
	resp, err := h.api.DecideApproval(as, openapi.DecideApprovalRequestObject{
		RunId: string(action.RunID),
		Body: &openapi.DecideApprovalJSONRequestBody{
			Approved: action.Approved,
			AtSeq:    action.AtSeq,
			Note:     ptr(noteOf(action, principal)),
		},
	})
	if err != nil {
		h.log.Error("a channel decision could not be recorded",
			"run", action.RunID, "err", err)
		return slack.AnswerFailed
	}

	switch resp.(type) {
	case openapi.DecideApproval200JSONResponse:
		if action.Approved {
			return slack.AnswerApproved
		}
		return slack.AnswerRefused
	case openapi.DecideApproval403ApplicationProblemPlusJSONResponse:
		return slack.AnswerForbidden
	case openapi.DecideApproval409ApplicationProblemPlusJSONResponse:
		// Somebody answered from the console while this message sat in a
		// channel, which is ordinary and not a failure.
		return slack.AnswerDecided
	default:
		return slack.AnswerGone
	}
}

// noteOf records where the decision came from, in the decision itself.
//
// An auditor reading the trail a year later should be able to tell that
// somebody pressed a button in Slack rather than opened the console. It is the
// same decision either way and it was not taken in the same place.
func noteOf(action slack.Interaction, principal domain.Principal) string {
	parts := []string{"decided in a conversation"}
	if principal.Display != "" {
		parts = append(parts, "by "+principal.Display)
	}
	if action.Conversation != "" {
		parts = append(parts, "in "+action.Conversation)
	}
	return strings.Join(parts, ", ")
}

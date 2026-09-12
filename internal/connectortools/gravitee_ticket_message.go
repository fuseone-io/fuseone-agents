package connectortools

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/fuseone/agents/internal/ticket"
)

// GraviteeTicketOutcomeRenderer turns the connector-owned safe projection
// into the final ticket message. The model never writes this answer and the
// remote API key has no field in the input type.
type GraviteeTicketOutcomeRenderer struct{}

func (GraviteeTicketOutcomeRenderer) RenderTicketOutcome(
	phase ticket.Phase, raw []byte,
) (string, error) {
	result, err := decodeGraviteeAcceptanceResult(raw)
	if err != nil {
		return "", err
	}
	switch phase {
	case ticket.PhaseCompleted:
		if result.Status != "accepted" || !completeGraviteeResources(result) {
			return "", errors.New("connector: completed Gravitee ticket has an invalid outcome")
		}
		expiration := "No expiration is configured; rotate the key as soon as the access is no longer needed."
		if result.RequestedExpiration != nil {
			expiration = *result.RequestedExpiration
		}
		return fmt.Sprintf(
			"## API key subscription approved\n\n"+
				"Subscription `%s` was approved in Gravitee.\n\n"+
				"**Expiration:** %s\n\n"+
				"FuseOne never posts the API key in Slack. Retrieve it only through the approved Gravitee flow, store it in a secret manager, never paste it into chat, source code or logs, and revoke or rotate it immediately if exposure is suspected.",
			result.SubscriptionID, expiration), nil
	case ticket.PhaseRejected:
		if !completeGraviteeResources(result) {
			return "", errors.New("connector: rejected Gravitee ticket has an invalid outcome")
		}
		return fmt.Sprintf(
			"## API key subscription not approved\n\n"+
				"Subscription `%s` did not complete in Gravitee. No API key was posted. Review the governed run before trying again.",
			result.SubscriptionID), nil
	case ticket.PhaseNeedsAttention:
		if result.Status != CodeConnectorNeedsAttention {
			return "", errors.New("connector: manual Gravitee ticket has an invalid outcome")
		}
		return fmt.Sprintf(
			"## Manual verification required\n\n"+
				"FuseOne could not prove whether subscription `%s` was accepted. Do not retry or issue another key. An operator must inspect this subscription in Gravitee. FuseOne will keep observing and will post the confirmed result here.",
			result.SubscriptionID), nil
	default:
		return "", errors.New("connector: Gravitee ticket outcome is not terminal")
	}
}

func decodeGraviteeAcceptanceResult(raw []byte) (GraviteeAcceptanceResult, error) {
	var result GraviteeAcceptanceResult
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return result, errors.New("connector: invalid Gravitee ticket outcome")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return result, errors.New("connector: invalid Gravitee ticket outcome")
	}
	if result.Operation != "gravitee.accept_subscription" ||
		!graviteeID.MatchString(result.SubscriptionID) || result.DecidedBy == "" {
		return result, errors.New("connector: invalid Gravitee ticket outcome")
	}
	if result.RequestedExpiration != nil {
		parsed, err := time.Parse(time.RFC3339Nano, *result.RequestedExpiration)
		if err != nil || parsed.UTC().Format(time.RFC3339Nano) != *result.RequestedExpiration {
			return result, errors.New("connector: invalid Gravitee ticket outcome")
		}
	}
	return result, nil
}

func completeGraviteeResources(result GraviteeAcceptanceResult) bool {
	return validRemoteResource(result.Application) && validRemoteResource(result.API) &&
		validRemoteResource(result.Plan)
}

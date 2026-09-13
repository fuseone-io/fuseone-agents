package connectortools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"path"
	"strings"
	"time"
	"unicode/utf8"
)

const maxGraviteeResponseBytes = 256 << 10

var (
	ErrGraviteeResponse = errors.New("connector: Gravitee returned an unusable subscription")
	ErrGraviteeScope    = errors.New("connector: the subscription is outside the configured Gravitee boundary")
	ErrGraviteeState    = errors.New("connector: the Gravitee subscription is not pending")
)

type GraviteeResource struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type GraviteeApplication struct {
	GraviteeResource
	PrimaryOwnerEmail string `json:"primaryOwnerEmail"`
}

// GraviteeObservation is the complete allowlist of remote fields that may
// become approval evidence. Consumer messages, metadata and API keys have no
// representation here.
type GraviteeObservation struct {
	SubscriptionID string              `json:"subscriptionId"`
	Status         string              `json:"status"`
	Application    GraviteeApplication `json:"application"`
	API            GraviteeResource    `json:"api"`
	Plan           GraviteeResource    `json:"plan"`
	PlanSecurity   string              `json:"planSecurity"`
	CreatedAt      string              `json:"createdAt"`
	UpdatedAt      string              `json:"updatedAt"`
	EndingAt       *string             `json:"endingAt,omitempty"`
}

type graviteeRemoteError struct {
	status    int
	ambiguous bool
	requestID string
}

func (e graviteeRemoteError) Error() string {
	if e.requestID == "" {
		return fmt.Sprintf("connector: Gravitee returned status %d", e.status)
	}
	return fmt.Sprintf("connector: Gravitee returned status %d (request %s)",
		e.status, e.requestID)
}

func ambiguousGraviteeResult(err error) bool {
	var remote graviteeRemoteError
	return errors.As(err, &remote) && remote.ambiguous
}

type HTTPGraviteeClient struct {
	client *http.Client
}

func NewHTTPGraviteeClient(client *http.Client) *HTTPGraviteeClient {
	if client == nil {
		client = guardedHTTPClient()
	}
	clone := *client
	clone.CheckRedirect = func(*http.Request, []*http.Request) error {
		return errors.New("connector: Gravitee redirects are refused")
	}
	if clone.Timeout == 0 || clone.Timeout > 30*time.Second {
		clone.Timeout = 30 * time.Second
	}
	return &HTTPGraviteeClient{client: &clone}
}

func (c *HTTPGraviteeClient) Inspect(
	ctx context.Context, cfg GraviteeConfig, credential SecretValue, subscriptionID string,
) (GraviteeObservation, error) {
	endpoint, err := graviteeSubscriptionURL(cfg, subscriptionID)
	if err != nil || credential.empty() {
		return GraviteeObservation{}, ErrGraviteeScope
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return GraviteeObservation{}, ErrGraviteeResponse
	}
	req.Header.Set("Authorization", "Bearer "+credential.value)
	req.Header.Set("Accept", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return GraviteeObservation{}, err
		}
		return GraviteeObservation{}, fmt.Errorf("connector: Gravitee request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 512))
		return GraviteeObservation{}, graviteeStatusError(resp)
	}
	return decodeGraviteeObservation(resp.Body, cfg, subscriptionID, "PENDING")
}

// Observe is the read-only half of reconciliation. Unlike Inspect it accepts
// terminal states, but it retains the same fixed projection and scope checks.
func (c *HTTPGraviteeClient) Observe(
	ctx context.Context, cfg GraviteeConfig, credential SecretValue, subscriptionID string,
) (GraviteeObservation, error) {
	endpoint, err := graviteeSubscriptionURL(cfg, subscriptionID)
	if err != nil || credential.empty() {
		return GraviteeObservation{}, ErrGraviteeScope
	}
	return c.read(ctx, endpoint, cfg, credential, subscriptionID,
		"PENDING", "ACCEPTED", "REJECTED", "CLOSED")
}

// Accept sends the one fixed write this connector owns. No caller can add a
// field to the body; in particular customApiKey has no representation here.
func (c *HTTPGraviteeClient) Accept(
	ctx context.Context, cfg GraviteeConfig, credential SecretValue, snapshot GraviteeSnapshot,
) (GraviteeObservation, error) {
	endpoint, err := graviteeSubscriptionURL(cfg, snapshot.SubscriptionID)
	if err != nil || credential.empty() {
		return GraviteeObservation{}, ErrGraviteeScope
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return GraviteeObservation{}, ErrGraviteeScope
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/_accept"
	body, err := json.Marshal(struct {
		Reason   string  `json:"reason"`
		EndingAt *string `json:"endingAt,omitempty"`
	}{Reason: "Approved through a FuseOne governed ticket", EndingAt: snapshot.RequestedExpiration})
	if err != nil {
		return GraviteeObservation{}, ErrGraviteeResponse
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return GraviteeObservation{}, ErrGraviteeResponse
	}
	req.Header.Set("Authorization", "Bearer "+credential.value)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		// Once the transport accepted a POST, cancellation and connection errors
		// cannot prove the remote side did not apply it.
		return GraviteeObservation{}, graviteeRemoteError{ambiguous: true}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 512))
		return GraviteeObservation{}, graviteeRemoteStatus(resp, ambiguousAcceptStatus(resp.StatusCode))
	}
	observed, err := decodeGraviteeObservation(resp.Body, cfg, snapshot.SubscriptionID, "ACCEPTED")
	if err != nil {
		return GraviteeObservation{}, graviteeRemoteError{status: resp.StatusCode, ambiguous: true,
			requestID: safeGraviteeRequestID(resp)}
	}
	if !sameRequestedExpiration(observed.EndingAt, snapshot.RequestedExpiration) {
		return GraviteeObservation{}, graviteeRemoteError{status: resp.StatusCode, ambiguous: true,
			requestID: safeGraviteeRequestID(resp)}
	}
	return observed, nil
}

func (c *HTTPGraviteeClient) read(
	ctx context.Context, endpoint string, cfg GraviteeConfig, credential SecretValue,
	subscriptionID string, statuses ...string,
) (GraviteeObservation, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return GraviteeObservation{}, ErrGraviteeResponse
	}
	req.Header.Set("Authorization", "Bearer "+credential.value)
	req.Header.Set("Accept", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return GraviteeObservation{}, err
		}
		return GraviteeObservation{}, fmt.Errorf("connector: Gravitee request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 512))
		return GraviteeObservation{}, graviteeStatusError(resp)
	}
	return decodeGraviteeObservation(resp.Body, cfg, subscriptionID, statuses...)
}

func graviteeSubscriptionURL(cfg GraviteeConfig, subscriptionID string) (string, error) {
	if err := validateGraviteeAddress("runtime", cfg.Address); err != nil ||
		!graviteeID.MatchString(subscriptionID) || len(cfg.AllowedReferences) != 1 {
		return "", ErrGraviteeScope
	}
	reference := cfg.AllowedReferences[0]
	if reference.Type != GraviteeReferenceAPI || !graviteeID.MatchString(reference.ID) ||
		!graviteeID.MatchString(cfg.Organization) || !graviteeID.MatchString(cfg.Environment) {
		return "", ErrGraviteeScope
	}
	base, err := url.Parse(strings.TrimRight(cfg.Address, "/") + "/")
	if err != nil {
		return "", ErrGraviteeScope
	}
	base.Path = path.Join(base.Path, "organizations", cfg.Organization, "environments",
		cfg.Environment, "apis", reference.ID, "subscriptions", subscriptionID)
	query := base.Query()
	for _, expand := range []string{"api", "application", "plan"} {
		query.Add("expands", expand)
	}
	base.RawQuery = query.Encode()
	return base.String(), nil
}

type graviteeSubscriptionWire struct {
	ID     string           `json:"id"`
	Status string           `json:"status"`
	API    GraviteeResource `json:"api"`
	Plan   struct {
		GraviteeResource
		Security struct {
			Type string `json:"type"`
		} `json:"security"`
	} `json:"plan"`
	Application struct {
		GraviteeResource
		PrimaryOwner struct {
			ID    string `json:"id"`
			Email string `json:"email"`
		} `json:"primaryOwner"`
	} `json:"application"`
	CreatedAt string  `json:"createdAt"`
	UpdatedAt string  `json:"updatedAt"`
	EndingAt  *string `json:"endingAt"`
}

func decodeGraviteeObservation(
	body io.Reader, cfg GraviteeConfig, subscriptionID string, statuses ...string,
) (GraviteeObservation, error) {
	raw, err := io.ReadAll(io.LimitReader(body, maxGraviteeResponseBytes+1))
	if err != nil || len(raw) > maxGraviteeResponseBytes {
		return GraviteeObservation{}, ErrGraviteeResponse
	}
	var wire graviteeSubscriptionWire
	if json.Unmarshal(raw, &wire) != nil {
		return GraviteeObservation{}, ErrGraviteeResponse
	}
	return wire.observation(cfg, subscriptionID, statuses...)
}

func (w graviteeSubscriptionWire) observation(
	cfg GraviteeConfig, subscriptionID string, statuses ...string,
) (GraviteeObservation, error) {
	if w.ID != subscriptionID || len(cfg.AllowedReferences) != 1 ||
		w.API.ID != cfg.AllowedReferences[0].ID {
		return GraviteeObservation{}, ErrGraviteeScope
	}
	if !containsGraviteeStatus(statuses, w.Status) {
		return GraviteeObservation{}, ErrGraviteeState
	}
	if w.Plan.Security.Type != "API_KEY" || !validRemoteResource(w.API) ||
		!validRemoteResource(w.Plan.GraviteeResource) ||
		!validRemoteResource(w.Application.GraviteeResource) ||
		!validOwner(w.Application.PrimaryOwner.ID, w.Application.PrimaryOwner.Email) {
		return GraviteeObservation{}, ErrGraviteeResponse
	}
	created, okCreated := canonicalRemoteTime(w.CreatedAt)
	updated, okUpdated := canonicalRemoteTime(w.UpdatedAt)
	if !okCreated || !okUpdated {
		return GraviteeObservation{}, ErrGraviteeResponse
	}
	ending, ok := canonicalOptionalRemoteTime(w.EndingAt)
	if !ok {
		return GraviteeObservation{}, ErrGraviteeResponse
	}
	return GraviteeObservation{
		SubscriptionID: w.ID, Status: w.Status, API: w.API,
		Plan: w.Plan.GraviteeResource, PlanSecurity: w.Plan.Security.Type,
		Application: GraviteeApplication{
			GraviteeResource:  w.Application.GraviteeResource,
			PrimaryOwnerEmail: strings.TrimSpace(w.Application.PrimaryOwner.Email),
		},
		CreatedAt: created, UpdatedAt: updated, EndingAt: ending,
	}, nil
}

func containsGraviteeStatus(allowed []string, got string) bool {
	for _, status := range allowed {
		if got == status {
			return true
		}
	}
	return false
}

func canonicalOptionalRemoteTime(raw *string) (*string, bool) {
	if raw == nil {
		return nil, true
	}
	canonical, ok := canonicalRemoteTime(*raw)
	return &canonical, ok
}

func sameRequestedExpiration(observed, requested *string) bool {
	return (observed == nil && requested == nil) ||
		(observed != nil && requested != nil && *observed == *requested)
}

func validRemoteResource(resource GraviteeResource) bool {
	return graviteeID.MatchString(resource.ID) && safeRemoteText(resource.Name, 512)
}

func validOwner(id, email string) bool {
	email = strings.TrimSpace(email)
	parsed, err := mail.ParseAddress(email)
	return graviteeID.MatchString(id) && err == nil && parsed.Address == email && len(email) <= 320
}

func safeRemoteText(value string, max int) bool {
	if value == "" || len(value) > max || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func canonicalRemoteTime(raw string) (string, bool) {
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return "", false
	}
	return parsed.UTC().Format(time.RFC3339Nano), true
}

func graviteeStatusError(resp *http.Response) error {
	return graviteeRemoteStatus(resp, false)
}

func graviteeRemoteStatus(resp *http.Response, ambiguous bool) error {
	return graviteeRemoteError{
		status: resp.StatusCode, ambiguous: ambiguous, requestID: safeGraviteeRequestID(resp),
	}
}

func safeGraviteeRequestID(resp *http.Response) string {
	requestID := strings.TrimSpace(resp.Header.Get("X-Request-ID"))
	if !graviteeID.MatchString(requestID) {
		return ""
	}
	return requestID
}

func ambiguousAcceptStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusConflict ||
		status == http.StatusTooEarly || status == http.StatusTooManyRequests || status >= 500
	/*
		A 4xx outside this list is a definitive refusal before the operation was
		accepted by the API contract. A 409 can mean a competing state change,
		and retry/5xx responses cannot prove whether processing already started.
	*/
}

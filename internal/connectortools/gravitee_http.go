package connectortools

import (
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
	return decodeGraviteeObservation(resp.Body, cfg, subscriptionID)
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
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

func decodeGraviteeObservation(
	body io.Reader, cfg GraviteeConfig, subscriptionID string,
) (GraviteeObservation, error) {
	raw, err := io.ReadAll(io.LimitReader(body, maxGraviteeResponseBytes+1))
	if err != nil || len(raw) > maxGraviteeResponseBytes {
		return GraviteeObservation{}, ErrGraviteeResponse
	}
	var wire graviteeSubscriptionWire
	if json.Unmarshal(raw, &wire) != nil {
		return GraviteeObservation{}, ErrGraviteeResponse
	}
	return wire.observation(cfg, subscriptionID)
}

func (w graviteeSubscriptionWire) observation(
	cfg GraviteeConfig, subscriptionID string,
) (GraviteeObservation, error) {
	if w.ID != subscriptionID || len(cfg.AllowedReferences) != 1 ||
		w.API.ID != cfg.AllowedReferences[0].ID {
		return GraviteeObservation{}, ErrGraviteeScope
	}
	if w.Status != "PENDING" {
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
	return GraviteeObservation{
		SubscriptionID: w.ID, Status: w.Status, API: w.API,
		Plan: w.Plan.GraviteeResource, PlanSecurity: w.Plan.Security.Type,
		Application: GraviteeApplication{
			GraviteeResource:  w.Application.GraviteeResource,
			PrimaryOwnerEmail: strings.TrimSpace(w.Application.PrimaryOwner.Email),
		},
		CreatedAt: created, UpdatedAt: updated,
	}, nil
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
	requestID := strings.TrimSpace(resp.Header.Get("X-Request-ID"))
	if !graviteeID.MatchString(requestID) {
		requestID = ""
	}
	if requestID == "" {
		return fmt.Errorf("connector: Gravitee returned status %d", resp.StatusCode)
	}
	return fmt.Errorf("connector: Gravitee returned status %d (request %s)",
		resp.StatusCode, requestID)
}

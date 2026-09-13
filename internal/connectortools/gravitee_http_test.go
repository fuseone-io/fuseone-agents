package connectortools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestGraviteeAccept_ownsTheBodyAndReturnsOnlyTheSafeAcceptedProjection(t *testing.T) {
	const (
		credential = "GRAVITEE-WRITE-CREDENTIAL-CANARY"
		apiKey     = "GRAVITEE-GENERATED-KEY-CANARY"
	)
	expires := "2026-09-13T12:00:00Z"
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/subscriptions/sub-42/_accept") {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer "+credential {
			t.Error("accept did not use the resolved connector credential")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(body) != 2 || body["endingAt"] != expires || body["customApiKey"] != nil {
			t.Fatalf("accept body = %#v", body)
		}
		_, _ = w.Write([]byte(strings.ReplaceAll(
			validGraviteeSubscription(), `"status":"PENDING"`,
			`"status":"ACCEPTED","endingAt":"`+expires+`","apiKey":"`+apiKey+`"`)))
	}))
	defer server.Close()

	cfg := graviteeInstance(area("acme", "platform"), graviteeSource("secrets")).Gravitee
	cfg.Address = server.URL
	snapshot := GraviteeSnapshot{SubscriptionID: "sub-42", RequestedExpiration: &expires}
	got, err := NewHTTPGraviteeClient(server.Client()).Accept(
		t.Context(), cfg, SecretValue{value: credential}, snapshot)
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if got.Status != "ACCEPTED" || !sameRequestedExpiration(got.EndingAt, &expires) {
		t.Fatalf("accepted observation = %+v", got)
	}
	if strings.Contains(fmt.Sprintf("%+v", got), apiKey) {
		t.Fatal("accepted projection carried the API key")
	}
}

func TestGraviteeAccept_distinguishesDefinitiveRefusalFromAnAmbiguousOutcome(t *testing.T) {
	for _, test := range []struct {
		status    int
		ambiguous bool
	}{{http.StatusBadRequest, false}, {http.StatusConflict, true},
		{http.StatusTooManyRequests, true}, {http.StatusInternalServerError, true}} {
		t.Run(fmt.Sprint(test.status), func(t *testing.T) {
			const canary = "GRAVITEE-ERROR-BODY-CANARY"
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("X-Request-ID", "request-safe")
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(canary))
			}))
			defer server.Close()
			cfg := graviteeInstance(area("acme", "platform"), graviteeSource("secrets")).Gravitee
			cfg.Address = server.URL
			_, err := NewHTTPGraviteeClient(server.Client()).Accept(t.Context(), cfg,
				SecretValue{value: "credential"}, GraviteeSnapshot{SubscriptionID: "sub-42"})
			if err == nil || ambiguousGraviteeResult(err) != test.ambiguous {
				t.Fatalf("Accept error = %v, ambiguous = %v", err, ambiguousGraviteeResult(err))
			}
			if strings.Contains(err.Error(), canary) {
				t.Fatal("the Gravitee error body escaped through the error")
			}
		})
	}
}

func TestGraviteeObserve_readsTerminalStateWithoutChangingTheProjection(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Replace(
			validGraviteeSubscription(), `"status":"PENDING"`, `"status":"REJECTED"`, 1)))
	}))
	defer server.Close()
	cfg := graviteeInstance(area("acme", "platform"), graviteeSource("secrets")).Gravitee
	cfg.Address = server.URL
	got, err := NewHTTPGraviteeClient(server.Client()).Observe(
		t.Context(), cfg, SecretValue{value: "credential"}, "sub-42")
	if err != nil || got.Status != "REJECTED" {
		t.Fatalf("Observe = (%+v, %v)", got, err)
	}
}

func TestGraviteeInspect_usesTheConfiguredPathAndProjectsOnlyApprovalFields(t *testing.T) {
	const (
		credential = "GRAVITEE-CREDENTIAL-CANARY"
		apiKey     = "GRAVITEE-API-KEY-CANARY"
	)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantPath := "/management/v2/organizations/org-prod/environments/env-prod/apis/checkout-api/subscriptions/sub-42"
		if r.URL.Path != wantPath {
			t.Errorf("path = %q, want %q", r.URL.Path, wantPath)
		}
		if got := r.URL.Query()["expands"]; fmt.Sprint(got) != "[api application plan]" {
			t.Errorf("expands = %v", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+credential {
			t.Errorf("authorization was not the selected credential")
		}
		_, _ = w.Write([]byte(`{
			"id":"sub-42","status":"PENDING",
			"api":{"id":"checkout-api","name":"Checkout"},
			"plan":{"id":"key-plan","name":"API keys","security":{"type":"API_KEY"}},
			"application":{"id":"app-7","name":"Portal","primaryOwner":{"id":"owner-1","email":"dev@example.com"}},
			"createdAt":"2026-09-10T12:00:00Z","updatedAt":"2026-09-11T12:00:00Z",
			"consumerMessage":"` + apiKey + `","metadata":{"api_key":"` + apiKey + `"}
		}`))
	}))
	defer server.Close()

	cfg := graviteeInstance(area("acme", "platform"), graviteeSource("secrets")).Gravitee
	cfg.Address = server.URL + "/management/v2"
	got, err := NewHTTPGraviteeClient(server.Client()).Inspect(
		context.Background(), cfg, SecretValue{value: credential}, "sub-42")
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if got.SubscriptionID != "sub-42" || got.API.ID != "checkout-api" ||
		got.Application.PrimaryOwnerEmail != "dev@example.com" || got.PlanSecurity != "API_KEY" {
		t.Fatalf("observation = %+v", got)
	}
	if strings.Contains(fmt.Sprintf("%+v", got), apiKey) {
		t.Fatalf("safe observation carried an API key")
	}
}

func TestGraviteeInspect_neverFollowsARedirectWithTheCredential(t *testing.T) {
	var reached atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		reached.Add(1)
	}))
	defer target.Close()
	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", target.URL)
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer source.Close()

	cfg := graviteeInstance(area("acme", "platform"), graviteeSource("secrets")).Gravitee
	cfg.Address = source.URL + "/management/v2"
	_, err := NewHTTPGraviteeClient(source.Client()).Inspect(
		context.Background(), cfg, SecretValue{value: "DO-NOT-FORWARD"}, "sub-42")
	if err == nil || reached.Load() != 0 {
		t.Fatalf("Inspect err = %v, redirected requests = %d", err, reached.Load())
	}
}

func TestGraviteeInspect_refusesWrongScopeStateAndPlan(t *testing.T) {
	cases := map[string][2]string{
		"wrong subscription": {`"id":"sub-42"`, `"id":"sub-other"`},
		"wrong api":          {`"api":{"id":"checkout-api","name":"Checkout"}`, `"api":{"id":"payments-api","name":"Payments"}`},
		"already accepted":   {`"status":"PENDING"`, `"status":"ACCEPTED"`},
		"not an api key":     {`"plan":{"id":"plan","name":"Keys","security":{"type":"API_KEY"}}`, `"plan":{"id":"plan","name":"OAuth","security":{"type":"OAUTH2"}}`},
		"group owner":        {`"primaryOwner":{"id":"owner","email":"dev@example.com"}`, `"primaryOwner":{"id":"group"}`},
	}
	for name, replacement := range cases {
		t.Run(name, func(t *testing.T) {
			body := strings.Replace(validGraviteeSubscription(), replacement[0], replacement[1], 1)
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			cfg := graviteeInstance(area("acme", "platform"), graviteeSource("secrets")).Gravitee
			cfg.Address = server.URL
			_, err := NewHTTPGraviteeClient(server.Client()).Inspect(
				context.Background(), cfg, SecretValue{value: "token"}, "sub-42")
			if err == nil {
				t.Fatal("unsafe subscription was accepted")
			}
		})
	}
}

func validGraviteeSubscription() string {
	return `{"id":"sub-42","status":"PENDING","api":{"id":"checkout-api","name":"Checkout"},` +
		`"plan":{"id":"plan","name":"Keys","security":{"type":"API_KEY"}},` +
		`"application":{"id":"app","name":"Portal","primaryOwner":{"id":"owner","email":"dev@example.com"}},` +
		`"createdAt":"2026-09-10T12:00:00Z","updatedAt":"2026-09-11T12:00:00Z"}`
}

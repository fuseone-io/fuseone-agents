package connectortools

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func cloudflareTestConfig(serverURL string) CloudflareConfig {
	return CloudflareConfig{
		BaseURL:   serverURL,
		AccountID: "acct1234",
		ListID:    "list5678",
	}
}

// The token travels as a bearer and nowhere else; the request lands on the
// one configured list's items path; the write body is exactly one item.
func TestHTTPCloudflareClient_addItem_sendsOneItemWithTheBearer(t *testing.T) {
	t.Parallel()
	var method, path, auth, body string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path, auth = r.Method, r.URL.Path, r.Header.Get("Authorization")
		raw := make([]byte, 1024)
		n, _ := r.Body.Read(raw)
		body = string(raw[:n])
		fmt.Fprint(w, `{"success":true,"result":{"operation_id":"op-9"}}`)
	}))
	defer server.Close()

	client := NewHTTPCloudflareClient(server.Client())
	operationID, err := client.AddItem(t.Context(), cloudflareTestConfig(server.URL),
		SecretValue{value: "CANARY-token"}, "198.51.100.7", "fuseone:auto:2026-01-10T03:00:00Z probe")
	if err != nil {
		t.Fatalf("AddItem: %v", err)
	}
	if operationID != "op-9" {
		t.Fatalf("operationID = %q", operationID)
	}
	if method != http.MethodPost ||
		path != "/client/v4/accounts/acct1234/rules/lists/list5678/items" {
		t.Fatalf("request = %s %s", method, path)
	}
	if auth != "Bearer CANARY-token" {
		t.Fatalf("authorization header = %q", auth)
	}
	if !strings.Contains(body, `"ip":"198.51.100.7"`) || strings.Contains(body, "CANARY") {
		t.Fatalf("body = %q", body)
	}
}

// A refused status becomes a status, never the body and never the token: the
// error string is what reaches logs and the run record.
func TestHTTPCloudflareClient_aFailedStatus_leaksNothing(t *testing.T) {
	t.Parallel()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"errors":[{"message":"secret-shaped remote text"}]}`)
	}))
	defer server.Close()

	client := NewHTTPCloudflareClient(server.Client())
	_, err := client.Items(t.Context(), cloudflareTestConfig(server.URL),
		SecretValue{value: "CANARY-token"})
	if err == nil {
		t.Fatal("Items accepted a 403")
	}
	if strings.Contains(err.Error(), "CANARY") || strings.Contains(err.Error(), "secret-shaped") {
		t.Fatalf("the error carries what it must not: %v", err)
	}
}

// A redirect is refused, not followed: a compromised endpoint must not be
// able to walk the bearer somewhere else.
func TestHTTPCloudflareClient_aRedirect_isRefused(t *testing.T) {
	t.Parallel()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://elsewhere.example/", http.StatusFound)
	}))
	defer server.Close()

	client := NewHTTPCloudflareClient(server.Client())
	if _, err := client.Items(t.Context(), cloudflareTestConfig(server.URL),
		SecretValue{value: "CANARY-token"}); err == nil {
		t.Fatal("a redirect was followed")
	}
}

// The cursor pagination is walked to the end, and a list that pages past the
// managed size is an error, not an endless read.
func TestHTTPCloudflareClient_items_walksTheCursor(t *testing.T) {
	t.Parallel()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("cursor") == "" {
			fmt.Fprint(w, `{"success":true,"result":[{"id":"a","ip":"192.0.2.1"}],"result_info":{"cursors":{"after":"c2"}}}`)
			return
		}
		fmt.Fprint(w, `{"success":true,"result":[{"id":"b","ip":"192.0.2.2"}],"result_info":{"cursors":{"after":""}}}`)
	}))
	defer server.Close()

	client := NewHTTPCloudflareClient(server.Client())
	items, err := client.Items(t.Context(), cloudflareTestConfig(server.URL),
		SecretValue{value: "CANARY-token"})
	if err != nil {
		t.Fatalf("Items: %v", err)
	}
	if len(items) != 2 || items[1].IP != "192.0.2.2" {
		t.Fatalf("items = %+v", items)
	}
}

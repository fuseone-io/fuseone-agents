package connectortools

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The password travels as basic auth and nowhere else; the index lands as a
// path segment on _search; a refused status carries no remote body.
func TestHTTPElasticsearchClient_search_authAndShape(t *testing.T) {
	t.Parallel()
	var path, auth, body string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, auth = r.URL.Path, r.Header.Get("Authorization")
		raw := make([]byte, 512)
		n, _ := r.Body.Read(raw)
		body = string(raw[:n])
		fmt.Fprint(w, `{"took":1,"hits":{"total":0}}`)
	}))
	defer server.Close()

	cfg := ElasticsearchConfig{BaseURL: server.URL, Username: "reader", Index: "logs-*"}
	client := NewHTTPElasticsearchClient(server.Client())
	out, err := client.Search(t.Context(), cfg, SecretValue{value: "CANARY-pass"},
		"logs-*", []byte(`{"size":0}`))
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !strings.Contains(string(out), `"took"`) {
		t.Fatalf("out = %s", out)
	}
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("reader:CANARY-pass"))
	if auth != want {
		t.Fatalf("auth = %q", auth)
	}
	if path != "/logs-%2A/_search" && path != "/logs-*/_search" {
		t.Fatalf("path = %q", path)
	}
	if strings.Contains(body, "CANARY") {
		t.Fatalf("body = %q", body)
	}
}

func TestHTTPElasticsearchClient_aFailedStatus_leaksNothing(t *testing.T) {
	t.Parallel()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"error":"secret-shaped remote text"}`)
	}))
	defer server.Close()
	client := NewHTTPElasticsearchClient(server.Client())
	_, err := client.Search(t.Context(),
		ElasticsearchConfig{BaseURL: server.URL, Username: "r", Index: "x"},
		SecretValue{value: "CANARY"}, "x", []byte(`{}`))
	if err == nil || strings.Contains(err.Error(), "secret-shaped") ||
		strings.Contains(err.Error(), "CANARY") {
		t.Fatalf("err = %v", err)
	}
}

func TestHTTPElasticsearchClient_aRedirect_isRefused(t *testing.T) {
	t.Parallel()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://elsewhere.example/", http.StatusFound)
	}))
	defer server.Close()
	client := NewHTTPElasticsearchClient(server.Client())
	if _, err := client.Search(t.Context(),
		ElasticsearchConfig{BaseURL: server.URL, Username: "r", Index: "x"},
		SecretValue{value: "p"}, "x", []byte(`{}`)); err == nil {
		t.Fatal("a redirect was followed")
	}
}

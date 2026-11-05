package connectortools

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func evidenceHTTPClient(t *testing.T, handler http.HandlerFunc) (*HTTPEvidenceClient, EvidenceConfig) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return NewHTTPEvidenceClient(server.Client()), EvidenceConfig{
		BaseURL: server.URL, AuthHeader: "Authorization", AuthPrefix: "ApiToken ",
	}
}

func TestEvidenceHTTP_sendsTheHeaderAndTheQueryWithGET(t *testing.T) {
	t.Parallel()
	client, cfg := evidenceHTTPClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/events" || r.URL.Query().Get("q") != "a b" {
			t.Errorf("request = %s %s", r.Method, r.URL)
		}
		if r.Header.Get("Authorization") != "ApiToken s3cret" {
			t.Errorf("header = %q", r.Header.Get("Authorization"))
		}
		_, _ = io.WriteString(w, `{"ok":1}`)
	})
	body, err := client.Fetch(context.Background(), cfg, SecretValue{value: "s3cret"},
		EvidenceRequest{Path: "/events", Query: url.Values{"q": {"a b"}}})
	if err != nil || string(body) != `{"ok":1}` {
		t.Fatalf("body = %s, err = %v", body, err)
	}
}

// A source's error body can quote the query, and an error string reaches logs.
func TestEvidenceHTTP_errorCarriesTheStatusAndNothingElse(t *testing.T) {
	t.Parallel()
	client, cfg := evidenceHTTPClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, "CANARY-body")
	})
	_, err := client.Fetch(context.Background(), cfg, SecretValue{value: "s"}, EvidenceRequest{Path: "/x"})
	var remote evidenceRemoteError
	if !errors.As(err, &remote) || remote.status != http.StatusForbidden || strings.Contains(err.Error(), "CANARY") {
		t.Fatalf("err = %v", err)
	}
}

// A custom header is not stripped across hosts the way Authorization is, so the
// token would reach whoever the source points at. The second server proves it.
func TestEvidenceHTTP_refusesRedirects(t *testing.T) {
	t.Parallel()
	var followed atomic.Int32
	elsewhere := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		followed.Add(1)
	}))
	t.Cleanup(elsewhere.Close)
	client, cfg := evidenceHTTPClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL, http.StatusFound)
	})
	_, err := client.Fetch(context.Background(), cfg, SecretValue{value: "s"}, EvidenceRequest{Path: "/x"})
	if err == nil || followed.Load() != 0 {
		t.Fatalf("err = %v, followed %d times", err, followed.Load())
	}
}

// A prefix of a page would parse as nothing or, worse, as fewer rows.
func TestEvidenceHTTP_oversizedPage_isAnErrorNotAPrefix(t *testing.T) {
	t.Parallel()
	client, cfg := evidenceHTTPClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, maxEvidencePageBytes+1))
	})
	if _, err := client.Fetch(context.Background(), cfg, SecretValue{value: "s"}, EvidenceRequest{Path: "/x"}); err == nil {
		t.Fatal("an oversized page was accepted")
	}
}

func TestEvidenceHTTP_noCredential_neverCalls(t *testing.T) {
	t.Parallel()
	client, cfg := evidenceHTTPClient(t, func(http.ResponseWriter, *http.Request) {
		t.Error("the source was called without a credential")
	})
	if _, err := client.Fetch(context.Background(), cfg, SecretValue{}, EvidenceRequest{Path: "/x"}); err == nil {
		t.Fatal("no error")
	}
}

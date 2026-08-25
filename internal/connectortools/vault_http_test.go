package connectortools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPVaultClient_writesKVv2SecretWithoutReturningSecretMaterial(t *testing.T) {
	t.Parallel()

	var got struct {
		Path      string
		Token     string
		Namespace string
		Data      map[string]string
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Path = r.URL.Path
		got.Token = r.Header.Get("X-Vault-Token")
		got.Namespace = r.Header.Get("X-Vault-Namespace")
		var body struct {
			Data map[string]string `json:"data"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		got.Data = body.Data
		_, _ = w.Write([]byte(`{"data":{"version":3}}`))
	}))
	defer server.Close()

	client := NewHTTPVaultClient(server.Client())
	result, err := client.WriteSecret(context.Background(), VaultConfig{
		Address:   server.URL,
		Mount:     "secret",
		Namespace: "tenant-a",
	}, "vault-token", "certs/app", map[string]VaultSecretField{
		"private_key": {Value: []byte("secret key material")},
	})
	if err != nil {
		t.Fatalf("WriteSecret: %v", err)
	}
	if result.Version != 3 {
		t.Fatalf("version = %d, want 3", result.Version)
	}
	if got.Path != "/v1/secret/data/certs/app" {
		t.Fatalf("path = %q", got.Path)
	}
	if got.Token != "vault-token" || got.Namespace != "tenant-a" {
		t.Fatalf("headers token=%q namespace=%q", got.Token, got.Namespace)
	}
	if got.Data["private_key"] != "secret key material" {
		t.Fatalf("data = %#v", got.Data)
	}
}

func TestHTTPVaultClient_readsMetadataAndSortsLiveVersions(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/kv/metadata/certs/app" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{
			"data": {
				"current_version": 4,
				"versions": {
					"4": {},
					"2": {},
					"3": {"destroyed": true}
				}
			}
		}`))
	}))
	defer server.Close()

	client := NewHTTPVaultClient(server.Client())
	got, err := client.ReadMetadata(context.Background(), VaultConfig{
		Address: server.URL,
		Mount:   "kv",
	}, "vault-token", "certs/app")
	if err != nil {
		t.Fatalf("ReadMetadata: %v", err)
	}
	if got.CurrentVersion != 4 || len(got.Versions) != 2 || got.Versions[0] != 2 || got.Versions[1] != 4 {
		t.Fatalf("metadata = %+v", got)
	}
}

func TestHTTPVaultClient_statusErrorsDoNotExposeVaultResponseBodies(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errors":["policy reveals secret-name"]}`))
	}))
	defer server.Close()

	client := NewHTTPVaultClient(server.Client())
	_, err := client.ReadMetadata(context.Background(), VaultConfig{
		Address: server.URL,
		Mount:   "kv",
	}, "vault-token", "certs/app")
	if err == nil {
		t.Fatal("ReadMetadata succeeded, want status error")
	}
	if strings.Contains(err.Error(), "secret-name") || strings.Contains(err.Error(), "policy") {
		t.Fatalf("error exposed Vault body: %v", err)
	}
	if !strings.Contains(err.Error(), "status 403") {
		t.Fatalf("error = %v, want status code", err)
	}
}

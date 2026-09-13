package connectortools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVaultKVSecret_readsOnlyTheNamedFieldIntoARedactedValue(t *testing.T) {
	const (
		accessToken = "GRAVITEE-TOKEN-%-CANARY"
		vaultToken  = "VAULT-TOKEN-CANARY"
		unused      = "UNRELATED-SECRET-CANARY"
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/secret/data/integrations/gravitee/prod" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("X-Vault-Token"); got != vaultToken {
			t.Errorf("vault token = %q", got)
		}
		_, _ = fmt.Fprintf(w, `{"data":{"data":{"access_token":%q,"unused":%q}}}`,
			accessToken, unused)
	}))
	t.Cleanup(server.Close)

	secret, err := NewHTTPVaultClient(server.Client()).ReadSecretField(
		context.Background(), VaultConfig{Address: server.URL, Mount: "secret"},
		vaultToken, "integrations/gravitee/prod", "access_token")
	if err != nil {
		t.Fatalf("ReadSecretField: %v", err)
	}
	if secret.value != accessToken {
		t.Fatalf("secret did not carry the selected field")
	}
	rendered, _ := json.Marshal(struct{ Secret SecretValue }{secret})
	for _, view := range []string{fmt.Sprint(secret), fmt.Sprintf("%+v", secret), string(rendered)} {
		if strings.Contains(view, accessToken) || strings.Contains(view, unused) {
			t.Fatalf("secret escaped through %q", view)
		}
	}
}

func TestVaultKVSecret_refusesMissingOrNonStringFieldsWithoutRepeatingTheBody(t *testing.T) {
	const marker = "VAULT-BODY-SECRET-CANARY"
	cases := map[string]string{
		"missing":    `{"data":{"data":{"another":"` + marker + `"}}}`,
		"non string": `{"data":{"data":{"access_token":{"secret":"` + marker + `"}}}}`,
		"blank":      `{"data":{"data":{"access_token":"   "}}}`,
	}
	for name, response := range cases {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(response))
			}))
			defer server.Close()
			_, err := NewHTTPVaultClient(server.Client()).ReadSecretField(
				context.Background(), VaultConfig{Address: server.URL, Mount: "secret"},
				"vault-token", "integrations/gravitee/prod", "access_token")
			if err == nil || strings.Contains(err.Error(), marker) {
				t.Fatalf("ReadSecretField err = %v", err)
			}
		})
	}
}

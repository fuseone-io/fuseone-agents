package known

import (
	"strings"
	"testing"
	"testing/fstest"
)

/*
A recipe is keyed by the local server name.

As the catalogue grows, two files with the same server name are not two
recipes: the second replaces the first in the map, and the page shows one of
them as though the other never existed. That is a build-time error, not a
runtime choice.
*/
func TestLoad_twoEntriesWithTheSameServer_areRefused(t *testing.T) {
	t.Parallel()

	_, err := load(fstest.MapFS{
		"servers/one.yaml": {Data: []byte(minimal("crm", "One"))},
		"servers/two.yaml": {Data: []byte(minimal("crm", "Two"))},
	})
	if err == nil || !strings.Contains(err.Error(), `duplicates server "crm"`) {
		t.Fatalf("load = %v, want duplicate server refusal", err)
	}
}

/*
Published is an assertion, not a default.

Missing status used to render as a maintained publisher recipe, which is the
same class of bug as a missing tool classification rendering as read. Silence
is not a judgement the console can show to an operator.
*/
func TestLoad_anEntryWithoutStatus_isRefused(t *testing.T) {
	t.Parallel()

	_, err := load(fstest.MapFS{
		"servers/crm.yaml": {Data: []byte(strings.ReplaceAll(minimal("crm", "CRM"), "status: published\n", ""))},
	})
	if err == nil || !strings.Contains(err.Error(), "does not say whether") {
		t.Fatalf("load = %v, want missing status refusal", err)
	}
}

/*
A credential is not a shape.

OAuth, bearer tokens, Basic auth, DSNs and generated config files fail in
different places and carry different authority. The catalogue must not reduce
all of them to a single "paste token" hint by omitting the structured auth
facts.
*/
func TestLoad_anEntryWithCredentialAndNoAuthMode_isRefused(t *testing.T) {
	t.Parallel()

	_, err := load(fstest.MapFS{
		"servers/crm.yaml": {Data: []byte(minimal("crm", "CRM") + "config: [credential]\n")},
	})
	if err == nil || !strings.Contains(err.Error(), "does not say what kind") {
		t.Fatalf("load = %v, want missing auth mode refusal", err)
	}
}

func TestLoad_anEntryWithCredentialAndOnlyNonCredentialAuthModes_isRefused(t *testing.T) {
	t.Parallel()

	_, err := load(fstest.MapFS{
		"servers/crm.yaml": {Data: []byte(minimal("crm", "CRM") + `config: [credential]
authModes:
  - type: none
    principal: none
`)},
	})
	if err == nil || !strings.Contains(err.Error(), "only names non-credential") {
		t.Fatalf("load = %v, want contradictory auth mode refusal", err)
	}
}

func minimal(server, title string) string {
	return "server: " + server + `
title: ` + title + `
category: data
publisher: Example
docs: https://example.com
docsFrom: publisher
provenance: documentation
status: published
note: test
`
}

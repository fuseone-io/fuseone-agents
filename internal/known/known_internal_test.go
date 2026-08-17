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

func minimal(server, title string) string {
	return "server: " + server + `
title: ` + title + `
category: data
publisher: Example
docs: https://example.com
docsFrom: publisher
provenance: documentation
note: test
`
}

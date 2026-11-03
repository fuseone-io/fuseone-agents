package main

import (
	"errors"
	"strings"
	"testing"
)

const validBots = `bots:
  triage: t-app[bot]
  architect: a-app[bot]
  developer: d-app[bot]
  reviewer: r-app[bot]
  qa: q-app[bot]
`

func TestParseBots_placeholderLeft_reportRefuses(t *testing.T) {
	t.Parallel()
	doc := strings.Replace(validBots, "a-app[bot]", "PLACEHOLDER-architect-app-slug[bot]", 1)
	m, err := parseBots([]byte(doc))
	if err != nil {
		t.Fatalf("a placeholder is a valid map for the canary: %v", err)
	}
	err = m.ready()
	if !errors.Is(err, errPlaceholder) || !strings.Contains(err.Error(), "architect") {
		t.Fatalf("ready() = %v, want errPlaceholder naming architect", err)
	}
}

func TestParseBots_shippedMap_parsesAndNamesItsPlaceholders(t *testing.T) {
	t.Parallel()
	m, err := parseBots(shippedBots)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(m.placeholders(), ","); got != "architect,developer" {
		t.Fatalf("placeholders = %q, want the two Apps not found on GitHub", got)
	}
}

func TestParseBots_malformed_refused(t *testing.T) {
	t.Parallel()
	for name, doc := range map[string]string{
		"unknown role":   validBots + "  merger: m-app[bot]\n",
		"missing role":   strings.Replace(validBots, "  qa: q-app[bot]\n", "", 1),
		"not a bot":      strings.Replace(validBots, "q-app[bot]", "q-app", 1),
		"shared login":   strings.Replace(validBots, "q-app[bot]", "r-app[bot]", 1),
		"not yaml":       "bots: [",
		"unknown field":  validBots + "people: []\n",
		"empty login":    strings.Replace(validBots, "q-app[bot]", `""`, 1),
		"no bots at all": "bots: {}\n",
	} {
		if _, err := parseBots([]byte(doc)); err == nil {
			t.Errorf("%s: parsed, want an error", name)
		}
	}
}

func TestClassify_actorKinds_neverGuessed(t *testing.T) {
	t.Parallel()
	m, err := parseBots([]byte(validBots))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		actor Actor
		want  Owner
	}{
		{"mapped bot", Actor{"Bot", "r-app[bot]"}, OwnerReviewer},
		{"a person", Actor{"User", "alice"}, OwnerPerson},
		{"a user named like a bot", Actor{"User", "r-app[bot]"}, OwnerUnmapped},
		{"unknown bot", Actor{"Bot", "r-app-2[bot]"}, OwnerUnmapped},
		{"near miss in case", Actor{"Bot", "R-app[bot]"}, OwnerUnmapped},
		{"ghost", Actor{}, OwnerUnmapped},
		{"mannequin", Actor{"Mannequin", "old"}, OwnerUnmapped},
	} {
		if got := m.classify(tc.actor); got != tc.want {
			t.Errorf("%s: classify = %q, want %q", tc.name, got, tc.want)
		}
	}
}

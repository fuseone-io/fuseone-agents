package domain_test

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

func TestToolDedupeValidate(t *testing.T) {
	tests := []struct {
		name string
		in   domain.ToolDedupe
		ok   bool
	}{
		{name: "empty is disabled", in: domain.ToolDedupe{}, ok: true},
		{name: "stable dotted paths", in: domain.ToolDedupe{
			WindowSeconds: 3600,
			ArgPaths:      []string{"repository.owner", "repository.name", "title"},
		}, ok: true},
		{name: "window is required", in: domain.ToolDedupe{ArgPaths: []string{"title"}}},
		{name: "path is required", in: domain.ToolDedupe{WindowSeconds: 60}},
		{name: "slash is not a field path", in: domain.ToolDedupe{
			WindowSeconds: 60,
			ArgPaths:      []string{"repository/name"},
		}},
		{name: "wildcards are not semantic keys", in: domain.ToolDedupe{
			WindowSeconds: 60,
			ArgPaths:      []string{"labels.*"},
		}},
		{name: "duplicates are refused", in: domain.ToolDedupe{
			WindowSeconds: 60,
			ArgPaths:      []string{"title", "title"},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.in.Validate()
			if tt.ok && err != nil {
				t.Fatalf("Validate: %v", err)
			}
			if !tt.ok && err == nil {
				t.Fatal("Validate accepted an invalid dedupe declaration")
			}
		})
	}
}

func TestToolDedupeClone(t *testing.T) {
	original := domain.ToolDedupe{WindowSeconds: 60, ArgPaths: []string{"title"}}
	cloned := original.Clone()
	cloned.ArgPaths[0] = "body"

	if original.ArgPaths[0] != "title" {
		t.Fatalf("Clone shares ArgPaths with the original: %+v", original)
	}
}

func TestToolDedupeFingerprint_usesOnlyDeclaredFields(t *testing.T) {
	dedupe := domain.ToolDedupe{WindowSeconds: 3600, ArgPaths: []string{
		"title", "repository.owner", "repository.name",
	}}
	first, err := dedupe.Fingerprint([]byte(`{
		"repository": {"owner": "fuseone", "name": "agents"},
		"title": "outage",
		"correlation_id": "a"
	}`))
	if err != nil {
		t.Fatalf("Fingerprint(first): %v", err)
	}
	second, err := dedupe.Fingerprint([]byte(`{
		"correlation_id": "b",
		"title": "outage",
		"repository": {"name": "agents", "owner": "fuseone"}
	}`))
	if err != nil {
		t.Fatalf("Fingerprint(second): %v", err)
	}
	if first != second {
		t.Fatalf("Fingerprint changed with JSON order or undeclared fields: %q != %q", first, second)
	}
}

func TestToolDedupeFingerprint_pathOrderIsNotPartOfTheKey(t *testing.T) {
	args := []byte(`{"repository":{"owner":"fuseone","name":"agents"},"title":"outage"}`)
	left, err := (domain.ToolDedupe{WindowSeconds: 3600, ArgPaths: []string{
		"title", "repository.name", "repository.owner",
	}}).Fingerprint(args)
	if err != nil {
		t.Fatalf("Fingerprint(left): %v", err)
	}
	right, err := (domain.ToolDedupe{WindowSeconds: 3600, ArgPaths: []string{
		"repository.owner", "title", "repository.name",
	}}).Fingerprint(args)
	if err != nil {
		t.Fatalf("Fingerprint(right): %v", err)
	}
	if left != right {
		t.Fatalf("Fingerprint changed with declaration order: %q != %q", left, right)
	}
}

func TestToolDedupeFingerprint_missingDeclaredFieldFailsClosed(t *testing.T) {
	_, err := (domain.ToolDedupe{
		WindowSeconds: 60,
		ArgPaths:      []string{"repository.owner", "repository.name"},
	}).Fingerprint([]byte(`{"repository":{"owner":"fuseone"}}`))
	if err == nil {
		t.Fatal("Fingerprint accepted args missing a declared field")
	}
}

func TestToolDedupeFingerprint_preservesLargeJSONNumbers(t *testing.T) {
	dedupe := domain.ToolDedupe{WindowSeconds: 60, ArgPaths: []string{"account_id"}}
	first, err := dedupe.Fingerprint([]byte(`{"account_id":9007199254740992}`))
	if err != nil {
		t.Fatalf("Fingerprint(first): %v", err)
	}
	second, err := dedupe.Fingerprint([]byte(`{"account_id":9007199254740993}`))
	if err != nil {
		t.Fatalf("Fingerprint(second): %v", err)
	}
	if first == second {
		t.Fatalf("Fingerprint collided for adjacent integers above 2^53: %q", first)
	}
}

func TestToolDedupeFingerprint_preservesNumericLiterals(t *testing.T) {
	dedupe := domain.ToolDedupe{WindowSeconds: 60, ArgPaths: []string{"quantity"}}
	integer, err := dedupe.Fingerprint([]byte(`{"quantity":1}`))
	if err != nil {
		t.Fatalf("Fingerprint(integer): %v", err)
	}
	decimal, err := dedupe.Fingerprint([]byte(`{"quantity":1.0}`))
	if err != nil {
		t.Fatalf("Fingerprint(decimal): %v", err)
	}
	if integer == decimal {
		t.Fatalf("Fingerprint collapsed distinct JSON number literals: %q", integer)
	}
}

func TestToolDedupeFingerprint_refusesTrailingJSON(t *testing.T) {
	_, err := (domain.ToolDedupe{
		WindowSeconds: 60,
		ArgPaths:      []string{"title"},
	}).Fingerprint([]byte(`{"title":"one"} {"title":"two"}`))
	if err == nil {
		t.Fatal("Fingerprint accepted more than one JSON value")
	}
}

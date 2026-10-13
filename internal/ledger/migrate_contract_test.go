package ledger

import (
	"io/fs"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestLegacyMigrationDigests_matchEmbeddedHistory(t *testing.T) {
	files, err := fs.Glob(migrationFS, "sql/*.sql")
	if err != nil {
		t.Fatalf("list migrations: %v", err)
	}
	migrations, err := embeddedMigrations(migrationFS, files)
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}

	expected := make(map[string]string)
	for _, migration := range migrations {
		if migration.version <= migrationDigestBoundary {
			expected[migration.version] = migration.digest
		}
	}
	if !reflect.DeepEqual(expected, legacyMigrationDigests) {
		missing := make([]string, 0)
		unexpected := make([]string, 0)
		for version := range expected {
			if _, ok := legacyMigrationDigests[version]; !ok {
				missing = append(missing, version)
			}
		}
		for version := range legacyMigrationDigests {
			if _, ok := expected[version]; !ok {
				unexpected = append(unexpected, version)
			}
		}
		sort.Strings(missing)
		sort.Strings(unexpected)
		t.Fatalf("legacy digest map differs from embedded history: missing=%v unexpected=%v", missing, unexpected)
	}
}

func TestValidateApplied_refusesChangedMigration(t *testing.T) {
	const version = "0081_inbox_run_index"
	migrations := []embeddedMigration{{version: version, body: []byte("changed"), digest: digestBytes([]byte("changed"))}}
	recorded := "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"

	err := validateApplied(migrations, map[string]appliedMigration{
		version: {digest: &recorded},
	})
	if err == nil {
		t.Fatal("validateApplied accepted a changed migration")
	}
	message := err.Error()
	if !strings.Contains(message, version) ||
		!strings.Contains(message, recorded) ||
		!strings.Contains(message, migrations[0].digest) {
		t.Fatalf("error = %q, want version and both digests", message)
	}
	if strings.Contains(message, "changed") {
		t.Fatalf("error exposed migration SQL: %q", message)
	}
}

func TestValidateApplied_refusesUnknownMigrationInsideKnownHistory(t *testing.T) {
	recorded := "digest"
	err := validateApplied([]embeddedMigration{{version: "0081_inbox_run_index"}}, map[string]appliedMigration{
		"0080_not_known": {digest: &recorded},
	})
	if err == nil || !strings.Contains(err.Error(), "0080_not_known") {
		t.Fatalf("error = %v, want unknown migration refusal", err)
	}
}

func TestValidateApplied_allowsUnknownMigrationAfterKnownHistory(t *testing.T) {
	recorded := "digest"
	err := validateApplied([]embeddedMigration{{version: "0081_inbox_run_index"}}, map[string]appliedMigration{
		"0082_future": {digest: &recorded},
	})
	if err != nil {
		t.Fatalf("validateApplied refused a future migration during rollback: %v", err)
	}
}

func TestValidateApplied_requiresDigestAfterLegacyBoundary(t *testing.T) {
	err := validateApplied([]embeddedMigration{{version: "0082_future", body: []byte("select 1")}}, map[string]appliedMigration{
		"0082_future": {},
	})
	if err == nil || !strings.Contains(err.Error(), "no recorded digest") {
		t.Fatalf("error = %v, want missing digest refusal", err)
	}
}

func TestDigestBytes_isStableOverExactBytes(t *testing.T) {
	if got, want := digestBytes([]byte("select 1;\n")), "4a45092ccf992ea92250053a80b931b787924ba61648f420555511b84f10ab6c"; got != want {
		t.Fatalf("digest = %q, want %q", got, want)
	}
	if digestBytes([]byte("select 1;\n")) == digestBytes([]byte("select 1;")) {
		t.Fatal("digest ignored the exact bytes")
	}
}

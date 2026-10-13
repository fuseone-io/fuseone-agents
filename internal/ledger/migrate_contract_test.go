package ledger

import (
	"bytes"
	"io"
	"io/fs"
	"log/slog"
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
		mismatched := make([]string, 0)
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
		for version, want := range expected {
			if got, ok := legacyMigrationDigests[version]; ok && got != want {
				mismatched = append(mismatched, version)
			}
		}
		sort.Strings(missing)
		sort.Strings(unexpected)
		sort.Strings(mismatched)
		t.Fatalf("legacy digest map differs from embedded history: missing=%v unexpected=%v mismatched=%v", missing, unexpected, mismatched)
	}
}

func TestValidateApplied_refusesChangedMigration(t *testing.T) {
	const version = "0081_inbox_run_index"
	migrations := []embeddedMigration{{version: version, body: []byte("changed"), digest: digestBytes([]byte("changed"))}}
	recorded := "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"

	err := validateApplied(migrations, map[string]appliedMigration{
		version: {digest: &recorded},
	}, discardMigrationLogger())
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
	}, discardMigrationLogger())
	if err == nil || !strings.Contains(err.Error(), "0080_not_known") {
		t.Fatalf("error = %v, want unknown migration refusal", err)
	}
}

func TestValidateApplied_allowsUnknownMigrationAfterKnownHistory(t *testing.T) {
	recorded := "digest"
	knownBody := []byte("known")
	knownDigest := digestBytes(knownBody)
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	err := validateApplied([]embeddedMigration{{version: "0081_inbox_run_index", body: knownBody, digest: knownDigest}}, map[string]appliedMigration{
		"0081_inbox_run_index": {digest: &knownDigest},
		"0082_future":          {digest: &recorded},
	}, logger)
	if err != nil {
		t.Fatalf("validateApplied refused a future migration during rollback: %v", err)
	}
	if !strings.Contains(logs.String(), "newer release") || !strings.Contains(logs.String(), "0082_future") {
		t.Fatalf("warning = %q, want future migration warning", logs.String())
	}
}

func TestValidateApplied_refusesFutureMigrationWhenWorkRemainsBelowIt(t *testing.T) {
	recorded := "digest"
	err := validateApplied([]embeddedMigration{
		{version: "0081_inbox_run_index"},
		{version: "0082_pending"},
	}, map[string]appliedMigration{
		"0083_future": {digest: &recorded},
	}, discardMigrationLogger())
	if err == nil || !strings.Contains(err.Error(), "0083_future") {
		t.Fatalf("error = %v, want divergent future migration refusal", err)
	}
}

func TestValidateApplied_refusesMalformedAndSameNumberUnknownMigrations(t *testing.T) {
	known := []embeddedMigration{{version: "0081_inbox_run_index"}}
	for _, version := range []string{"0081_other_name", "zzz_backfill"} {
		recorded := "digest"
		err := validateApplied(known, map[string]appliedMigration{version: {digest: &recorded}}, discardMigrationLogger())
		if err == nil || !strings.Contains(err.Error(), version) {
			t.Errorf("error for %s = %v, want refusal", version, err)
		}
	}
}

func TestValidateApplied_malformedEmbeddedVersionDoesNotDisarmInsideHistoryRefusal(t *testing.T) {
	// A letter-prefixed embedded name sorts after every NNNN_ file, so if the
	// max-known scan mishandles it the ceiling collapses to zero and an unknown
	// row INSIDE known history slips through the skew tolerance.
	knownBody := []byte("known")
	knownDigest := digestBytes(knownBody)
	divergent := "digest"
	err := validateApplied([]embeddedMigration{
		{version: "0081_inbox_run_index", body: knownBody, digest: knownDigest},
		{version: "zzz_seed", body: knownBody, digest: knownDigest},
	}, map[string]appliedMigration{
		"0081_inbox_run_index": {digest: &knownDigest},
		"zzz_seed":             {digest: &knownDigest},
		"0079_divergent":       {digest: &divergent},
	}, discardMigrationLogger())
	if err == nil || !strings.Contains(err.Error(), "0079_divergent") {
		t.Fatalf("error = %v, want inside-history refusal despite malformed embedded version", err)
	}
}

func TestValidateApplied_requiresDigestAfterLegacyBoundary(t *testing.T) {
	err := validateApplied([]embeddedMigration{{version: "0082_future", body: []byte("select 1")}}, map[string]appliedMigration{
		"0082_future": {},
	}, discardMigrationLogger())
	if err == nil || !strings.Contains(err.Error(), "no recorded digest") {
		t.Fatalf("error = %v, want missing digest refusal", err)
	}
}

func discardMigrationLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestDigestBytes_isStableOverExactBytes(t *testing.T) {
	if got, want := digestBytes([]byte("select 1;\n")), "4a45092ccf992ea92250053a80b931b787924ba61648f420555511b84f10ab6c"; got != want {
		t.Fatalf("digest = %q, want %q", got, want)
	}
	if digestBytes([]byte("select 1;\n")) == digestBytes([]byte("select 1;")) {
		t.Fatal("digest ignored the exact bytes")
	}
}

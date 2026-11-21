package regression_test

import (
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/regression"
)

/*
A case keeps the world it was corrected in (NT-018 phase 5): the answers the author gave in a
rehearsal, named by one object, the world. A version records that object by reference and
digest beside the input and the expectations, so a battery replays exactly what was recorded,
and a case with no world is the line it has always been.
*/

// keptWorld stores a world object for a case as content of its own kind, owned by the case.
func keptWorld(t *testing.T, store *ledger.Content, id string, body string) (ref, digest string) {
	t.Helper()
	ref, err := store.PutFor(t.Context(), regression.WorldKind, "suporte/"+id, 1, []byte(body))
	if err != nil {
		t.Fatalf("keep the world of %s: %v", id, err)
	}
	return ref, ref[strings.LastIndex(ref, "/")+1:]
}

func withWorld(c domain.RegressionCase, ref string) domain.RegressionCase {
	c.World = ref
	return c
}

func TestRecord_aCaseWithAWorld_aVersionNamesItByReferenceAndDigest(t *testing.T) {
	pool := activatedCorpus(t)
	store := regression.NewStore(pool)
	ref, short := keptWorld(t, ledger.NewContent(pool), "reg-3", `{"case":"reg-3","entries":[]}`)

	if err := store.Record(t.Context(), withWorld(aCase("reg-3", never("x")), ref)); err != nil {
		t.Fatalf("Record: %v", err)
	}
	var gotRef, gotDigest string
	if err := pool.QueryRow(t.Context(), `
		select world_ref, world_digest from dataset_version_cases
		where dataset_id = 'default:suporte' and version = 2 and case_id = 'reg-3'`).Scan(&gotRef, &gotDigest); err != nil {
		t.Fatalf("read the version's case: %v", err)
	}
	if gotRef != ref || !strings.HasPrefix(gotDigest, short) {
		t.Errorf("version names world %q digest %q, want %q and the object's digest", gotRef, gotDigest, ref)
	}
	cases, err := store.AtVersion(t.Context(), "suporte", 2)
	if err != nil {
		t.Fatalf("AtVersion: %v", err)
	}
	if got := worldOf(cases, "reg-3"); got != ref {
		t.Errorf("AtVersion reads world %q, want %q", got, ref)
	}
}

func TestRecord_changingOnlyTheWorld_writesANewVersion(t *testing.T) {
	pool := activatedCorpus(t)
	store := regression.NewStore(pool)
	content := ledger.NewContent(pool)
	first, _ := keptWorld(t, content, "reg-3", `{"case":"reg-3","entries":[1]}`)
	second, _ := keptWorld(t, content, "reg-3", `{"case":"reg-3","entries":[2]}`)

	if err := store.Record(t.Context(), withWorld(aCase("reg-3", never("x")), first)); err != nil {
		t.Fatalf("Record: %v", err)
	}
	_, before := latestVersion(t, pool)
	if err := store.Record(t.Context(), withWorld(aCase("reg-3", never("x")), second)); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if version, after := latestVersion(t, pool); version != 3 || after == before {
		t.Errorf("version = %d, digest %s -> %s, want a new version with a new digest", version, before, after)
	}
}

// A case with no world reads as one: no reference, and the version's line is the one a
// binary before worlds wrote, so the first write after this migration records nothing new.
func TestAtVersion_aCaseWithNoWorld_readsNone(t *testing.T) {
	pool := activatedCorpus(t)
	cases, err := regression.NewStore(pool).AtVersion(t.Context(), "suporte", 1)
	if err != nil {
		t.Fatalf("AtVersion: %v", err)
	}
	for _, c := range cases {
		if c.World != "" {
			t.Errorf("%s reads world %q, want none", c.ID, c.World)
		}
	}
}

// Only a writer that records versions may set a case's world: one that does not would move
// what a battery replays under every version that names the case.
func TestUndeclaredWriter_settingAWorld_refusedByTheDatabase(t *testing.T) {
	pool := activatedCorpus(t)
	_, err := pool.Exec(t.Context(), `update regression_cases set world_ref = 'case_world://suporte/reg-1/1/x' where case_id = 'reg-1'`)
	if err == nil || !strings.Contains(err.Error(), "does not record dataset versions") {
		t.Errorf("err = %v, want the undeclared write refused", err)
	}
}

// A binary from before worlds still writes a version: the columns it does not name default to
// no world, which is what its cases are.
func TestPreviousBinary_insertingAVersionCase_stillAccepted(t *testing.T) {
	pool := activatedCorpus(t)
	if _, err := pool.Exec(t.Context(), `
		insert into dataset_versions (dataset_id, version, digest, created_by)
		values ('default:suporte', 9, 'd', 'usr_ana')`); err != nil {
		t.Fatalf("insert a version: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `
		insert into dataset_version_cases
			(dataset_id, version, case_id, input_ref, input_digest, expectations_ref, expectations_digest, from_run)
		values ('default:suporte', 9, 'reg-9', 'i', 'id', 'e', 'ed', '')`); err != nil {
		t.Errorf("the previous binary's insert: %v", err)
	}
}

func worldOf(cases []domain.RegressionCase, id string) string {
	for _, c := range cases {
		if c.ID == id {
			return c.World
		}
	}
	return "<absent>"
}

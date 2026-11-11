package spec_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/testdb"
)

/*
A version's identity is claimed against the full digest of its bytes (NT-018).

The version id is 48 bits of the SHA-256. Insert-or-nothing on that id alone
means a second definition landing on the same id is silently answered with the
first one's row: the publication judges one text and makes the other current.
These tests are the claim that closes it, for both writers of agent_specs.
*/

// writers are the two ways a version reaches agent_specs: the publisher the
// console and API use, and the registry the disk sync calls directly.
var writers = []struct {
	name    string
	publish func(ctx context.Context, pool *pgxpool.Pool, s spec.Spec) error
}{
	{"publisher", func(ctx context.Context, pool *pgxpool.Pool, s spec.Spec) error {
		_, err := spec.NewPublisher(pool, fixedClock{}).Publish(ctx,
			spec.Publication{Spec: s, By: "usr_ana", Company: "acme"})
		return err
	}},
	{"registry", func(ctx context.Context, pool *pgxpool.Pool, s spec.Spec) error {
		return spec.NewRegistry(pool).Publish(ctx, s, "worker", "acme")
	}},
}

func TestParse_digestIsTheSha256OfTheExactBytes(t *testing.T) {
	t.Parallel()
	for _, one := range []struct{ name, bytes string }{
		{"lf", definition},
		// CRLF parses to the same definition and is different bytes, which is
		// why the digest cannot be recomputed from a Spec.
		{"crlf", strings.ReplaceAll(definition, "\n", "\r\n")},
	} {
		t.Run(one.name, func(t *testing.T) {
			t.Parallel()
			s, err := spec.Parse("triage.agent.md", []byte(one.bytes))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			sum := sha256.Sum256([]byte(one.bytes))
			if want := hex.EncodeToString(sum[:]); s.Digest != want {
				t.Errorf("Digest = %q, want %q", s.Digest, want)
			}
			if string(s.Version) != "v"+s.Digest[:12] {
				t.Errorf("Version %q is not the digest's prefix %q", s.Version, s.Digest[:12])
			}
		})
	}
}

func TestPublish_aSpecThatDoesNotCarryItsBytesDigest_isRefused(t *testing.T) {
	for _, w := range writers {
		t.Run(w.name, func(t *testing.T) {
			pool := openSpecPool(t)
			ctx := context.Background()
			s := published(t, definition)
			other := published(t, definition+"\nAnd say thanks.\n")

			for _, bad := range []struct{ name, digest string }{
				{"no digest", ""},
				// Well formed, and of other bytes: the prefix check alone refuses it.
				{"another definition's digest", other.Digest},
			} {
				forged := s
				forged.Digest = bad.digest
				if err := w.publish(ctx, pool, forged); !errors.Is(err, spec.ErrDigestMissing) {
					t.Errorf("%s: err = %v, want ErrDigestMissing", bad.name, err)
				}
			}
			if n := countRows(t, pool, `select count(*) from agent_specs`); n != 0 {
				t.Errorf("agent_specs has %d rows, want none", n)
			}
		})
	}
}

func TestPublish_claimsTheFullDigestAndPresentsIt(t *testing.T) {
	for _, w := range writers {
		t.Run(w.name, func(t *testing.T) {
			pool := openSpecPool(t)
			ctx := context.Background()
			s := published(t, definition)

			if err := w.publish(ctx, pool, s); err != nil {
				t.Fatalf("publish: %v", err)
			}
			var claimed, presented string
			if err := pool.QueryRow(ctx, `
				select c.digest, s.digest from agent_version_digests c
				join agent_specs s using (agent_id, version_id)
				where c.agent_id = $1 and c.version_id = $2`,
				string(s.ID), string(s.Version)).Scan(&claimed, &presented); err != nil {
				t.Fatalf("read the claim: %v", err)
			}
			if claimed != s.Digest || presented != s.Digest {
				t.Errorf("claim %q, row %q, want both %q", claimed, presented, s.Digest)
			}
			// Publishing the same bytes again is what it always was: nothing new.
			if err := w.publish(ctx, pool, s); err != nil {
				t.Errorf("republish of the same bytes: %v", err)
			}
		})
	}
}

func TestPublish_otherBytesUnderAClaimedVersion_isACollision(t *testing.T) {
	for _, w := range writers {
		t.Run(w.name, func(t *testing.T) {
			pool := openSpecPool(t)
			ctx := context.Background()
			first := published(t, definition)
			if err := w.publish(ctx, pool, first); err != nil {
				t.Fatalf("publish: %v", err)
			}

			// A second definition whose 48-bit id collides with the first. The
			// collision cannot be found by search, so it is made: same id, other
			// text, the full digest of that other text behind the same prefix.
			colliding := published(t, definition+"\nNever refund.\n")
			colliding.Version = first.Version
			colliding.Digest = first.Digest[:12] + colliding.Digest[12:]
			if err := w.publish(ctx, pool, colliding); !errors.Is(err, spec.ErrVersionCollision) {
				t.Fatalf("err = %v, want ErrVersionCollision", err)
			}
			if got := instructionsOf(t, pool, first); got != first.Instructions {
				t.Errorf("instructions = %q, want the first definition's", got)
			}
		})
	}
}

func TestPublish_aRowNobodyClaimed_isRefusedAndNeverAdopted(t *testing.T) {
	for _, w := range writers {
		t.Run(w.name, func(t *testing.T) {
			pool := openSpecPool(t)
			ctx := context.Background()
			s := published(t, definition)
			// What the previous binary writes: the row, and no claim.
			plantUnclaimed(t, pool, s)

			if err := w.publish(ctx, pool, s); !errors.Is(err, spec.ErrDigestUnknown) {
				t.Fatalf("err = %v, want ErrDigestUnknown", err)
			}
			if n := countRows(t, pool, `select count(*) from agent_version_digests`); n != 0 {
				t.Errorf("%d claims, want none: the row's bytes were never known", n)
			}
		})
	}
}

func TestPublish_aVersionClaimedWithoutDigest_isRefused(t *testing.T) {
	for _, w := range writers {
		t.Run(w.name, func(t *testing.T) {
			pool := openSpecPool(t)
			ctx := context.Background()
			s := published(t, definition)
			plantUnclaimed(t, pool, s)
			if _, err := pool.Exec(ctx, `
				insert into agent_version_digests (agent_id, version_id, digest)
				values ($1, $2, null)`, string(s.ID), string(s.Version)); err != nil {
				t.Fatalf("plant the null claim: %v", err)
			}

			if err := w.publish(ctx, pool, s); !errors.Is(err, spec.ErrDigestUnknown) {
				t.Fatalf("err = %v, want ErrDigestUnknown", err)
			}
		})
	}
}

func TestVersionClaims_cannotBeChanged(t *testing.T) {
	pool := openSpecPool(t)
	ctx := context.Background()
	s := published(t, definition)
	if err := spec.NewRegistry(pool).Publish(ctx, s, "worker", "acme"); err != nil {
		t.Fatalf("publish: %v", err)
	}
	for _, change := range []string{
		// A well-formed digest, so only the trigger can refuse it, not the check.
		`update agent_version_digests set digest = repeat('a', 64)`,
		`delete from agent_version_digests`,
	} {
		if _, err := pool.Exec(ctx, change); err == nil {
			t.Errorf("%q succeeded, want a claim to be permanent", change)
		}
	}
}

// plantUnclaimed inserts the row exactly as a binary without claims does.
func plantUnclaimed(t *testing.T, pool *pgxpool.Pool, s spec.Spec) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		insert into agent_specs (
			agent_id, version_id, company_id, area_id, name,
			provider, model, effort, tools, budget, triggers,
			instructions, source, published_by, emits, steps,
			memory_learning, approvals
		) values ($1, $2, 'acme', 'cx', 'Ticket triage',
			'openai', 'test-model', '', '{}', '{}'::jsonb, '[]'::jsonb,
			'old text', 'old.agent.md', 'worker', '[]'::jsonb,
			'[]'::jsonb, '{"mode":"off"}'::jsonb, '{}'::jsonb)
		on conflict (agent_id, version_id) do nothing`,
		string(s.ID), string(s.Version)); err != nil {
		t.Fatalf("plant the unclaimed row: %v", err)
	}
}

func countRows(t *testing.T, pool *pgxpool.Pool, query string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), query).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

func instructionsOf(t *testing.T, pool *pgxpool.Pool, s spec.Spec) string {
	t.Helper()
	var text string
	if err := pool.QueryRow(context.Background(), `
		select instructions from agent_specs where agent_id = $1 and version_id = $2`,
		string(s.ID), string(s.Version)).Scan(&text); err != nil {
		t.Fatalf("read instructions: %v", err)
	}
	return text
}

func TestMigration_everyVersionPublishedBeforeClaims_isClaimedWithoutDigest(t *testing.T) {
	pool := testdb.IsolatedThrough(t, "0105_mcp_rediscoveries")
	ctx := context.Background()
	s := published(t, definition)
	plantUnclaimed(t, pool, s)

	if err := ledger.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	var digest *string
	if err := pool.QueryRow(ctx, `
		select digest from agent_version_digests where agent_id = $1 and version_id = $2`,
		string(s.ID), string(s.Version)).Scan(&digest); err != nil {
		t.Fatalf("read the claim: %v", err)
	}
	if digest != nil {
		t.Errorf("digest = %q, want NULL: the old row's bytes were never kept", *digest)
	}
	if err := spec.NewRegistry(pool).Publish(ctx, s, "worker", "acme"); !errors.Is(err, spec.ErrDigestUnknown) {
		t.Errorf("publish over it: err = %v, want ErrDigestUnknown", err)
	}
}

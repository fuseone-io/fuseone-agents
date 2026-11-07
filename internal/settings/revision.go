package settings

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

// unreadableFingerprint stands in for a secret that no longer opens, so such a row still
// has a revision a client can read and name. A key cannot produce it: fingerprints begin
// with fp_.
const unreadableFingerprint = "unreadable"

// Revision names a row by what it configures: its value, whether it is switched on, what
// else a read says about it, and a fingerprint of its secret. Never by who wrote it last,
// or when. The value is canonicalised because Postgres stores jsonb in an order of its own
// and because an absent field and a null one configure the same thing.
func (s *Store) Revision(set Setting, material []byte, extra any) domain.Revision {
	return domain.NewRevision(string(set.Kind), canonicalConfig(set, extra), s.Fingerprint(set, material))
}

// RevisionUnreadable is Revision for a row whose secret will not open.
func (s *Store) RevisionUnreadable(set Setting, extra any) domain.Revision {
	return domain.NewRevision(string(set.Kind), canonicalConfig(set, extra), unreadableFingerprint)
}

func canonicalConfig(set Setting, extra any) []byte {
	encoded, err := json.Marshal(struct {
		Value   any  `json:"value"`
		Enabled bool `json:"enabled"`
		Extra   any  `json:"extra,omitempty"`
	}{canonicalValue(set.Value), set.Enabled, extra})
	if err != nil {
		// Every part came from a decoded document or a fixed type.
		panic("settings: a configuration cannot be encoded: " + err.Error())
	}
	return encoded
}

// canonicalValue decodes a document so encoding it again sorts its keys, keeps its numbers
// exactly as written, and leaves out null. Empty lists stay: for some fields an empty list
// is a decision and an absent one is nobody having made it. A value that does not parse
// stands for itself, so it can still be compared.
func canonicalValue(raw []byte) any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return string(raw)
	}
	return withoutNulls(value)
}

func withoutNulls(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, inner := range typed {
			if inner != nil {
				out[key] = withoutNulls(inner)
			}
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, inner := range typed {
			out[i] = withoutNulls(inner)
		}
		return out
	}
	return value
}

// GuardKey names the lock that holds writers of one row apart.
func GuardKey(set Setting) string { return "fuseone:settings:" + contextFor(set) }

// GuardTx holds this row's writers apart until the transaction ends, including when there
// is no row yet and so nothing a row lock could hold. A statement of its own, before
// anything is read: READ COMMITTED gives each statement a fresh view.
func GuardTx(ctx context.Context, conn DB, set Setting) error {
	if _, err := conn.Exec(ctx, `select pg_advisory_xact_lock(hashtextextended($1, 0))`, GuardKey(set)); err != nil {
		return fmt.Errorf("settings: guard %s/%s: %w", set.Kind, set.Name, err)
	}
	return nil
}

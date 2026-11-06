package settings

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

/*
Reading what belongs to one owner, without credentials and without the rest of
the kind.

Answered from the indexes of migration 0101, whatever scope the rows are at: a
read of one connection has to cost the same however many others exist.
*/

const settingColumns = `scope_kind, company_id, area_id, name, value, secret is not null, enabled, updated_by, updated_at`

// ListByChannel returns the settings of a kind whose value names this channel.
func (s *Store) ListByChannel(ctx context.Context, kind Kind, channelName string) ([]Setting, error) {
	rows, err := s.pool.Query(ctx, `select `+settingColumns+`
		from settings where kind = $1 and value->>'channel' = $2
		order by scope_kind, company_id, area_id, name`, string(kind), channelName)
	if err != nil {
		return nil, fmt.Errorf("settings: list %s of %s: %w", kind, channelName, err)
	}
	return scanSettings(rows, kind)
}

// ListUnderPrefix returns the settings of a kind whose name starts with prefix.
//
// A range rather than LIKE: a prefix holding % or _ would match other rows, and
// a pattern arriving as a parameter cannot use the index. The operators compare
// bytes, which is the order of the index and of the upper bound worked out here.
func (s *Store) ListUnderPrefix(ctx context.Context, kind Kind, prefix string) ([]Setting, error) {
	if prefix == "" {
		return nil, fmt.Errorf("settings: list %s under an empty prefix would be all of them", kind)
	}
	upper := []byte(prefix)
	upper[len(upper)-1]++
	rows, err := s.pool.Query(ctx, `select `+settingColumns+`
		from settings where kind = $1 and name ~>=~ $2 and name ~<~ $3
		order by scope_kind, company_id, area_id, name`, string(kind), prefix, string(upper))
	if err != nil {
		return nil, fmt.Errorf("settings: list %s under %s: %w", kind, prefix, err)
	}
	return scanSettings(rows, kind)
}

func scanSettings(rows pgx.Rows, kind Kind) ([]Setting, error) {
	defer rows.Close()
	var out []Setting
	for rows.Next() {
		var (
			set           = Setting{Kind: kind}
			scopeKind     string
			company, area string
		)
		if err := rows.Scan(&scopeKind, &company, &area, &set.Name, &set.Value,
			&set.HasSecret, &set.Enabled, &set.UpdatedBy, &set.UpdatedAt); err != nil {
			return nil, err
		}
		set.ScopeKind = ScopeKind(scopeKind)
		set.Scope = domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}
		out = append(out, set)
	}
	return out, rows.Err()
}

// Fingerprint names what a row seals, without saying what it is, or nothing when
// there is nothing to name. Bound to the row, as sealing it was.
func (s *Store) Fingerprint(set Setting, material []byte) string {
	if s.vault == nil || len(material) == 0 {
		return ""
	}
	return s.vault.Fingerprint(material, contextFor(set))
}

package audit

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
)

// Postgres reads both trails as one ordered stream.
//
// A union in the database rather than two queries merged in Go: merging pages
// from two sources in the process cannot be paginated correctly — the tenth
// page of a merge is not the merge of two tenth pages — and an audit trail
// that silently drops entries between pages is not one.
type Postgres struct{ pool *pgxpool.Pool }

func NewPostgres(pool *pgxpool.Pool) *Postgres { return &Postgres{pool: pool} }

// verbs maps a ledger step to the past-tense verb the trail reads in. Only
// steps a person would audit are here; the rest are the interpreter's
// bookkeeping and would bury the decisions among them.
const ledgerVerbs = `
	case
		when kind = 'approval_decided' then
			case when payload->>'approved' = 'true' then 'approval.granted' else 'approval.refused' end
		when payload->>'verdict' = '1' then 'gate.allowed'
		when payload->>'verdict' = '2' then 'gate.constrained'
		when payload->>'verdict' = '3' then 'gate.escalated'
		when payload->>'verdict' = '4' then 'gate.blocked'
		else 'gate.decided'
	end`

/*
Read answers one page of the trail, and says where the next one starts.

The cursor is returned rather than derived by the caller, because only this
knows which record each entry came from and therefore which of the two
positions moved. A branch that contributed nothing to a page keeps the position
it arrived with: its rows are still ahead, behind the other record's.
*/
func (p *Postgres) Read(ctx context.Context, filter Filter, limit int) ([]Entry, string, error) {
	query, args := buildTrail(filter, limit)
	if query == "" {
		return nil, "", nil
	}

	rows, err := p.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, "", fmt.Errorf("audit: read trail: %w", err)
	}
	defer rows.Close()

	// Carried forward, not reset. The page may have been filled entirely by
	// one record while the other still has entries behind the boundary.
	next := DecodeCursor(filter.Cursor)
	var out []Entry
	for rows.Next() {
		entry, eventID, err := scanEntry(rows)
		if err != nil {
			return nil, "", err
		}
		advance(&next, entry, eventID)
		out = append(out, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	// Short of the limit means the trail ended, and offering a cursor there
	// would send the reader to an empty page.
	if len(out) < limit {
		return out, "", nil
	}
	return out, next.Encode(), nil
}

// advance moves the position of the record this entry came from.
func advance(c *Cursor, e Entry, eventID int64) {
	if e.Source == SourceAdmin {
		c.Admin = &AdminMark{At: e.At, ID: eventID}
		return
	}
	c.Ledger = &LedgerMark{At: e.At, Run: string(e.RunID), Seq: e.Seq}
}

// scanEntry reads one row, and the administrative row number that orders it.
func scanEntry(rows pgx.Rows) (Entry, int64, error) {
	var (
		entry         Entry
		company, area string
		detail        []byte
		mark          string
	)
	if err := rows.Scan(&entry.At, &entry.Source, &entry.Actor, &entry.Verb, &entry.Target,
		&company, &area, &detail, &entry.RunID, &entry.Seq, &entry.Hash, &mark); err != nil {
		return Entry{}, 0, err
	}
	entry.At = entry.At.UTC()
	entry.Scope = domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}
	if len(detail) > 0 {
		_ = json.Unmarshal(detail, &entry.Detail)
	}
	var eventID int64
	if entry.Source == SourceAdmin {
		_, _ = fmt.Sscanf(mark, "%d", &eventID)
	}
	return entry, eventID, nil
}

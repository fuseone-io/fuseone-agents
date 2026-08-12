package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

// DefaultRetention is what an installation keeps when nobody has said
// otherwise (PRD AU-11).
//
// A real number rather than "keep everything". An installation nobody
// configured still has an obligation not to hold personal data for ever, and
// five years is long enough that no early customer meets it by surprise.
const DefaultRetention = 5 * 365 * 24 * time.Hour

// MinRetention is the floor a window may be set to.
//
// Not a matter of taste: this is the one setting where a typo destroys data on
// the next sweep and cannot be undone. A zero in this field would erase the
// installation, so the field refuses to hold one.
const MinRetention = 24 * time.Hour

// ErrRetentionTooShort means the window would erase almost everything.
var ErrRetentionTooShort = errors.New("admin: retention must be at least a day")

// Retention is how long content is kept before it is erased.
//
// Only content: the ledger is immutable and the hash chain survives every
// erasure, because what goes is the referenced payload and never the step
// (AU-04, NF-09).
type Retention struct {
	pool     *pgxpool.Pool
	settings *settings.Store
}

func NewRetention(pool *pgxpool.Pool, store *settings.Store) *Retention {
	return &Retention{pool: pool, settings: store}
}

const retentionName = "retention"

type storedRetention struct {
	Hours int64 `json:"hours"`
}

// Window returns what this installation keeps, or the default.
func (r *Retention) Window(ctx context.Context) (time.Duration, error) {
	found, err := r.settings.List(ctx, settings.KindRetention)
	if err != nil {
		return 0, err
	}
	for _, set := range found {
		if set.Name != retentionName {
			continue
		}
		var stored storedRetention
		if err := json.Unmarshal(set.Value, &stored); err != nil {
			return 0, fmt.Errorf("admin: decode retention: %w", err)
		}
		if window := time.Duration(stored.Hours) * time.Hour; window >= MinRetention {
			return window, nil
		}
	}
	return DefaultRetention, nil
}

// SetWindow records how long content is kept, and who decided.
func (r *Retention) SetWindow(
	ctx context.Context, by domain.UserID, scope domain.Scope, window time.Duration,
) error {
	if window < MinRetention {
		return ErrRetentionTooShort
	}

	value, err := json.Marshal(storedRetention{Hours: int64(window / time.Hour)})
	if err != nil {
		return fmt.Errorf("admin: encode retention: %w", err)
	}

	return writeSetting(ctx, r.pool, r.settings, by, scope, settings.Setting{
		ScopeKind: settings.ScopeInstallation,
		Kind:      settings.KindRetention,
		Name:      retentionName,
		Value:     value,
		Enabled:   true,
		UpdatedBy: string(by),
	}, "retention.changed", retentionName, map[string]any{
		// Shortening retention destroys data on the next sweep. Who decided
		// that, and when, is what an auditor asks about afterwards.
		"hours": int64(window / time.Hour),
	})
}

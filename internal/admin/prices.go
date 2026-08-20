package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/model"
	"github.com/fuseone/agents/internal/settings"
)

// KindModelPrice is the settings kind this file owns.
const KindModelPrice settings.Kind = "model_price"

/*
ModelPrice is what an installation pays for one model.

Configured rates are the installation's contract. Public market defaults are
shown when no contract rate exists, but they do not feed Cost.Micros: they are
usually in USD, while the ledger and ceilings are in the installation's
currency. A wrong source beside a right-looking number is how a cost screen
becomes a promise the platform cannot keep.

The four rates stay separate. A cache read costs a fraction of an input token,
and collapsing them into one number is what makes an agent's cost impossible to
diagnose (PRD FO-08).
*/
type ModelPrice struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Source   string `json:"source,omitempty"`

	InputMicros      int64  `json:"inputMicros"`
	OutputMicros     int64  `json:"outputMicros"`
	CacheReadMicros  int64  `json:"cacheReadMicros"`
	CacheWriteMicros int64  `json:"cacheWriteMicros"`
	Currency         string `json:"currency,omitempty"`
	SourceURL        string `json:"sourceUrl,omitempty"`
	SourceUpdatedAt  string `json:"sourceUpdatedAt,omitempty"`
}

// ErrNoModel means a rate was given for a provider without naming a model.
var ErrNoModel = errors.New("admin: a rate needs a provider and a model")

// PutPrice records a rate and who set it.
func (i *Integrations) PutPrice(
	ctx context.Context, by domain.UserID, scope domain.Scope, price ModelPrice,
) error {
	if strings.TrimSpace(price.Provider) == "" || strings.TrimSpace(price.Model) == "" {
		// A rate for a provider alone would price every model it serves the
		// same, and the largest and smallest in one family differ by an order
		// of magnitude.
		return ErrNoModel
	}
	price.Source = model.PriceSourceConfigured
	price.Currency = ""
	price.SourceURL = ""
	price.SourceUpdatedAt = ""

	value, err := json.Marshal(price)
	if err != nil {
		return fmt.Errorf("admin: encode price: %w", err)
	}

	tx, err := i.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("admin: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := i.settings.PutTx(ctx, tx, settings.Setting{
		ScopeKind: settings.ScopeInstallation,
		Kind:      KindModelPrice,
		Name:      price.Provider + "/" + price.Model,
		Value:     value,
		Enabled:   true,
		UpdatedBy: string(by),
	}); err != nil {
		return err
	}
	if err := Record(ctx, tx, Event{
		Principal: by, Scope: scope, Action: "price.changed",
		Target: price.Provider + "/" + price.Model,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Prices lists every rate configured.
func (i *Integrations) Prices(ctx context.Context) ([]ModelPrice, error) {
	stored, err := i.settings.List(ctx, KindModelPrice)
	if err != nil {
		return nil, fmt.Errorf("admin: list prices: %w", err)
	}

	out := make([]ModelPrice, 0, len(stored))
	for _, s := range stored {
		var price ModelPrice
		if err := json.Unmarshal(s.Value, &price); err != nil {
			return nil, fmt.Errorf("admin: decode price %s: %w", s.Name, err)
		}
		if price.Source == "" {
			price.Source = model.PriceSourceConfigured
		}
		out = append(out, price)
	}
	return out, nil
}

// DeletePrice withdraws a rate. What was already recorded keeps the money it
// was recorded with: a ledger entry is not re-priced because somebody changed
// a table today.
func (i *Integrations) DeletePrice(
	ctx context.Context, by domain.UserID, scope domain.Scope, provider, model string,
) error {
	return removeSetting(ctx, i.pool, i.settings, by, scope, KindModelPrice, provider+"/"+model, "price.removed")
}

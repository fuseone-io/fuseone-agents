package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

const moneyName = "installation"

var (
	ErrMoneyInvalid = errors.New("admin: invalid money settings")
	currencyCode    = regexp.MustCompile(`^[A-Z]{3}$`)
)

// DefaultMoney keeps the product's historical display until an operator
// chooses a different installation currency.
var DefaultMoney = MoneyConfig{Currency: "BRL"}

// MoneyConfig is the unit Cost.Micros and Budget.Micros are interpreted in.
//
// This is not an exchange-rate mechanism. Existing ledger rows are still the
// same integer; changing currency changes the unit future readers attach to
// that integer, so installations should set it before relying on money
// ceilings.
type MoneyConfig struct {
	Currency string `json:"currency"`
}

// Money stores how this installation names money.
type Money struct {
	pool     *pgxpool.Pool
	settings *settings.Store
}

func NewMoney(pool *pgxpool.Pool, store *settings.Store) *Money {
	return &Money{pool: pool, settings: store}
}

func (m *Money) Current(ctx context.Context) (MoneyConfig, error) {
	found, err := m.settings.List(ctx, settings.KindMoney)
	if err != nil {
		return MoneyConfig{}, err
	}
	for _, set := range found {
		if set.Name != moneyName {
			continue
		}
		var stored MoneyConfig
		if err := json.Unmarshal(set.Value, &stored); err != nil {
			return MoneyConfig{}, fmt.Errorf("admin: decode money settings: %w", err)
		}
		return normalizeMoney(stored)
	}
	return DefaultMoney, nil
}

func (m *Money) Set(
	ctx context.Context, by domain.UserID, scope domain.Scope, config MoneyConfig,
) error {
	normalized, err := normalizeMoney(config)
	if err != nil {
		return err
	}
	value, err := json.Marshal(normalized)
	if err != nil {
		return fmt.Errorf("admin: encode money settings: %w", err)
	}

	return writeSetting(ctx, m.pool, m.settings, by, scope, settings.Setting{
		ScopeKind: settings.ScopeInstallation,
		Kind:      settings.KindMoney,
		Name:      moneyName,
		Value:     value,
		Enabled:   true,
		UpdatedBy: string(by),
	}, "money.changed", moneyName, map[string]any{
		"currency": normalized.Currency,
	})
}

func normalizeMoney(config MoneyConfig) (MoneyConfig, error) {
	currency := strings.ToUpper(strings.TrimSpace(config.Currency))
	if !currencyCode.MatchString(currency) {
		return MoneyConfig{}, fmt.Errorf("%w: currency must be a three-letter ISO code", ErrMoneyInvalid)
	}
	return MoneyConfig{Currency: currency}, nil
}

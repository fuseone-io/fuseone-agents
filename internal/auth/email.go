package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

// EmailOf resolves one active human. It is deliberately narrower than People:
// a connector proving remote ownership should not load the installation's
// directory or accidentally accept a service principal with an email-shaped
// subject.
func (p *Postgres) EmailOf(ctx context.Context, id domain.UserID) (string, error) {
	var email string
	err := p.pool.QueryRow(ctx, `
		select coalesce(email, '') from principals
		where principal_id = $1 and kind = 'user' and disabled_at is null`, string(id)).Scan(&email)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrBadCredential
	}
	if err != nil {
		return "", fmt.Errorf("auth: read person email: %w", err)
	}
	if strings.TrimSpace(email) == "" {
		return "", ErrBadCredential
	}
	return strings.TrimSpace(email), nil
}

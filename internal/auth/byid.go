package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

/*
PrincipalByID loads a person the platform already decided is this person.

The other two lookups start from a credential — a session cookie, an API token
— and prove who the caller is on the way. This one starts from an identifier
somebody bound in the administration area, so it proves nothing about who is
asking and must never be reached from anything that has not already done that
proving itself.

There is exactly one caller: the channel interaction endpoint, after it has
verified the request's signature and resolved the account to a principal. What
that path is really doing is exchanging one proof for another — Slack's
signature for this installation's idea of a person — and the exchange is only
sound because the binding it goes through is an administrative act with a name
and a date against it.

Disabled is refused here rather than filtered later: somebody taken out of the
directory has to stop being able to approve things through a channel at the
same moment they stop being able to sign in.
*/
func (p *Postgres) PrincipalByID(
	ctx context.Context, id domain.UserID,
) (domain.Principal, error) {
	var (
		kind, disp, subject string
		disabled            *time.Time
	)

	err := p.pool.QueryRow(ctx, `
		select kind, display, subject, disabled_at
		from principals where principal_id = $1`, string(id),
	).Scan(&kind, &disp, &subject, &disabled)

	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Principal{}, ErrBadCredential
	}
	if err != nil {
		return domain.Principal{}, fmt.Errorf("auth: read principal: %w", err)
	}
	if disabled != nil {
		return domain.Principal{}, ErrBadCredential
	}

	grants, err := p.grantsFor(ctx, string(id))
	if err != nil {
		return domain.Principal{}, err
	}

	return domain.Principal{
		ID:      id,
		Subject: subject,
		Display: disp,
		Kind:    domain.PrincipalKind(kind),
		Grants:  grants,
	}, nil
}

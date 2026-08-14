package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

/*
Signing in with the administrator's own password.

The path that exists so an installation cannot lock itself out, which makes
its two requirements opposites: it has to work when nothing else does, and it
must not be a way in for somebody guessing. The first is why it exists at all;
the second is why everything below is about slowing an attacker down.

Local accounts exist beside the provider, never instead of it. An account
that arrived through a provider has no password and no username, and a row
with neither cannot sign in this way at all.
*/

// MaxSignInAttempts is how many wrong passwords before the account is shut.
const MaxSignInAttempts = 5

// LockoutFor is how long it stays shut.
//
// Minutes rather than for ever: an administrator who mistyped their own
// password must not need database access to get back in, or the lockout
// becomes the outage it was meant to prevent. Long enough that guessing at
// this rate is not a strategy.
const LockoutFor = 15 * time.Minute

// ErrLockedOut means too many wrong passwords, recently.
var ErrLockedOut = errors.New("auth: too many attempts; try again later")

// ErrNoPassword means this principal has no password set.
var ErrNoPassword = errors.New("auth: this account has no password")

// Local signs in with a stored password.
type Local struct {
	pool *pgxpool.Pool
	dir  *Postgres
	now  func() time.Time
}

func NewLocal(pool *pgxpool.Pool, dir *Postgres) *Local {
	return &Local{pool: pool, dir: dir, now: time.Now}
}

/*
Verify checks a username and password, and answers with whose they are.

It stops there rather than issuing a session: cookies belong to the HTTP layer
and there is more than one of them — the session and the value the console
echoes back on writes — and a second place that set only the first would
produce a caller who is signed in and cannot do anything.

Every failure answers ErrBadCredential, whatever actually went wrong: no such
username, no password set, the wrong one. Distinguishing them turns this into
a way to find out which accounts exist, which is the first thing somebody does
before they start guessing.

The lockout is checked before the password and not after. A lock the right
password opens is not a lock — opening it is precisely what the attacker is
working towards.
*/
func (l *Local) Verify(ctx context.Context, username, password string) (string, error) {
	var (
		principalID string
		hash        *string
		locked      *time.Time
	)
	err := l.pool.QueryRow(ctx, `
		select principal_id, password_hash, locked_until from principals
		where lower(username) = lower($1) and disabled_at is null`,
		username).Scan(&principalID, &hash, &locked)

	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrBadCredential
	}
	if err != nil {
		return "", fmt.Errorf("auth: read principal: %w", err)
	}
	if locked != nil && locked.After(l.now()) {
		return "", ErrLockedOut
	}
	if hash == nil || !PasswordMatches(*hash, password) {
		if err := l.failed(ctx, principalID); err != nil {
			return "", err
		}
		return "", ErrBadCredential
	}
	if err := l.succeeded(ctx, principalID); err != nil {
		return "", err
	}
	return principalID, nil
}

// Any reports whether anybody can sign in with a password at all.
//
// What the sign-in screen asks before showing a form: an installation whose
// people all arrive through a provider has no password to type, and a form
// for one would be an invitation to guess.
func (l *Local) Any(ctx context.Context) (bool, error) {
	var any bool
	if err := l.pool.QueryRow(ctx, `
		select exists(select 1 from principals
		              where username is not null and password_hash is not null
		                and disabled_at is null)`).Scan(&any); err != nil {
		return false, fmt.Errorf("auth: read local accounts: %w", err)
	}
	return any, nil
}

// SetPassword records a password for one principal.
//
// Every session that principal holds elsewhere stays: changing a password is
// ordinarily somebody setting one for the first time, and signing themselves
// out of the browser they are typing in would be a surprise. Revoking is a
// separate act, and it is the one to reach for when a password leaked.
func (l *Local) SetPassword(ctx context.Context, principalID, password string) error {
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	tag, err := l.pool.Exec(ctx, `
		update principals
		set password_hash = $2, password_set_at = now(),
		    failed_sign_ins = 0, locked_until = null
		where principal_id = $1 and disabled_at is null`, principalID, hash)
	if err != nil {
		return fmt.Errorf("auth: set password: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("auth: no such principal: %s", principalID)
	}
	return nil
}

// HasPassword reports whether this principal can sign in with one, which is
// what the console asks before telling an administrator they have no way back
// in if their provider breaks.
func (l *Local) HasPassword(ctx context.Context, principalID string) (bool, error) {
	var set bool
	err := l.pool.QueryRow(ctx, `
		select password_hash is not null from principals
		where principal_id = $1`, principalID).Scan(&set)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("auth: read password state: %w", err)
	}
	return set, nil
}

// Attempts reports the failures counted and how long the account is shut for.
func (l *Local) Attempts(ctx context.Context, principalID string) (int, time.Time, error) {
	var (
		failed int
		locked *time.Time
	)
	err := l.pool.QueryRow(ctx, `
		select failed_sign_ins, locked_until from principals
		where principal_id = $1`, principalID).Scan(&failed, &locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, time.Time{}, nil
	}
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("auth: read attempts: %w", err)
	}
	if locked == nil {
		return failed, time.Time{}, nil
	}
	return failed, *locked, nil
}

// failed counts one wrong password, and shuts the account at the ceiling.
func (l *Local) failed(ctx context.Context, principalID string) error {
	if _, err := l.pool.Exec(ctx, `
		update principals set
			failed_sign_ins = failed_sign_ins + 1,
			locked_until = case
				when failed_sign_ins + 1 >= $2 then now() + $3::interval
				else locked_until
			end
		where principal_id = $1`,
		principalID, MaxSignInAttempts, LockoutFor.String()); err != nil {
		return fmt.Errorf("auth: count a failed sign-in: %w", err)
	}
	return nil
}

// succeeded forgets the count. Otherwise a week of ordinary typos adds up to
// a lockout on a morning nobody did anything wrong.
func (l *Local) succeeded(ctx context.Context, principalID string) error {
	if _, err := l.pool.Exec(ctx, `
		update principals set failed_sign_ins = 0, locked_until = null
		where principal_id = $1`, principalID); err != nil {
		return fmt.Errorf("auth: clear failed sign-ins: %w", err)
	}
	return nil
}

/*
Create makes an account that does not need an identity provider.

Not the way a customer with a provider should add people — that is what the
provider is for, and grants for somebody it vouched for are set on the person
it already knows. This is for the installation that has no provider yet, and
for the small one that never will: the four roles exist to hold an author and
an approver apart, and an installation with one account cannot show the
separation it is sold on.

The username is compared without case, so `Ana` and `ana` are one account
rather than two that look identical in a list.
*/
func (l *Local) Create(
	ctx context.Context, username, display, email, password, by string,
) (string, error) {
	username = strings.TrimSpace(username)
	if err := validUsername(username); err != nil {
		return "", err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return "", err
	}
	if display = strings.TrimSpace(display); display == "" {
		display = username
	}

	principalID := "usr_" + shortHash("local|"+strings.ToLower(username))
	var stored *string
	if email = strings.TrimSpace(email); email != "" {
		stored = &email
	}

	tx, err := l.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("auth: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		insert into principals
			(principal_id, kind, provider, subject, display, email,
			 username, password_hash, password_set_at)
		values ($1, 'user', 'local', $2, $3, $4, $2, $5, now())`,
		principalID, username, display, stored, hash); err != nil {
		if isUniqueViolation(err) {
			return "", ErrUsernameTaken
		}
		return "", fmt.Errorf("auth: create local account: %w", err)
	}

	// Recorded, because an account that can sign in appearing with no trace
	// of who made it is the thing an auditor comes looking for.
	if _, err := tx.Exec(ctx, `
		insert into admin_events (principal_id, action, target, detail)
		values ($1, 'person.created', $2, jsonb_build_object('username', $3::text))`,
		by, principalID, username); err != nil {
		return "", fmt.Errorf("auth: record the account: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("auth: commit: %w", err)
	}
	return principalID, nil
}

// SetUsername gives an existing principal a handle to sign in with, which is
// how the administrator the setup token created gets one.
func (l *Local) SetUsername(ctx context.Context, principalID, username string) error {
	username = strings.TrimSpace(username)
	if err := validUsername(username); err != nil {
		return err
	}
	tag, err := l.pool.Exec(ctx, `
		update principals set username = $2
		where principal_id = $1 and disabled_at is null`, principalID, username)
	if isUniqueViolation(err) {
		return ErrUsernameTaken
	}
	if err != nil {
		return fmt.Errorf("auth: set username: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("auth: no such principal: %s", principalID)
	}
	return nil
}

// ErrUsernameTaken means somebody already signs in with that handle.
var ErrUsernameTaken = errors.New("auth: that username is taken")

// validUsername keeps a handle to what a person can type and an operator can
// read in a log: letters, digits, dot, hyphen and underscore.
func validUsername(username string) error {
	if len(username) < 3 || len(username) > 40 {
		return errors.New("auth: a username is between 3 and 40 characters")
	}
	for _, r := range username {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.' || r == '-' || r == '_':
		default:
			return fmt.Errorf(
				"auth: a username holds letters, digits, dot, hyphen and underscore, not %q", r)
		}
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}

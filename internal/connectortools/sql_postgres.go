package connectortools

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// PostgresSQLExecutor opens one TLS-verified pgx connection per governed
// execution. There is deliberately no pool: a pooled connection could outlive
// the Vault lease whose credential opened it.
type PostgresSQLExecutor struct {
	roots *x509.CertPool
}

// NewPostgresSQLExecutor uses roots when supplied and the host system roots
// otherwise. Installations with a private database CA add it to the worker
// trust store; tests inject their disposable CA directly.
func NewPostgresSQLExecutor(roots *x509.CertPool) *PostgresSQLExecutor {
	return &PostgresSQLExecutor{roots: roots}
}

func (e *PostgresSQLExecutor) Open(
	ctx context.Context, cfg SQLConfig, credential Credential, timeout time.Duration,
) (SQLSession, error) {
	connConfig, err := postgresConnectionConfig(cfg, credential, e.roots, timeout)
	if err != nil {
		return nil, err
	}
	conn, err := pgx.ConnectConfig(ctx, connConfig)
	if err != nil {
		// The runtime flattens this before it reaches a record or a caller. Keep
		// the driver error here so context cancellation remains recognisable.
		return nil, err
	}
	return &postgresSQLSession{conn: conn}, nil
}

func postgresConnectionConfig(
	cfg SQLConfig, credential Credential, roots *x509.CertPool, timeout time.Duration,
) (*pgx.ConnConfig, error) {
	// ParseConfig must create the value; pgx deliberately panics when a caller
	// hand-builds one and could miss its private defaults. The parsed string is
	// constant and carries no destination or credential.
	parsed, err := pgx.ParseConfig("postgres://localhost/postgres?sslmode=verify-full")
	if err != nil {
		return nil, fmt.Errorf("connector: initialise postgres configuration: %w", err)
	}
	configurePostgresConnection(parsed, cfg, credential, roots, timeout)
	return parsed, nil
}

func configurePostgresConnection(
	parsed *pgx.ConnConfig, cfg SQLConfig, credential Credential,
	roots *x509.CertPool, timeout time.Duration,
) {
	parsed.Host = cfg.Host
	parsed.Port = uint16(cfg.Port)
	parsed.Database = cfg.Database
	parsed.User = credential.Username()
	parsed.Password = credential.Password()
	parsed.TLSConfig = &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    roots,
		ServerName: cfg.Host,
	}
	// Keep this hardening independent of the ParseConfig seed above. If that
	// seed is ever weakened to a mode with fallbacks, TLS failure must still be
	// a refusal and never a retry in clear text.
	parsed.Fallbacks = nil
	serverTimeout := postgresTimeout(timeout)
	parsed.RuntimeParams = map[string]string{
		"application_name":                    "fuseone-sql-connector",
		"statement_timeout":                   serverTimeout,
		"idle_in_transaction_session_timeout": serverTimeout,
		"datestyle":                           "ISO, MDY",
		"intervalstyle":                       "iso_8601",
		"timezone":                            "UTC",
		"extra_float_digits":                  "3",
		"bytea_output":                        "hex",
	}
}

func postgresTimeout(timeout time.Duration) string {
	millis := timeout.Milliseconds()
	if timeout%time.Millisecond != 0 {
		millis++
	}
	if millis < 1 {
		millis = 1
	}
	return fmt.Sprintf("%dms", millis)
}

type postgresSQLSession struct {
	conn *pgx.Conn
}

func (s *postgresSQLSession) Describe(ctx context.Context, statement string) (int, error) {
	// The unnamed statement can be replaced on this single-use connection.
	// Keeping SQL out of the name also avoids PostgreSQL's 63-byte identifier
	// truncation turning a naming detail into an accidental invariant.
	description, err := s.conn.Prepare(ctx, "", statement)
	if err != nil {
		return 0, err
	}
	return len(description.ParamOIDs), nil
}

func (s *postgresSQLSession) Query(
	ctx context.Context, statement string, args []any, sink SQLSink,
) error {
	tx, err := s.conn.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer rollbackSQLTransaction(tx)

	queryArgs := make([]any, 0, len(args)+1)
	// One text result format applies to every result column in PostgreSQL's
	// Bind message. The governed representation may opt a known-safe type back
	// into structured JSON, but an unknown type must never fall through to a
	// pgx Go struct or a binary blob.
	queryArgs = append(queryArgs, pgx.QueryResultFormats{pgx.TextFormatCode})
	queryArgs = append(queryArgs, args...)
	rows, err := tx.Query(ctx, statement, queryArgs...)
	if err != nil {
		return err
	}
	defer rows.Close()

	columns := make([]string, len(rows.FieldDescriptions()))
	for i, field := range rows.FieldDescriptions() {
		columns[i] = field.Name
	}
	if err := sink.Columns(columns); err != nil {
		return err
	}
	for rows.Next() {
		encoded, err := postgresJSONRow(rows)
		if err != nil {
			return fmt.Errorf("connector: postgres returned a row that cannot be encoded")
		}
		if err := sink.Row(encoded); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func rollbackSQLTransaction(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := tx.Rollback(ctx)
	if err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		// The connection is closed immediately after this method returns. There
		// is no safe error detail to add and no recovery to attempt here.
		return
	}
}

func (s *postgresSQLSession) Close(ctx context.Context) error {
	// pgconn.Close always closes its underlying net.Conn, including when the
	// graceful Terminate exchange exceeds ctx. The real-database test observes
	// the backend disappearing when ctx is already cancelled.
	return s.conn.Close(ctx)
}

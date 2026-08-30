package connectortools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// read stops at the template's bounds while rows arrive. The row that crosses
// a limit is discarded: half a row is malformed, not a smaller answer.
func read(
	ctx context.Context, session SQLSession, tpl SQLTemplate, args []any,
	credential Credential, out *SQLResult,
) error {
	sink := &boundedSink{
		result: out, limit: tpl.MaxBytes, maxRows: tpl.MaxRows,
		forbidden: credentialShapes(credential),
	}
	err := session.Query(ctx, tpl.SQL, args, sink)
	if errors.Is(err, ErrResultTooLarge) && !errors.Is(err, ErrAnswerShapeTooLarge) {
		return nil
	}
	if errors.Is(err, ErrAnswerShapeTooLarge) || errors.Is(err, ErrSinkOutOfOrder) ||
		errors.Is(err, ErrCredentialInResult) {
		return err
	}
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return fmt.Errorf("connector: template %s failed against the database", tpl.ID)
	}
	if !sink.sawColumns {
		return fmt.Errorf("%w: no columns were announced", ErrSinkOutOfOrder)
	}
	return nil
}

// boundedSink measures the JSON that will be stored, including columns and
// provenance, rather than estimating row bytes and under-counting the envelope.
type boundedSink struct {
	result     *SQLResult
	limit      int
	maxRows    int
	size       int
	sawColumns bool
	forbidden  []string
}

func (s *boundedSink) Columns(names []string) error {
	if s.sawColumns {
		return fmt.Errorf("%w: columns were announced twice", ErrSinkOutOfOrder)
	}
	if s.containsSecret([]byte(strings.Join(names, "\x00"))) {
		return ErrCredentialInResult
	}
	s.sawColumns = true
	s.result.Columns = names
	// false is longer than true, and not_attempted is the longest revocation
	// outcome, so fields decided after this measurement cannot grow it.
	probe := SQLResult{
		Columns: names, Rows: []json.RawMessage{}, Truncated: false,
		IssuanceOutcome: s.result.IssuanceOutcome,
		Issuance:        s.result.Issuance,
		Revocation:      RevocationNotAttempted,
	}
	encoded, err := json.Marshal(probe)
	if err != nil {
		return fmt.Errorf("connector: the answer cannot be measured")
	}
	s.size = len(encoded)
	if s.size > s.limit {
		s.result.Truncated = true
		return ErrAnswerShapeTooLarge
	}
	return nil
}

func (s *boundedSink) Row(row json.RawMessage) error {
	if !s.sawColumns {
		return fmt.Errorf("%w: a row arrived before the columns", ErrSinkOutOfOrder)
	}
	if s.containsSecret(row) {
		return ErrCredentialInResult
	}
	cost := len(row)
	if len(s.result.Rows) > 0 {
		cost++
	}
	if len(s.result.Rows) >= s.maxRows || s.size+cost > s.limit {
		s.result.Truncated = true
		return ErrResultTooLarge
	}
	s.result.Rows = append(s.result.Rows, row)
	s.size += cost
	return nil
}

func (s *boundedSink) containsSecret(value []byte) bool {
	for _, secret := range s.forbidden {
		if secret != "" && strings.Contains(string(value), secret) {
			return true
		}
	}
	return false
}

func credentialShapes(credential Credential) []string {
	var shapes []string
	for _, secret := range []string{credential.Username(), credential.Password()} {
		if secret == "" {
			continue
		}
		encoded, _ := json.Marshal(secret)
		shapes = append(shapes,
			secret,
			strings.Trim(string(encoded), `"`),
			url.QueryEscape(secret),
			url.PathEscape(secret),
			strings.ReplaceAll(secret, "\n", ""),
		)
	}
	return shapes
}

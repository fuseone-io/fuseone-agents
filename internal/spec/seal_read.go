package spec

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/fuseone/agents/internal/domain"
)

// SealExpectation is what a reader already knows a seal must be: the agent,
// version and scope the caller holds for it.
type SealExpectation struct {
	Agent   domain.AgentID
	Version domain.VersionID
	Scope   domain.Scope
}

var sealIDShape = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

/*
ReadSeal reads one seal's bytes and parses them, refusing anything that is not exactly
what the seal promises (NT-018 phase 2): the row must name the expected agent, version and
scope, the claim must hold a digest, the bytes must hash to it, and they must parse to that
agent in that scope. Each refusal is domain.ErrCandidateUnreadable.

A store that cannot answer is not a seal that is gone: only erased or absent content is
unreadable, and any other fault comes back as itself, so the caller can try again.
*/
func ReadSeal(
	ctx context.Context, rows SealRows, content Content, id string, want SealExpectation,
) (Spec, error) {
	row, err := sealRow(ctx, rows, id)
	if err != nil {
		return Spec{}, err
	}
	if row.Agent != want.Agent || row.Version != want.Version || row.Scope != want.Scope {
		return Spec{}, unreadable(id, "it names another agent, version or scope")
	}
	if row.Digest == "" {
		return Spec{}, unreadable(id, "its version was claimed without a digest")
	}
	data, err := content.Get(ctx, row.ContentRef)
	if errors.Is(err, domain.ErrContentErased) || errors.Is(err, domain.ErrContentAbsent) {
		return Spec{}, unreadable(id, "its bytes are gone")
	}
	if err != nil {
		return Spec{}, fmt.Errorf("read seal %s: %w", id, err)
	}
	if digestOf(data) != row.Digest {
		return Spec{}, unreadable(id, "its bytes do not hash to the claim")
	}
	return parseSealed(id, data, want)
}

func sealRow(ctx context.Context, rows SealRows, id string) (SealRow, error) {
	if !sealIDShape.MatchString(id) {
		return SealRow{}, unreadable(id, "it is not a seal id")
	}
	row, err := rows.Seal(ctx, id)
	if errors.Is(err, ErrSealNotFound) {
		return SealRow{}, unreadable(id, "no such seal")
	}
	if err != nil {
		return SealRow{}, fmt.Errorf("read seal %s: %w", id, err)
	}
	return row, nil
}

func parseSealed(id string, data []byte, want SealExpectation) (Spec, error) {
	sealed, err := Parse("sealed.agent.md", data)
	if err != nil {
		return Spec{}, unreadable(id, "its bytes do not parse")
	}
	scope := domain.Scope{Company: sealed.Company, Area: sealed.Area}
	if sealed.ID != want.Agent || sealed.Version != want.Version || scope != want.Scope {
		return Spec{}, unreadable(id, "its bytes name another agent, version or scope")
	}
	return sealed, nil
}

func unreadable(id, why string) error {
	return fmt.Errorf("seal %s: %s: %w", id, why, domain.ErrCandidateUnreadable)
}

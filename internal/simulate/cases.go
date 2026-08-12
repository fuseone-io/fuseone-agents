// Package simulate runs an agent against real occurrences that already
// happened, before anybody switches it on.
//
// The PRD calls this the central safety mechanism, and says why: a human
// description of a process is always incomplete — people describe the happy
// path and omit the exception. Simulation is what exposes that gap before
// production, and it is the only validation legible to somebody who cannot
// read a specification. Without it, non-technical authoring is reckless.
package simulate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// Store is where the case set lives, declared here by the consumer. It is the
// ledger's claim check: a case is a real customer record, and it belongs
// under the same retention as every other bulky payload (AU-04).
type Store interface {
	PutFor(ctx context.Context, kind, owner string, seq int64, data []byte) (string, error)
}

// OwnerKind is what a case set is filed under in the content store.
const OwnerKind = "case"

// ErrEmpty means the file held no case at all.
var ErrEmpty = errors.New("simulate: the file holds no case")

/*
Load stores one case per line.

Uploaded rather than fetched from the systems themselves. A connector reading
the last fifty tickets would end the one property that makes the authoring path
defensible — that it never touches production — and it would be an integration
project per customer besides (PRD N4). A file works on day one, for the first
agent, in an installation with no way out to the internet.

A line that is not JSON refuses the whole file and says which line. Loading
forty-nine of fifty and mentioning nothing would give somebody a simulation
whose coverage is a lie, and an author told the line number can fix the export.

Filed under the simulation rather than the agent, so a set stays the set that
was run. Correcting an agent by example means running the same occurrences
against the next version (FU-12), and a set that the next upload overwrote
would make the comparison meaningless.

The cases come back as well as going in, so whoever opens runs for them
straight away does not parse the same file twice and risk the two parses
disagreeing about what a case is.
*/
func Load(ctx context.Context, store Store, simulation string, file []byte) ([][]byte, error) {
	var loaded [][]byte
	for i, line := range bytes.Split(file, []byte("\n")) {
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			// Every export ends in a newline. Counting it would put an empty
			// occurrence in a report somebody reads as real.
			continue
		}
		if !json.Valid(trimmed) {
			return nil, fmt.Errorf("simulate: line %d is not JSON", i+1)
		}

		loaded = append(loaded, trimmed)
		if _, err := store.PutFor(ctx, OwnerKind, simulation, int64(len(loaded)), trimmed); err != nil {
			return nil, fmt.Errorf("simulate: store case %d: %w", len(loaded), err)
		}
	}

	if len(loaded) == 0 {
		return nil, ErrEmpty
	}
	return loaded, nil
}

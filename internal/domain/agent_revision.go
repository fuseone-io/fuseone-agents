package domain

import (
	"errors"
	"strings"
)

/*
AgentRevision is an agent's entity-tag: its current version and whether it is
out of circulation, the two things publishing, retiring and restoring change
(NT-017, phase 1).

Nothing else is in it. An identical publish answers the version the agent
already had, so it leaves the tag alone; pausing and promoting are operations
on the running agent, not the definition a client manages, and a tag that
moved with them would refuse a publish nobody raced.
*/
func AgentRevision(version VersionID, retired bool) Revision {
	state := "live"
	if retired {
		state = "retired"
	}
	return NewRevision("agent", []byte(version), state)
}

// ErrBadIfNoneMatch is an If-None-Match a write cannot honour.
var ErrBadIfNoneMatch = errors.New("If-None-Match on a write is * or absent")

/*
ParseIfNoneMatch reads If-None-Match on a write. Nil is no header.

Only `*` is honoured: "create this, and only if nothing is there". A list of
tags would make a write conditional on the resource not being in some state,
which nothing here asks for, so it is refused rather than ignored — ignored,
it would turn off the protection its sender believes it asked for.
*/
func ParseIfNoneMatch(header *string) (absent bool, err error) {
	if header == nil {
		return false, nil
	}
	if strings.Trim(*header, " \t") != "*" {
		return false, ErrBadIfNoneMatch
	}
	return true, nil
}

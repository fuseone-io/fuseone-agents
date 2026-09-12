package domain

import "strings"

// TicketKey names one support thread independently of any run it produces.
type TicketKey string

// TicketRef names the exact request shape a run is allowed to advance.
// Revision is monotonic within Key; zero names no revision.
type TicketRef struct {
	Key      TicketKey `json:"key"`
	Revision int64     `json:"revision"`
}

func (r TicketRef) Valid() bool {
	return strings.TrimSpace(string(r.Key)) != "" && r.Revision > 0
}

// TicketContext is authority the platform derived from a support thread.
//
// RequestedBy is deliberately separate from a run's OnBehalfOf identity: a
// watched conversation may run under an administrator-chosen principal while
// the requester remains the linked person who wrote the root message.
// AddressedBy is the configured channel actor that selected recipients. It is
// provenance, never approval authority, and may be empty before addressing.
type TicketContext struct {
	Ref         TicketRef `json:"ref"`
	RequestedBy UserID    `json:"requested_by"`
	AddressedBy string    `json:"addressed_by,omitempty"`
}

func (c TicketContext) Valid() bool {
	return c.Ref.Valid() && strings.TrimSpace(string(c.RequestedBy)) != ""
}

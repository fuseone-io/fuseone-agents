package ticket

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

type Memory struct {
	mu            sync.RWMutex
	tickets       map[domain.TicketKey]Ticket
	revisions     map[domain.TicketKey]map[int64]Revision
	events        map[string]domain.TicketRef
	attempts      map[string]ExternalAttempt
	outcomeClaims map[domain.TicketRef]outcomeClaim
	announced     map[domain.TicketRef]bool
	superseded    map[domain.TicketRef]bool
}

type outcomeClaim struct {
	owner string
	until time.Time
}

func NewMemory() *Memory {
	return &Memory{
		tickets:       make(map[domain.TicketKey]Ticket),
		revisions:     make(map[domain.TicketKey]map[int64]Revision),
		events:        make(map[string]domain.TicketRef),
		attempts:      make(map[string]ExternalAttempt),
		outcomeClaims: make(map[domain.TicketRef]outcomeClaim),
		announced:     make(map[domain.TicketRef]bool),
		superseded:    make(map[domain.TicketRef]bool),
	}
}

func (m *Memory) ClaimOutcomes(
	ctx context.Context, owner string, now time.Time, lease time.Duration, limit int,
) ([]OutcomeNotice, error) {
	if err := validateOutcomeClaim(owner, now, lease, limit); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	available := make([]OutcomeNotice, 0)
	for key, revisions := range m.revisions {
		for _, revision := range revisions {
			ref := revision.Ref
			if m.announced[ref] || revision.Outcome == nil ||
				(revision.Phase != PhaseCompleted && revision.Phase != PhaseRejected &&
					revision.Phase != PhaseNeedsAttention) {
				continue
			}
			claim := m.outcomeClaims[ref]
			if claim.owner != "" && claim.until.After(now) {
				continue
			}
			available = append(available, OutcomeNotice{
				Ref: ref, Origin: m.tickets[key].Origin,
				Phase: revision.Phase, Result: revision.Outcome.Result,
			})
		}
	}
	slices.SortFunc(available, func(a, b OutcomeNotice) int {
		if a.Ref.Key != b.Ref.Key {
			return strings.Compare(string(a.Ref.Key), string(b.Ref.Key))
		}
		return cmp.Compare(a.Ref.Revision, b.Ref.Revision)
	})
	if len(available) > limit {
		available = available[:limit]
	}
	for _, notice := range available {
		m.outcomeClaims[notice.Ref] = outcomeClaim{owner: owner, until: now.Add(lease)}
	}
	return available, nil
}

func (m *Memory) MarkOutcomeAnnounced(
	ctx context.Context, ref domain.TicketRef, owner string, at time.Time,
) error {
	if err := validateOutcomeMark(ref, owner, at); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.announced[ref] {
		return nil
	}
	if m.outcomeClaims[ref].owner != owner {
		return ErrMoved
	}
	m.announced[ref] = true
	delete(m.outcomeClaims, ref)
	return nil
}

func (m *Memory) Open(ctx context.Context, in OpenInput) (Ticket, bool, error) {
	if err := ctx.Err(); err != nil {
		return Ticket{}, false, err
	}
	if err := validateOpen(in); err != nil {
		return Ticket{}, false, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if ref, exists := m.events[in.EventID]; exists {
		if ref.Key != in.Key {
			return Ticket{}, false, ErrEventTaken
		}
		return cloneTicket(m.tickets[in.Key]), false, nil
	}
	if current, exists := m.tickets[in.Key]; exists {
		return cloneTicket(current), false, ErrMoved
	}
	open := 0
	for _, held := range m.tickets {
		if held.Scope == in.Scope && !held.Current.Phase.terminal() {
			open++
		}
	}
	if open >= MaxOpenPerScope {
		return Ticket{}, false, ErrTooManyOpen
	}
	ref := domain.TicketRef{Key: in.Key, Revision: 1}
	revision := Revision{
		Ref: ref, Phase: PhaseCollecting, Draft: in.Draft,
		CreatedAt: in.At.UTC(), UpdatedAt: in.At.UTC(),
	}
	ticket := Ticket{
		Key: in.Key, Origin: in.Origin, Scope: in.Scope,
		Agent: in.Agent, RunAs: in.RunAs, RequestedBy: in.RequestedBy,
		AddressedBy: in.AddressedBy, Current: revision,
		CreatedAt: in.At.UTC(), UpdatedAt: in.At.UTC(),
	}
	m.tickets[in.Key] = ticket
	m.revisions[in.Key] = map[int64]Revision{1: revision}
	m.events[in.EventID] = ref
	return cloneTicket(ticket), true, nil
}

func (m *Memory) AtOrigin(ctx context.Context, origin Origin) (Ticket, error) {
	if err := ctx.Err(); err != nil {
		return Ticket{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, held := range m.tickets {
		if held.Origin == origin {
			return cloneTicket(held), nil
		}
	}
	return Ticket{}, ErrNotFound
}

func (m *Memory) Current(ctx context.Context, key domain.TicketKey) (Ticket, error) {
	if err := ctx.Err(); err != nil {
		return Ticket{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	ticket, ok := m.tickets[key]
	if !ok {
		return Ticket{}, ErrNotFound
	}
	return cloneTicket(ticket), nil
}

func (m *Memory) Revision(ctx context.Context, ref domain.TicketRef) (Revision, error) {
	if err := ctx.Err(); err != nil {
		return Revision{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	revision, ok := m.revisions[ref.Key][ref.Revision]
	if !ok {
		return Revision{}, ErrNotFound
	}
	return cloneRevision(revision), nil
}

func (m *Memory) SupersededApprovals(
	ctx context.Context, current domain.TicketRef,
) ([]SupersededApproval, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !current.Valid() {
		return nil, ErrNotFound
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	revisions, ok := m.revisions[current.Key]
	if !ok {
		return []SupersededApproval{}, nil
	}
	out := make([]SupersededApproval, 0)
	for revision := int64(1); revision < current.Revision; revision++ {
		stored, exists := revisions[revision]
		if exists && stored.Phase == PhaseCancelled && stored.Approval != nil &&
			!m.superseded[stored.Ref] {
			out = append(out, SupersededApproval{Ref: stored.Ref, Approval: *stored.Approval})
		}
	}
	return out, nil
}

func (m *Memory) MarkApprovalSuperseded(
	ctx context.Context, ref domain.TicketRef, at time.Time,
) error {
	if err := validateSupersededMark(ref, at); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	revision, ok := m.revisions[ref.Key][ref.Revision]
	if !ok {
		return ErrNotFound
	}
	if revision.Phase != PhaseCancelled || revision.Approval == nil {
		return ErrMoved
	}
	m.superseded[ref] = true
	return nil
}

func (m *Memory) ApprovalRoute(
	ctx context.Context, ref domain.TicketRef,
) (ApprovalRoute, error) {
	if err := ctx.Err(); err != nil {
		return ApprovalRoute{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	held, ok := m.tickets[ref.Key]
	if !ok {
		return ApprovalRoute{}, ErrNotFound
	}
	revision, ok := m.revisions[ref.Key][ref.Revision]
	if !ok {
		return ApprovalRoute{}, ErrNotFound
	}
	return ApprovalRoute{
		Origin: held.Origin, Recipients: append([]domain.UserID(nil), revision.Recipients...),
	}, nil
}

func (m *Memory) EventRevision(
	ctx context.Context, eventID string,
) (Ticket, Revision, error) {
	if err := ctx.Err(); err != nil {
		return Ticket{}, Revision{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	ref, ok := m.events[eventID]
	if !ok {
		return Ticket{}, Revision{}, ErrNotFound
	}
	held, ok := m.tickets[ref.Key]
	if !ok {
		return Ticket{}, Revision{}, ErrNotFound
	}
	revision, ok := m.revisions[ref.Key][ref.Revision]
	if !ok {
		return Ticket{}, Revision{}, ErrNotFound
	}
	return cloneTicket(held), cloneRevision(revision), nil
}

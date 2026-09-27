package ticket

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

// Key names one Slack root without relying on a separator being absent from
// any vendor namespace.
func Key(connection, conversation, root string) (domain.TicketKey, error) {
	const maxPartBytes = 512
	parts := []string{connection, conversation, root}
	var out strings.Builder
	for _, part := range parts {
		if strings.TrimSpace(part) == "" || len(part) > maxPartBytes {
			return "", errors.New("ticket: every key part must be present and bounded")
		}
		out.WriteString(strconv.Itoa(len(part)))
		out.WriteByte(':')
		out.WriteString(part)
	}
	return domain.TicketKey(out.String()), nil
}

func validateOpen(in OpenInput) error {
	if strings.TrimSpace(string(in.Key)) == "" || !in.Origin.Valid() || !in.Scope.Valid() ||
		in.Agent == "" || in.RunAs == "" || in.RequestedBy == "" ||
		strings.TrimSpace(in.EventID) == "" || !in.Draft.Valid() || in.At.IsZero() {
		return errors.New("ticket: incomplete open request")
	}
	want, err := Key(in.Origin.Connection, in.Origin.Conversation, in.Origin.Root)
	if err != nil || want != in.Key {
		return errors.New("ticket: key does not name its origin")
	}
	// A room the ticket is worked in is one conversation on the same
	// connection, and never the support thread's own.
	room := strings.TrimSpace(in.ReviewIn)
	if room != in.ReviewIn || len(room) > 512 || room == in.Origin.Conversation {
		return errors.New("ticket: review room is not one other conversation")
	}
	return nil
}

func validateReviewClaim(owner string, now time.Time, lease time.Duration, limit int) error {
	if strings.TrimSpace(owner) == "" || now.IsZero() || lease <= 0 || limit <= 0 {
		return errors.New("ticket: incomplete review claim")
	}
	return nil
}

func validateReviewMark(key domain.TicketKey, root, owner string, at time.Time) error {
	if strings.TrimSpace(string(key)) == "" || strings.TrimSpace(root) == "" ||
		len(root) > 512 || strings.TrimSpace(owner) == "" || at.IsZero() {
		return errors.New("ticket: incomplete review thread")
	}
	return nil
}

func validateRevise(in ReviseInput) error {
	if !in.Ref.Valid() || strings.TrimSpace(in.EventID) == "" || in.By == "" ||
		!in.Draft.Valid() || in.At.IsZero() {
		return errors.New("ticket: incomplete revision")
	}
	return nil
}

// requireReviser answers whether this correction may replace the ticket's
// current revision. Outside the review room the requester is the only author a
// ticket has; inside it, the room being open is what the store can check.
func requireReviser(ticket Ticket, in ReviseInput) error {
	if in.InReview {
		if !ticket.Review.Open() {
			return ErrNotRequester
		}
		return nil
	}
	if ticket.RequestedBy != in.By {
		return ErrNotRequester
	}
	return nil
}

func validateApproval(in ApprovalInput) error {
	if !in.Ref.Valid() || in.RunID == "" || in.AtSeq <= 0 ||
		!in.Snapshot.Valid() || in.At.IsZero() {
		return errors.New("ticket: incomplete approval request")
	}
	return nil
}

func validateInspection(in InspectionInput) error {
	if !in.Ref.Valid() || !in.Snapshot.Valid() || in.At.IsZero() {
		return errors.New("ticket: incomplete inspection")
	}
	return nil
}

func validateAddress(in AddressInput) ([]domain.UserID, error) {
	if !in.Ref.Valid() || strings.TrimSpace(in.EventID) == "" ||
		strings.TrimSpace(in.By) == "" || in.At.IsZero() || len(in.Recipients) > 20 {
		return nil, errors.New("ticket: incomplete addressing request")
	}
	recipients := append([]domain.UserID(nil), in.Recipients...)
	for _, recipient := range recipients {
		if strings.TrimSpace(string(recipient)) == "" {
			return nil, errors.New("ticket: empty recipient")
		}
	}
	slices.Sort(recipients)
	return slices.Compact(recipients), nil
}

func validateClaim(in ClaimInput) error {
	if !in.Ref.Valid() || in.RunID == "" || in.ApprovalAtSeq <= 0 || in.At.IsZero() {
		return errors.New("ticket: incomplete execution claim")
	}
	return nil
}

func validateFinish(in FinishInput) error {
	if !in.Execution.Valid() || !in.Result.Valid() || in.At.IsZero() ||
		(in.Phase != PhaseCompleted && in.Phase != PhaseRejected) {
		return errors.New("ticket: incomplete execution result")
	}
	return nil
}

func validateAttention(in AttentionInput) error {
	if !in.Execution.Valid() || !in.Result.Valid() || in.At.IsZero() {
		return errors.New("ticket: incomplete manual intervention result")
	}
	return nil
}

func validateClose(in CloseInput) error {
	if !in.Ref.Valid() || !in.Result.Valid() || in.At.IsZero() ||
		(in.Phase != PhaseRejected && in.Phase != PhaseCancelled) {
		return errors.New("ticket: incomplete terminal result")
	}
	return nil
}

func validateOutcomeClaim(owner string, now time.Time, lease time.Duration, limit int) error {
	if strings.TrimSpace(owner) == "" || now.IsZero() || lease <= 0 || limit <= 0 || limit > 100 {
		return errors.New("ticket: invalid outcome claim")
	}
	return nil
}

func validateOutcomeMark(ref domain.TicketRef, owner string, at time.Time) error {
	if !ref.Valid() || strings.TrimSpace(owner) == "" || at.IsZero() {
		return errors.New("ticket: invalid outcome announcement")
	}
	return nil
}

func validateSupersededMark(ref domain.TicketRef, at time.Time) error {
	if !ref.Valid() || at.IsZero() {
		return errors.New("ticket: invalid superseded approval mark")
	}
	return nil
}

func requireCurrent(ticket Ticket, ref domain.TicketRef) error {
	if ticket.Current.Ref != ref {
		return fmt.Errorf("%w: current revision is %d", ErrMoved, ticket.Current.Ref.Revision)
	}
	return nil
}

func requireMutable(ticket Ticket, ref domain.TicketRef) error {
	if err := requireCurrent(ticket, ref); err != nil {
		return err
	}
	if ticket.Current.Phase.terminal() {
		return ErrTerminal
	}
	return nil
}

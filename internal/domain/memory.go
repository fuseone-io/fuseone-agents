package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	// ToolMemoryFind is the platform-owned read path for durable agent memory.
	//
	// Memory is reached as a tool rather than injected into the prompt, so the
	// Gate records the read and the labels on remembered data keep flowing into
	// later actions.
	ToolMemoryFind ToolID = "$fuseone.memory.find"
	// ToolMemorySuggest lets an opted-in agent propose a structured assertion.
	//
	// A suggestion is not remembered truth. It enters a review/confirmation
	// queue, carries the run's labels, and is invisible to ToolMemoryFind until
	// a person accepts it or the agent's learning policy confirms repeated
	// observations.
	ToolMemorySuggest ToolID = "$fuseone.memory.suggest"

	MaxMemoryKindBytes      = 64
	MaxMemorySubjectBytes   = 200
	MaxMemorySignatureBytes = 200
	MaxMemoryClaimBytes     = 1200
	MaxMemoryEvidence       = 8
	MaxMemoryFindLimit      = 10
	MaxMemoryListLimit      = 100
	MaxMemorySuggestLimit   = 100

	// ArtifactMemorySuggestion names the tool-call arguments that produced a
	// memory suggestion. The bytes live in the run's argument content record;
	// the memory evidence carries only the digest and the run/step reference.
	ArtifactMemorySuggestion = "memory_suggestion"
)

type MemoryStatus string

const (
	MemoryActive       MemoryStatus = "active"
	MemoryDisabled     MemoryStatus = "disabled"
	MemoryExpired      MemoryStatus = "expired"
	MemorySourceErased MemoryStatus = "source_erased"
)

type MemorySuggestionStatus string

const (
	MemorySuggestionPending       MemorySuggestionStatus = "pending"
	MemorySuggestionAccepted      MemorySuggestionStatus = "accepted"
	MemorySuggestionDismissed     MemorySuggestionStatus = "dismissed"
	MemorySuggestionAutoConfirmed MemorySuggestionStatus = "auto_confirmed"
	MemorySuggestionSourceErased  MemorySuggestionStatus = "source_erased"
)

type MemoryLearningMode string

const (
	MemoryLearningOff         MemoryLearningMode = "off"
	MemoryLearningReview      MemoryLearningMode = "review"
	MemoryLearningAutoConfirm MemoryLearningMode = "auto_confirm"
)

// MemoryLearningPolicy is versioned with the agent definition.
//
// The model never supplies it. It says whether the platform may accept memory
// suggestions from this agent, and whether repeated equivalent suggestions may
// become active without a separate human review.
type MemoryLearningPolicy struct {
	Mode            MemoryLearningMode `json:"mode" yaml:"mode,omitempty"`
	MinObservations int64              `json:"min_observations,omitempty" yaml:"min_observations,omitempty"`
	TTLDays         int                `json:"ttl_days,omitempty" yaml:"ttl_days,omitempty"`
}

const (
	DefaultMemoryLearningMinObservations int64 = 3
	DefaultMemoryLearningTTLDays               = 30
	MaxMemoryLearningMinObservations     int64 = MaxMemoryEvidence
	MaxMemoryLearningTTLDays                   = 365
)

type MemoryEvidence struct {
	RunID    RunID  `json:"run_id"`
	Artifact string `json:"artifact"`
	Digest   string `json:"digest"`
}

// MemoryAssertion is one structured remembered fact.
//
// The assertion is deliberately not free-form memory prose. Claim is the small
// statement the model may read, while evidence points back to ledger/content
// records that explain where the assertion came from.
type MemoryAssertion struct {
	ID        string           `json:"id"`
	Scope     Scope            `json:"scope"`
	AgentID   AgentID          `json:"agent_id"`
	Kind      string           `json:"kind"`
	Subject   string           `json:"subject"`
	Signature string           `json:"signature"`
	Claim     string           `json:"claim"`
	Evidence  []MemoryEvidence `json:"evidence"`

	Observations int64  `json:"observations"`
	Confirmed    int64  `json:"confirmed"`
	Labels       Labels `json:"labels"`

	Status    MemoryStatus `json:"status"`
	ExpiresAt *time.Time   `json:"expires_at,omitempty"`
	CreatedBy UserID       `json:"created_by"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedBy UserID       `json:"updated_by"`
	UpdatedAt time.Time    `json:"updated_at"`
}

// MemorySuggestion is a proposed assertion that has not necessarily become
// active memory.
//
// Its ID includes Claim, unlike MemoryAssertionID. Repeated suggestions of the
// same subject/signature but a different claim must not be counted as evidence
// for the old claim.
type MemorySuggestion struct {
	ID           string                 `json:"id"`
	AssertionID  string                 `json:"assertion_id"`
	Scope        Scope                  `json:"scope"`
	AgentID      AgentID                `json:"agent_id"`
	Kind         string                 `json:"kind"`
	Subject      string                 `json:"subject"`
	Signature    string                 `json:"signature"`
	Claim        string                 `json:"claim"`
	Evidence     []MemoryEvidence       `json:"evidence"`
	Observations int64                  `json:"observations"`
	Labels       Labels                 `json:"labels"`
	Status       MemorySuggestionStatus `json:"status"`
	ExpiresAt    *time.Time             `json:"expires_at,omitempty"`
	CreatedBy    UserID                 `json:"created_by"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedBy    UserID                 `json:"updated_by"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

type MemorySuggestResult string

const (
	MemorySuggestPending       MemorySuggestResult = "pending"
	MemorySuggestAutoConfirmed MemorySuggestResult = "auto_confirmed"
	MemorySuggestAlreadyActive MemorySuggestResult = "already_active"
	MemorySuggestIgnored       MemorySuggestResult = "ignored"
)

type MemorySuggestionOutcome struct {
	Suggestion MemorySuggestion
	Assertion  *MemoryAssertion
	Result     MemorySuggestResult
}

type MemoryQuery struct {
	Scope     Scope
	AgentID   AgentID
	Kind      string
	Subject   string
	Signature string
	Search    string
	Limit     int
	Now       time.Time
}

var memoryKindPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

func (s MemoryStatus) Valid() bool {
	switch s {
	case MemoryActive, MemoryDisabled, MemoryExpired, MemorySourceErased:
		return true
	default:
		return false
	}
}

func (s MemorySuggestionStatus) Valid() bool {
	switch s {
	case MemorySuggestionPending, MemorySuggestionAccepted,
		MemorySuggestionDismissed, MemorySuggestionAutoConfirmed,
		MemorySuggestionSourceErased:
		return true
	default:
		return false
	}
}

func (m MemoryLearningMode) Valid() bool {
	switch m {
	case MemoryLearningOff, MemoryLearningReview, MemoryLearningAutoConfirm:
		return true
	default:
		return false
	}
}

func (p MemoryLearningPolicy) Normalize() MemoryLearningPolicy {
	if !p.Mode.Valid() {
		p.Mode = MemoryLearningOff
	}
	if p.Mode == "" {
		p.Mode = MemoryLearningOff
	}
	if p.Mode == MemoryLearningOff {
		return MemoryLearningPolicy{Mode: MemoryLearningOff}
	}
	if p.MinObservations <= 0 {
		p.MinObservations = DefaultMemoryLearningMinObservations
	}
	if p.MinObservations < 2 {
		p.MinObservations = 2
	}
	if p.MinObservations > MaxMemoryLearningMinObservations {
		p.MinObservations = MaxMemoryLearningMinObservations
	}
	if p.TTLDays <= 0 {
		p.TTLDays = DefaultMemoryLearningTTLDays
	}
	if p.TTLDays > MaxMemoryLearningTTLDays {
		p.TTLDays = MaxMemoryLearningTTLDays
	}
	return p
}

func (p MemoryLearningPolicy) Enabled() bool {
	switch p.Normalize().Mode {
	case MemoryLearningReview, MemoryLearningAutoConfirm:
		return true
	default:
		return false
	}
}

// AutoConfirms reports whether a suggestion carrying labels may become active
// without human review. Labels are the accumulated suggestion labels, not only
// the labels of the latest run.
func (p MemoryLearningPolicy) AutoConfirms(labels Labels) bool {
	return p.Normalize().Mode == MemoryLearningAutoConfirm && !labels.HasAny(LabelUntrusted)
}

// ReviewRequired reports whether a suggestion can only enter the human review
// queue. Untrusted observations are never auto-confirmed: the model may propose
// them without a second runtime approval, but a person must decide before they
// become active memory.
func (p MemoryLearningPolicy) ReviewRequired(labels Labels) bool {
	p = p.Normalize()
	return p.Mode == MemoryLearningReview ||
		(p.Mode == MemoryLearningAutoConfirm && labels.HasAny(LabelUntrusted))
}

// ForSuggestion is the policy a single suggested observation runs under.
func (p MemoryLearningPolicy) ForSuggestion(labels Labels) MemoryLearningPolicy {
	p = p.Normalize()
	if p.Mode == MemoryLearningAutoConfirm && labels.HasAny(LabelUntrusted) {
		p.Mode = MemoryLearningReview
	}
	return p
}

func (p MemoryLearningPolicy) ExpiresAt(now time.Time) *time.Time {
	p = p.Normalize()
	if p.Mode == MemoryLearningOff {
		return nil
	}
	expires := now.UTC().AddDate(0, 0, p.TTLDays)
	return &expires
}

func (p MemoryLearningPolicy) Validate() error {
	if p.Mode != "" && !p.Mode.Valid() {
		return fmt.Errorf("memory learning mode is invalid")
	}
	p = p.Normalize()
	if p.Mode == MemoryLearningOff {
		return nil
	}
	if p.MinObservations < 2 || p.MinObservations > MaxMemoryLearningMinObservations {
		return fmt.Errorf("memory learning min_observations must be 2-%d", MaxMemoryLearningMinObservations)
	}
	if p.TTLDays < 1 || p.TTLDays > MaxMemoryLearningTTLDays {
		return fmt.Errorf("memory learning ttl_days must be 1-%d", MaxMemoryLearningTTLDays)
	}
	return nil
}

func (a MemoryAssertion) Validate() error {
	if !a.Scope.Valid() {
		return fmt.Errorf("memory scope is required")
	}
	if !validMemoryKind(a.Kind) {
		return fmt.Errorf("memory kind is invalid")
	}
	if strings.TrimSpace(a.Subject) == "" || len(a.Subject) > MaxMemorySubjectBytes {
		return fmt.Errorf("memory subject is required and must fit %d bytes", MaxMemorySubjectBytes)
	}
	if strings.TrimSpace(a.Signature) == "" || len(a.Signature) > MaxMemorySignatureBytes {
		return fmt.Errorf("memory signature is required and must fit %d bytes", MaxMemorySignatureBytes)
	}
	if strings.TrimSpace(a.Claim) == "" || len(a.Claim) > MaxMemoryClaimBytes {
		return fmt.Errorf("memory claim is required and must fit %d bytes", MaxMemoryClaimBytes)
	}
	if len(a.Evidence) == 0 || len(a.Evidence) > MaxMemoryEvidence {
		return fmt.Errorf("memory evidence must contain 1-%d records", MaxMemoryEvidence)
	}
	for _, ev := range a.Evidence {
		if ev.RunID == "" || strings.TrimSpace(ev.Artifact) == "" || strings.TrimSpace(ev.Digest) == "" {
			return fmt.Errorf("memory evidence must name run, artifact and digest")
		}
	}
	if a.Observations < 0 || a.Confirmed < 0 || a.Confirmed > a.Observations {
		return fmt.Errorf("memory counts are inconsistent")
	}
	if a.Status == "" {
		a.Status = MemoryActive
	}
	if !a.Status.Valid() {
		return fmt.Errorf("memory status is invalid")
	}
	return nil
}

func (s MemorySuggestion) Validate() error {
	a := MemoryAssertion{
		ID: s.AssertionID, Scope: s.Scope, AgentID: s.AgentID,
		Kind: s.Kind, Subject: s.Subject, Signature: s.Signature,
		Claim: s.Claim, Evidence: s.Evidence,
		Observations: s.Observations, Confirmed: 0,
		Labels: s.Labels, Status: MemoryActive, ExpiresAt: s.ExpiresAt,
		CreatedBy: s.CreatedBy, CreatedAt: s.CreatedAt,
		UpdatedBy: s.UpdatedBy, UpdatedAt: s.UpdatedAt,
	}
	if err := a.Validate(); err != nil {
		return err
	}
	if s.ID == "" || s.AssertionID == "" {
		return fmt.Errorf("memory suggestion id is required")
	}
	if s.Status == "" {
		s.Status = MemorySuggestionPending
	}
	if !s.Status.Valid() {
		return fmt.Errorf("memory suggestion status is invalid")
	}
	return nil
}

func MemoryAssertionID(a MemoryAssertion) string {
	h := sha256.New()
	writeMemoryPart(h, string(a.Scope.Company))
	writeMemoryPart(h, string(a.Scope.Area))
	writeMemoryPart(h, string(a.AgentID))
	writeMemoryPart(h, a.Kind)
	writeMemoryPart(h, a.Subject)
	writeMemoryPart(h, a.Signature)
	return "mem_" + hex.EncodeToString(h.Sum(nil))[:24]
}

func MemorySuggestionID(s MemorySuggestion) string {
	h := sha256.New()
	writeMemoryPart(h, string(s.Scope.Company))
	writeMemoryPart(h, string(s.Scope.Area))
	writeMemoryPart(h, string(s.AgentID))
	writeMemoryPart(h, s.Kind)
	writeMemoryPart(h, s.Subject)
	writeMemoryPart(h, s.Signature)
	writeMemoryPart(h, s.Claim)
	return "mems_" + hex.EncodeToString(h.Sum(nil))[:24]
}

func MemoryFindLimit(limit int) int {
	switch {
	case limit <= 0:
		return MaxMemoryFindLimit
	case limit > MaxMemoryFindLimit:
		return MaxMemoryFindLimit
	default:
		return limit
	}
}

func MemorySuggestLimit(limit int) int {
	switch {
	case limit <= 0:
		return MaxMemorySuggestLimit
	case limit > MaxMemorySuggestLimit:
		return MaxMemorySuggestLimit
	default:
		return limit
	}
}

func MemoryListLimit(limit int) int {
	switch {
	case limit <= 0:
		return MaxMemoryListLimit
	case limit > MaxMemoryListLimit:
		return MaxMemoryListLimit
	default:
		return limit
	}
}

func validMemoryKind(v string) bool {
	return v != "" && len(v) <= MaxMemoryKindBytes && memoryKindPattern.MatchString(v)
}

type byteWriter interface {
	Write([]byte) (int, error)
}

func writeMemoryPart(w byteWriter, v string) {
	_, _ = w.Write([]byte(v))
	_, _ = w.Write([]byte{0})
}

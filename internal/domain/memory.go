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

	MaxMemoryKindBytes      = 64
	MaxMemorySubjectBytes   = 200
	MaxMemorySignatureBytes = 200
	MaxMemoryClaimBytes     = 1200
	MaxMemoryEvidence       = 8
	MaxMemoryFindLimit      = 10
	MaxMemoryListLimit      = 100
)

type MemoryStatus string

const (
	MemoryActive       MemoryStatus = "active"
	MemoryDisabled     MemoryStatus = "disabled"
	MemoryExpired      MemoryStatus = "expired"
	MemorySourceErased MemoryStatus = "source_erased"
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

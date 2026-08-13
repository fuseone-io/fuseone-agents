package export

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

/*
The export's own shape for a step.

Not the domain type's. That struct has no JSON names, so it would travel as Go
field names and a base64 payload — a document somebody has to write a tool to
open, which is the opposite of what a signed export is for. Worse, it would
make every rename inside the codebase a silent change to a format an auditor
is holding a five-year-old copy of.

So the format is written down here, on purpose, and the domain type is free to
change behind it.
*/
type step struct {
	Run        string          `json:"run"`
	Seq        int64           `json:"seq"`
	Kind       string          `json:"kind"`
	Company    string          `json:"company"`
	Area       string          `json:"area"`
	Agent      string          `json:"agent,omitempty"`
	Version    string          `json:"version,omitempty"`
	OnBehalfOf string          `json:"onBehalfOf,omitempty"`
	Payload    json.RawMessage `json:"payload,omitempty"`
	Labels     []string        `json:"labels,omitempty"`
	Cost       *cost           `json:"cost,omitempty"`
	IdemKey    string          `json:"idemKey,omitempty"`
	PolicyHash string          `json:"policyHash,omitempty"`
	At         time.Time       `json:"at"`

	// Hex, like every hash this product shows a person anywhere else. An
	// auditor comparing one against the console should not have to convert.
	PrevHash string `json:"prevHash,omitempty"`
	Hash     string `json:"hash"`
}

type cost struct {
	InputTokens      int64 `json:"inputTokens,omitempty"`
	OutputTokens     int64 `json:"outputTokens,omitempty"`
	CacheReadTokens  int64 `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens int64 `json:"cacheWriteTokens,omitempty"`
	Micros           int64 `json:"micros,omitempty"`
}

func toStep(s domain.Step) step {
	out := step{
		Run: string(s.RunID), Seq: s.Seq, Kind: string(s.Kind),
		Company: string(s.Scope.Company), Area: string(s.Scope.Area),
		Agent: string(s.AgentID), Version: string(s.VersionID),
		OnBehalfOf: string(s.OnBehalfOf), Payload: json.RawMessage(s.Payload),
		Labels: s.Labels, IdemKey: s.IdemKey, PolicyHash: s.PolicyHash,
		At:   s.At.UTC(),
		Hash: hex.EncodeToString(s.Hash),
	}
	if len(s.PrevHash) > 0 {
		out.PrevHash = hex.EncodeToString(s.PrevHash)
	}
	if !s.Cost.IsZero() {
		out.Cost = &cost{
			InputTokens: s.Cost.InputTokens, OutputTokens: s.Cost.OutputTokens,
			CacheReadTokens: s.Cost.CacheReadTokens, CacheWriteTokens: s.Cost.CacheWriteTokens,
			Micros: s.Cost.Micros,
		}
	}
	return out
}

/*
fromStep rebuilds what was hashed.

The payload goes back through the canonicaliser because the document is
indented for a reader, and indenting reshapes an embedded object. The hash was
computed over canonical bytes, so canonicalising on the way back in is what
makes a legible file and a verifiable one the same file.
*/
func fromStep(s step) (domain.Step, error) {
	hash, err := hex.DecodeString(s.Hash)
	if err != nil {
		return domain.Step{}, fmt.Errorf("export: step %s/%d has an unreadable hash: %w", s.Run, s.Seq, err)
	}
	var prev []byte
	if s.PrevHash != "" {
		if prev, err = hex.DecodeString(s.PrevHash); err != nil {
			return domain.Step{}, fmt.Errorf("export: step %s/%d has an unreadable previous hash: %w", s.Run, s.Seq, err)
		}
	}

	out := domain.Step{
		RunID: domain.RunID(s.Run), Seq: s.Seq, Kind: domain.StepKind(s.Kind),
		Scope:   domain.Scope{Company: domain.CompanyID(s.Company), Area: domain.AreaID(s.Area)},
		AgentID: domain.AgentID(s.Agent), VersionID: domain.VersionID(s.Version),
		OnBehalfOf: domain.UserID(s.OnBehalfOf),
		Payload:    domain.CanonicalJSON(s.Payload),
		Labels:     domain.NewLabels(s.Labels...),
		IdemKey:    s.IdemKey, PolicyHash: s.PolicyHash,
		At:       s.At.UTC().Truncate(time.Microsecond),
		PrevHash: prev, Hash: hash,
	}
	if s.Cost != nil {
		out.Cost = domain.Cost{
			InputTokens: s.Cost.InputTokens, OutputTokens: s.Cost.OutputTokens,
			CacheReadTokens: s.Cost.CacheReadTokens, CacheWriteTokens: s.Cost.CacheWriteTokens,
			Micros: s.Cost.Micros,
		}
	}
	return out, nil
}

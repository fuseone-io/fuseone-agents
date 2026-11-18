package draft

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/spec"
)

const (
	SourcePartName              = "name"
	SourcePartScope             = "scope"
	SourcePartStandingApprovals = "standingApprovals"
)

// SourcePart is one independently sourced value of a draft. Fingerprint is
// only for comparing two states; Human marks values a proposal cannot write.
type SourcePart struct {
	Fingerprint string
	Human       bool
}

// sourceFingerprint intentionally accepts different authored value types: a
// canonical JSON representation makes all of them comparable without reflection.
func sourceFingerprint(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("unencodable:%T:%v", value, err)
	}
	return string(encoded)
}

// SourceParts returns every independently erasable value currently present.
func SourceParts(d Draft) map[string]SourcePart {
	out := make(map[string]SourcePart)
	add := func(name string, value any, present, human bool) {
		if present {
			out[name] = SourcePart{Fingerprint: sourceFingerprint(value), Human: human}
		}
	}
	s := d.Spec
	add(SourcePartName, s.Name, s.Name != "", true)
	add(SourcePartScope, struct {
		Company domain.CompanyID
		Area    domain.AreaID
	}{s.Company, s.Area}, s.Company != "" || s.Area != "", true)
	add(SourcePartStandingApprovals, s.StandingApprovals, true, true)
	add("model", struct{ Provider, Model, Effort string }{s.Provider, s.Model, s.Effort},
		s.Provider != "" || s.Model != "" || s.Effort != "", false)
	add("fallback", s.Fallback, !s.Fallback.IsZero(), false)
	add("cachePrefixTtl", s.CachePrefixTTL, s.CachePrefixTTL != "", false)
	add("replyLocale", s.ReplyLocale, s.ReplyLocale != "", false)
	add("budget", s.Budget, s.Budget != (domain.Budget{}), false)
	add("triggers", s.Triggers, len(s.Triggers) != 0, false)
	add("emits", s.Emits, len(s.Emits) != 0, false)
	if learning := s.MemoryLearning.Normalize(); learning.Enabled() {
		add("memoryLearning", learning, true, false)
	}
	if approvals := s.Approvals.Normalize(); approvals.Direct {
		add("approvals", approvals, true, false)
	}
	for _, tool := range s.Tools {
		add("tools/"+string(tool), tool, true, false)
	}
	for _, step := range s.Steps {
		digest := sha256.Sum256([]byte(step.Name))
		add("steps/"+hex.EncodeToString(digest[:]), step, true, false)
	}
	for _, block := range d.Blocks {
		add("block/"+block.ID, block.Text, block.Text != "", false)
	}
	return out
}

func HumanSourcePart(name string) bool {
	return name == SourcePartName || name == SourcePartScope || name == SourcePartStandingApprovals
}

// EraseSourceParts removes values while retaining an empty block as its stable marker.
func EraseSourceParts(d *Draft, names []string) {
	for _, name := range names {
		switch {
		case strings.HasPrefix(name, "block/"):
			eraseBlock(d, strings.TrimPrefix(name, "block/"))
		case strings.HasPrefix(name, "tools/"):
			tool := domain.ToolID(strings.TrimPrefix(name, "tools/"))
			d.Spec.Tools = slices.DeleteFunc(d.Spec.Tools, func(one domain.ToolID) bool { return one == tool })
		case strings.HasPrefix(name, "steps/"):
			eraseStep(d, strings.TrimPrefix(name, "steps/"))
		default:
			eraseField(d, name)
		}
	}
}

func eraseBlock(d *Draft, id string) {
	for i := range d.Blocks {
		if d.Blocks[i].ID == id {
			d.Blocks[i].Text, d.Blocks[i].Raw = "", ""
		}
	}
}

func eraseStep(d *Draft, digest string) {
	d.Spec.Steps = slices.DeleteFunc(d.Spec.Steps, func(step spec.Step) bool {
		sum := sha256.Sum256([]byte(step.Name))
		return hex.EncodeToString(sum[:]) == digest
	})
}

func eraseField(d *Draft, name string) {
	switch name {
	case "model":
		d.Spec.Provider, d.Spec.Model, d.Spec.Effort = "", "", ""
	case "fallback":
		d.Spec.Fallback = domain.ModelPair{}
	case "cachePrefixTtl":
		d.Spec.CachePrefixTTL = ""
	case "replyLocale":
		d.Spec.ReplyLocale = ""
	case "budget":
		d.Spec.Budget = domain.Budget{}
	case "triggers":
		d.Spec.Triggers = nil
	case "emits":
		d.Spec.Emits = nil
	case "memoryLearning":
		d.Spec.MemoryLearning = domain.MemoryLearningPolicy{Mode: domain.MemoryLearningOff}
	case "approvals":
		d.Spec.Approvals = domain.ApprovalPolicy{}
	}
}

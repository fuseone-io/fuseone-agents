package trigger

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

// The claim-checks an event grants its listeners: what the source run
// published, by name, carried forward with the source's labels.

func (d *Dispatcher) contextArtifacts(
	ctx context.Context, source domain.RunSummary, event domain.AgentEvent,
) ([]domain.ContextArtifact, error) {
	if d.reader == nil || len(event.Artifacts) == 0 {
		return nil, nil
	}
	steps, err := d.reader.Read(ctx, source.RunID, domain.FirstSeq)
	if err != nil {
		return nil, fmt.Errorf("trigger: read source run %s for context: %w", source.RunID, err)
	}
	published, err := publishedArtifacts(source, steps)
	if err != nil {
		return nil, err
	}
	if len(published) == 0 {
		return nil, nil
	}

	out := make([]domain.ContextArtifact, 0, len(event.Artifacts))
	for _, name := range event.Artifacts {
		if artifact, ok := published[name]; ok {
			out = append(out, artifact)
		}
	}
	return out, nil
}

func publishedArtifacts(
	source domain.RunSummary, steps []domain.Step,
) (map[string]domain.ContextArtifact, error) {
	out := map[string]domain.ContextArtifact{}
	for _, step := range steps {
		if step.Kind != domain.StepRunFinished {
			continue
		}
		var p domain.RunFinishedPayload
		if err := json.Unmarshal(step.Payload, &p); err != nil {
			return nil, fmt.Errorf("trigger: decode finished payload of %s: %w", source.RunID, err)
		}
		if artifact, ok := finalAnswerArtifact(source, p); ok {
			out[artifact.Name] = artifact
		}
		for _, artifact := range p.Artifacts {
			if artifact, ok := normalizeArtifact(source, artifact); ok {
				out[artifact.Name] = artifact
			}
		}
	}
	return out, nil
}

func finalAnswerArtifact(
	source domain.RunSummary, p domain.RunFinishedPayload,
) (domain.ContextArtifact, bool) {
	if p.OutcomeRef == "" || p.OutcomeDigest == "" {
		return domain.ContextArtifact{}, false
	}
	return domain.ContextArtifact{
		Name: domain.ArtifactFinalAnswer, Kind: "text",
		Ref: p.OutcomeRef, Digest: p.OutcomeDigest,
		SourceRun: source.RunID, SourceAgent: source.AgentID,
		Labels: source.Labels.Clone(),
	}, true
}

func normalizeArtifact(
	source domain.RunSummary, artifact domain.ContextArtifact,
) (domain.ContextArtifact, bool) {
	if artifact.Name == "" || artifact.Ref == "" || artifact.Digest == "" {
		return domain.ContextArtifact{}, false
	}
	artifact.SourceRun = firstRun(artifact.SourceRun, source.RunID)
	if artifact.SourceAgent == "" {
		artifact.SourceAgent = source.AgentID
	}
	if len(artifact.Labels) == 0 {
		artifact.Labels = source.Labels.Clone()
	} else {
		artifact.Labels = artifact.Labels.Clone()
	}
	return artifact, true
}

func firstRun(value, fallback domain.RunID) domain.RunID {
	if value != "" {
		return value
	}
	return fallback
}

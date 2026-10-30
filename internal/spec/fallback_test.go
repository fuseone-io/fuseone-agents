package spec_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/spec"
)

// withFallback declares a fallback pair on the shared valid definition.
func withFallback(pair string) string {
	return strings.Replace(valid, "effort: medium\n", "effort: medium\n"+pair, 1)
}

func TestParse_fallback_reachesTheSpec(t *testing.T) {
	t.Parallel()

	s := parse(t, withFallback("fallback: {provider: openai, model: gpt-test}\n"))

	want := domain.ModelPair{Provider: "openai", Model: "gpt-test"}
	if s.Fallback != want {
		t.Errorf("Fallback = %+v, want %+v", s.Fallback, want)
	}
}

func TestParse_noFallback_declaresNone(t *testing.T) {
	t.Parallel()

	// Not declaring one is today's behaviour, and an installation that fails
	// over in its own proxy depends on the platform not inventing one.
	if s := parse(t, valid); !s.Fallback.IsZero() {
		t.Errorf("Fallback = %+v, want none", s.Fallback)
	}
}

func TestParse_fallback_refusals(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name, pair, want string
	}{
		{
			// The pair the run already uses: switching to it is retrying under
			// another name, and the trail would record a switch that was not.
			name: "identical to the primary",
			pair: "fallback: {provider: anthropic, model: claude-opus-5}\n",
			want: "fallback is the agent's own provider and model",
		},
		{
			name: "no provider",
			pair: "fallback: {model: gpt-test}\n",
			want: "fallback needs a provider and a model",
		},
		{
			name: "no model",
			pair: "fallback: {provider: openai}\n",
			want: "fallback needs a provider and a model",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := spec.Parse("test.agent.md", []byte(withFallback(tc.pair)))
			if !errors.Is(err, spec.ErrInvalid) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Parse = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestParse_fallbackSameProviderOtherModel_isAccepted(t *testing.T) {
	t.Parallel()

	// A smaller model at the same vendor is a real fallback when the outage is
	// one model's capacity rather than the vendor's.
	s := parse(t, withFallback("fallback: {provider: anthropic, model: claude-haiku-5}\n"))
	if s.Fallback.Model != "claude-haiku-5" {
		t.Errorf("Fallback = %+v", s.Fallback)
	}
}

func TestRender_fallback_survivesTheRoundTrip(t *testing.T) {
	t.Parallel()

	original := parse(t, withFallback("fallback: {provider: openai, model: gpt-test}\n"))
	rendered, err := spec.Render(original)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	back, err := spec.Parse("console", rendered)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if back.Fallback != original.Fallback {
		t.Errorf("round trip Fallback = %+v, want %+v", back.Fallback, original.Fallback)
	}
}

func TestRender_noFallback_writesNoBlock(t *testing.T) {
	t.Parallel()

	rendered, err := spec.Render(parse(t, valid))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	// An empty block would change the digest of every agent published before
	// fallbacks existed, so republishing one unchanged made a new version.
	if strings.Contains(string(rendered), "fallback") {
		t.Errorf("rendered = %s, want no fallback block", rendered)
	}
}

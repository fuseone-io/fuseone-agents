package guard

import (
	"errors"
	"strings"
	"testing"
)

// passThroughQAEnv serves QA and the Merger from the caller's tokens, with
// QA's bot listed as qa and FACTORY_QA_BOT_ID unset: one source of truth.
func passThroughQAEnv(qaBot string) map[string]string {
	env := passThroughEnv()
	delete(env, "FACTORY_QA_BOT_ID")
	env["FACTORY_ROLE_BOTS"] = "qa=" + qaBot + ",merger=106"
	return env
}

func TestLoadConfig_passThrough_qaIdentityDerivedFromTheAllowlist(t *testing.T) {
	t.Parallel()

	cfg, err := load(passThroughQAEnv("9001"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.QABotID != 9001 {
		t.Fatalf("QA bot id = %d, want the qa bot of FACTORY_ROLE_BOTS", cfg.QABotID)
	}
}

func TestLoadConfig_passThrough_explicitQABotEqualToTheAllowlist_starts(t *testing.T) {
	t.Parallel()

	env := passThroughQAEnv("9001")
	env["FACTORY_QA_BOT_ID"] = "9001"
	if cfg, err := load(env); err != nil || cfg.QABotID != 9001 {
		t.Fatalf("QA bot id %d, err %v", cfg.QABotID, err)
	}
}

// QA is served by bot 9001, and the Merger would trust factory/qa from 7777:
// a status 9001 never set could pass the merge. The server refuses to start.
func TestLoadConfig_passThrough_explicitQABotDivergingFromTheAllowlist_refused(t *testing.T) {
	t.Parallel()

	env := passThroughQAEnv("9001")
	env["FACTORY_QA_BOT_ID"] = "7777"
	if _, err := load(env); !errors.Is(err, ErrConfig) || !strings.Contains(err.Error(), "FACTORY_QA_BOT_ID") {
		t.Fatalf("err = %v, want ErrConfig naming FACTORY_QA_BOT_ID", err)
	}
}

func TestLoadConfig_passThroughMergerWithoutAQAIdentity_refused(t *testing.T) {
	t.Parallel()

	env := passThroughQAEnv("9001")
	env["FACTORY_ROLE_BOTS"] = "merger=106"
	if _, err := load(env); !errors.Is(err, ErrConfig) || !strings.Contains(err.Error(), "FACTORY_ROLE_BOTS") {
		t.Fatalf("err = %v, want ErrConfig naming FACTORY_ROLE_BOTS", err)
	}
}

// QA run elsewhere: the Merger alone, with the QA bot named explicitly.
func TestLoadConfig_passThroughMergerWithAnExplicitQABot_starts(t *testing.T) {
	t.Parallel()

	env := passThroughQAEnv("9001")
	env["FACTORY_ROLE_BOTS"] = "merger=106"
	env["FACTORY_QA_BOT_ID"] = "9001"
	if cfg, err := load(env); err != nil || cfg.QABotID != 9001 {
		t.Fatalf("QA bot id %d, err %v", cfg.QABotID, err)
	}
}

// End to end from the environment: the Merger accepts factory/qa only from
// the bot the allowlist serves as QA. The harness's status comes from bot
// 9001; listed as qa it merges, and with another bot listed it is untrusted.
func TestMergeIfReady_passThrough_trustsOnlyTheAllowlistedQABot(t *testing.T) {
	t.Parallel()

	for qa, want := range map[string]Reason{"9001": ReasonMerged, "7777": ReasonQAUntrustedCreator} {
		t.Run("qa="+qa, func(t *testing.T) {
			t.Parallel()
			loaded, err := load(passThroughQAEnv(qa))
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			h := newHarness(t)
			cfg := h.cfg
			cfg.QABotID = loaded.QABotID
			res, err := NewMerger(cfg, h.client("merger-token")).MergeIfReady(t.Context(), MergeInput{PR: testPR, HeadSHA: headA})
			if err != nil || res.Reason != want {
				t.Fatalf("result %+v, err %v, want %s", res, err, want)
			}
		})
	}
}

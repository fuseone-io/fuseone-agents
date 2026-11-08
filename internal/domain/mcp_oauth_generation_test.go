package domain

import "testing"

func TestRegranted_generationFollowsThePastedTokens(t *testing.T) {
	t.Parallel()

	held := MCPOAuthGrant{AccessToken: "a", RefreshToken: "r", ExpiresAtUnix: 10, ClientID: "c", Generation: "g-held"}
	sameTokens := MCPOAuthGrant{AccessToken: "a", RefreshToken: "r", ExpiresAtUnix: 10, ClientID: "another"}
	newTokens := MCPOAuthGrant{AccessToken: "a", RefreshToken: "r2", ExpiresAtUnix: 10, ClientID: "c"}

	kept := Regranted(&held, &sameTokens)
	if kept.Generation != "g-held" {
		t.Errorf("the tokens held, sent back, made generation %q", kept.Generation)
	}
	moved := Regranted(&held, &newTokens)
	if moved.Generation == "" || moved.Generation == "g-held" {
		t.Errorf("new tokens kept generation %q", moved.Generation)
	}
	// Chained while the grant exists: the tokens first pasted, sent back over their rotation,
	// are a change, though they once had that generation.
	pasted := Regranted(nil, &newTokens)
	rotated := KeptGeneration(*pasted, MCPOAuthGrant{AccessToken: "rotated", RefreshToken: "r3", ExpiresAtUnix: 20})
	if back := Regranted(&rotated, &newTokens); back.Generation == pasted.Generation {
		t.Errorf("sent back over its rotation, a grant kept generation %q", back.Generation)
	}
	if Regranted(&held, nil) != nil {
		t.Error("a removed grant came back")
	}
}

// A removal leaves nothing to chain to. The same tokens pasted again name the state they
// named before, and other tokens do not.
func TestRegranted_acrossARemoval_theSameTokensComeBackUnderTheirGeneration(t *testing.T) {
	t.Parallel()

	pasted := MCPOAuthGrant{AccessToken: "a", RefreshToken: "r", ExpiresAtUnix: 10}
	first := Regranted(nil, &pasted)
	removed := Regranted(first, nil)
	if removed != nil {
		t.Fatal("a removed grant came back")
	}
	if third := Regranted(removed, &pasted); third.Generation != first.Generation {
		t.Errorf("pasted, removed and pasted again: %q, then %q", first.Generation, third.Generation)
	}
	other := MCPOAuthGrant{AccessToken: "a", RefreshToken: "r2", ExpiresAtUnix: 10}
	if fresh := Regranted(removed, &other); fresh.Generation == first.Generation {
		t.Errorf("other tokens after a removal took generation %q", fresh.Generation)
	}
}

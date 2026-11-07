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
	// Chained to what came before, so a grant removed and pasted again is not the
	// revision a client read before the removal.
	if fresh := Regranted(nil, &newTokens); fresh.Generation == moved.Generation {
		t.Errorf("the same tokens over another history made the same generation %q", fresh.Generation)
	}
	if Regranted(&held, nil) != nil {
		t.Error("a removed grant came back")
	}
}

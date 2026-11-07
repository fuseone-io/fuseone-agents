package domain

import "strconv"

/*
Regranted is the grant a person's write leaves, with its generation worked out.

A worker rotates the access token, the refresh token and when it expires, with nobody having
decided anything, so none of the three can be in a revision. But a person pasting a grant is
replacing them, and two people doing it over the same revision are two writers: one has to
land and the other be told. The generation is what lets the revision tell them apart. It
moves when a person sends tokens other than the ones held, and only then; a worker's
rotation carries it across (see KeptGeneration).

Chained to the generation before it, so a grant removed and then pasted again does not come
back under a revision a client read before the removal.
*/
func Regranted(stored, merged *MCPOAuthGrant) *MCPOAuthGrant {
	if merged == nil {
		return nil
	}
	out := *merged
	var before string
	if stored != nil {
		before = stored.Generation
		if sameTokens(*stored, out) {
			out.Generation = before
			return &out
		}
	}
	out.Generation = string(NewRevision("mcp_oauth_generation", []byte(before),
		out.AccessToken, out.RefreshToken, strconv.FormatInt(out.ExpiresAtUnix, 10)))
	return &out
}

// KeptGeneration is a worker's refreshed grant, still under the generation a person's paste
// gave the one it refreshed.
func KeptGeneration(stored, refreshed MCPOAuthGrant) MCPOAuthGrant {
	refreshed.Generation = stored.Generation
	return refreshed
}

func sameTokens(a, b MCPOAuthGrant) bool {
	return a.AccessToken == b.AccessToken && a.RefreshToken == b.RefreshToken && a.ExpiresAtUnix == b.ExpiresAtUnix
}

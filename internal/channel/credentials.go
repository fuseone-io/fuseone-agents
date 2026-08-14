package channel

import "encoding/json"

/*
A channel's secrets, sealed as one.

Two of them for Slack and they are not interchangeable: the bot token is a
bearer this installation sends, and the signing secret is what it checks
arriving requests against. A webhook trigger stores its secret hashed, which is
right for a bearer somebody compares and useless here — HMAC needs the secret
itself, and a hash cannot produce one.

Both live in the vault, in one sealed document, because a setting holds one
secret and splitting them across two settings would let an installation exist
with a token and no way to trust what comes back.
*/
type Credentials struct {
	// Token is what this installation sends: Slack's xoxb- bot token.
	Token string `json:"token,omitempty"`
	// Signing is what it checks with. Absent means the inbound surface is
	// closed for this channel, which is the safe reading of "not configured".
	Signing string `json:"signing,omitempty"`
}

// Sealed renders the pair for the vault.
func (c Credentials) Sealed() string {
	if c == (Credentials{}) {
		return ""
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return ""
	}
	return string(raw)
}

/*
ReadCredentials reads what the vault gave back.

A value that is not a document is the bot token on its own. That is what
installations configured before there was anything to check signatures with
hold, and refusing to read it would take their notifications away to add a
feature they have not switched on.
*/
func ReadCredentials(sealed string) Credentials {
	if sealed == "" {
		return Credentials{}
	}
	var c Credentials
	if err := json.Unmarshal([]byte(sealed), &c); err != nil || c == (Credentials{}) {
		return Credentials{Token: sealed}
	}
	return c
}

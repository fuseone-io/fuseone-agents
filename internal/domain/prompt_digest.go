package domain

/*
PromptDigests fingerprint a planning call's tools and stable system blocks.

A provider caches by exact prefix, so a block that changes between two calls
discards everything after it. Sizes alone cannot show that: two blocks of the
same length with different bytes read the same, and an agent that never read
its cache could not be diagnosed from the ledger. Comparing the digests of two
consecutive planned steps names the block that moved.

Not the whole cache key: the cached message history and request parameters
such as tool_choice, thinking and effort can also invalidate a cache, and none
of them is measured here.

Each value is a 64-bit hex prefix of a SHA-256 over the block's wire bytes.
It is diagnostic only — too short to serve integrity or authorisation — and
it identifies without revealing: the bytes behind it stay where the prompt's
sources already live.
*/
type PromptDigests struct {
	Tools  string   `json:"tools,omitempty"`
	System []string `json:"system,omitempty"`
}

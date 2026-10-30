package domain

/*
PromptDigests fingerprint each block of a planning call's cacheable prefix.

A provider caches by exact prefix, so a block that changes between two calls
discards everything after it. Sizes alone cannot show that: two blocks of the
same length with different bytes read the same, and an agent that never read
its cache could not be diagnosed from the ledger. Comparing the digests of two
consecutive planned steps names the block that moved.

Each value is a short hex prefix of a SHA-256 over the block's wire encoding.
It identifies, it does not reveal: the bytes behind it stay where the prompt's
sources already live.
*/
type PromptDigests struct {
	Tools  string   `json:"tools,omitempty"`
	System []string `json:"system,omitempty"`
}

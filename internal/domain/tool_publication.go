package domain

// ToolPublication is what one worker's catalogue tells the administration.
//
// Entries are the tools to record. Servers are the servers whose discovery
// the worker holds in full: for each, Entries is everything it offers, so a
// tool of that server missing from Entries is no longer offered. A server not
// named is not judged at all — another worker may hold it, or its discovery
// failed here — and its recorded tools stay as they are.
//
// Withdrawn are servers the configuration no longer wants. Their tools stop
// being offered unless the configuration wants them again by the time the
// publication is recorded.
type ToolPublication struct {
	Servers   []string
	Entries   []ToolEntry
	Withdrawn []string
}

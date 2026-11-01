package domain

import "time"

// ToolPublication is what one worker's catalogue tells the administration.
//
// Entries are the tools to record. Servers are the servers whose discovery
// the worker holds in full: for each, Entries is everything it offers. A
// server not named is not judged at all — another worker may hold it, or its
// discovery failed here — and an entry whose server is not named is ignored.
type ToolPublication struct {
	Servers []PublishedServer
	Entries []ToolEntry
}

// PublishedServer is one server a publication speaks for, and the
// configuration its discovery was made under.
//
// The administration judges the discovery against the configuration it holds
// when it records it, not when the worker read it. A discovery made under a
// configuration that has since changed — a server deleted, switched off, or
// saved with other toolsets — describes a server that no longer exists in
// that form, and recording it would put back tools nobody offers.
type PublishedServer struct {
	Name string
	// Generation is when the server's configuration last changed, as the
	// worker read it before connecting: the configuration row's updated_at.
	Generation time.Time
	// Unmanaged marks a server this installation's configuration does not
	// hold — one started from the command line. Nothing in the configuration
	// can judge it, so its own worker's discovery is the only authority on it.
	Unmanaged bool
}

// ServerNames is the names of the servers a publication speaks for.
func (p ToolPublication) ServerNames() []string {
	names := make([]string, len(p.Servers))
	for i, s := range p.Servers {
		names[i] = s.Name
	}
	return names
}

package domain

import "time"

/*
MCPRediscoveryReceipt is one worker saying it asked a tool server again what it
offers (#287), and how that went.

A request is a generation, not a message: each worker compares the generation it
last answered with the one requested and owes one rediscovery when it is behind.
So a worker that was down, or that starts later, is not left out, and a request
nobody consumed is not lost. Worker is the name the worker publishes its
catalogue under; it names a process, never a person.
*/
type MCPRediscoveryReceipt struct {
	Server     string
	Worker     string
	Generation int64
	Succeeded  bool
	At         time.Time
	// Scope is where the server answered was stored (NT-019): a receipt
	// about a server of the name stored elsewhere is not this one's.
	Scope Scope
}

// MCPRediscoveryStatus is the latest request for one server and every worker's
// newest receipt for it. Generation zero is a server nobody has asked about.
type MCPRediscoveryStatus struct {
	Server      string
	Generation  int64
	RequestedBy UserID
	RequestedAt time.Time
	Receipts    []MCPRediscoveryReceipt
}

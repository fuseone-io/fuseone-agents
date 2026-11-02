package main

import "github.com/fuseone/agents/internal/httpapi"

// ledgerAPI is the API over the ledger with every read the ledger answers on
// its own, whichever store openStore returned: the runs, and a card's trail
// (#204). serve builds on it and the journey tests read through it, so a read
// left unwired here is an empty trail a test sees.
func ledgerAPI(store Store, version string) *httpapi.Server {
	return httpapi.NewServer(store, version).WithWorkItems(store)
}

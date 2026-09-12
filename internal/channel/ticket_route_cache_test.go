package channel

import (
	"fmt"
	"testing"
	"time"
)

func TestTicketRoutes_configurationChurnDoesNotGrowTheMatcherCacheForever(t *testing.T) {
	routes := &TicketRoutes{compiled: make(map[string]compiledTicketRule)}
	rule := TicketRule{
		OpenFrom: TicketOpenLinkedUsers, AddressFrom: "app:A-ticket",
		Patterns: []string{"api key"},
	}
	for i := range maxCompiledTicketRules + 1 {
		if _, err := routes.matcher(fmt.Sprintf("room-%d", i), time.Unix(int64(i), 0), rule); err != nil {
			t.Fatalf("matcher %d: %v", i, err)
		}
	}
	if len(routes.compiled) > maxCompiledTicketRules {
		t.Fatalf("matcher cache grew to %d entries, limit %d", len(routes.compiled), maxCompiledTicketRules)
	}
}

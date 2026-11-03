package domain_test

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

func TestSlackRequester_namesTheAccountAndReadsItBack(t *testing.T) {
	t.Parallel()

	requester := domain.SlackRequester("U0ANA")
	if requester != "slack:U0ANA" {
		t.Fatalf("requester = %q", requester)
	}
	account, ok := domain.SlackAccountOf(requester)
	if !ok || account != "U0ANA" {
		t.Errorf("account = %q, %v", account, ok)
	}
}

func TestSlackAccountOf_aPersonIsNoAccount(t *testing.T) {
	t.Parallel()

	for _, id := range []domain.UserID{"usr_dev", "", "slack:", "slack: U1", domain.SystemWorker} {
		if account, ok := domain.SlackAccountOf(id); ok {
			t.Errorf("SlackAccountOf(%q) = %q, want no account", id, account)
		}
	}
}

package connect

import (
	"context"
	"testing"
)

// An account is somebody in one workspace. Read through any other connection,
// a ticket's requester would be described by another company's Slack.
func TestAccount_isReadFromTheConnectionNamed(t *testing.T) {
	store, _ := stored(t)
	configure(t, store, "acme")
	configure(t, store, "globex")
	drivers, reached := spies(store)

	got, err := drivers.Account(context.Background(), "globex", "U09")
	if err != nil {
		t.Fatalf("Account: %v", err)
	}
	if _, wrong := reached["xoxb-acme"]; wrong || got.Name != "xoxb-globex/U09" {
		t.Fatalf("account = %+v, reached = %v, want the named connection's own bot", got, reached)
	}
}

func TestAccountEmail_isTheEmailTheNamedConnectionShows(t *testing.T) {
	store, _ := stored(t)
	configure(t, store, "acme")
	drivers, _ := spies(store)

	email, err := drivers.AccountEmail(context.Background(), "acme", "U09")
	if err != nil || email != "U09@xoxb-acme" {
		t.Fatalf("AccountEmail = %q, %v", email, err)
	}
}

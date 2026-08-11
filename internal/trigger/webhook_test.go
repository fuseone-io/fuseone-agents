package trigger_test

import (
	"testing"

	"github.com/fuseone/agents/internal/trigger"
)

// A webhook is a door into the installation with an agent behind it. These are
// the properties that keep it from being an open one.

func TestNewSecret_isNotRecoverableFromWhatIsStored(t *testing.T) {
	t.Parallel()

	secret, hash, err := trigger.NewSecret()
	if err != nil {
		t.Fatalf("NewSecret: %v", err)
	}

	// A database dump must not hand somebody the ability to make agents run.
	if string(hash) == secret {
		t.Fatal("the stored value is the secret itself")
	}
	if !trigger.MatchesSecret(hash, secret) {
		t.Error("the secret does not match its own hash")
	}
}

func TestNewSecret_differsEveryTime(t *testing.T) {
	t.Parallel()

	first, _, _ := trigger.NewSecret()
	second, _, _ := trigger.NewSecret()
	if first == second {
		t.Fatal("two secrets came out identical")
	}
}

func TestMatchesSecret_refusesTheWrongSecret(t *testing.T) {
	t.Parallel()

	_, hash, _ := trigger.NewSecret()
	if trigger.MatchesSecret(hash, "not-the-secret") {
		t.Error("a wrong secret was accepted")
	}
}

func TestMatchesSecret_refusesEverythingWhenNoSecretIsSet(t *testing.T) {
	t.Parallel()

	// A declared path with no secret is closed, not open. The opposite would
	// leave an agent reachable by anybody who guessed the path.
	if trigger.MatchesSecret(nil, "") {
		t.Error("an unarmed hook accepted an empty secret")
	}
	if trigger.MatchesSecret(nil, "anything") {
		t.Error("an unarmed hook accepted a secret")
	}
}

func TestDeliveryKey_separatesPathsThatShareADeliveryId(t *testing.T) {
	t.Parallel()

	// Two senders choose delivery ids independently. Without the path in the
	// key, one system's delivery "1" would silence another's.
	if trigger.DeliveryKey("crm/ticket", "1") == trigger.DeliveryKey("erp/order", "1") {
		t.Error("two hooks collided on the same delivery id")
	}
}

func TestDeliveryKey_isStableForTheSameDelivery(t *testing.T) {
	t.Parallel()

	// The whole point: a redelivered webhook names the run that already exists.
	if trigger.DeliveryKey("crm/ticket", "d-9") != trigger.DeliveryKey("crm/ticket", "d-9") {
		t.Error("the same delivery produced two keys")
	}
}

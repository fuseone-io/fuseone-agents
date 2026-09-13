package connectors

import "testing"

func TestCatalog_graviteeDeclaresOnlyTheTicketOperations(t *testing.T) {
	t.Parallel()

	connector := connectorByID(t, "gravitee")
	if connector.Maturity != MaturityRuntime {
		t.Fatalf("gravitee maturity = %q, want runtime after the end-to-end proofs pass", connector.Maturity)
	}
	if len(connector.Operations) != 2 {
		t.Fatalf("gravitee operations = %d, want inspect and accept", len(connector.Operations))
	}

	inspect := operationByID(t, connector, "gravitee.inspect_subscription")
	if !hasEffect(inspect, EffectRead) || inspect.Approval != ApprovalNone {
		t.Fatalf("inspect = %+v, want a read that can build the approval snapshot", inspect)
	}
	accept := operationByID(t, connector, "gravitee.accept_subscription")
	if !hasEffect(accept, EffectWrite) || accept.Approval != ApprovalRequired {
		t.Fatalf("accept = %+v, want a write with required approval", accept)
	}

	for _, op := range connector.Operations {
		if op.SecretHandling != SecretNone {
			t.Errorf("%s secret handling = %q, want credentials kept inside the worker", op.ID, op.SecretHandling)
		}
		if op.CachePolicy != CacheNever {
			t.Errorf("%s cache policy = %q, want explicit never", op.ID, op.CachePolicy)
		}
	}
}

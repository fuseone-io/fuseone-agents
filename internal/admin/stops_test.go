package admin_test

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
)

func TestStops_survivesTheProcessThatThrewIt(t *testing.T) {
	pool := freshPool(t)
	ctx := context.Background()

	// An in-memory switch comes back on when the process does, which is the
	// opposite of what somebody wants from the control they reach for during
	// an incident.
	if err := admin.NewStops(pool).Stop(ctx, domain.Stop{
		Level: domain.StopInstallation, By: "usr_ana",
		Reason: "incidente no provedor de pagamentos",
	}); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	inForce, err := admin.NewStops(pool).InForce(ctx)
	if err != nil {
		t.Fatalf("InForce: %v", err)
	}
	if len(inForce) != 1 || inForce[0].Level != domain.StopInstallation {
		t.Fatalf("InForce = %+v, want the installation stop", inForce)
	}
	if inForce[0].Reason == "" || inForce[0].By != "usr_ana" {
		t.Errorf("stop = %+v, want it to say who and why", inForce[0])
	}
}

func TestStops_started_leavesTheTrailShowingBoth(t *testing.T) {
	pool := freshPool(t)
	ctx := context.Background()
	stops := admin.NewStops(pool)
	stop := domain.Stop{
		Level: domain.StopScope, Scope: domain.Scope{Company: "acme", Area: "cx"},
		By: "usr_ana", Reason: "auditoria",
	}

	if err := stops.Stop(ctx, stop); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := stops.Start(ctx, stop); err != nil {
		t.Fatalf("Start: %v", err)
	}

	inForce, err := stops.InForce(ctx)
	if err != nil {
		t.Fatalf("InForce: %v", err)
	}
	if len(inForce) != 0 {
		t.Errorf("InForce = %+v, want nothing stopped", inForce)
	}

	// Starting again is not the same as never having stopped. An area that
	// was quiet for an hour is the explanation for an hour of no runs.
	events, _, err := admin.NewCurator(pool).Events(ctx, "", "", 10)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	var stopped, started bool
	for _, e := range events {
		stopped = stopped || e.Action == "platform.stopped"
		started = started || e.Action == "platform.started"
	}
	if !stopped || !started {
		t.Errorf("trail = %+v, want both the stop and the start", events)
	}
}

func TestStops_withNoReason_isRefused(t *testing.T) {
	pool := freshPool(t)

	// The first question in an incident call is "did we do this on purpose?".
	err := admin.NewStops(pool).Stop(context.Background(), domain.Stop{
		Level: domain.StopInstallation, By: "usr_ana",
	})
	if err == nil {
		t.Error("Stop with no reason was accepted")
	}
}

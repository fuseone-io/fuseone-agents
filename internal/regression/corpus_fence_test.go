package regression_test

import (
	"context"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/regression"
)

// The corpus fence is held shared by whatever copies an agent's authored material into a
// durable form, a seal included (NT-018 phase 2): erasure waits for it, and two holds do
// not wait for each other.
func TestCorpusFenceHold_makesAnExclusiveWait_andNotAnotherHold(t *testing.T) {
	pool := poolFor(t)
	fence := regression.NewCorpusFence(pool)
	agent := domain.AgentID("fence-" + time.Now().Format("150405.000000000"))
	ctx := context.Background()

	entered, release, held := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		held <- fence.Hold(ctx, agent, func(context.Context) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered

	if err := fence.Hold(ctx, agent, func(context.Context) error { return nil }); err != nil {
		t.Fatalf("a second hold: %v", err)
	}
	exclusive := make(chan error, 1)
	go func() {
		exclusive <- regression.NewStore(pool).Exclusive(ctx, []domain.AgentID{agent},
			func(context.Context) error { return nil })
	}()
	select {
	case err := <-exclusive:
		t.Fatalf("the exclusive fence did not wait for the hold: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	if err := <-held; err != nil {
		t.Fatalf("Hold: %v", err)
	}
	if err := <-exclusive; err != nil {
		t.Fatalf("Exclusive: %v", err)
	}
}

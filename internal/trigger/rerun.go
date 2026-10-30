package trigger

import (
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

// RerunKey names an intention to run again from a recorded run.
//
// The source is part of the key, so a key a client already spent opening some
// other run can never answer this request with that unrelated run.
func RerunKey(source domain.RunID, key string) string {
	return fmt.Sprintf("rerun:%s:%s", source, key)
}

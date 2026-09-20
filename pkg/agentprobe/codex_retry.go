package agentprobe

import (
	"context"
	"time"
)

// Native child activity can precede materialization of its readable turn.
// Retry only inside the original behavior deadline, without granting ownership.
func codexReadRetryPause(ctx context.Context) error {
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

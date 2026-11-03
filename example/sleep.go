package main

import (
	"context"
	"time"
)

// sleep ждёт указанное время или завершается досрочно при отмене контекста.
func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

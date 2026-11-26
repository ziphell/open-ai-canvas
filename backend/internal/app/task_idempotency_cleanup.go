package app

import (
	"context"
	"log"
	"time"
)

func (s *Service) CleanupTaskIdempotency(ctx context.Context) (int64, error) {
	return s.repo.WithContext(ctx).DeleteTaskIdempotencyBefore(time.Now(), 1000)
}

func (s *Service) startTaskIdempotencyCleanup(ctx context.Context) {
	s.runWorkerLoop(func(ctx context.Context) {
		cleanup := func() {
			deleted, err := s.CleanupTaskIdempotency(ctx)
			if err != nil && ctx.Err() == nil {
				log.Printf("task idempotency cleanup failed: deleted=%d error=%v", deleted, err)
			}
		}
		cleanup()
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cleanup()
			}
		}
	})
}

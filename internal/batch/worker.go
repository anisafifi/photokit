package batch

import (
	"context"
	"sync"
)

// Run executes fn for every item using at most workers goroutines. Items are
// handed out lazily; once ctx is cancelled no further items are started and
// Run returns after in-flight calls finish. fn receives the item index so
// callers can store results in a pre-sized slice without locking.
func Run[T any](ctx context.Context, items []T, workers int, fn func(ctx context.Context, idx int, item T)) {
	if len(items) == 0 {
		return
	}
	if workers < 1 {
		workers = 1
	}
	if workers > len(items) {
		workers = len(items)
	}

	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				fn(ctx, i, items[i])
			}
		}()
	}

feed:
	for i := range items {
		select {
		case <-ctx.Done():
			break feed
		case jobs <- i:
		}
	}
	close(jobs)
	wg.Wait()
}

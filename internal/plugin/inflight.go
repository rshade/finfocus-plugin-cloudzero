package plugin

import (
	"context"
	"sync"
	"time"

	"github.com/rshade/finfocus-plugin-cloudzero/internal/cache"
)

// cachedFetch returns the value cached under key or, on a miss, the result of
// one fetch shared by every concurrent caller with that key. The fetch runs
// detached from its first caller's cancellation, and its result is cached for
// ttl. A failed fetch is not cached.
func cachedFetch[T any](
	ctx context.Context, store *cache.Cache, calls *inflight[T], key string, ttl time.Duration,
	fetch func(context.Context) (T, error),
) (T, error) {
	if value, ok := cachedAs[T](store, key); ok {
		return value, nil
	}
	return calls.do(ctx, key, func() (T, error) {
		// A call for this key may have finished, and filled the cache, after
		// the check above.
		if value, ok := cachedAs[T](store, key); ok {
			return value, nil
		}
		fetchCtx, cancel := detached(ctx)
		defer cancel()
		value, err := fetch(fetchCtx)
		if err != nil {
			var zero T
			return zero, err
		}
		store.Set(key, value, ttl)
		return value, nil
	})
}

func cachedAs[T any](store *cache.Cache, key string) (T, bool) {
	cached, ok := store.Get(key)
	if !ok {
		var zero T
		return zero, false
	}
	value, cast := cached.(T)
	return value, cast
}

// detached keeps ctx's values and deadline but not its cancellation, so a
// caller that gives up does not stop a fetch that others wait on, and the
// fetch still ends by the deadline its first caller had.
func detached(ctx context.Context) (context.Context, context.CancelFunc) {
	free := context.WithoutCancel(ctx)
	if deadline, ok := ctx.Deadline(); ok {
		return context.WithDeadline(free, deadline)
	}
	return free, func() {}
}

// inflight shares one CloudZero fetch among concurrent callers with the same
// key. The fetch runs on its own goroutine, so a caller that gives up does not
// stop it for the others. A finished call leaves the map before its done
// channel closes, so the next caller after a failure starts a new fetch.
type inflight[T any] struct {
	mu    sync.Mutex
	calls map[string]*call[T]
}

type call[T any] struct {
	done    chan struct{}
	result  T
	err     error
	waiters int
}

func newInflight[T any]() *inflight[T] {
	return &inflight[T]{mu: sync.Mutex{}, calls: map[string]*call[T]{}}
}

// do returns the result of the fetch in flight for key, starting fetch when
// there is none. It returns ctx.Err() if ctx ends first, and starts nothing
// for a ctx that has already ended.
func (g *inflight[T]) do(ctx context.Context, key string, fetch func() (T, error)) (T, error) {
	var zero T
	g.mu.Lock()
	c, ok := g.calls[key]
	if !ok {
		if err := ctx.Err(); err != nil {
			g.mu.Unlock()
			return zero, err
		}
		c = &call[T]{done: make(chan struct{}), result: zero, err: nil, waiters: 0}
		g.calls[key] = c
		go g.run(key, c, fetch)
	}
	c.waiters++
	g.mu.Unlock()
	defer g.leave(c)

	select {
	case <-c.done:
		return c.result, c.err
	case <-ctx.Done():
		return zero, ctx.Err()
	}
}

func (g *inflight[T]) run(key string, c *call[T], fetch func() (T, error)) {
	c.result, c.err = fetch()
	g.mu.Lock()
	delete(g.calls, key)
	g.mu.Unlock()
	close(c.done)
}

func (g *inflight[T]) leave(c *call[T]) {
	g.mu.Lock()
	c.waiters--
	g.mu.Unlock()
}

// waiting reports how many callers are waiting on unfinished fetches.
func (g *inflight[T]) waiting() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	total := 0
	for _, c := range g.calls {
		total += c.waiters
	}
	return total
}

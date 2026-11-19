package plugin

import (
	"context"
	"sync"

	"github.com/rshade/finfocus-plugin-cloudzero/internal/czclient"
)

// inflight shares one CloudZero fetch among concurrent callers with the same
// key. The fetch runs on its own goroutine, so a caller that gives up does not
// stop it for the others. A finished call leaves the map before its done
// channel closes, so the next caller after a failure starts a new fetch.
type inflight struct {
	mu    sync.Mutex
	calls map[string]*call
}

type call struct {
	done    chan struct{}
	rows    []czclient.CostRow
	err     error
	waiters int
}

func newInflight() *inflight {
	return &inflight{mu: sync.Mutex{}, calls: map[string]*call{}}
}

// do returns the result of the fetch in flight for key, starting fetch when
// there is none. It returns ctx.Err() if ctx ends first, and starts nothing
// for a ctx that has already ended.
func (g *inflight) do(
	ctx context.Context, key string, fetch func() ([]czclient.CostRow, error),
) ([]czclient.CostRow, error) {
	g.mu.Lock()
	c, ok := g.calls[key]
	if !ok {
		if err := ctx.Err(); err != nil {
			g.mu.Unlock()
			return nil, err
		}
		c = &call{done: make(chan struct{}), rows: nil, err: nil, waiters: 0}
		g.calls[key] = c
		go g.run(key, c, fetch)
	}
	c.waiters++
	g.mu.Unlock()
	defer g.leave(c)

	select {
	case <-c.done:
		return c.rows, c.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (g *inflight) run(key string, c *call, fetch func() ([]czclient.CostRow, error)) {
	c.rows, c.err = fetch()
	g.mu.Lock()
	delete(g.calls, key)
	g.mu.Unlock()
	close(c.done)
}

func (g *inflight) leave(c *call) {
	g.mu.Lock()
	c.waiters--
	g.mu.Unlock()
}

// waiting reports how many callers are waiting on unfinished fetches.
func (g *inflight) waiting() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	total := 0
	for _, c := range g.calls {
		total += c.waiters
	}
	return total
}

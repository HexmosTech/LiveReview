//go:build !unix

package repocache

import (
	"context"
	"sync"
	"time"
)

// locks: entry -> *sync.Mutex. ponytail: in-process only, so run a single worker on non-unix hosts.
var locks sync.Map

func lockEntry(ctx context.Context, entry string, wait bool) (unlock func(), ok bool, err error) {
	mu, _ := locks.LoadOrStore(entry, &sync.Mutex{})
	m := mu.(*sync.Mutex)
	for !m.TryLock() {
		if !wait {
			return nil, false, nil
		}
		select {
		case <-ctx.Done():
			return nil, false, ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return m.Unlock, true, nil
}

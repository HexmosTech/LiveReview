//go:build unix

package repocache

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// lockEntry takes <entry>.lock with flock, so every LiveReview process (workers, API) sees it.
// wait=false returns ok=false at once when another job holds it; wait=true retries until ctx ends.
func lockEntry(ctx context.Context, entry string, wait bool) (unlock func(), ok bool, err error) {
	path := entry + ".lock"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, false, err
	}
	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
		if err != nil {
			return nil, false, err
		}
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			// The previous holder may have deleted the file along with its repo; lock the current one.
			held, err1 := f.Stat()
			now, err2 := os.Stat(path)
			if err1 == nil && err2 == nil && os.SameFile(held, now) {
				return func() { f.Close() }, true, nil // closing the file releases the lock
			}
			f.Close()
			continue
		}
		f.Close()
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, false, err
		}
		if !wait {
			return nil, false, nil
		}
		select {
		case <-ctx.Done():
			return nil, false, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

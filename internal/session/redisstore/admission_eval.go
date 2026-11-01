package redisstore

import (
	"context"
	"time"
)

// A single EVAL on the owner's no-retry client preserves uncertain/partial
// charges. Never use EVALSHA fallback, retry NOSCRIPT, or refund a later failure.
func runAdmission(ctx context.Context, store *Store, script string, keys []string, args []any) (bool, error) {
	if ctx == nil {
		return false, ErrInvalid
	}
	call, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := call.Err(); err != nil {
		return false, err
	}
	value, err := store.client.Eval(call, script, keys, args...).Result()
	if err != nil {
		return false, storageError{err}
	}
	if err = call.Err(); err != nil {
		return false, storageError{err}
	}
	code, ok := value.(int64)
	if !ok || (code != 0 && code != 1) {
		return false, storageError{ErrConflict}
	}
	return code == 1, nil
}

package sesh

import (
	"context"
	"time"
)

// Store is the interface for session stores.
type Store interface {
	// Find should return the data for a session id from the store. If the
	// session id is not found, expired or tampered, the data will be nil and the
	// time will be zero, but there will be no error. The err return value should
	// be used for system errors only.
	Find(ctx context.Context, id string) (data []byte, expiry time.Time, err error)

	// Upsert the session id data and expiry to the store, with the given If the
	// session id already exists, then the data and expiry time should be
	// overwritten.
	Upsert(ctx context.Context, id string, data []byte, expiry time.Time) (err error)

	// Delete removes the session id and corresponding data from the session
	// store. If the id does not exist then Delete should be a no-op and return
	// nil (not an error).
	Delete(ctx context.Context, id string) (err error)
}

package sesh

import (
	"context"
	"time"
)

// Store is the interface for session stores.
type Store interface {
	// Delete should remove the session id and corresponding data from the
	// session store. If the id does not exist then Delete should be a no-op
	// and return nil (not an error).
	Delete(ctx context.Context, id string) (err error)

	// Find should return the data for a session id from the store. If the
	// session id is not found or is expired, the found return value should
	// be false (and the err return value should be nil). Similarly, tampered
	// or malformed tokens should result in a found return value of false and a
	// nil err value. The err return value should be used for system errors only.
	Load(ctx context.Context, id string) (data []byte, expiry time.Time, err error)

	// Save the session id and data to the store, with the given
	// expiry time. If the session id already exists, then the data and
	// expiry time should be overwritten.
	Save(ctx context.Context, id string, data []byte, expiry time.Time) (err error)
}

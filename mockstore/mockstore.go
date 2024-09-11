package mockstore

import (
	"context"
	"time"

	"github.com/matthewmueller/sesh"
)

func New() *Store {
	return &Store{}
}

// Store is a mock session store
type Store struct {
	MockFind   func(ctx context.Context, id string) (data []byte, expiry time.Time, err error)
	MockUpsert func(ctx context.Context, id string, data []byte, expiry time.Time) (err error)
	MockDelete func(ctx context.Context, id string) (err error)
}

var _ sesh.Store = (*Store)(nil)

func (m *Store) Find(ctx context.Context, id string) (data []byte, expiry time.Time, err error) {
	return m.MockFind(ctx, id)
}

func (m *Store) Upsert(ctx context.Context, id string, data []byte, expiry time.Time) (err error) {
	return m.MockUpsert(ctx, id, data, expiry)
}

func (m *Store) Delete(ctx context.Context, id string) (err error) {
	return m.MockDelete(ctx, id)
}

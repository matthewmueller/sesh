package mockstore_test

import (
	"context"
	"testing"
	"time"

	"github.com/matryer/is"
	"github.com/matthewmueller/sesh/mockstore"
)

func TestMockStore(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	store := mockstore.New()
	inputExpires := time.Now().Add(1 * time.Hour)
	calls := 0

	// Mock Find function
	store.MockFind = func(ctx context.Context, id string) (data []byte, expiry time.Time, err error) {
		calls++
		if id == "test-id" {
			return []byte("test-data"), inputExpires, nil
		}
		return nil, time.Time{}, nil
	}
	// Mock Upsert function
	store.MockUpsert = func(ctx context.Context, id string, data []byte, expiry time.Time) error {
		calls++
		return nil
	}
	// Mock Delete function
	store.MockDelete = func(ctx context.Context, id string) error {
		calls++
		return nil
	}
	// Test Find
	data, expiry, err := store.Find(ctx, "test-id")
	is.NoErr(err)
	is.Equal("test-data", string(data))
	is.Equal(inputExpires.Unix(), expiry.Unix())

	// Test Upsert
	is.NoErr(store.Upsert(ctx, "test-id", []byte("new-data"), inputExpires))
	is.NoErr(err)

	// Test Delete
	err = store.Delete(ctx, "test-id")
	is.NoErr(err)
}

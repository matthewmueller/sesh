package sqstore_test

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/matryer/is"
	"github.com/matthewmueller/sesh/sqstore"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/sync/errgroup"
)

func TestUpsertFind(t *testing.T) {
	ctx := context.Background()
	is := is.New(t)
	dir := t.TempDir()
	db, err := sql.Open("sqlite3", filepath.Join(dir, "test.db"))
	is.NoErr(err)
	defer db.Close()
	store := sqstore.New(db)
	is.NoErr(store.Migrate(ctx))
	inputData := []byte("encoded_data")
	inputExpiry := time.Now().Add(time.Minute)
	err = store.Upsert(ctx, "session_token", inputData, inputExpiry)
	is.NoErr(err)
	actualData, actualExpiry, err := store.Find(ctx, "session_token")
	is.NoErr(err)
	is.Equal(string(actualData), string(inputData))
	is.Equal(actualExpiry.Unix(), inputExpiry.Unix())
}

func TestFindMissing(t *testing.T) {
	ctx := context.Background()
	is := is.New(t)
	dir := t.TempDir()
	db, err := sql.Open("sqlite3", filepath.Join(dir, "test.db"))
	is.NoErr(err)
	defer db.Close()
	store := sqstore.New(db)
	is.NoErr(store.Migrate(ctx))
	data, expiry, err := store.Find(ctx, "session_token")
	is.NoErr(err)
	is.Equal(data, nil)
	is.True(expiry.IsZero())
}

func TestDelete(t *testing.T) {
	ctx := context.Background()
	is := is.New(t)
	dir := t.TempDir()
	db, err := sql.Open("sqlite3", filepath.Join(dir, "test.db"))
	is.NoErr(err)
	defer db.Close()
	store := sqstore.New(db)
	is.NoErr(store.Migrate(ctx))
	inputData := []byte("encoded_data")
	inputExpiry := time.Now().Add(time.Minute)
	err = store.Upsert(ctx, "session_token", inputData, inputExpiry)
	is.NoErr(err)
	actualData, actualExpiry, err := store.Find(ctx, "session_token")
	is.NoErr(err)
	is.Equal(string(actualData), string(inputData))
	is.Equal(actualExpiry.Unix(), inputExpiry.Unix())
	err = store.Delete(ctx, "session_token")
	is.NoErr(err)
	data, expiry, err := store.Find(ctx, "session_token")
	is.NoErr(err)
	is.Equal(data, nil)
	is.True(expiry.IsZero())
}

func TestUpdate(t *testing.T) {
	ctx := context.Background()
	is := is.New(t)
	dir := t.TempDir()
	db, err := sql.Open("sqlite3", filepath.Join(dir, "test.db"))
	is.NoErr(err)
	defer db.Close()
	store := sqstore.New(db)
	is.NoErr(store.Migrate(ctx))
	inputData := []byte("encoded_data")
	inputExpiry := time.Now().Add(time.Minute)
	err = store.Upsert(ctx, "session_token", inputData, inputExpiry)
	is.NoErr(err)
	actualData, actualExpiry, err := store.Find(ctx, "session_token")
	is.NoErr(err)
	is.Equal(string(actualData), string(inputData))
	is.Equal(actualExpiry.Unix(), inputExpiry.Unix())
	newData := []byte("new_encoded_data")
	newExpiry := time.Now().Add(time.Minute)
	err = store.Upsert(ctx, "session_token", newData, newExpiry)
	is.NoErr(err)
	actualData, actualExpiry, err = store.Find(ctx, "session_token")
	is.NoErr(err)
	is.Equal(string(actualData), string(newData))
	is.Equal(actualExpiry.Unix(), newExpiry.Unix())
}

func TestCleanup(t *testing.T) {
	ctx := context.Background()
	is := is.New(t)
	dir := t.TempDir()
	db, err := sql.Open("sqlite3", filepath.Join(dir, "test.db"))
	is.NoErr(err)
	defer db.Close()
	store := sqstore.New(db)
	is.NoErr(store.Migrate(ctx))
	inputData := []byte("encoded_data")
	inputExpiry := time.Now().Add(-time.Minute)
	err = store.Upsert(ctx, "session_token", inputData, inputExpiry)
	is.NoErr(err)
	data, expiry, err := store.Find(ctx, "session_token")
	is.NoErr(err)
	is.Equal(data, nil)
	is.True(expiry.IsZero())
	err = store.Cleanup(ctx)
	is.NoErr(err)
	data, expiry, err = store.Find(ctx, "session_token")
	is.NoErr(err)
	is.Equal(data, nil)
	is.True(expiry.IsZero())
}

func TestReset(t *testing.T) {
	ctx := context.Background()
	is := is.New(t)
	dir := t.TempDir()
	db, err := sql.Open("sqlite3", filepath.Join(dir, "test.db"))
	is.NoErr(err)
	defer db.Close()
	store := sqstore.New(db)
	is.NoErr(store.Migrate(ctx))
	err = store.Upsert(ctx, "s1", []byte("1"), time.Now().Add(time.Minute))
	is.NoErr(err)
	err = store.Upsert(ctx, "s2", []byte("2"), time.Now().Add(time.Minute))
	is.NoErr(err)
	err = store.Reset(ctx)
	is.NoErr(err)
	data, expiry, err := store.Find(ctx, "s1")
	is.NoErr(err)
	is.Equal(data, nil)
	is.True(expiry.IsZero())
	data, expiry, err = store.Find(ctx, "s2")
	is.NoErr(err)
	is.Equal(data, nil)
	is.True(expiry.IsZero())
}

func TestParallel(t *testing.T) {
	ctx := context.Background()
	is := is.New(t)
	dir := t.TempDir()
	db, err := sql.Open("sqlite3", filepath.Join(dir, "test.db"))
	is.NoErr(err)
	defer db.Close()
	store := sqstore.New(db)
	is.NoErr(store.Migrate(ctx))

	eg := errgroup.Group{}
	for i := 0; i < 100; i++ {
		eg.Go(func() error {
			inputKey := "s" + strconv.Itoa(i)
			inputExpiry := time.Now().Add(time.Minute)
			inputData := []byte("d" + strconv.Itoa(i))
			err := store.Upsert(ctx, inputKey, inputData, inputExpiry)
			if err != nil {
				return err
			}
			actualData, actualExpiry, err := store.Find(ctx, inputKey)
			if err != nil {
				return err
			}
			if string(actualData) != "d"+strconv.Itoa(i) {
				return fmt.Errorf("expected %q, got %q", "d"+strconv.Itoa(i), string(actualData))
			}
			if actualExpiry.Unix() != inputExpiry.Unix() {
				return fmt.Errorf("expected %v, got %v", inputExpiry, actualExpiry)
			}
			return nil
		})
	}
	is.NoErr(eg.Wait())
}

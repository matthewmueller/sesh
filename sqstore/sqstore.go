package sqstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/matthewmueller/sesh"
)

// Schema for the session table
const schema = `
	CREATE TABLE IF NOT EXISTS %[1]s (
		id TEXT PRIMARY KEY,
		data BLOB NOT NULL,
		expiry INTEGER NOT NULL
	);
	CREATE INDEX IF NOT EXISTS %[1]s_expiry_idx ON %[1]s(expiry);
`

func New(db *sql.DB) *Store {
	return &Store{db, "sessions", time.Now}
}

type Store struct {
	db    *sql.DB
	Table string

	// Used for testing
	Now func() time.Time
}

var _ sesh.Store = (*Store)(nil)

func (s *Store) Migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, fmt.Sprintf(schema, s.Table))
	return err
}

// Find returns the data for a session id from the store. If the session is not
// found, expired or tampered, the data will be nil and the time will be zero,
// but there will be no error.
func (s *Store) Find(ctx context.Context, id string) (data []byte, expiry time.Time, err error) {
	const query = `SELECT data, expiry FROM %[1]s WHERE id = ?`
	row := s.db.QueryRowContext(ctx, fmt.Sprintf(query, s.Table), id)
	var unixTimeSec int64
	if err := row.Scan(&data, &unixTimeSec); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, time.Time{}, err
		}
		// No session found
		return nil, time.Time{}, nil
	}
	// Check if the session has expired
	expiry = time.Unix(unixTimeSec, 0)
	if expiry.Before(s.Now()) {
		return nil, time.Time{}, nil
	}
	return data, expiry, nil
}

func (s *Store) Delete(ctx context.Context, id string) error {
	const sql = `DELETE FROM %[1]s WHERE id = ?`
	_, err := s.db.ExecContext(ctx, fmt.Sprintf(sql, s.Table), id)
	return err
}

func (s *Store) Upsert(ctx context.Context, id string, data []byte, expiry time.Time) error {
	const sql = `INSERT INTO %[1]s (id, data, expiry) VALUES (?, ?, ?) ON CONFLICT(id) DO UPDATE SET data = ?, expiry = ?`
	unixTimeSec := expiry.Unix()
	_, err := s.db.ExecContext(ctx, fmt.Sprintf(sql, s.Table), id, data, unixTimeSec, data, unixTimeSec)
	return err
}

// Cleanup removes expired sessions from the store.
func (s *Store) Cleanup(ctx context.Context) error {
	const sql = `DELETE FROM %[1]s WHERE expiry < ?`
	_, err := s.db.ExecContext(ctx, fmt.Sprintf(sql, s.Table), s.Now().Unix())
	return err
}

// Reset removes all sessions from the store.
func (s *Store) Reset(ctx context.Context) error {
	const sql = `DELETE FROM %[1]s`
	_, err := s.db.ExecContext(ctx, fmt.Sprintf(sql, s.Table))
	return err
}

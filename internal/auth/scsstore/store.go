package scsstore

import (
	"bytes"
	"database/sql"
	"encoding/gob"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Store implements scs.Store backed by PostgreSQL.
// The sessions table carries a user_id column (NULL for pre-login sessions)
// extracted from the gob-encoded SCS session data on each Commit, enabling
// admin session listing and per-user revocation without a separate table.
type Store struct{ db *sql.DB }

func New(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) Find(token string) ([]byte, bool, error) {
	var data []byte
	err := s.db.QueryRow(
		`SELECT data FROM sessions WHERE token = $1 AND expiry > NOW()`,
		token,
	).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("find session: %w", err)
	}
	return data, true, nil
}

func (s *Store) Commit(token string, b []byte, expiry time.Time) error {
	userID := extractUserID(b)
	_, err := s.db.Exec(
		`INSERT INTO sessions (token, data, expiry, user_id)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (token) DO UPDATE
		   SET data = EXCLUDED.data,
		       expiry = EXCLUDED.expiry,
		       user_id = COALESCE(EXCLUDED.user_id, sessions.user_id)`,
		token, b, expiry, userID,
	)
	if err != nil {
		return fmt.Errorf("commit session: %w", err)
	}
	return nil
}

func (s *Store) Delete(token string) error {
	if _, err := s.db.Exec(`DELETE FROM sessions WHERE token = $1`, token); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// extractUserID decodes the SCS gob payload and returns the "userID" value as
// a *uuid.UUID for the nullable user_id column. Returns nil for pre-login sessions.
func extractUserID(b []byte) *uuid.UUID {
	var sd struct {
		Deadline time.Time
		Values   map[string]any
	}
	if err := gob.NewDecoder(bytes.NewReader(b)).Decode(&sd); err != nil {
		return nil
	}
	v, ok := sd.Values["userID"]
	if !ok {
		return nil
	}
	s, ok := v.(string)
	if !ok {
		return nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return nil
	}
	return &id
}

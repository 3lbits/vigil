package scsstore

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/gob"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

// encodeSCSPayload creates a gob-encoded SCS session payload matching the
// format that scs writes when saving a session.
func encodeSCSPayload(t *testing.T, values map[string]any) []byte {
	t.Helper()
	type scsSession struct {
		Deadline time.Time
		Values   map[string]any
	}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(scsSession{
		Deadline: time.Now().Add(time.Hour),
		Values:   values,
	}); err != nil {
		t.Fatalf("gob encode: %v", err)
	}
	return buf.Bytes()
}

// ── extractUserID ─────────────────────────────────────────────────────────────

func TestExtractUserID_ValidUUID(t *testing.T) {
	id := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	b := encodeSCSPayload(t, map[string]any{"userID": id.String()})

	got := extractUserID(b)
	if got == nil {
		t.Fatal("expected non-nil UUID, got nil")
	}
	if *got != id {
		t.Errorf("got %v, want %v", *got, id)
	}
}

func TestExtractUserID_MissingKey_ReturnsNil(t *testing.T) {
	b := encodeSCSPayload(t, map[string]any{"other": "value"})
	if got := extractUserID(b); got != nil {
		t.Errorf("expected nil for missing key, got %v", *got)
	}
}

func TestExtractUserID_WrongType_ReturnsNil(t *testing.T) {
	b := encodeSCSPayload(t, map[string]any{"userID": 42})
	if got := extractUserID(b); got != nil {
		t.Errorf("expected nil for wrong type, got %v", *got)
	}
}

func TestExtractUserID_MalformedUUIDString_ReturnsNil(t *testing.T) {
	b := encodeSCSPayload(t, map[string]any{"userID": "not-a-uuid"})
	if got := extractUserID(b); got != nil {
		t.Errorf("expected nil for malformed UUID string, got %v", *got)
	}
}

func TestExtractUserID_MalformedGob_ReturnsNil(t *testing.T) {
	if got := extractUserID([]byte("not-gob")); got != nil {
		t.Errorf("expected nil for malformed gob, got %v", *got)
	}
}

func TestExtractUserID_EmptyPayload_ReturnsNil(t *testing.T) {
	if got := extractUserID([]byte{}); got != nil {
		t.Errorf("expected nil for empty payload, got %v", *got)
	}
}

type sqlStubState struct {
	queryRows     [][]driver.Value
	queryErr      error
	execErr       error
	lastQuery     string
	lastExecQuery string
	lastExecArgs  []driver.NamedValue
}

type sqlStubDriver struct{ state *sqlStubState }
type sqlStubConn struct{ state *sqlStubState }
type sqlStubRows struct {
	cols []string
	rows [][]driver.Value
	idx  int
}

func (d *sqlStubDriver) Open(string) (driver.Conn, error) { return &sqlStubConn{state: d.state}, nil }
func (c *sqlStubConn) Close() error                       { return nil }
func (c *sqlStubConn) Begin() (driver.Tx, error)          { return nil, errors.New("unsupported") }
func (c *sqlStubConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unsupported")
}
func (c *sqlStubConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	c.state.lastQuery = query
	if c.state.queryErr != nil {
		return nil, c.state.queryErr
	}
	return &sqlStubRows{
		cols: []string{"data"},
		rows: c.state.queryRows,
	}, nil
}
func (c *sqlStubConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.state.lastExecQuery = query
	c.state.lastExecArgs = args
	if c.state.execErr != nil {
		return nil, c.state.execErr
	}
	return driver.RowsAffected(1), nil
}
func (r *sqlStubRows) Columns() []string { return r.cols }
func (r *sqlStubRows) Close() error      { return nil }
func (r *sqlStubRows) Next(dest []driver.Value) error {
	if r.idx >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.idx])
	r.idx++
	return nil
}

var sqlStubDriverSeq uint64

func openSQLStubDB(t *testing.T, state *sqlStubState) *sql.DB {
	t.Helper()
	name := "scsstore_sqlstub_" + uuid.NewString() + "_" + strings.ReplaceAll(time.Now().UTC().Format(time.RFC3339Nano), ":", "_")
	if state == nil {
		state = &sqlStubState{}
	}
	sql.Register(name, &sqlStubDriver{state: state})
	db, err := sql.Open(name, strconv.FormatUint(atomic.AddUint64(&sqlStubDriverSeq, 1), 10))
	if err != nil {
		t.Fatalf("open sql stub db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestStoreFind(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		state := &sqlStubState{queryRows: [][]driver.Value{{[]byte("payload")}}}
		s := New(openSQLStubDB(t, state))
		got, ok, err := s.Find("tok")
		if err != nil {
			t.Fatalf("find: %v", err)
		}
		if !ok || string(got) != "payload" {
			t.Fatalf("unexpected find result ok=%v data=%q", ok, string(got))
		}
	})

	t.Run("not found", func(t *testing.T) {
		s := New(openSQLStubDB(t, &sqlStubState{}))
		got, ok, err := s.Find("tok")
		if err != nil {
			t.Fatalf("find: %v", err)
		}
		if ok || got != nil {
			t.Fatalf("expected not found, got ok=%v data=%v", ok, got)
		}
	})

	t.Run("db error", func(t *testing.T) {
		s := New(openSQLStubDB(t, &sqlStubState{queryErr: errors.New("db down")}))
		_, _, err := s.Find("tok")
		if err == nil || !strings.Contains(err.Error(), "find session") {
			t.Fatalf("expected wrapped find session error, got %v", err)
		}
	})
}

func TestStoreCommit_Success(t *testing.T) {
	state := &sqlStubState{}
	s := New(openSQLStubDB(t, state))
	if err := s.Commit("tok", encodeSCSPayload(t, map[string]any{"userID": uuid.NewString()}), time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if !strings.Contains(state.lastExecQuery, "INSERT INTO sessions") {
		t.Fatalf("unexpected exec query: %q", state.lastExecQuery)
	}
	if len(state.lastExecArgs) == 0 {
		t.Fatal("expected exec args for commit")
	}
}

func TestStoreCommit_DBError(t *testing.T) {
	s := New(openSQLStubDB(t, &sqlStubState{execErr: errors.New("db down")}))
	err := s.Commit("tok", []byte("payload"), time.Now().Add(time.Hour))
	if err == nil || !strings.Contains(err.Error(), "commit session") {
		t.Fatalf("expected wrapped commit error, got %v", err)
	}
}

func TestStoreDelete_Success(t *testing.T) {
	state := &sqlStubState{}
	s := New(openSQLStubDB(t, state))
	if err := s.Delete("tok"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if !strings.Contains(state.lastExecQuery, "DELETE FROM sessions") {
		t.Fatalf("unexpected delete query: %q", state.lastExecQuery)
	}
}

func TestStoreDelete_DBError(t *testing.T) {
	s := New(openSQLStubDB(t, &sqlStubState{execErr: errors.New("db down")}))
	err := s.Delete("tok")
	if err == nil || !strings.Contains(err.Error(), "delete session") {
		t.Fatalf("expected wrapped delete error, got %v", err)
	}
}

// Package storage owns the SQLite schema, migrations and every query. All
// statements are scoped by user_id: the database holds many users and no
// query may leak another user's rows (PRD §5.3, §6 item 8).
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// ErrNotFound is returned when a statement matches no row.
var ErrNotFound = errors.New("storage: not found")

// ErrDuplicate is returned when a UNIQUE constraint is violated.
var ErrDuplicate = errors.New("storage: duplicate")

// DB wraps *sql.DB. The pool is limited to a single connection: SQLite has
// one writer, and letting database/sql open several write connections causes
// random SQLITE_BUSY (PRD §5.6).
type DB struct {
	*sql.DB
}

// nowFunc is the clock used for created_at/updated_at. Tests override it.
var nowFunc = func() time.Time { return time.Now().UTC() }

func nowStamp() string { return nowFunc().UTC().Format(time.RFC3339) }

// DSN builds the modernc.org/sqlite connection string for path. The path goes
// into Opaque, not Path: url.URL{Scheme: "file", Path: "./moneybot.db"} renders
// "file://./moneybot.db", whose authority is ".", and the driver rejects that
// with "invalid uri authority". Opaque renders the SQLite URI form
// "file:./moneybot.db", which works for relative and absolute paths alike.
func DSN(path string) string {
	u := url.URL{Scheme: "file", Opaque: path}
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "journal_mode(WAL)")
	u.RawQuery = q.Encode()
	return u.String()
}

// OpenRaw opens path without running migrations.
func OpenRaw(ctx context.Context, path string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", DSN(path))
	if err != nil {
		return nil, fmt.Errorf("buka database: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	if err := sqlDB.PingContext(ctx); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return &DB{DB: sqlDB}, nil
}

// Open opens path and brings the schema up to date.
func Open(ctx context.Context, path string) (*DB, error) {
	db, err := OpenRaw(ctx, path)
	if err != nil {
		return nil, err
	}
	if err := Migrate(ctx, db.DB); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// wrapErr maps driver errors onto the package's sentinel errors.
func wrapErr(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if strings.Contains(msg, "UNIQUE constraint failed") {
		return fmt.Errorf("%w: %s", ErrDuplicate, msg)
	}
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

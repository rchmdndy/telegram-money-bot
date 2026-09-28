package storage

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// LatestVersion returns the highest embedded migration version.
func LatestVersion() (int, error) {
	names, err := migrationNames()
	if err != nil {
		return 0, err
	}
	if len(names) == 0 {
		return 0, nil
	}
	return versionOf(names[len(names)-1])
}

func migrationNames() ([]string, error) {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("baca migrasi: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names, nil
}

func versionOf(name string) (int, error) {
	idx := strings.IndexByte(name, '_')
	if idx <= 0 {
		return 0, fmt.Errorf("nama migrasi tidak valid: %q", name)
	}
	v, err := strconv.Atoi(name[:idx])
	if err != nil {
		return 0, fmt.Errorf("nama migrasi tidak valid: %q", name)
	}
	return v, nil
}

// Migrate applies every embedded migration newer than PRAGMA user_version,
// each in its own transaction. Running it twice is a no-op (PRD §5.7).
func Migrate(ctx context.Context, db *sql.DB) error {
	var current int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&current); err != nil {
		return fmt.Errorf("baca user_version: %w", err)
	}
	names, err := migrationNames()
	if err != nil {
		return err
	}
	for _, name := range names {
		version, err := versionOf(name)
		if err != nil {
			return err
		}
		if version <= current {
			continue
		}
		body, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("baca migrasi %s: %w", name, err)
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("mulai migrasi %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("jalankan migrasi %s: %w", name, err)
		}
		// PRAGMA tidak bisa diparameterisasi, jadi dirangkai ke dalam SQL.
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
			tx.Rollback()
			return fmt.Errorf("set user_version %d: %w", version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migrasi %s: %w", name, err)
		}
		current = version
	}
	return nil
}

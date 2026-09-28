package storage

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/rchmdndy/telegram-money-bot/internal/persona"
)

// seedExpenseCategories are created for every new user (PRD §4.4).
var seedExpenseCategories = []string{"Makan", "Transport", "Rumah Tangga", "Kesehatan", "Hiburan", "Lainnya"}

// seedIncomeCategories are created for every new user (PRD §4.4).
var seedIncomeCategories = []string{"Gaji", "Lain-lain"}

// seedPeriodEffectiveFrom is a sentinel so transactions dated before the user
// registered still resolve to a period (ResolveActive picks the greatest
// effective_from <= date, and any real date sorts after 1970-01-01).
const seedPeriodEffectiveFrom = "1970-01-01"

// EnsureUser registers userID if needed and seeds categories, settings and
// the default period. It is idempotent: the second call reports created false
// and duplicates nothing.
//
// Seeding runs only for a user row this call created. It must not run on every
// call: the seed categories are keyed UNIQUE (user_id, kind, name), which is
// the same attribute /kategori rename changes, so re-inserting them would
// resurrect the old name as a second category.
func (db *DB) EnsureUser(ctx context.Context, userID int64) (bool, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("mulai transaksi: %w", err)
	}
	defer tx.Rollback()

	created := false
	res, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO users (user_id, created_at) VALUES (?, ?)`, userID, nowStamp())
	if err != nil {
		return false, wrapErr(err)
	}
	if n, err := res.RowsAffected(); err == nil && n > 0 {
		created = true
	}

	if created {
		if err := seedUser(ctx, tx, userID); err != nil {
			return false, err
		}
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit: %w", err)
	}
	return created, nil
}

// seedUser writes the default categories, settings and period of a brand new
// user. It runs inside EnsureUser's transaction and only when that call
// actually inserted the users row.
func seedUser(ctx context.Context, tx *sql.Tx, userID int64) error {
	order := 1
	for _, name := range seedExpenseCategories {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO categories (user_id, name, kind, active, sort_order, created_at)
			VALUES (?, ?, ?, 1, ?, ?)`, userID, name, string(KindExpense), order, nowStamp()); err != nil {
			return wrapErr(err)
		}
		order++
	}
	order = 1
	for _, name := range seedIncomeCategories {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO categories (user_id, name, kind, active, sort_order, created_at)
			VALUES (?, ?, ?, 1, ?, ?)`, userID, name, string(KindIncome), order, nowStamp()); err != nil {
			return wrapErr(err)
		}
		order++
	}

	// Persona default diambil dari katalog Go supaya nilai di kode dan
	// CHECK di skema tidak bisa menyimpang.
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO settings
		(user_id, reminder_enabled, reminder_time, last_reminder_date, persona)
		VALUES (?, 1, '19:00', '', ?)`, userID, string(persona.Default())); err != nil {
		return wrapErr(err)
	}

	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO periods
		(user_id, name, start_day, end_day, effective_from, created_at)
		VALUES (?, 'Gaji', 21, 20, ?, ?)`, userID, seedPeriodEffectiveFrom, nowStamp()); err != nil {
		return wrapErr(err)
	}
	return nil
}

// UserExists reports whether userID is registered.
func (db *DB) UserExists(ctx context.Context, userID int64) (bool, error) {
	var one int
	err := db.QueryRowContext(ctx, `SELECT 1 FROM users WHERE user_id = ?`, userID).Scan(&one)
	if err != nil {
		if isNoRows(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

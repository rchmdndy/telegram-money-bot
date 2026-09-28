package storage

import (
	"context"
	"database/sql"
	"fmt"
)

// TransactionRow is a transaction joined with its category.
type TransactionRow struct {
	Transaction
	CategoryName   string
	CategoryActive bool
}

const txSelect = `SELECT t.id, t.user_id, t.occurred_on, t.kind, t.category_id, t.amount, t.note,
       t.created_at, t.updated_at, c.name, c.active
  FROM transactions t JOIN categories c ON c.id = t.category_id`

func scanTxRow(row interface{ Scan(...any) error }) (TransactionRow, error) {
	var r TransactionRow
	var kind string
	var active int
	if err := row.Scan(&r.ID, &r.UserID, &r.OccurredOn, &kind, &r.CategoryID, &r.Amount, &r.Note,
		&r.CreatedAt, &r.UpdatedAt, &r.CategoryName, &active); err != nil {
		return TransactionRow{}, err
	}
	r.Kind = Kind(kind)
	r.CategoryActive = active != 0
	return r, nil
}

func scanTxRows(rows *sql.Rows) ([]TransactionRow, error) {
	defer rows.Close()
	var out []TransactionRow
	for rows.Next() {
		r, err := scanTxRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// checkCategory verifies that categoryID belongs to userID, is active-or-not
// but exists, and that its kind matches kind (PRD §5.3 note: SQLite cannot
// enforce a conditional FK, so it is checked here).
func (db *DB) checkCategory(ctx context.Context, userID, categoryID int64, kind Kind) error {
	var catKind string
	err := db.QueryRowContext(ctx, `SELECT kind FROM categories WHERE user_id = ? AND id = ?`,
		userID, categoryID).Scan(&catKind)
	if err != nil {
		if isNoRows(err) {
			return ErrNotFound
		}
		return err
	}
	if Kind(catKind) != kind {
		return fmt.Errorf("kategori bertipe %s tidak cocok dengan transaksi %s", catKind, kind)
	}
	return nil
}

// CreateTransaction inserts a transaction and returns its id. created_at and
// updated_at are both stamped now.
func (db *DB) CreateTransaction(ctx context.Context, tx Transaction) (int64, error) {
	if !tx.Kind.Valid() {
		return 0, fmt.Errorf("jenis transaksi tidak valid: %q", tx.Kind)
	}
	if err := db.checkCategory(ctx, tx.UserID, tx.CategoryID, tx.Kind); err != nil {
		return 0, err
	}
	now := nowStamp()
	res, err := db.ExecContext(ctx, `INSERT INTO transactions
		(user_id, occurred_on, kind, category_id, amount, note, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		tx.UserID, tx.OccurredOn, string(tx.Kind), tx.CategoryID, tx.Amount, tx.Note, now, now)
	if err != nil {
		return 0, wrapErr(err)
	}
	return res.LastInsertId()
}

// GetTransaction returns one transaction owned by userID.
func (db *DB) GetTransaction(ctx context.Context, userID, id int64) (TransactionRow, error) {
	r, err := scanTxRow(db.QueryRowContext(ctx, txSelect+` WHERE t.user_id = ? AND t.id = ?`, userID, id))
	if err != nil {
		if isNoRows(err) {
			return TransactionRow{}, ErrNotFound
		}
		return TransactionRow{}, err
	}
	return r, nil
}

// UpdateTransaction rewrites the editable fields. user_id and created_at are
// never touched; updated_at is always refreshed (PRD §4.9).
func (db *DB) UpdateTransaction(ctx context.Context, tx Transaction) error {
	if !tx.Kind.Valid() {
		return fmt.Errorf("jenis transaksi tidak valid: %q", tx.Kind)
	}
	if err := db.checkCategory(ctx, tx.UserID, tx.CategoryID, tx.Kind); err != nil {
		return err
	}
	res, err := db.ExecContext(ctx, `UPDATE transactions
		SET occurred_on = ?, kind = ?, category_id = ?, amount = ?, note = ?, updated_at = ?
		WHERE user_id = ? AND id = ?`,
		tx.OccurredOn, string(tx.Kind), tx.CategoryID, tx.Amount, tx.Note, nowStamp(), tx.UserID, tx.ID)
	if err != nil {
		return wrapErr(err)
	}
	return requireAffected(res)
}

// DeleteTransaction removes a transaction permanently (PRD §4.9: no soft
// delete, no undo).
func (db *DB) DeleteTransaction(ctx context.Context, userID, id int64) error {
	res, err := db.ExecContext(ctx, `DELETE FROM transactions WHERE user_id = ? AND id = ?`, userID, id)
	if err != nil {
		return wrapErr(err)
	}
	return requireAffected(res)
}

// ListTransactionsByDate returns userID's transactions on one date.
func (db *DB) ListTransactionsByDate(ctx context.Context, userID int64, date string) ([]TransactionRow, error) {
	rows, err := db.QueryContext(ctx, txSelect+` WHERE t.user_id = ? AND t.occurred_on = ? ORDER BY t.id`, userID, date)
	if err != nil {
		return nil, err
	}
	return scanTxRows(rows)
}

// ListRecentTransactions returns the newest transactions across dates.
func (db *DB) ListRecentTransactions(ctx context.Context, userID int64, limit int) ([]TransactionRow, error) {
	rows, err := db.QueryContext(ctx,
		txSelect+` WHERE t.user_id = ? ORDER BY t.occurred_on DESC, t.id DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	return scanTxRows(rows)
}

// ListTransactionsInRange returns userID's transactions in an inclusive range,
// oldest first.
func (db *DB) ListTransactionsInRange(ctx context.Context, userID int64, start, end string) ([]TransactionRow, error) {
	rows, err := db.QueryContext(ctx,
		txSelect+` WHERE t.user_id = ? AND t.occurred_on >= ? AND t.occurred_on <= ? ORDER BY t.occurred_on, t.id`,
		userID, start, end)
	if err != nil {
		return nil, err
	}
	return scanTxRows(rows)
}

// SumByCategoryOnDate totals one category's transactions on one date.
func (db *DB) SumByCategoryOnDate(ctx context.Context, userID, categoryID int64, date string) (int64, error) {
	var total int64
	err := db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(amount), 0) FROM transactions
		 WHERE user_id = ? AND category_id = ? AND occurred_on = ?`,
		userID, categoryID, date).Scan(&total)
	return total, err
}

// CountTransactionsInRange counts userID's transactions in an inclusive range.
func (db *DB) CountTransactionsInRange(ctx context.Context, userID int64, start, end string) (int, error) {
	var n int
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM transactions WHERE user_id = ? AND occurred_on >= ? AND occurred_on <= ?`,
		userID, start, end).Scan(&n)
	return n, err
}

// SumByKindInRange totals userID's transactions of one kind in an inclusive
// range. Used by the report and reminder renderers.
func (db *DB) SumByKindInRange(ctx context.Context, userID int64, kind Kind, start, end string) (int64, error) {
	var total int64
	err := db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(amount), 0) FROM transactions
		 WHERE user_id = ? AND kind = ? AND occurred_on >= ? AND occurred_on <= ?`,
		userID, string(kind), start, end).Scan(&total)
	return total, err
}

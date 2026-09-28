package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

func isNoRows(err error) bool { return errors.Is(err, sql.ErrNoRows) }

func scanCategory(row interface{ Scan(...any) error }) (Category, error) {
	var c Category
	var kind string
	var active int
	if err := row.Scan(&c.ID, &c.UserID, &c.Name, &kind, &active, &c.SortOrder, &c.CreatedAt); err != nil {
		return Category{}, err
	}
	c.Kind = Kind(kind)
	c.Active = active != 0
	return c, nil
}

const categoryCols = `id, user_id, name, kind, active, sort_order, created_at`

// CreateCategory inserts a category for userID, appending it after the
// existing ones of the same kind.
func (db *DB) CreateCategory(ctx context.Context, userID int64, name string, kind Kind) (int64, error) {
	if !kind.Valid() {
		return 0, fmt.Errorf("jenis kategori tidak valid: %q", kind)
	}
	var next int
	if err := db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(sort_order), 0) + 1 FROM categories WHERE user_id = ? AND kind = ?`,
		userID, string(kind)).Scan(&next); err != nil {
		return 0, err
	}
	res, err := db.ExecContext(ctx, `INSERT INTO categories (user_id, name, kind, active, sort_order, created_at)
		VALUES (?, ?, ?, 1, ?, ?)`, userID, name, string(kind), next, nowStamp())
	if err != nil {
		return 0, wrapErr(err)
	}
	return res.LastInsertId()
}

// ListCategories returns userID's categories of one kind.
func (db *DB) ListCategories(ctx context.Context, userID int64, kind Kind, activeOnly bool) ([]Category, error) {
	q := `SELECT ` + categoryCols + ` FROM categories WHERE user_id = ? AND kind = ?`
	if activeOnly {
		q += ` AND active = 1`
	}
	q += ` ORDER BY sort_order, name COLLATE NOCASE`
	rows, err := db.QueryContext(ctx, q, userID, string(kind))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Category
	for rows.Next() {
		c, err := scanCategory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetCategory returns one category owned by userID.
func (db *DB) GetCategory(ctx context.Context, userID, id int64) (Category, error) {
	c, err := scanCategory(db.QueryRowContext(ctx,
		`SELECT `+categoryCols+` FROM categories WHERE user_id = ? AND id = ?`, userID, id))
	if err != nil {
		if isNoRows(err) {
			return Category{}, ErrNotFound
		}
		return Category{}, err
	}
	return c, nil
}

// RenameCategory changes a category's name.
func (db *DB) RenameCategory(ctx context.Context, userID, id int64, name string) error {
	res, err := db.ExecContext(ctx, `UPDATE categories SET name = ? WHERE user_id = ? AND id = ?`, name, userID, id)
	if err != nil {
		return wrapErr(err)
	}
	return requireAffected(res)
}

// SetCategoryActive toggles a category's visibility.
func (db *DB) SetCategoryActive(ctx context.Context, userID, id int64, active bool) error {
	res, err := db.ExecContext(ctx, `UPDATE categories SET active = ? WHERE user_id = ? AND id = ?`,
		boolToInt(active), userID, id)
	if err != nil {
		return wrapErr(err)
	}
	return requireAffected(res)
}

// CountActiveCategories counts userID's active categories of one kind.
func (db *DB) CountActiveCategories(ctx context.Context, userID int64, kind Kind) (int, error) {
	var n int
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM categories WHERE user_id = ? AND kind = ? AND active = 1`,
		userID, string(kind)).Scan(&n)
	return n, err
}

// CategoryNameTaken reports whether name is already used by another category
// of the same user and kind (case-insensitive, per the DB UNIQUE index).
func (db *DB) CategoryNameTaken(ctx context.Context, userID int64, kind Kind, name string, exceptID int64) (bool, error) {
	var n int
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM categories WHERE user_id = ? AND kind = ? AND name = ? COLLATE NOCASE AND id <> ?`,
		userID, string(kind), name, exceptID).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// CategoryNames returns the active category names of one kind, joined with
// ", " — used for the "Pilihan: ..." error message.
func (db *DB) CategoryNames(ctx context.Context, userID int64, kind Kind) (string, error) {
	cats, err := db.ListCategories(ctx, userID, kind, true)
	if err != nil {
		return "", err
	}
	names := make([]string, 0, len(cats))
	for _, c := range cats {
		names = append(names, c.Name)
	}
	return strings.Join(names, ", "), nil
}

func requireAffected(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

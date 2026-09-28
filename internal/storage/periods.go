package storage

import (
	"context"
	"fmt"
)

// PeriodRecord is one row of the `periods` table (PRD §4.5).
type PeriodRecord struct {
	ID            int64
	UserID        int64
	Name          string
	StartDay      int
	EndDay        int
	EffectiveFrom string
	CreatedAt     string
}

// CreatePeriod adds a schedule change effective from effectiveFrom. Older
// dates keep resolving to the previous row because ResolveActive always picks
// the greatest effective_from <= date (PRD §4.5).
func (db *DB) CreatePeriod(ctx context.Context, userID int64, name string, startDay int, effectiveFrom string) (int64, error) {
	if startDay < 2 || startDay > 28 {
		return 0, fmt.Errorf("tanggal awal periode harus 2–28, bukan %d", startDay)
	}
	res, err := db.ExecContext(ctx, `INSERT INTO periods
		(user_id, name, start_day, end_day, effective_from, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		userID, name, startDay, startDay-1, effectiveFrom, nowStamp())
	if err != nil {
		return 0, wrapErr(err)
	}
	return res.LastInsertId()
}

// ListPeriods returns userID's periods, newest schedule first.
func (db *DB) ListPeriods(ctx context.Context, userID int64) ([]PeriodRecord, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, user_id, name, start_day, end_day, effective_from, created_at
		FROM periods WHERE user_id = ? ORDER BY effective_from DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PeriodRecord
	for rows.Next() {
		var p PeriodRecord
		if err := rows.Scan(&p.ID, &p.UserID, &p.Name, &p.StartDay, &p.EndDay, &p.EffectiveFrom, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DeletePeriod removes a schedule row owned by userID.
func (db *DB) DeletePeriod(ctx context.Context, userID, id int64) error {
	res, err := db.ExecContext(ctx, `DELETE FROM periods WHERE user_id = ? AND id = ?`, userID, id)
	if err != nil {
		return wrapErr(err)
	}
	return requireAffected(res)
}

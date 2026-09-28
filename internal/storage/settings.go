package storage

import "context"

// ReminderTarget is the minimal settings projection the scheduler needs.
type ReminderTarget struct {
	UserID       int64
	ReminderTime string
	Persona      string
}

// GetSettings returns userID's settings row.
func (db *DB) GetSettings(ctx context.Context, userID int64) (Settings, error) {
	var s Settings
	var enabled int
	err := db.QueryRowContext(ctx, `SELECT user_id, reminder_enabled, reminder_time, last_reminder_date, persona
		FROM settings WHERE user_id = ?`, userID).
		Scan(&s.UserID, &enabled, &s.ReminderTime, &s.LastReminderDate, &s.Persona)
	if err != nil {
		if isNoRows(err) {
			return Settings{}, ErrNotFound
		}
		return Settings{}, err
	}
	s.ReminderEnabled = enabled != 0
	return s, nil
}

func (db *DB) updateSetting(ctx context.Context, userID int64, clause string, args ...any) error {
	args = append(args, userID)
	res, err := db.ExecContext(ctx, `UPDATE settings SET `+clause+` WHERE user_id = ?`, args...)
	if err != nil {
		return wrapErr(err)
	}
	return requireAffected(res)
}

// SetReminderEnabled turns the daily reminder on or off.
func (db *DB) SetReminderEnabled(ctx context.Context, userID int64, enabled bool) error {
	return db.updateSetting(ctx, userID, `reminder_enabled = ?`, boolToInt(enabled))
}

// SetReminderTime stores the reminder time as 'HH:MM'.
func (db *DB) SetReminderTime(ctx context.Context, userID int64, hhmm string) error {
	return db.updateSetting(ctx, userID, `reminder_time = ?`, hhmm)
}

// SetLastReminderDate records the last day a reminder was sent, so the
// scheduler sends at most one per day per user (PRD §4.8).
func (db *DB) SetLastReminderDate(ctx context.Context, userID int64, date string) error {
	return db.updateSetting(ctx, userID, `last_reminder_date = ?`, date)
}

// SetPersona stores the chosen persona ID.
func (db *DB) SetPersona(ctx context.Context, userID int64, personaID string) error {
	return db.updateSetting(ctx, userID, `persona = ?`, personaID)
}

// ListReminderTargets returns users whose reminder is enabled and has not been
// sent today. last_reminder_date is NOT NULL DEFAULT ” precisely so this
// comparison never yields NULL (PRD §4.8).
func (db *DB) ListReminderTargets(ctx context.Context, today string) ([]ReminderTarget, error) {
	rows, err := db.QueryContext(ctx, `SELECT user_id, reminder_time, persona FROM settings
		WHERE reminder_enabled = 1 AND last_reminder_date <> ? ORDER BY user_id`, today)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReminderTarget
	for rows.Next() {
		var t ReminderTarget
		if err := rows.Scan(&t.UserID, &t.ReminderTime, &t.Persona); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// conversationCols keeps the column order in one place.
const conversationCols = `user_id, state, payload, updated_at`

// GetConversation returns the user's active conversation, or ErrNotFound when
// none is running (PRD §4.11: the row is deleted when a conversation ends).
func (db *DB) GetConversation(ctx context.Context, userID int64) (Conversation, error) {
	var c Conversation
	err := db.QueryRowContext(ctx, `SELECT `+conversationCols+` FROM conversations WHERE user_id = ?`, userID).
		Scan(&c.UserID, &c.State, &c.Payload, &c.UpdatedAt)
	if err != nil {
		if isNoRows(err) {
			return Conversation{}, ErrNotFound
		}
		return Conversation{}, err
	}
	return c, nil
}

// SetConversation upserts the user's conversation. updated_at always comes
// from nowFunc so callers cannot supply a stale timestamp.
func (db *DB) SetConversation(ctx context.Context, c Conversation) error {
	_, err := db.ExecContext(ctx, `INSERT INTO conversations (user_id, state, payload, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET state = excluded.state, payload = excluded.payload, updated_at = excluded.updated_at`,
		c.UserID, c.State, c.Payload, nowStamp())
	return wrapErr(err)
}

// DeleteConversation ends the user's conversation. Deleting a missing row is
// not an error.
func (db *DB) DeleteConversation(ctx context.Context, userID int64) error {
	_, err := db.ExecContext(ctx, `DELETE FROM conversations WHERE user_id = ?`, userID)
	return err
}

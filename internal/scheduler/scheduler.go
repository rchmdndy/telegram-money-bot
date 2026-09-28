// Package scheduler sends the daily reminder (PRD §4.8). It owns only the
// clock: a goroutine wakes on a fixed interval, asks storage which users are
// due today, and hands each one to a sender. Everything user-facing — the
// message text, the persona, the summary of the day — belongs to the sender.
package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/dandy/telegram_money_bot/internal/storage"
)

// DefaultInterval is how often the scheduler looks for due reminders
// (PRD §5.1: goroutine + time.Ticker 30s).
const DefaultInterval = 30 * time.Second

// dateLayout is the format of settings.last_reminder_date and of every date
// the scheduler compares (PRD §5.3).
const dateLayout = "2006-01-02"

// timeLayout is the format of settings.reminder_time ('HH:MM').
const timeLayout = "15:04"

// ReminderSender delivers one user's reminder. bot.Handler implements it; the
// scheduler needs nothing else from the bot package.
type ReminderSender interface {
	SendReminder(ctx context.Context, userID int64) error
}

// Scheduler wakes on an interval and sends the reminders that are due.
type Scheduler struct {
	db       *storage.DB
	sender   ReminderSender
	loc      *time.Location
	interval time.Duration
	now      func() time.Time
	log      *slog.Logger
}

// New builds a Scheduler. A nil location falls back to UTC and a nil logger to
// slog.Default, matching bot.New.
func New(db *storage.DB, sender ReminderSender, loc *time.Location, log *slog.Logger) *Scheduler {
	if loc == nil {
		loc = time.UTC
	}
	if log == nil {
		log = slog.Default()
	}
	return &Scheduler{
		db:       db,
		sender:   sender,
		loc:      loc,
		interval: DefaultInterval,
		now:      time.Now,
		log:      log,
	}
}

// SetInterval overrides the polling interval (tests).
func (s *Scheduler) SetInterval(d time.Duration) {
	if d > 0 {
		s.interval = d
	}
}

// SetClock overrides the clock (tests).
func (s *Scheduler) SetClock(now func() time.Time) {
	if now != nil {
		s.now = now
	}
}

// Run ticks until ctx is cancelled. One Tick runs immediately so that a
// restart after the configured time still delivers today's reminder; the
// per-user last_reminder_date guard keeps that from ever sending twice, and
// there is no catch-up for days the process was down (PRD §4.8).
func (s *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	s.Tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.Tick(ctx)
		}
	}
}

// Tick sends one reminder to every user whose reminder time has arrived and who
// has not been reminded today. Errors are logged per user and never stop the
// loop, so one unreachable chat cannot silence the rest.
func (s *Scheduler) Tick(ctx context.Context) {
	now := s.now().In(s.loc)
	today := now.Format(dateLayout)
	nowHHMM := now.Format(timeLayout)

	targets, err := s.db.ListReminderTargets(ctx, today)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		s.log.Error("gagal membaca daftar reminder", "err", err)
		return
	}
	for _, t := range targets {
		if ctx.Err() != nil {
			return
		}
		// Lexicographic comparison is chronological for zero-padded 'HH:MM'
		// (PRD §5.3 stores the column in that shape).
		if t.ReminderTime > nowHHMM {
			continue
		}
		if err := s.sender.SendReminder(ctx, t.UserID); err != nil {
			if ctx.Err() != nil {
				return
			}
			s.log.Error("gagal mengirim reminder", "user_id", t.UserID, "err", err)
			continue
		}
		s.log.Info("reminder terkirim", "user_id", t.UserID, "time", t.ReminderTime)
	}
}

package scheduler

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/rchmdndy/telegram-money-bot/internal/storage"
)

// testClock is 10:00 WIB on 27 Sep 2026; a reminder set to 09:00 is due, one at
// 23:00 is not.
var testClock = time.Date(2026, 9, 27, 3, 0, 0, 0, time.UTC)

// wib is the default TZ of PRD §5.5, as a fixed zone so the test never depends
// on the host's tzdata.
var wib = time.FixedZone("WIB", 7*60*60)

const testToday = "2026-09-27"

// fakeSender records the users it was asked to remind, and can fail for
// selected ones. Like bot.Handler it marks the user as reminded only after a
// successful send, which is what stops a second reminder in the same day.
type fakeSender struct {
	db     *storage.DB
	today  string
	got    []int64
	failOn map[int64]bool
}

func (f *fakeSender) SendReminder(ctx context.Context, userID int64) error {
	f.got = append(f.got, userID)
	if f.failOn[userID] {
		return errors.New("chat tidak terjangkau")
	}
	return f.db.SetLastReminderDate(ctx, userID, f.today)
}

func newTestScheduler(t *testing.T) (*Scheduler, *fakeSender, *storage.DB, context.Context) {
	t.Helper()
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	sender := &fakeSender{db: db, today: testToday, failOn: map[int64]bool{}}
	s := New(db, sender, wib, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.SetClock(func() time.Time { return testClock })
	return s, sender, db, ctx
}

func mustUser(t *testing.T, db *storage.DB, ctx context.Context, userID int64) {
	t.Helper()
	if _, err := db.EnsureUser(ctx, userID); err != nil {
		t.Fatalf("EnsureUser(%d): %v", userID, err)
	}
}

func mustTime(t *testing.T, db *storage.DB, ctx context.Context, userID int64, hhmm string) {
	t.Helper()
	if err := db.SetReminderTime(ctx, userID, hhmm); err != nil {
		t.Fatalf("SetReminderTime(%d): %v", userID, err)
	}
}

func TestTickSendsOnlyToUsersWhoseTimeHasArrived(t *testing.T) {
	s, sender, db, ctx := newTestScheduler(t)
	mustUser(t, db, ctx, 1)
	mustUser(t, db, ctx, 2)
	mustTime(t, db, ctx, 1, "09:00") // due: 09:00 <= 10:00
	mustTime(t, db, ctx, 2, "23:00") // not due yet

	s.Tick(ctx)

	if len(sender.got) != 1 || sender.got[0] != 1 {
		t.Fatalf("pengiriman = %v, want hanya user 1", sender.got)
	}
}

func TestTickSendsAtTheExactMinute(t *testing.T) {
	s, sender, db, ctx := newTestScheduler(t)
	mustUser(t, db, ctx, 1)
	mustTime(t, db, ctx, 1, "10:00") // exactly now

	s.Tick(ctx)

	if len(sender.got) != 1 {
		t.Fatalf("pengiriman = %v, want user 1 tepat pada menitnya", sender.got)
	}
}

func TestTickDoesNotSendTwiceInADay(t *testing.T) {
	s, sender, db, ctx := newTestScheduler(t)
	mustUser(t, db, ctx, 1)
	mustTime(t, db, ctx, 1, "09:00")

	s.Tick(ctx)
	s.Tick(ctx)

	if len(sender.got) != 1 {
		t.Fatalf("pengiriman = %v, want sekali saja", sender.got)
	}
	settings, err := db.GetSettings(ctx, 1)
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if settings.LastReminderDate != testToday {
		t.Fatalf("last_reminder_date = %q, want %q", settings.LastReminderDate, testToday)
	}
}

func TestTickSkipsDisabledReminders(t *testing.T) {
	s, sender, db, ctx := newTestScheduler(t)
	mustUser(t, db, ctx, 1)
	mustTime(t, db, ctx, 1, "09:00")
	if err := db.SetReminderEnabled(ctx, 1, false); err != nil {
		t.Fatalf("SetReminderEnabled: %v", err)
	}

	s.Tick(ctx)

	if len(sender.got) != 0 {
		t.Fatalf("reminder mati tetap dikirim: %v", sender.got)
	}
}

func TestTickKeepsGoingWhenOneSenderFails(t *testing.T) {
	s, sender, db, ctx := newTestScheduler(t)
	mustUser(t, db, ctx, 1)
	mustUser(t, db, ctx, 2)
	mustTime(t, db, ctx, 1, "09:00")
	mustTime(t, db, ctx, 2, "09:00")
	sender.failOn[1] = true

	s.Tick(ctx)

	if len(sender.got) != 2 {
		t.Fatalf("pengiriman = %v, want dua percobaan", sender.got)
	}
	// The failing user stays a target, so the next tick retries them.
	targets, err := db.ListReminderTargets(ctx, testToday)
	if err != nil {
		t.Fatalf("ListReminderTargets: %v", err)
	}
	if len(targets) != 1 || targets[0].UserID != 1 {
		t.Fatalf("target tersisa = %+v, want hanya user 1", targets)
	}
}

func TestRunStopsWithTheContext(t *testing.T) {
	s, _, db, ctx := newTestScheduler(t)
	mustUser(t, db, ctx, 1)
	mustTime(t, db, ctx, 1, "09:00")
	s.SetInterval(10 * time.Millisecond)

	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		s.Run(runCtx)
		close(done)
	}()

	// Run ticks immediately, so the reminder must arrive without waiting for a
	// ticker period.
	deadline := time.After(2 * time.Second)
	for {
		settings, err := db.GetSettings(ctx, 1)
		if err != nil {
			t.Fatalf("GetSettings: %v", err)
		}
		if settings.LastReminderDate == testToday {
			break
		}
		select {
		case <-deadline:
			t.Fatal("Run tidak mengirim reminder saat mulai")
		case <-time.After(5 * time.Millisecond):
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run tidak berhenti setelah context dibatalkan")
	}
}

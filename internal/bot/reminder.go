package bot

import (
	"context"
	"errors"

	"github.com/rchmdndy/telegram-money-bot/internal/period"
	"github.com/rchmdndy/telegram-money-bot/internal/persona"
	"github.com/rchmdndy/telegram-money-bot/internal/report"
	"github.com/rchmdndy/telegram-money-bot/internal/storage"
)

// SendReminder builds and sends userID's daily reminder (PRD §4.8). The
// reminder is always sent while it is enabled, so a day without transactions
// still produces the reminder.empty message. last_reminder_date is written
// only after the message went out, so a failed send is retried on the next
// tick instead of being lost for the day.
//
// The chat ID equals the user ID: the bot only ever answers a user in the chat
// that user writes from, and reminders are personal.
func (h *Handler) SendReminder(ctx context.Context, userID int64) error {
	settings, err := h.db.GetSettings(ctx, userID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil
		}
		return err
	}
	if !settings.ReminderEnabled {
		return nil
	}
	p := persona.ID(settings.Persona)
	if !persona.Valid(p) {
		p = persona.Netral
	}

	today := h.today()
	rows, err := h.db.ListTransactionsByDate(ctx, userID, today)
	if err != nil {
		return err
	}
	day := report.Aggregate(reportRows(rows), period.Range{Start: today, End: today}, "")

	pr, r := h.reminderPeriod(ctx, userID, today)
	income, err := h.db.SumByKindInRange(ctx, userID, storage.KindIncome, r.Start, r.End)
	if err != nil {
		return err
	}
	expense, err := h.db.SumByKindInRange(ctx, userID, storage.KindExpense, r.Start, r.End)
	if err != nil {
		return err
	}
	dayNo, _ := period.DayNumber(r, today)
	dayTotal, _ := period.Days(r)

	text := report.RenderReminder(p, report.Reminder{
		Date:            today,
		DayExpense:      day.ExpenseTotal,
		DayExpenseCount: day.ExpenseCount,
		DayIncome:       day.IncomeTotal,
		DayIncomeCount:  day.IncomeCount,
		DayCategories:   day.Expense,
		PeriodName:      pr.Name,
		PeriodRange:     r,
		DayNo:           dayNo,
		DayTotal:        dayTotal,
		PeriodIncome:    income,
		PeriodExpense:   expense,
	})
	if err := h.send(ctx, userID, text, mainReplyKeyboard()); err != nil {
		return err
	}
	return h.db.SetLastReminderDate(ctx, userID, today)
}

// reminderPeriod resolves the period covering date. A user always has at least
// one period row (EnsureUser seeds it), but a row that cannot be read must not
// cost the user their reminder, so the default 21→20 window is the fallback.
func (h *Handler) reminderPeriod(ctx context.Context, userID int64, date string) (period.Period, period.Range) {
	if pr, r, err := h.activePeriod(ctx, userID); err == nil {
		return pr, r
	}
	r, err := period.Resolve(date, 21)
	if err != nil {
		return period.Period{Name: persona.LabelDefaultPeriodName}, period.Range{Start: date, End: date}
	}
	return period.Period{Name: persona.LabelDefaultPeriodName}, r
}

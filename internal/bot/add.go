package bot

import (
	"context"
	"errors"
	"strconv"
	"unicode/utf8"

	"github.com/rchmdndy/telegram-money-bot/internal/money"
	"github.com/rchmdndy/telegram-money-bot/internal/period"
	"github.com/rchmdndy/telegram-money-bot/internal/persona"
	"github.com/rchmdndy/telegram-money-bot/internal/report"
	"github.com/rchmdndy/telegram-money-bot/internal/storage"
)

// startAdd opens the add flow at its first step, the date (PRD §4.2).
// Pressing ➕ is an explicit restart: any earlier conversation is replaced.
func (h *Handler) startAdd(ctx context.Context, chatID, userID int64, p persona.ID, kind storage.Kind) error {
	pl := payload{Cur: &txData{Kind: string(kind)}}
	if err := h.saveConversation(ctx, userID, StateTxDate, pl); err != nil {
		return err
	}
	return h.ask(ctx, chatID, p, "tx.prompt.date", map[string]string{"valid": persona.LabelDateFormats}, dateKeyboard())
}

// cbDate handles the date keyboard of the add flow (PRD §4.2 step 2).
func (h *Handler) cbDate(ctx context.Context, chatID, userID int64, p persona.ID, messageID int, sess session, data string) error {
	if !sess.ok || sess.pl.Cur == nil {
		return h.validate(ctx, chatID, p, "val.no_conv", nil, mainReplyKeyboard())
	}
	pl := sess.pl
	if err := h.clearKeyboard(ctx, chatID, messageID); err != nil {
		return err
	}
	switch data {
	case cbDateToday:
		pl.Cur.OccurredOn = h.today()
	case cbDateYesterday:
		yesterday, err := period.ShiftDays(h.today(), -1)
		if err != nil {
			return err
		}
		pl.Cur.OccurredOn = yesterday
	case cbDatePick:
		// Keep the state and ask for a typed date instead.
		return h.ask(ctx, chatID, p, "tx.prompt.date", map[string]string{"valid": persona.LabelDateFormats}, Keyboard{})
	}
	return h.sendCategoryPicker(ctx, chatID, userID, p, pl, StateTxAmount)
}

// sendCategoryPicker stores the payload and shows the category keyboard of the
// kind being recorded, two categories per row plus `[⬅️ Kembali]` (PRD §4.2).
func (h *Handler) sendCategoryPicker(ctx context.Context, chatID, userID int64, p persona.ID, pl payload, state string) error {
	cats, err := h.db.ListCategories(ctx, userID, storage.Kind(pl.Cur.Kind), true)
	if err != nil {
		return err
	}
	if len(cats) == 0 {
		if err := h.dropConversation(ctx, userID); err != nil {
			return err
		}
		return h.validate(ctx, chatID, p, "val.cat_empty", nil, mainReplyKeyboard())
	}
	if err := h.saveConversation(ctx, userID, state, pl); err != nil {
		return err
	}
	return h.say(ctx, chatID, p, "category.list", nil, categoryKeyboard(cats, cbCatPickPrefix, true))
}

// cbCategoryPick handles a category button. The add flow uses it as step 3 of
// PRD §4.2; the edit flow reuses the same picker with payload.Field set to
// fieldCategory (PRD §4.9).
func (h *Handler) cbCategoryPick(ctx context.Context, chatID, userID int64, p persona.ID, messageID int, sess session, rawID string) error {
	if !sess.ok || sess.pl.Cur == nil {
		return h.validate(ctx, chatID, p, "val.no_conv", nil, mainReplyKeyboard())
	}
	pl := sess.pl
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil {
		return h.validate(ctx, chatID, p, "val.tx_missing", nil, Keyboard{})
	}
	cat, err := h.db.GetCategory(ctx, userID, id)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return h.validate(ctx, chatID, p, "val.tx_missing", nil, Keyboard{})
		}
		return err
	}
	if cat.Kind != storage.Kind(pl.Cur.Kind) {
		return h.validate(ctx, chatID, p, "val.tx_missing", nil, Keyboard{})
	}
	if err := h.clearKeyboard(ctx, chatID, messageID); err != nil {
		return err
	}
	pl.Cur.CategoryID = cat.ID
	pl.Cur.CategoryName = cat.Name

	// Any picker the edit flow opened — a category change or a type change —
	// returns to the old → new card. The add flow leaves Field empty and goes
	// on to the amount prompt.
	if pl.Field != "" {
		return h.showEditCard(ctx, chatID, userID, p, pl)
	}
	if err := h.saveConversation(ctx, userID, StateTxAmount, pl); err != nil {
		return err
	}
	return h.ask(ctx, chatID, p, "tx.prompt.amount", nil, Keyboard{})
}

// inputDate consumes a typed date while in tx.date (PRD §4.2 step 2).
func (h *Handler) inputDate(ctx context.Context, chatID, userID int64, p persona.ID, pl payload, text string) error {
	if pl.Cur == nil {
		return h.validate(ctx, chatID, p, "val.no_conv", nil, mainReplyKeyboard())
	}
	date, err := period.ParseDate(text)
	if err != nil {
		return h.validate(ctx, chatID, p, "val.date", map[string]string{"valid": persona.LabelDateFormats}, Keyboard{})
	}
	pl.Cur.OccurredOn = date
	return h.sendCategoryPicker(ctx, chatID, userID, p, pl, StateTxAmount)
}

// inputAmount consumes an amount while in tx.amount (PRD §4.2 step 4).
func (h *Handler) inputAmount(ctx context.Context, chatID, userID int64, p persona.ID, pl payload, text string) error {
	amount, err := money.Parse(text)
	if err != nil {
		return h.validate(ctx, chatID, p, "val.amount", map[string]string{"limit": money.Format(money.MaxAmount)}, Keyboard{})
	}
	pl.Cur.Amount = amount
	if err := h.saveConversation(ctx, userID, StateTxConfirm, pl); err != nil {
		return err
	}
	return h.sendConfirmCard(ctx, chatID, p, pl)
}

// sendConfirmCard shows the confirmation card of the add and quick flows
// (PRD §4.2 step 5, §4.13.3).
func (h *Handler) sendConfirmCard(ctx context.Context, chatID int64, p persona.ID, pl payload) error {
	cur := pl.Cur
	if cur == nil {
		return h.validate(ctx, chatID, p, "val.no_conv", nil, mainReplyKeyboard())
	}
	category := cur.CategoryName
	if pl.Fuzzy != "" {
		category += " " + persona.FuzzySource(pl.Fuzzy)
	}
	card := report.Card{
		Header:   persona.Render(p, "tx.confirm.header", map[string]string{"kind": kindWord(cur.Kind)}),
		Date:     period.FormatDayDate(cur.OccurredOn),
		Category: category,
		Amount:   money.Format(cur.Amount),
		Note:     noteOrDash(cur.Note),
	}
	return h.send(ctx, chatID, report.RenderCard(card), confirmKeyboard())
}

// cbNote switches the add flow to the note prompt (PRD §4.2 step 5).
func (h *Handler) cbNote(ctx context.Context, chatID, userID int64, p persona.ID, messageID int, sess session) error {
	if !sess.ok || sess.pl.Cur == nil {
		return h.validate(ctx, chatID, p, "val.no_conv", nil, mainReplyKeyboard())
	}
	if err := h.clearKeyboard(ctx, chatID, messageID); err != nil {
		return err
	}
	state := StateTxNote
	if sess.pl.Quick {
		state = StateQuickNote
	}
	if err := h.saveConversation(ctx, userID, state, sess.pl); err != nil {
		return err
	}
	return h.ask(ctx, chatID, p, "tx.prompt.note", nil, Keyboard{})
}

// inputNote stores a typed note and returns to the confirmation card.
func (h *Handler) inputNote(ctx context.Context, chatID, userID int64, p persona.ID, pl payload, text string) error {
	if pl.Cur == nil {
		return h.validate(ctx, chatID, p, "val.no_conv", nil, mainReplyKeyboard())
	}
	if utf8.RuneCountInString(text) > quickMaxNoteLen {
		return h.validate(ctx, chatID, p, "val.note", nil, Keyboard{})
	}
	pl.Cur.Note = text
	state := StateTxConfirm
	if pl.Quick {
		state = StateQuickConfirm
	}
	if err := h.saveConversation(ctx, userID, state, pl); err != nil {
		return err
	}
	return h.sendConfirmCard(ctx, chatID, p, pl)
}

// cbSave inserts the confirmed transaction (PRD §4.2 step 6, §4.13.3).
func (h *Handler) cbSave(ctx context.Context, chatID, userID int64, p persona.ID, messageID int, sess session) error {
	if !sess.ok || sess.pl.Cur == nil {
		return h.validate(ctx, chatID, p, "val.no_conv", nil, mainReplyKeyboard())
	}
	if err := h.clearKeyboard(ctx, chatID, messageID); err != nil {
		return err
	}
	cur := sess.pl.Cur
	if cur.OccurredOn == "" {
		cur.OccurredOn = h.today()
	}
	tx := storage.Transaction{
		UserID:     userID,
		OccurredOn: cur.OccurredOn,
		Kind:       storage.Kind(cur.Kind),
		CategoryID: cur.CategoryID,
		Amount:     cur.Amount,
		Note:       cur.Note,
	}
	if _, err := h.db.CreateTransaction(ctx, tx); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return h.validate(ctx, chatID, p, "val.tx_missing", nil, mainReplyKeyboard())
		}
		return err
	}
	if err := h.dropConversation(ctx, userID); err != nil {
		return err
	}
	total, err := h.db.SumByCategoryOnDate(ctx, userID, cur.CategoryID, cur.OccurredOn)
	if err != nil {
		return err
	}
	vars := map[string]string{
		"category": cur.CategoryName,
		"amount":   money.Format(total),
	}
	return h.say(ctx, chatID, p, "tx.saved", vars, mainReplyKeyboard())
}

// cbCancel drops the conversation from any state (PRD §4.2, §4.9).
func (h *Handler) cbCancel(ctx context.Context, chatID, userID int64, p persona.ID, messageID int, active bool) error {
	if !active {
		return h.validate(ctx, chatID, p, "val.no_conv", nil, mainReplyKeyboard())
	}
	if err := h.clearKeyboard(ctx, chatID, messageID); err != nil {
		return err
	}
	if err := h.dropConversation(ctx, userID); err != nil {
		return err
	}
	return h.say(ctx, chatID, p, "tx.cancelled", nil, mainReplyKeyboard())
}

// quickMaxNoteLen mirrors quick.MaxNoteLen so the add flow enforces the same
// limit without importing the parser.
const quickMaxNoteLen = 200

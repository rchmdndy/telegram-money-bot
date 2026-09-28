package bot

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/dandy/telegram_money_bot/internal/money"
	"github.com/dandy/telegram_money_bot/internal/period"
	"github.com/dandy/telegram_money_bot/internal/persona"
	"github.com/dandy/telegram_money_bot/internal/report"
	"github.com/dandy/telegram_money_bot/internal/storage"
)

// cbTxEdit opens the edit flow for one transaction (PRD §4.9). The callback
// carries the transaction ID, so the field menu needs no stored state.
func (h *Handler) cbTxEdit(ctx context.Context, chatID, userID int64, p persona.ID, rawID string) error {
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil {
		return h.validate(ctx, chatID, p, "val.tx_missing", nil, Keyboard{})
	}
	row, err := h.db.GetTransaction(ctx, userID, id)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return h.validate(ctx, chatID, p, "val.tx_missing", nil, Keyboard{})
		}
		return err
	}
	// A fresh edit replaces any running conversation, like ➕ does.
	pl := payload{Cur: txDataFromRow(row)}
	if err := h.saveConversation(ctx, userID, StateTxEditDate, pl); err != nil {
		return err
	}
	return h.send(ctx, chatID, renderCard(p, "", pl.Cur), editMenuKeyboard(row.ID))
}

// renderCard renders one transaction as the four-line card. header is optional
// and is rendered only when non-empty (report.RenderCard).
func renderCard(p persona.ID, header string, cur *txData) string {
	return report.RenderCard(report.Card{
		Header:   header,
		Date:     period.FormatDayDate(cur.OccurredOn),
		Category: cur.CategoryName,
		Amount:   money.Format(cur.Amount),
		Note:     noteOrDash(cur.Note),
	})
}

// txDataFromRow copies a stored transaction into the editable working copy.
func txDataFromRow(row storage.TransactionRow) *txData {
	return &txData{
		ID:           row.ID,
		Kind:         string(row.Kind),
		OccurredOn:   row.OccurredOn,
		CategoryID:   row.CategoryID,
		CategoryName: row.CategoryName,
		Amount:       row.Amount,
		Note:         row.Note,
	}
}

// cbEditField handles one field button of the edit menu (PRD §4.9 step 2).
func (h *Handler) cbEditField(ctx context.Context, chatID, userID int64, p persona.ID, messageID int, sess session, rest string) error {
	idPart, field, ok := strings.Cut(rest, ":")
	if !ok {
		return h.validate(ctx, chatID, p, "val.tx_missing", nil, Keyboard{})
	}
	id, err := strconv.ParseInt(idPart, 10, 64)
	if err != nil {
		return h.validate(ctx, chatID, p, "val.tx_missing", nil, Keyboard{})
	}
	row, err := h.db.GetTransaction(ctx, userID, id)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return h.validate(ctx, chatID, p, "val.tx_missing", nil, Keyboard{})
		}
		return err
	}
	if err := h.clearKeyboard(ctx, chatID, messageID); err != nil {
		return err
	}
	// The row the button names is the source of truth, so a stale menu on an
	// older message can never edit the wrong transaction.
	pl := payload{Cur: txDataFromRow(row), Field: field}

	switch field {
	case fieldDone:
		if err := h.dropConversation(ctx, userID); err != nil {
			return err
		}
		return h.say(ctx, chatID, p, "tx.cancelled", nil, mainReplyKeyboard())
	case fieldDate:
		if err := h.saveConversation(ctx, userID, StateTxEditDate, pl); err != nil {
			return err
		}
		return h.ask(ctx, chatID, p, "tx.prompt.date", map[string]string{"valid": persona.LabelDateFormats}, Keyboard{})
	case fieldAmount:
		if err := h.saveConversation(ctx, userID, StateTxEditAmount, pl); err != nil {
			return err
		}
		return h.ask(ctx, chatID, p, "tx.prompt.amount", nil, Keyboard{})
	case fieldNote:
		if err := h.saveConversation(ctx, userID, StateTxEditNote, pl); err != nil {
			return err
		}
		return h.ask(ctx, chatID, p, "tx.prompt.note", nil, Keyboard{})
	case fieldCategory:
		pl.Cur.CategoryID = 0
		pl.Cur.CategoryName = ""
		return h.sendCategoryPicker(ctx, chatID, userID, p, pl, StateTxEditDate)
	case fieldKind:
		if err := h.saveConversation(ctx, userID, StateTxEditKind, pl); err != nil {
			return err
		}
		return h.say(ctx, chatID, p, "category.list", nil, kindKeyboard())
	}
	return h.validate(ctx, chatID, p, "val.tx_missing", nil, Keyboard{})
}

// cbCatKind handles the kind choice of the edit flow (PRD §4.9: changing the
// type REQUIRES a new category, so the picker opens immediately) and of the
// new-category flow (PRD §4.4).
func (h *Handler) cbCatKind(ctx context.Context, chatID, userID int64, p persona.ID, messageID int, sess session, rawKind string) error {
	if err := h.clearKeyboard(ctx, chatID, messageID); err != nil {
		return err
	}
	if !sess.ok {
		return h.validate(ctx, chatID, p, "val.no_conv", nil, mainReplyKeyboard())
	}
	kind := storage.Kind(rawKind)
	if !kind.Valid() {
		return h.validate(ctx, chatID, p, "val.tx_missing", nil, Keyboard{})
	}
	if sess.pl.CatMode == catModeAdd {
		pl := sess.pl
		pl.CatKind = string(kind)
		if err := h.saveConversation(ctx, userID, StateCatName, pl); err != nil {
			return err
		}
		return h.ask(ctx, chatID, p, "prompt.cat_name", nil, Keyboard{})
	}
	if sess.pl.Cur == nil {
		return h.validate(ctx, chatID, p, "val.no_conv", nil, mainReplyKeyboard())
	}
	pl := sess.pl
	pl.Cur.Kind = string(kind)
	pl.Cur.CategoryID = 0
	pl.Cur.CategoryName = ""
	return h.sendCategoryPicker(ctx, chatID, userID, p, pl, StateTxEditDate)
}

// editInput handles typed input while an edit field is being changed.
func (h *Handler) editInput(ctx context.Context, chatID, userID int64, p persona.ID, pl payload, text string) error {
	if pl.Cur == nil {
		return h.validate(ctx, chatID, p, "val.no_conv", nil, mainReplyKeyboard())
	}
	switch pl.Field {
	case fieldDate:
		date, err := period.ParseDate(text)
		if err != nil {
			return h.validate(ctx, chatID, p, "val.date", map[string]string{"valid": persona.LabelDateFormats}, Keyboard{})
		}
		pl.Cur.OccurredOn = date
	case fieldAmount:
		amount, err := money.Parse(text)
		if err != nil {
			return h.validate(ctx, chatID, p, "val.amount", map[string]string{"limit": money.Format(money.MaxAmount)}, Keyboard{})
		}
		pl.Cur.Amount = amount
	case fieldNote:
		if utf8.RuneCountInString(text) > quickMaxNoteLen {
			return h.validate(ctx, chatID, p, "val.note", nil, Keyboard{})
		}
		pl.Cur.Note = text
	default:
		return h.validate(ctx, chatID, p, "val.tx_missing", nil, Keyboard{})
	}
	return h.showEditCard(ctx, chatID, userID, p, pl)
}

// showEditCard renders the old → new confirmation of the edit flow
// (PRD §4.9 step 3). The old values come from the stored row, so every field
// the user did not touch keeps its value on the card.
func (h *Handler) showEditCard(ctx context.Context, chatID, userID int64, p persona.ID, pl payload) error {
	row, err := h.db.GetTransaction(ctx, userID, pl.Cur.ID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return h.validate(ctx, chatID, p, "val.tx_missing", nil, mainReplyKeyboard())
		}
		return err
	}
	pl.Orig = txDataFromRow(row)
	if err := h.saveConversation(ctx, userID, StateTxEditDate, pl); err != nil {
		return err
	}

	date := period.FormatDayDate(pl.Cur.OccurredOn)
	category := pl.Cur.CategoryName
	amount := money.Format(pl.Cur.Amount)
	note := noteOrDash(pl.Cur.Note)
	switch pl.Field {
	case fieldDate:
		date = period.FormatDayDate(pl.Orig.OccurredOn) + persona.LabelArrow + date
	case fieldCategory:
		category = pl.Orig.CategoryName + persona.LabelArrow + category
	case fieldAmount:
		amount = money.Format(pl.Orig.Amount) + persona.LabelArrow + amount
	case fieldNote:
		note = noteOrDash(pl.Orig.Note) + persona.LabelArrow + note
	}
	header := persona.Render(p, "tx.edit.confirm.header", map[string]string{"kind": editWord(pl.Field)})
	return h.send(ctx, chatID, report.RenderCard(report.Card{
		Header:   header,
		Date:     date,
		Category: category,
		Amount:   amount,
		Note:     note,
	}), editKeyboard())
}

// cbSaveEdit persists the edited transaction (PRD §4.9 step 4).
func (h *Handler) cbSaveEdit(ctx context.Context, chatID, userID int64, p persona.ID, sess session) error {
	cur := sess.pl.Cur
	tx := storage.Transaction{
		ID:         cur.ID,
		UserID:     userID,
		OccurredOn: cur.OccurredOn,
		Kind:       storage.Kind(cur.Kind),
		CategoryID: cur.CategoryID,
		Amount:     cur.Amount,
		Note:       cur.Note,
	}
	if err := h.db.UpdateTransaction(ctx, tx); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return h.validate(ctx, chatID, p, "val.tx_missing", nil, mainReplyKeyboard())
		}
		return err
	}
	if err := h.dropConversation(ctx, userID); err != nil {
		return err
	}
	return h.say(ctx, chatID, p, "tx.updated", map[string]string{
		"category": cur.CategoryName,
		"amount":   money.Format(cur.Amount),
	}, mainReplyKeyboard())
}

// cbTxDelete deletes one transaction (PRD §4.9). No second confirmation: the
// button is already specific to a single transaction.
func (h *Handler) cbTxDelete(ctx context.Context, chatID, userID int64, p persona.ID, rawID string) error {
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil {
		return h.validate(ctx, chatID, p, "val.tx_missing", nil, Keyboard{})
	}
	row, err := h.db.GetTransaction(ctx, userID, id)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return h.validate(ctx, chatID, p, "val.tx_missing", nil, Keyboard{})
		}
		return err
	}
	if err := h.db.DeleteTransaction(ctx, userID, id); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return h.validate(ctx, chatID, p, "val.tx_missing", nil, Keyboard{})
		}
		return err
	}
	return h.say(ctx, chatID, p, "tx.deleted", map[string]string{
		"category": row.CategoryName,
		"amount":   money.Format(row.Amount),
	}, Keyboard{})
}

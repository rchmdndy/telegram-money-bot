package bot

import (
	"context"
	"errors"
	"strings"

	"github.com/rchmdndy/telegram-money-bot/internal/money"
	"github.com/rchmdndy/telegram-money-bot/internal/persona"
	"github.com/rchmdndy/telegram-money-bot/internal/quick"
	"github.com/rchmdndy/telegram-money-bot/internal/storage"
)

// quickInput parses a free-text quick input, matches the category and shows the
// confirmation card. It never saves on its own, not even for an exact match
// (PRD §4.13: "fuzzy tidak pernah langsung menyimpan").
func (h *Handler) quickInput(ctx context.Context, chatID, userID int64, p persona.ID, text string) error {
	res, err := quick.Parse(text)
	if err != nil {
		// The parser failed before reporting a kind, so recover it from the
		// type token of the raw line.
		return h.quickError(ctx, chatID, userID, p, err, text, quickKindFromText(text))
	}
	kind := storage.Kind(res.Kind)
	cats, err := h.db.ListCategories(ctx, userID, kind, true)
	if err != nil {
		return err
	}
	match, err := quick.MatchCategory(res.Category, quickCategories(cats))
	if err != nil {
		return h.quickError(ctx, chatID, userID, p, err, res.Category, kind)
	}
	pl := payload{
		Cur: &txData{
			Kind:         res.Kind,
			OccurredOn:   h.today(),
			CategoryID:   match.ID,
			CategoryName: match.Name,
			Amount:       res.Amount,
			Note:         res.Note,
		},
		Quick: true,
	}
	if match.Fuzzy {
		pl.Fuzzy = res.Category
	}
	if err := h.saveConversation(ctx, userID, StateQuickConfirm, pl); err != nil {
		return err
	}
	return h.sendConfirmCard(ctx, chatID, p, pl)
}

// quickError maps a parser error to its persona message (PRD §4.13.4). Every
// one of them names the rule, and a message that starts like a quick input
// always gets an error instead of the idle hint. kind is the transaction kind
// the rejected line asked for, so the {list} of a category error names the
// categories the user could actually pick.
func (h *Handler) quickError(ctx context.Context, chatID, userID int64, p persona.ID, err error, input string, kind storage.Kind) error {
	switch {
	case errors.Is(err, quick.ErrKind):
		return h.say(ctx, chatID, p, "quick.kind", nil, mainReplyKeyboard())
	case errors.Is(err, quick.ErrAmount):
		return h.validate(ctx, chatID, p, "val.amount", map[string]string{"limit": money.Format(money.MaxAmount)}, mainReplyKeyboard())
	case errors.Is(err, quick.ErrNoteLong):
		return h.say(ctx, chatID, p, "quick.note_long", nil, mainReplyKeyboard())
	case errors.Is(err, quick.ErrCategory), errors.Is(err, quick.ErrAmbiguous):
		list, listErr := h.db.CategoryNames(ctx, userID, kind)
		if listErr != nil {
			return listErr
		}
		return h.say(ctx, chatID, p, "quick.category", map[string]string{
			"input": input,
			"list":  list,
		}, mainReplyKeyboard())
	}
	return h.say(ctx, chatID, p, "quick.format", nil, mainReplyKeyboard())
}

// quickKindFromText recovers the kind of a quick input whose parse failed: the
// type token is the first field, exactly as the parser reads it.
func quickKindFromText(text string) storage.Kind {
	first, _, _ := strings.Cut(text, ",")
	if strings.EqualFold(strings.TrimSpace(first), "i") {
		return storage.KindIncome
	}
	return storage.KindExpense
}

// quickCategories projects stored categories onto the parser's input type.
func quickCategories(cats []storage.Category) []quick.Category {
	out := make([]quick.Category, 0, len(cats))
	for _, c := range cats {
		out = append(out, quick.Category{ID: c.ID, Name: c.Name})
	}
	return out
}

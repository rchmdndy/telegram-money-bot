package bot

import (
	"errors"
	"testing"

	"github.com/rchmdndy/telegram-money-bot/internal/persona"
	"github.com/go-telegram/bot/models"
)

// textUpdate builds the minimal update shape the Telegram library hands to a
// text handler.
func textUpdate(userID int64, text string) *models.Update {
	return &models.Update{
		ID: 7,
		Message: &models.Message{
			ID:   11,
			From: &models.User{ID: userID},
			Chat: models.Chat{ID: userID},
			Text: text,
		},
	}
}

func TestOnMessageIgnoresUnusableUpdates(t *testing.T) {
	h, sender, _, ctx := newTestHandler(t)
	botUser := &models.User{ID: 1, IsBot: true}
	cases := map[string]*models.Update{
		"message kosong": {ID: 1},
		"tanpa pengirim": {ID: 2, Message: &models.Message{ID: 1, Chat: models.Chat{ID: 1}, Text: "/help"}},
		"pengirim bot":   {ID: 3, Message: &models.Message{ID: 1, From: botUser, Chat: models.Chat{ID: 1}, Text: "/help"}},
		"teks kosong":    textUpdate(1, "   "),
		"bukan teks":     {ID: 5, Message: &models.Message{ID: 1, From: &models.User{ID: 1}, Chat: models.Chat{ID: 1}}},
	}
	for name, update := range cases {
		t.Run(name, func(t *testing.T) {
			sender.reset()
			h.onMessage(ctx, nil, update)
			if len(sender.messages) != 0 {
				t.Fatalf("pesan = %q, want tidak ada balasan", sender.messages[0].text)
			}
		})
	}
}

func TestOnMessageDispatchesToTheHandler(t *testing.T) {
	h, sender, _, ctx := newTestHandler(t)
	h.onMessage(ctx, nil, textUpdate(1, "/help"))
	if got, want := sender.last(t), say("help.list", nil); got != want {
		t.Fatalf("balasan = %q, want %q", got, want)
	}
	if sender.messages[0].chatID != 1 {
		t.Fatalf("chat id = %d, want 1", sender.messages[0].chatID)
	}
}

func TestOnCallbackUsesTheMessageChatAndID(t *testing.T) {
	h, sender, _, ctx := newTestHandler(t)
	// Cancel only clears the keyboard when a conversation is open, so seed one.
	mustSend(t, h, ctx, 1, persona.BtnExpense)
	sender.reset()
	update := &models.Update{
		ID: 8,
		CallbackQuery: &models.CallbackQuery{
			ID:      "cb-1",
			From:    models.User{ID: 1},
			Message: models.MaybeInaccessibleMessage{Message: &models.Message{ID: 99, Chat: models.Chat{ID: 1}}},
			Data:    cbCancel,
		},
	}
	h.onCallback(ctx, nil, update)
	if len(sender.answered) != 1 || sender.answered[0] != "cb-1" {
		t.Fatalf("callback dijawab = %v, want [cb-1]", sender.answered)
	}
	if len(sender.edited) != 1 || sender.edited[0] != 99 {
		t.Fatalf("keyboard dihapus pada pesan = %v, want [99]", sender.edited)
	}
	if got, want := sender.last(t), say("tx.cancelled", nil); got != want {
		t.Fatalf("balasan = %q, want %q", got, want)
	}
}
func TestOnCallbackFallsBackWhenTheMessageIsGone(t *testing.T) {
	h, sender, _, ctx := newTestHandler(t)
	update := &models.Update{
		ID: 9,
		CallbackQuery: &models.CallbackQuery{
			ID:   "cb-2",
			From: models.User{ID: 1},
			Data: cbCancel,
		},
	}
	h.onCallback(ctx, nil, update)
	if len(sender.answered) != 1 {
		t.Fatalf("callback dijawab = %v, want satu jawaban", sender.answered)
	}
	if len(sender.edited) != 0 {
		t.Fatalf("keyboard dihapus pada pesan = %v, want kosong", sender.edited)
	}
	if got, want := sender.last(t), valid("val.no_conv", nil); got != want {
		t.Fatalf("balasan = %q, want %q", got, want)
	}
}

func TestOnCallbackIgnoresBots(t *testing.T) {
	h, sender, _, ctx := newTestHandler(t)
	update := &models.Update{
		ID: 10,
		CallbackQuery: &models.CallbackQuery{
			ID:   "cb-3",
			From: models.User{ID: 2, IsBot: true},
			Data: cbCancel,
		},
	}
	h.onCallback(ctx, nil, update)
	if len(sender.answered) != 0 || len(sender.messages) != 0 {
		t.Fatalf("callback bot ditanggapi: %v %v", sender.answered, sender.messages)
	}
}

// PRD §5.6: a failing update is logged and answered, never fatal.
func TestGuardRepliesOnErrorAndPanic(t *testing.T) {
	cases := map[string]func() error{
		"error": func() error { return errors.New("gagal") },
		"panic": func() error { panic("gagal") },
	}
	for name, fn := range cases {
		t.Run(name, func(t *testing.T) {
			h, sender, _, ctx := newTestHandler(t)
			h.guard(ctx, 1, 1, 42, fn)
			if got, want := sender.last(t), say("error.generic", nil); got != want {
				t.Fatalf("balasan = %q, want %q", got, want)
			}
		})
	}
}

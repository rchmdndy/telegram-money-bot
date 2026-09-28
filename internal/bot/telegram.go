package bot

import (
	"context"
	"fmt"
	"runtime/debug"
	"strings"

	telegram "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Register wires a Handler into the long-polling bot (PRD §5.4). Two catch-all
// handlers cover every text message and every inline callback: the pattern is
// empty and MatchTypePrefix matches any string, so the library's own matching
// is what filters out updates of the other kind (a text handler never sees an
// update whose Message is nil, and vice versa).
func Register(b *telegram.Bot, h *Handler) {
	b.RegisterHandler(telegram.HandlerTypeMessageText, "", telegram.MatchTypePrefix, h.onMessage)
	b.RegisterHandler(telegram.HandlerTypeCallbackQueryData, "", telegram.MatchTypePrefix, h.onCallback)
}

// onMessage handles one text update. Messages the bot cannot act on (channels,
// other bots, non-text payloads such as stickers or photos) are dropped
// silently: there is no one to answer.
func (h *Handler) onMessage(ctx context.Context, _ *telegram.Bot, update *models.Update) {
	msg := update.Message
	if msg == nil || msg.From == nil || msg.From.IsBot {
		return
	}
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return
	}
	h.guard(ctx, msg.Chat.ID, msg.From.ID, update.ID, func() error {
		return h.HandleMessage(ctx, msg.Chat.ID, msg.From.ID, msg.Text)
	})
}

// onCallback handles one inline-button press. The chat ID comes from the
// message the button belongs to; when Telegram no longer exposes that message
// (too old, or the bot was restarted into a new message), the user's own ID is
// used instead — every reminder and card the bot sends goes to the private
// chat with that user, so the two are the same ID there. A zero message ID
// makes clearKeyboard a no-op, which is the correct outcome when the keyboard
// cannot be reached.
func (h *Handler) onCallback(ctx context.Context, _ *telegram.Bot, update *models.Update) {
	q := update.CallbackQuery
	if q == nil || q.From.IsBot {
		return
	}
	chatID := q.From.ID
	messageID := 0
	if q.Message.Message != nil {
		chatID = q.Message.Message.Chat.ID
		messageID = q.Message.Message.ID
	}
	h.guard(ctx, chatID, q.From.ID, update.ID, func() error {
		return h.HandleCallback(ctx, chatID, q.From.ID, messageID, q.ID, q.Data)
	})
}

// guard runs one update through fn and keeps a bad update from taking the
// process down (PRD §5.6: log with the update_id, reply error.generic, never
// panic). fn's error and a recovered panic are treated the same way: the user
// gets one error reply, the log gets the detail.
func (h *Handler) guard(ctx context.Context, chatID, userID, updateID int64, fn func() error) {
	defer func() {
		if r := recover(); r != nil {
			h.log.Error("panic saat memproses update",
				"update_id", updateID, "user_id", userID, "panic", r, "stack", string(debug.Stack()))
			h.replyError(ctx, chatID, userID, updateID, fmt.Errorf("panic: %v", r))
		}
	}()
	if err := fn(); err != nil {
		h.replyError(ctx, chatID, userID, updateID, err)
	}
}

// replyError logs a failed update and tells the user, in their own persona,
// that it failed.
func (h *Handler) replyError(ctx context.Context, chatID, userID, updateID int64, err error) {
	h.log.Error("gagal memproses update", "update_id", updateID, "user_id", userID, "err", err)
	p := h.personaID(ctx, userID)
	if serr := h.say(ctx, chatID, p, "error.generic", nil, mainReplyKeyboard()); serr != nil {
		h.log.Error("gagal mengirim pesan error", "update_id", updateID, "user_id", userID, "err", serr)
	}
}

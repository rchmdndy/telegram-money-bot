package bot

import (
	"bytes"
	"context"

	telegram "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// InlineButton is one inline-keyboard button: a caption plus the callback data
// the handler dispatches on.
type InlineButton struct {
	Text string
	Data string
}

// Keyboard is the markup attached to an outgoing message. A non-nil Inline
// sends an inline keyboard (an empty, non-nil Inline removes the buttons of an
// existing message); a non-nil Reply sends a reply keyboard. Only one may be
// set, and a zero Keyboard sends no markup at all.
type Keyboard struct {
	Inline [][]InlineButton
	Reply  [][]string
}

// Sender is the outbound Telegram surface the handlers use. It exists so the
// conversation state machine can be tested without a network call (PRD §6:
// "handler diuji lewat interface Sender yang bisa di-mock").
type Sender interface {
	// SendMessage sends text and returns the new message ID (needed to edit
	// the card later).
	SendMessage(ctx context.Context, chatID int64, text string, k Keyboard) (int, error)
	// EditMessageKeyboard replaces only the markup of an existing message.
	EditMessageKeyboard(ctx context.Context, chatID int64, messageID int, k Keyboard) error
	// SendDocument uploads content as filename, with an optional caption.
	SendDocument(ctx context.Context, chatID int64, filename string, content []byte, caption string) error
	// AnswerCallback stops the client-side spinner of an inline button.
	AnswerCallback(ctx context.Context, callbackID string) error
}

// TelegramSender is the Sender implementation backed by go-telegram/bot.
type TelegramSender struct {
	Bot *telegram.Bot
}

func (s *TelegramSender) SendMessage(ctx context.Context, chatID int64, text string, k Keyboard) (int, error) {
	msg, err := s.Bot.SendMessage(ctx, &telegram.SendMessageParams{
		ChatID:      chatID,
		Text:        text,
		ReplyMarkup: replyMarkup(k),
	})
	if err != nil {
		return 0, err
	}
	return msg.ID, nil
}

func (s *TelegramSender) EditMessageKeyboard(ctx context.Context, chatID int64, messageID int, k Keyboard) error {
	_, err := s.Bot.EditMessageReplyMarkup(ctx, &telegram.EditMessageReplyMarkupParams{
		ChatID:      chatID,
		MessageID:   messageID,
		ReplyMarkup: replyMarkup(k),
	})
	return err
}

func (s *TelegramSender) SendDocument(ctx context.Context, chatID int64, filename string, content []byte, caption string) error {
	_, err := s.Bot.SendDocument(ctx, &telegram.SendDocumentParams{
		ChatID:   chatID,
		Document: &models.InputFileUpload{Filename: filename, Data: bytes.NewReader(content)},
		Caption:  caption,
	})
	return err
}

func (s *TelegramSender) AnswerCallback(ctx context.Context, callbackID string) error {
	_, err := s.Bot.AnswerCallbackQuery(ctx, &telegram.AnswerCallbackQueryParams{CallbackQueryID: callbackID})
	return err
}

// replyMarkup converts a Keyboard into the library's markup type. A nil
// Keyboard yields a nil markup, which leaves the message without a keyboard.
func replyMarkup(k Keyboard) models.ReplyMarkup {
	if k.Inline != nil {
		rows := make([][]models.InlineKeyboardButton, 0, len(k.Inline))
		for _, row := range k.Inline {
			buttons := make([]models.InlineKeyboardButton, 0, len(row))
			for _, b := range row {
				buttons = append(buttons, models.InlineKeyboardButton{Text: b.Text, CallbackData: b.Data})
			}
			rows = append(rows, buttons)
		}
		return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
	}
	if k.Reply != nil {
		rows := make([][]models.KeyboardButton, 0, len(k.Reply))
		for _, row := range k.Reply {
			buttons := make([]models.KeyboardButton, 0, len(row))
			for _, caption := range row {
				buttons = append(buttons, models.KeyboardButton{Text: caption})
			}
			rows = append(rows, buttons)
		}
		return &models.ReplyKeyboardMarkup{
			Keyboard:       rows,
			ResizeKeyboard: true,
			IsPersistent:   true,
		}
	}
	return nil
}

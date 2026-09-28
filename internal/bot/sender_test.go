package bot

import (
	"testing"

	"github.com/go-telegram/bot/models"
)

// TestReplyMarkup pins the Keyboard-to-markup mapping, including the invariant
// clearKeyboard depends on: an empty, non-nil Inline must still produce a
// non-nil inline markup, otherwise the buttons of an existing message could
// never be removed.
func TestReplyMarkup(t *testing.T) {
	if got := replyMarkup(Keyboard{}); got != nil {
		t.Fatalf("keyboard kosong = %#v, want nil", got)
	}

	got := replyMarkup(Keyboard{Inline: [][]InlineButton{}})
	m, ok := got.(*models.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("inline kosong = %#v, want *models.InlineKeyboardMarkup", got)
	}
	if m.InlineKeyboard == nil || len(m.InlineKeyboard) != 0 {
		t.Fatalf("inline kosong = %+v, want baris kosong non-nil", m.InlineKeyboard)
	}

	got = replyMarkup(Keyboard{Inline: [][]InlineButton{{{Text: "A", Data: "a"}, {Text: "B", Data: "b"}}}})
	m, ok = got.(*models.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("inline = %#v, want *models.InlineKeyboardMarkup", got)
	}
	if len(m.InlineKeyboard) != 1 || len(m.InlineKeyboard[0]) != 2 {
		t.Fatalf("inline = %+v, want 1 baris 2 tombol", m.InlineKeyboard)
	}
	if m.InlineKeyboard[0][0].Text != "A" || m.InlineKeyboard[0][0].CallbackData != "a" {
		t.Fatalf("tombol pertama = %+v", m.InlineKeyboard[0][0])
	}

	got = replyMarkup(Keyboard{Reply: [][]string{{"X", "Y"}}})
	r, ok := got.(*models.ReplyKeyboardMarkup)
	if !ok {
		t.Fatalf("reply = %#v, want *models.ReplyKeyboardMarkup", got)
	}
	if !r.ResizeKeyboard || !r.IsPersistent {
		t.Fatalf("reply flags = resize:%v persistent:%v, want true/true", r.ResizeKeyboard, r.IsPersistent)
	}
	if len(r.Keyboard) != 1 || r.Keyboard[0][0].Text != "X" {
		t.Fatalf("reply = %+v", r.Keyboard)
	}
}

package bot

import (
	"reflect"
	"testing"

	"github.com/rchmdndy/telegram-money-bot/internal/period"
	"github.com/rchmdndy/telegram-money-bot/internal/persona"
	"github.com/rchmdndy/telegram-money-bot/internal/storage"
)

func TestStartSendsWelcome(t *testing.T) {
	h, sender, _, ctx := newTestHandler(t)
	mustSend(t, h, ctx, 1, "/start")

	if got, want := sender.last(t), say("welcome", nil); got != want {
		t.Fatalf("welcome = %q, want %q", got, want)
	}
	if got := sender.messages[0].key; !reflect.DeepEqual(got, mainReplyKeyboard()) {
		t.Fatalf("keyboard /start = %+v, want %+v", got, mainReplyKeyboard())
	}
}

// TestCategoryChoiceEmptyAfterDeactivatingAll drives sendCategoryChoice down
// its val.cat_empty branch: every seeded category is deactivated, so the
// deactivate picker has nothing to offer.
func TestCategoryChoiceEmptyAfterDeactivatingAll(t *testing.T) {
	h, sender, db, ctx := newTestHandler(t)
	mustSend(t, h, ctx, 1, "/start")
	for _, kind := range []storage.Kind{storage.KindExpense, storage.KindIncome} {
		cats, err := db.ListCategories(ctx, 1, kind, false)
		if err != nil {
			t.Fatalf("ListCategories: %v", err)
		}
		for _, c := range cats {
			if err := db.SetCategoryActive(ctx, 1, c.ID, false); err != nil {
				t.Fatalf("SetCategoryActive: %v", err)
			}
		}
	}

	mustCallback(t, h, ctx, 1, cbCatMenuOff)
	if got, want := sender.last(t), valid("val.cat_empty", nil); got != want {
		t.Fatalf("picker kosong = %q, want %q", got, want)
	}
	if got := sender.messages[len(sender.messages)-1].key; !reflect.DeepEqual(got, categoryMenuKeyboard()) {
		t.Fatalf("keyboard picker kosong = %+v, want %+v", got, categoryMenuKeyboard())
	}
}

func TestPersonaCallbackSwitchesPersona(t *testing.T) {
	h, sender, db, ctx := newTestHandler(t)
	mustCallback(t, h, ctx, 1, cbPersonaPrefix+string(persona.Netral))

	if got, want := sender.last(t), say("persona.changed", map[string]string{"persona": persona.Labels()[persona.Netral]}); got != want {
		t.Fatalf("ganti persona = %q, want %q", got, want)
	}
	s, err := db.GetSettings(ctx, 1)
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if s.Persona != string(persona.Netral) {
		t.Fatalf("persona tersimpan = %q, want %q", s.Persona, persona.Netral)
	}
}

// TestPeriodeSetPromptFlow walks /periode set with no day: the prompt asks for
// a day, the typed day inherits the active period name, and the conversation
// ends.
func TestPeriodeSetPromptFlow(t *testing.T) {
	h, sender, db, ctx := newTestHandler(t)
	mustSend(t, h, ctx, 1, "/periode set")

	if got, want := sender.last(t), prompt("prompt.period_start_day", nil); got != want {
		t.Fatalf("prompt = %q, want %q", got, want)
	}
	if got := conversation(t, db, ctx, 1); got != StatePeriodStartDay {
		t.Fatalf("state = %q, want %q", got, StatePeriodStartDay)
	}

	mustSend(t, h, ctx, 1, "25")
	r, err := period.Resolve(testToday, 25)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := say("period.saved", map[string]string{
		"period": persona.LabelDefaultPeriodName,
		"start":  period.FormatShort(r.Start),
	})
	if got := sender.last(t); got != want {
		t.Fatalf("periode tersimpan = %q, want %q", got, want)
	}
	if got := conversation(t, db, ctx, 1); got != "" {
		t.Fatalf("state setelah simpan = %q, want kosong", got)
	}
}

// TestTypedReminderTimeEndsTheConversation covers the reminder.time state: an
// invalid time keeps the state, a valid one stores it and ends the
// conversation.
func TestTypedReminderTimeEndsTheConversation(t *testing.T) {
	h, sender, db, ctx := newTestHandler(t)
	mustSend(t, h, ctx, 1, "/start")
	if err := h.saveConversation(ctx, 1, StateReminderTime, payload{}); err != nil {
		t.Fatalf("saveConversation: %v", err)
	}

	mustSend(t, h, ctx, 1, "25:00")
	if got, want := sender.last(t), valid("val.time", nil); got != want {
		t.Fatalf("jam tidak valid = %q, want %q", got, want)
	}
	if got := conversation(t, db, ctx, 1); got != StateReminderTime {
		t.Fatalf("state setelah jam tidak valid = %q, want %q", got, StateReminderTime)
	}

	mustSend(t, h, ctx, 1, "07:30")
	if got, want := sender.last(t), say("reminder.past", map[string]string{"time": "07:30"}); got != want {
		t.Fatalf("jam tersimpan = %q, want %q", got, want)
	}
	if got := conversation(t, db, ctx, 1); got != "" {
		t.Fatalf("state setelah jam valid = %q, want kosong", got)
	}
	s, err := db.GetSettings(ctx, 1)
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if s.ReminderTime != "07:30" {
		t.Fatalf("reminder_time = %q, want 07:30", s.ReminderTime)
	}
}

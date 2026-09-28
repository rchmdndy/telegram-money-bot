package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/dandy/telegram_money_bot/internal/storage"
)

// Conversation states (PRD §4.11). The values are the literal strings stored
// in conversations.state. The list is enforced in code rather than by a DB
// CHECK so a new state needs no migration (PRD §4.11), and the row is deleted
// when a conversation ends, so `idle` is never stored.
const (
	StateTxDate         = "tx.date"
	StateTxAmount       = "tx.amount"
	StateTxNote         = "tx.note"
	StateTxConfirm      = "tx.confirm"
	StateCatName        = "cat.name"
	StatePeriodStartDay = "period.start_day"
	StateReminderTime   = "reminder.time"
	StateTxEditDate     = "tx.edit.date"
	StateTxEditAmount   = "tx.edit.amount"
	StateTxEditNote     = "tx.edit.note"
	StateTxEditKind     = "tx.edit.kind"
	StateQuickConfirm   = "quick.confirm"
	StateQuickNote      = "quick.note"
)

// conversationStates lists every state in the order PRD §4.11 lists them.
var conversationStates = []string{
	StateTxDate,
	StateTxAmount,
	StateTxNote,
	StateTxConfirm,
	StateCatName,
	StatePeriodStartDay,
	StateReminderTime,
	StateTxEditDate,
	StateTxEditAmount,
	StateTxEditNote,
	StateTxEditKind,
	StateQuickConfirm,
	StateQuickNote,
}

// States returns a copy of every state the machine may store.
func States() []string { return append([]string(nil), conversationStates...) }

// ValidState reports whether state is one the machine knows.
func ValidState(state string) bool {
	for _, s := range conversationStates {
		if s == state {
			return true
		}
	}
	return false
}

// Category-flow modes carried by payload.CatMode.
const (
	catModeAdd    = "add"
	catModeRename = "rename"
	catModeOff    = "off"
)

// txData is the working copy of one transaction inside a conversation. The add
// flow fills it step by step (PRD §4.11 payload example); the edit flow starts
// from the stored row and mutates the field being changed.
type txData struct {
	ID           int64  `json:"id,omitempty"`
	Kind         string `json:"kind,omitempty"`
	OccurredOn   string `json:"occurred_on,omitempty"`
	CategoryID   int64  `json:"category_id,omitempty"`
	CategoryName string `json:"category_name,omitempty"`
	Amount       int64  `json:"amount,omitempty"`
	Note         string `json:"note,omitempty"`
}

// payload is the JSON blob stored in conversations.payload (PRD §4.11).
type payload struct {
	Cur        *txData `json:"cur,omitempty"`         // value being built
	Orig       *txData `json:"orig,omitempty"`        // value before an edit
	Field      string  `json:"field,omitempty"`       // field being edited (§4.9)
	Quick      bool    `json:"quick,omitempty"`       // started by quick input (§4.13)
	CatMode    string  `json:"cat_mode,omitempty"`    // add | rename | off (§4.4)
	CatID      int64   `json:"cat_id,omitempty"`      // category being renamed
	CatKind    string  `json:"cat_kind,omitempty"`    // kind of a new category
	PeriodName string  `json:"period_name,omitempty"` // name for a new period (§4.5)
	Fuzzy      string  `json:"fuzzy,omitempty"`       // raw category guess of a quick input (§4.13)
}

// loadConversation returns the user's active conversation. ok is false when
// there is none. A row holding an unknown state or unreadable payload is
// dropped rather than trusted, so a bug can never wedge the user.
func (h *Handler) loadConversation(ctx context.Context, userID int64) (storage.Conversation, payload, bool, error) {
	conv, err := h.db.GetConversation(ctx, userID)
	if errors.Is(err, storage.ErrNotFound) {
		return storage.Conversation{}, payload{}, false, nil
	}
	if err != nil {
		return storage.Conversation{}, payload{}, false, err
	}
	if !ValidState(conv.State) {
		h.log.Warn("state percakapan tidak dikenal", "user_id", userID, "state", conv.State)
		if err := h.db.DeleteConversation(ctx, userID); err != nil {
			return conv, payload{}, false, err
		}
		return conv, payload{}, false, nil
	}
	var pl payload
	if conv.Payload != "" {
		if err := json.Unmarshal([]byte(conv.Payload), &pl); err != nil {
			h.log.Warn("payload percakapan rusak", "user_id", userID, "err", err)
		}
	}
	return conv, pl, true, nil
}

// saveConversation stores state and payload, replacing any earlier row.
func (h *Handler) saveConversation(ctx context.Context, userID int64, state string, pl payload) error {
	if !ValidState(state) {
		return fmt.Errorf("bot: state %q", state)
	}
	raw, err := json.Marshal(pl)
	if err != nil {
		return err
	}
	return h.db.SetConversation(ctx, storage.Conversation{UserID: userID, State: state, Payload: string(raw)})
}

// dropConversation ends the user's conversation. A missing row is not an error.
func (h *Handler) dropConversation(ctx context.Context, userID int64) error {
	return h.db.DeleteConversation(ctx, userID)
}

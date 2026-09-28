package bot

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/rchmdndy/telegram-money-bot/internal/persona"
	"github.com/rchmdndy/telegram-money-bot/internal/quick"
	"github.com/rchmdndy/telegram-money-bot/internal/storage"
)

// Handler runs the conversation state machine, the commands and the callbacks
// of the bot. It owns no Telegram types: everything outbound goes through
// Sender, so the whole package is testable without a network (PRD §6).
type Handler struct {
	db     *storage.DB
	sender Sender
	loc    *time.Location
	now    func() time.Time
	log    *slog.Logger
}

// New builds a Handler. loc decides every user-visible date and time
// (PRD §4.8); a nil logger falls back to slog.Default.
func New(db *storage.DB, sender Sender, loc *time.Location, log *slog.Logger) *Handler {
	if loc == nil {
		loc = time.UTC
	}
	if log == nil {
		log = slog.Default()
	}
	return &Handler{db: db, sender: sender, loc: loc, now: time.Now, log: log}
}

// SetClock replaces the clock, for tests.
func (h *Handler) SetClock(now func() time.Time) {
	if now != nil {
		h.now = now
	}
}

// today is the current date in the configured zone as 'YYYY-MM-DD'.
func (h *Handler) today() string { return h.now().In(h.loc).Format("2006-01-02") }

// nowHHMM is the current time in the configured zone as 'HH:MM'.
func (h *Handler) nowHHMM() string { return h.now().In(h.loc).Format("15:04") }

// ensureUser registers a first-time user and seeds their defaults.
func (h *Handler) ensureUser(ctx context.Context, userID int64) error {
	_, err := h.db.EnsureUser(ctx, userID)
	return err
}

// personaID returns the user's persona. A value the catalog does not know
// falls back to netral, which is also what persona.render does for unknown
// IDs, so a corrupt column can never break a reply (PRD §6).
func (h *Handler) personaID(ctx context.Context, userID int64) persona.ID {
	s, err := h.db.GetSettings(ctx, userID)
	if err != nil {
		return persona.Default()
	}
	id := persona.ID(s.Persona)
	if !persona.Valid(id) {
		return persona.Netral
	}
	return id
}

func (h *Handler) send(ctx context.Context, chatID int64, text string, k Keyboard) error {
	_, err := h.sender.SendMessage(ctx, chatID, text, k)
	return err
}

// say renders a catalog key in the user's persona.
func (h *Handler) say(ctx context.Context, chatID int64, p persona.ID, key string, vars map[string]string, k Keyboard) error {
	return h.send(ctx, chatID, persona.Render(p, key, vars), k)
}

// validate renders a validation key in the user's persona (PRD §4.12 rule 4).
func (h *Handler) validate(ctx context.Context, chatID int64, p persona.ID, key string, vars map[string]string, k Keyboard) error {
	return h.send(ctx, chatID, persona.Validation(p, key, vars), k)
}

// ask renders a prompt key in the user's persona.
func (h *Handler) ask(ctx context.Context, chatID int64, p persona.ID, key string, vars map[string]string, k Keyboard) error {
	return h.send(ctx, chatID, persona.Prompt(p, key, vars), k)
}

// clearKeyboard removes the buttons of an already sent message, so a stale
// keyboard cannot be pressed twice.
func (h *Handler) clearKeyboard(ctx context.Context, chatID int64, messageID int) error {
	if messageID == 0 {
		return nil
	}
	return h.sender.EditMessageKeyboard(ctx, chatID, messageID, Keyboard{Inline: [][]InlineButton{}})
}

// session is the loaded conversation handed to the callbacks.
type session struct {
	state string
	pl    payload
	ok    bool
}

// loadSession loads the user's conversation into a session.
func (h *Handler) loadSession(ctx context.Context, userID int64) (session, error) {
	conv, pl, ok, err := h.loadConversation(ctx, userID)
	if err != nil {
		return session{}, err
	}
	return session{state: conv.State, pl: pl, ok: ok}, nil
}

// parseCommand splits a slash command into its name and its argument string.
// A trailing @botname is stripped, so /rekap@MyBot works in a group.
func parseCommand(text string) (cmd, args string, ok bool) {
	if !strings.HasPrefix(text, "/") {
		return "", "", false
	}
	body := text[1:]
	if i := strings.IndexAny(body, " \t\n"); i >= 0 {
		cmd, args = body[:i], strings.TrimSpace(body[i+1:])
	} else {
		cmd = body
	}
	if at := strings.Index(cmd, "@"); at >= 0 {
		cmd = cmd[:at]
	}
	if cmd == "" {
		return "", "", false
	}
	return strings.ToLower(cmd), args, true
}

// replyButtonKind maps the two ➕ reply buttons to a transaction kind.
func replyButtonKind(text string) (storage.Kind, bool) {
	switch text {
	case persona.BtnExpense:
		return storage.KindExpense, true
	case persona.BtnIncome:
		return storage.KindIncome, true
	}
	return "", false
}

// HandleMessage dispatches one incoming text message (PRD §5.4: command?
// button? conversation state? then query).
func (h *Handler) HandleMessage(ctx context.Context, chatID, userID int64, text string) error {
	if err := h.ensureUser(ctx, userID); err != nil {
		return err
	}
	p := h.personaID(ctx, userID)
	text = strings.TrimSpace(text)

	if cmd, args, ok := parseCommand(text); ok {
		return h.command(ctx, chatID, userID, p, cmd, args)
	}
	switch text {
	case persona.BtnRekap:
		return h.command(ctx, chatID, userID, p, "rekap", "")
	case persona.BtnSettings:
		return h.command(ctx, chatID, userID, p, "settings", "")
	}
	if kind, ok := replyButtonKind(text); ok {
		return h.startAdd(ctx, chatID, userID, p, kind)
	}

	sess, err := h.loadSession(ctx, userID)
	if err != nil {
		return err
	}
	if sess.ok {
		return h.stateInput(ctx, chatID, userID, p, sess, text)
	}
	if quick.IsQuickInput(text) {
		return h.quickInput(ctx, chatID, userID, p, text)
	}
	return h.say(ctx, chatID, p, "idle.hint", nil, mainReplyKeyboard())
}

// command routes a slash command (PRD §4.10).
func (h *Handler) command(ctx context.Context, chatID, userID int64, p persona.ID, cmd, args string) error {
	switch cmd {
	case "start":
		return h.cmdStart(ctx, chatID, userID, p)
	case "help":
		return h.say(ctx, chatID, p, "help.list", nil, mainReplyKeyboard())
	case "rekap":
		return h.cmdRekap(ctx, chatID, userID, p, args)
	case "hari":
		return h.cmdHari(ctx, chatID, userID, p)
	case "terakhir":
		return h.cmdTerakhir(ctx, chatID, userID, p)
	case "kategori":
		return h.cmdKategori(ctx, chatID, userID, p)
	case "periode":
		return h.cmdPeriode(ctx, chatID, userID, p, args)
	case "reminder":
		return h.cmdReminder(ctx, chatID, userID, p, args)
	case "export":
		return h.cmdExport(ctx, chatID, userID, p, args)
	case "batal":
		return h.cmdBatal(ctx, chatID, userID, p)
	case "settings":
		return h.cmdSettings(ctx, chatID, userID, p)
	case "persona":
		return h.cmdPersona(ctx, chatID, userID, p, args)
	}
	return h.say(ctx, chatID, p, "idle.hint", nil, mainReplyKeyboard())
}

// cmdBatal cancels the running conversation from any state (PRD §4.9).
func (h *Handler) cmdBatal(ctx context.Context, chatID, userID int64, p persona.ID) error {
	sess, err := h.loadSession(ctx, userID)
	if err != nil {
		return err
	}
	if !sess.ok {
		return h.validate(ctx, chatID, p, "val.no_conv", nil, mainReplyKeyboard())
	}
	if err := h.dropConversation(ctx, userID); err != nil {
		return err
	}
	return h.say(ctx, chatID, p, "tx.cancelled", nil, mainReplyKeyboard())
}

// stateInput handles free text while a conversation is active. Every branch
// either advances the machine or rejects with a specific message while
// leaving the state unchanged (PRD §4.11).
func (h *Handler) stateInput(ctx context.Context, chatID, userID int64, p persona.ID, sess session, text string) error {
	pl := sess.pl
	switch sess.state {
	case StateTxDate:
		return h.inputDate(ctx, chatID, userID, p, pl, text)
	case StateTxAmount:
		if pl.Cur == nil || pl.Cur.CategoryID == 0 {
			return h.sendCategoryPicker(ctx, chatID, userID, p, pl, StateTxAmount)
		}
		return h.inputAmount(ctx, chatID, userID, p, pl, text)
	case StateTxNote, StateQuickNote:
		return h.inputNote(ctx, chatID, userID, p, pl, text)
	case StateTxConfirm, StateQuickConfirm:
		return h.sendConfirmCard(ctx, chatID, p, pl)
	case StateCatName:
		return h.inputCatName(ctx, chatID, userID, p, pl, text)
	case StatePeriodStartDay:
		return h.inputPeriodStartDay(ctx, chatID, userID, p, pl, text)
	case StateReminderTime:
		return h.inputReminderTime(ctx, chatID, userID, p, text)
	case StateTxEditDate, StateTxEditAmount, StateTxEditNote, StateTxEditKind:
		return h.editInput(ctx, chatID, userID, p, pl, text)
	}
	return h.validate(ctx, chatID, p, "val.no_conv", nil, mainReplyKeyboard())
}

// HandleCallback dispatches one inline-button press. The spinner is answered
// first so the client never waits on the database work that follows.
func (h *Handler) HandleCallback(ctx context.Context, chatID, userID int64, messageID int, callbackID, data string) error {
	if err := h.sender.AnswerCallback(ctx, callbackID); err != nil {
		h.log.Warn("gagal menjawab callback", "user_id", userID, "err", err)
	}
	if err := h.ensureUser(ctx, userID); err != nil {
		return err
	}
	p := h.personaID(ctx, userID)
	sess, err := h.loadSession(ctx, userID)
	if err != nil {
		return err
	}

	switch {
	case data == cbDateToday, data == cbDateYesterday, data == cbDatePick:
		return h.cbDate(ctx, chatID, userID, p, messageID, sess, data)
	case data == cbSave:
		if sess.ok && sess.pl.Cur != nil && sess.pl.Cur.ID != 0 {
			return h.cbSaveEdit(ctx, chatID, userID, p, sess)
		}
		return h.cbSave(ctx, chatID, userID, p, messageID, sess)
	case data == cbNote:
		return h.cbNote(ctx, chatID, userID, p, messageID, sess)
	case data == cbCancel:
		return h.cbCancel(ctx, chatID, userID, p, messageID, sess.ok)
	case data == cbBack:
		return h.cbCancel(ctx, chatID, userID, p, messageID, sess.ok)
	case data == cbCatMenuAdd, data == cbCatMenuRename, data == cbCatMenuOff:
		return h.cbCategoryMenu(ctx, chatID, userID, p, data)
	case strings.HasPrefix(data, cbEditPrefix):
		return h.cbEditField(ctx, chatID, userID, p, messageID, sess, strings.TrimPrefix(data, cbEditPrefix))
	case strings.HasPrefix(data, cbTxEditPrefix):
		return h.cbTxEdit(ctx, chatID, userID, p, strings.TrimPrefix(data, cbTxEditPrefix))
	case strings.HasPrefix(data, cbTxDeletePrefix):
		return h.cbTxDelete(ctx, chatID, userID, p, strings.TrimPrefix(data, cbTxDeletePrefix))
	case strings.HasPrefix(data, cbCatPickPrefix):
		return h.cbCategoryPick(ctx, chatID, userID, p, messageID, sess, strings.TrimPrefix(data, cbCatPickPrefix))
	case strings.HasPrefix(data, cbCatRenamePrefix):
		return h.cbCategoryRename(ctx, chatID, userID, p, strings.TrimPrefix(data, cbCatRenamePrefix))
	case strings.HasPrefix(data, cbCatOffPrefix):
		return h.cbCategoryOff(ctx, chatID, userID, p, strings.TrimPrefix(data, cbCatOffPrefix))
	case strings.HasPrefix(data, cbKindPrefix):
		return h.cbCatKind(ctx, chatID, userID, p, messageID, sess, strings.TrimPrefix(data, cbKindPrefix))
	case strings.HasPrefix(data, cbPersonaPrefix):
		return h.cbPersona(ctx, chatID, userID, p, strings.TrimPrefix(data, cbPersonaPrefix))
	}
	h.log.Warn("callback tidak dikenal", "user_id", userID, "data", data)
	return nil
}

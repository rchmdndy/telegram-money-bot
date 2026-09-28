package bot

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rchmdndy/telegram-money-bot/internal/csv"
	"github.com/rchmdndy/telegram-money-bot/internal/period"
	"github.com/rchmdndy/telegram-money-bot/internal/persona"
	"github.com/rchmdndy/telegram-money-bot/internal/report"
	"github.com/rchmdndy/telegram-money-bot/internal/storage"
)

// maxCategoryName is the category name limit of PRD §4.4.
const maxCategoryName = 32

// maxActiveCategories is the practical limit of PRD §4.4: past it the
// keyboard becomes a wall of text.
const maxActiveCategories = 20

// cmdStart answers /start with the welcome and the main keyboard (PRD §4.10).
// Registration already happened in HandleMessage.
func (h *Handler) cmdStart(ctx context.Context, chatID, userID int64, p persona.ID) error {
	return h.say(ctx, chatID, p, "welcome", nil, mainReplyKeyboard())
}

// cmdRekap renders /rekap for a range argument (PRD §4.6).
func (h *Handler) cmdRekap(ctx context.Context, chatID, userID int64, p persona.ID, args string) error {
	r, name, err := h.resolveRange(ctx, userID, args)
	if err != nil {
		return h.validate(ctx, chatID, p, "val.range", nil, mainReplyKeyboard())
	}
	rows, err := h.db.ListTransactionsInRange(ctx, userID, r.Start, r.End)
	if err != nil {
		return err
	}
	rekap := report.Aggregate(reportRows(rows), r, name)
	for _, msg := range report.RenderRekap(p, rekap) {
		if err := h.send(ctx, chatID, msg, Keyboard{}); err != nil {
			return err
		}
	}
	return nil
}

// resolveRange maps a /rekap or /export argument to a range plus the name the
// header shows (PRD §4.6 table).
func (h *Handler) resolveRange(ctx context.Context, userID int64, args string) (period.Range, string, error) {
	today := h.today()
	switch strings.ToLower(strings.TrimSpace(args)) {
	case "", "periode":
		p, r, err := h.activePeriod(ctx, userID)
		if err != nil {
			return period.Range{}, "", err
		}
		return r, p.Name, nil
	case "hari":
		return period.Range{Start: today, End: today}, persona.LabelRangeToday, nil
	case "kemarin":
		yesterday, err := period.ShiftDays(today, -1)
		if err != nil {
			return period.Range{}, "", err
		}
		return period.Range{Start: yesterday, End: yesterday}, persona.LabelRangeYesterday, nil
	case "minggu":
		r, err := period.WeekRange(today)
		if err != nil {
			return period.Range{}, "", err
		}
		return r, persona.LabelRangeWeek, nil
	case "bulan":
		r, err := period.MonthRange(today)
		if err != nil {
			return period.Range{}, "", err
		}
		return r, persona.LabelRangeMonth, nil
	}
	start, end, ok := strings.Cut(strings.TrimSpace(args), "..")
	if !ok {
		return period.Range{}, "", fmt.Errorf("rentang %q", args)
	}
	startDate, err := h.parseISODate(start)
	if err != nil {
		return period.Range{}, "", err
	}
	endDate, err := h.parseISODate(end)
	if err != nil {
		return period.Range{}, "", err
	}
	if startDate > endDate {
		return period.Range{}, "", fmt.Errorf("rentang terbalik %q", args)
	}
	return period.Range{Start: startDate, End: endDate}, persona.LabelRangeCustom, nil
}

// parseISODate accepts the 'YYYY-MM-DD' form the range arguments use, which is
// deliberately not what period.ParseDate accepts from a user typing a date.
func (h *Handler) parseISODate(s string) (string, error) {
	t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(s), h.loc)
	if err != nil {
		return "", err
	}
	return t.Format("2006-01-02"), nil
}

// activePeriod resolves the period covering today (PRD §4.5).
func (h *Handler) activePeriod(ctx context.Context, userID int64) (period.Period, period.Range, error) {
	records, err := h.db.ListPeriods(ctx, userID)
	if err != nil {
		return period.Period{}, period.Range{}, err
	}
	periods := make([]period.Period, 0, len(records))
	for _, rec := range records {
		periods = append(periods, period.Period{
			Name:          rec.Name,
			StartDay:      rec.StartDay,
			EndDay:        rec.EndDay,
			EffectiveFrom: rec.EffectiveFrom,
		})
	}
	return period.ResolveActive(h.today(), periods)
}

// cmdHari lists today's transactions with the per-row buttons (PRD §4.9).
func (h *Handler) cmdHari(ctx context.Context, chatID, userID int64, p persona.ID) error {
	rows, err := h.db.ListTransactionsByDate(ctx, userID, h.today())
	if err != nil {
		return err
	}
	return h.sendList(ctx, chatID, p, persona.TitleToday, rows, false)
}

// cmdTerakhir lists the ten most recent transactions (PRD §4.9).
func (h *Handler) cmdTerakhir(ctx context.Context, chatID, userID int64, p persona.ID) error {
	rows, err := h.db.ListRecentTransactions(ctx, userID, 10)
	if err != nil {
		return err
	}
	return h.sendList(ctx, chatID, p, persona.TitleRecent, rows, true)
}

// sendList renders a transaction list together with the matching per-row
// buttons. Rows are ordered newest-first for /terakhir, so the keyboard is
// built from the same slice in the same order.
func (h *Handler) sendList(ctx context.Context, chatID int64, p persona.ID, title string, rows []storage.TransactionRow, showDate bool) error {
	txs := reportRows(rows)
	var total int64
	for _, t := range txs {
		total += t.Amount
	}
	text := report.RenderList(p, report.List{Title: title, Rows: txs, Total: total, ShowDate: showDate})
	if len(rows) == 0 {
		return h.send(ctx, chatID, text, Keyboard{})
	}
	return h.send(ctx, chatID, text, transactionKeyboard(rows))
}

// reportRows converts storage rows to the report projection.
func reportRows(rows []storage.TransactionRow) []report.Tx {
	out := make([]report.Tx, 0, len(rows))
	for _, r := range rows {
		out = append(out, report.Tx{
			ID:         r.ID,
			OccurredOn: r.OccurredOn,
			Kind:       string(r.Kind),
			Category:   r.CategoryName,
			Note:       r.Note,
			Amount:     r.Amount,
		})
	}
	return out
}

// cmdKategori shows the category list and its menu (PRD §4.4).
func (h *Handler) cmdKategori(ctx context.Context, chatID, userID int64, p persona.ID) error {
	text, err := h.categoryListText(ctx, userID, p)
	if err != nil {
		return err
	}
	return h.send(ctx, chatID, text, categoryMenuKeyboard())
}

// categoryListText renders every category of both kinds, marking the inactive
// ones. The header is the persona key of PRD §4.12.
func (h *Handler) categoryListText(ctx context.Context, userID int64, p persona.ID) (string, error) {
	var b strings.Builder
	b.WriteString(persona.Render(p, "category.list", nil))
	for _, kind := range []storage.Kind{storage.KindExpense, storage.KindIncome} {
		cats, err := h.db.ListCategories(ctx, userID, kind, false)
		if err != nil {
			return "", err
		}
		for _, c := range cats {
			b.WriteString("\n")
			b.WriteString(persona.LabelItemPrefix + c.Name)
			if !c.Active {
				b.WriteString(" " + persona.LabelInactive)
			}
		}
	}
	return b.String(), nil
}

// cbCategoryMenu opens one of the three /kategori actions (PRD §4.4).
func (h *Handler) cbCategoryMenu(ctx context.Context, chatID, userID int64, p persona.ID, data string) error {
	switch data {
	case cbCatMenuAdd:
		pl := payload{CatMode: catModeAdd}
		if err := h.saveConversation(ctx, userID, StateCatName, pl); err != nil {
			return err
		}
		return h.ask(ctx, chatID, p, "prompt.cat_kind", nil, kindKeyboard())
	case cbCatMenuRename:
		return h.sendCategoryChoice(ctx, chatID, userID, p, cbCatRenamePrefix, "prompt.cat_rename_pick", false)
	case cbCatMenuOff:
		return h.sendCategoryChoice(ctx, chatID, userID, p, cbCatOffPrefix, "prompt.cat_off_pick", true)
	}
	return nil
}

// sendCategoryChoice shows a category picker for the rename and deactivate
// actions. Deactivate offers only the active ones: the others are already off.
func (h *Handler) sendCategoryChoice(ctx context.Context, chatID, userID int64, p persona.ID, prefix, promptKey string, activeOnly bool) error {
	var cats []storage.Category
	for _, kind := range []storage.Kind{storage.KindExpense, storage.KindIncome} {
		list, err := h.db.ListCategories(ctx, userID, kind, activeOnly)
		if err != nil {
			return err
		}
		cats = append(cats, list...)
	}
	if len(cats) == 0 {
		return h.validate(ctx, chatID, p, "val.cat_empty", nil, categoryMenuKeyboard())
	}
	return h.ask(ctx, chatID, p, promptKey, nil, categoryKeyboard(cats, prefix, false))
}

// cbCategoryRename asks for the new name of the chosen category (PRD §4.4).
func (h *Handler) cbCategoryRename(ctx context.Context, chatID, userID int64, p persona.ID, rawID string) error {
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
	pl := payload{CatMode: catModeRename, CatID: cat.ID, CatKind: string(cat.Kind)}
	if err := h.saveConversation(ctx, userID, StateCatName, pl); err != nil {
		return err
	}
	return h.ask(ctx, chatID, p, "prompt.cat_rename_name", map[string]string{"category": cat.Name}, Keyboard{})
}

// cbCategoryOff deactivates the chosen category. Deactivation is not deletion,
// so no second confirmation is needed (PRD §4.4).
func (h *Handler) cbCategoryOff(ctx context.Context, chatID, userID int64, p persona.ID, rawID string) error {
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
	if err := h.db.SetCategoryActive(ctx, userID, cat.ID, false); err != nil {
		return err
	}
	return h.say(ctx, chatID, p, "category.deactivated", map[string]string{"category": cat.Name}, categoryMenuKeyboard())
}

// inputCatName consumes a category name for the add and rename flows
// (PRD §4.4, state cat.name).
func (h *Handler) inputCatName(ctx context.Context, chatID, userID int64, p persona.ID, pl payload, text string) error {
	name := strings.TrimSpace(text)
	if n := utf8.RuneCountInString(name); n < 1 || n > maxCategoryName {
		return h.validate(ctx, chatID, p, "val.cat_name", nil, Keyboard{})
	}
	kind := storage.Kind(pl.CatKind)
	if !kind.Valid() {
		return h.validate(ctx, chatID, p, "val.cat_empty", nil, mainReplyKeyboard())
	}
	exceptID := int64(0)
	if pl.CatMode == catModeRename {
		exceptID = pl.CatID
	}
	taken, err := h.db.CategoryNameTaken(ctx, userID, kind, name, exceptID)
	if err != nil {
		return err
	}
	if taken {
		return h.validate(ctx, chatID, p, "val.cat_dup", map[string]string{"category": name}, Keyboard{})
	}
	if pl.CatMode == catModeRename {
		if err := h.db.RenameCategory(ctx, userID, pl.CatID, name); err != nil {
			return err
		}
		if err := h.dropConversation(ctx, userID); err != nil {
			return err
		}
		return h.say(ctx, chatID, p, "category.renamed", map[string]string{"category": name}, categoryMenuKeyboard())
	}
	count, err := h.db.CountActiveCategories(ctx, userID, kind)
	if err != nil {
		return err
	}
	if count >= maxActiveCategories {
		return h.validate(ctx, chatID, p, "val.cat_max", map[string]string{"max": strconv.Itoa(maxActiveCategories)}, Keyboard{})
	}
	if _, err := h.db.CreateCategory(ctx, userID, name, kind); err != nil {
		if errors.Is(err, storage.ErrDuplicate) {
			return h.validate(ctx, chatID, p, "val.cat_dup", map[string]string{"category": name}, Keyboard{})
		}
		return err
	}
	if err := h.dropConversation(ctx, userID); err != nil {
		return err
	}
	return h.say(ctx, chatID, p, "category.saved", map[string]string{"category": name}, categoryMenuKeyboard())
}

// cmdPeriode handles /periode, /periode set and /periode list (PRD §4.5).
func (h *Handler) cmdPeriode(ctx context.Context, chatID, userID int64, p persona.ID, args string) error {
	fields := strings.Fields(args)
	if len(fields) == 0 {
		return h.sendPeriodStatus(ctx, chatID, userID, p)
	}
	switch strings.ToLower(fields[0]) {
	case "set":
		if len(fields) < 2 {
			pl := payload{PeriodName: h.currentPeriodName(ctx, userID)}
			if err := h.saveConversation(ctx, userID, StatePeriodStartDay, pl); err != nil {
				return err
			}
			return h.ask(ctx, chatID, p, "prompt.period_start_day", nil, Keyboard{})
		}
		day, err := strconv.Atoi(fields[1])
		if err != nil || !period.ValidStartDay(day) {
			return h.validate(ctx, chatID, p, "val.start_day", nil, mainReplyKeyboard())
		}
		name := strings.TrimSpace(strings.Join(fields[2:], " "))
		if name == "" {
			name = h.currentPeriodName(ctx, userID)
		}
		return h.savePeriod(ctx, chatID, userID, p, name, day)
	case "list":
		return h.sendPeriodList(ctx, chatID, userID, p)
	}
	return h.sendPeriodStatus(ctx, chatID, userID, p)
}

// sendPeriodStatus shows the active period and the day position inside it
// (PRD §4.5, key period.status).
func (h *Handler) sendPeriodStatus(ctx context.Context, chatID, userID int64, p persona.ID) error {
	active, r, err := h.activePeriod(ctx, userID)
	if err != nil {
		return h.validate(ctx, chatID, p, "val.no_conv", nil, mainReplyKeyboard())
	}
	dayNo, err := period.DayNumber(r, h.today())
	if err != nil {
		return err
	}
	days, err := period.Days(r)
	if err != nil {
		return err
	}
	return h.say(ctx, chatID, p, "period.status", map[string]string{
		"period":    active.Name,
		"start":     period.FormatShort(r.Start),
		"end":       period.FormatShort(r.End),
		"day_no":    strconv.Itoa(dayNo),
		"day_total": strconv.Itoa(days),
	}, mainReplyKeyboard())
}

// sendPeriodList shows the period history, newest first (PRD §4.5).
func (h *Handler) sendPeriodList(ctx context.Context, chatID, userID int64, p persona.ID) error {
	records, err := h.db.ListPeriods(ctx, userID)
	if err != nil {
		return err
	}
	var b strings.Builder
	for i, rec := range records {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(persona.LabelItemPrefix + rec.Name + " " +
			strconv.Itoa(rec.StartDay) + persona.LabelDayRange + strconv.Itoa(rec.EndDay) + " ")
		// The initial period carries the seedPeriodEffectiveFrom sentinel
		// rather than a schedule-change date, so rendering it verbatim would
		// show the user `mulai 1970-01-01`.
		if rec.IsSeed() {
			b.WriteString(persona.LabelEffectiveFromStart)
			continue
		}
		b.WriteString(persona.LabelEffectiveFrom + " " + rec.EffectiveFrom)
	}
	return h.send(ctx, chatID, b.String(), mainReplyKeyboard())
}

// currentPeriodName is the name a new period inherits when the user does not
// give one.
func (h *Handler) currentPeriodName(ctx context.Context, userID int64) string {
	active, _, err := h.activePeriod(ctx, userID)
	if err != nil || active.Name == "" {
		return persona.LabelDefaultPeriodName
	}
	return active.Name
}

// savePeriod stores a new period effective today and replies period.saved
// (PRD §4.5).
func (h *Handler) savePeriod(ctx context.Context, chatID, userID int64, p persona.ID, name string, startDay int) error {
	today := h.today()
	if _, err := h.db.CreatePeriod(ctx, userID, name, startDay, today); err != nil {
		if errors.Is(err, storage.ErrDuplicate) {
			return h.validate(ctx, chatID, p, "val.start_day", nil, mainReplyKeyboard())
		}
		return err
	}
	if err := h.dropConversation(ctx, userID); err != nil {
		return err
	}
	r, err := period.Resolve(today, startDay)
	if err != nil {
		return err
	}
	return h.say(ctx, chatID, p, "period.saved", map[string]string{
		"period": name,
		"start":  period.FormatShort(r.Start),
	}, mainReplyKeyboard())
}

// inputPeriodStartDay consumes the start day for /periode set with no argument
// (PRD §4.5, state period.start_day).
func (h *Handler) inputPeriodStartDay(ctx context.Context, chatID, userID int64, p persona.ID, pl payload, text string) error {
	day, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || !period.ValidStartDay(day) {
		return h.validate(ctx, chatID, p, "val.start_day", nil, Keyboard{})
	}
	name := pl.PeriodName
	if name == "" {
		name = h.currentPeriodName(ctx, userID)
	}
	return h.savePeriod(ctx, chatID, userID, p, name, day)
}

// cmdReminder handles /reminder and its on/off/HH:MM forms (PRD §4.8).
func (h *Handler) cmdReminder(ctx context.Context, chatID, userID int64, p persona.ID, args string) error {
	arg := strings.TrimSpace(args)
	switch strings.ToLower(arg) {
	case "":
		return h.sendSettingsSummary(ctx, chatID, userID, p)
	case "on":
		if err := h.db.SetReminderEnabled(ctx, userID, true); err != nil {
			return err
		}
		return h.say(ctx, chatID, p, "reminder.on", nil, mainReplyKeyboard())
	case "off":
		if err := h.db.SetReminderEnabled(ctx, userID, false); err != nil {
			return err
		}
		return h.say(ctx, chatID, p, "reminder.off", nil, mainReplyKeyboard())
	}
	hhmm, err := normalizeHHMM(arg)
	if err != nil {
		return h.validate(ctx, chatID, p, "val.time", nil, mainReplyKeyboard())
	}
	return h.setReminderTime(ctx, chatID, userID, p, hhmm)
}

// setReminderTime stores a valid HH:MM and tells the user whether it can still
// fire today (PRD §4.8, keys reminder.set / reminder.past).
func (h *Handler) setReminderTime(ctx context.Context, chatID, userID int64, p persona.ID, hhmm string) error {
	if err := h.db.SetReminderTime(ctx, userID, hhmm); err != nil {
		return err
	}
	key := "reminder.set"
	if h.nowHHMM() >= hhmm {
		key = "reminder.past"
	}
	return h.say(ctx, chatID, p, key, map[string]string{"time": hhmm}, mainReplyKeyboard())
}

// inputReminderTime consumes a typed reminder time (state reminder.time). The
// conversation ends on success, like every other input* step, so the user is
// not left typing times forever.
func (h *Handler) inputReminderTime(ctx context.Context, chatID, userID int64, p persona.ID, text string) error {
	hhmm, err := normalizeHHMM(text)
	if err != nil {
		return h.validate(ctx, chatID, p, "val.time", nil, Keyboard{})
	}
	if err := h.dropConversation(ctx, userID); err != nil {
		return err
	}
	return h.setReminderTime(ctx, chatID, userID, p, hhmm)
}

// normalizeHHMM validates a 24-hour HH:MM string and returns it in the exact
// stored form, so '9:05' is stored as '09:05' (PRD §4.8).
func normalizeHHMM(s string) (string, error) {
	t, err := time.Parse("15:04", strings.TrimSpace(s))
	if err != nil {
		return "", err
	}
	return t.Format("15:04"), nil
}

// cmdExport sends the CSV of a range (PRD §4.7).
func (h *Handler) cmdExport(ctx context.Context, chatID, userID int64, p persona.ID, args string) error {
	r, _, err := h.resolveRange(ctx, userID, args)
	if err != nil {
		return h.validate(ctx, chatID, p, "val.range", nil, mainReplyKeyboard())
	}
	rows, err := h.db.ListTransactionsInRange(ctx, userID, r.Start, r.End)
	if err != nil {
		return err
	}
	out := make([]csv.Row, 0, len(rows))
	for _, row := range rows {
		out = append(out, csv.Row{
			Date:     row.OccurredOn,
			Kind:     string(row.Kind),
			Category: row.CategoryName,
			Note:     row.Note,
			Amount:   row.Amount,
		})
	}
	filename := csv.Filename(r.Start, r.End)
	return h.sender.SendDocument(ctx, chatID, filename, csv.Export(out), filename)
}

// cmdSettings summarises the user's settings (PRD §4.10).
func (h *Handler) cmdSettings(ctx context.Context, chatID, userID int64, p persona.ID) error {
	return h.sendSettingsSummary(ctx, chatID, userID, p)
}

// sendSettingsSummary renders settings.summary and the category list, which
// together cover every setting the PRD lists for /settings (PRD §4.10).
func (h *Handler) sendSettingsSummary(ctx context.Context, chatID, userID int64, p persona.ID) error {
	settings, err := h.db.GetSettings(ctx, userID)
	if err != nil {
		return err
	}
	active, _, err := h.activePeriod(ctx, userID)
	if err != nil {
		return err
	}
	text := persona.Render(p, "settings.summary", map[string]string{
		"period":  active.Name,
		"time":    settings.ReminderTime,
		"persona": persona.Labels()[p],
	})
	if !settings.ReminderEnabled {
		text += " " + persona.LabelReminderOff
	}
	cats, err := h.categoryListText(ctx, userID, p)
	if err != nil {
		return err
	}
	return h.send(ctx, chatID, text+"\n\n"+cats, mainReplyKeyboard())
}

// cmdPersona shows or changes the persona (PRD §4.12).
func (h *Handler) cmdPersona(ctx context.Context, chatID, userID int64, p persona.ID, args string) error {
	fields := strings.Fields(args)
	if len(fields) >= 2 && strings.EqualFold(fields[0], "set") {
		return h.setPersona(ctx, chatID, userID, p, fields[1])
	}
	return h.ask(ctx, chatID, p, "prompt.persona",
		map[string]string{"persona": persona.Labels()[p]}, personaKeyboard(p))
}

// setPersona validates an ID against the catalog and stores it (PRD §4.12).
func (h *Handler) setPersona(ctx context.Context, chatID, userID int64, p persona.ID, rawID string) error {
	id := persona.ID(strings.ToLower(strings.TrimSpace(rawID)))
	if !persona.Valid(id) {
		return h.validate(ctx, chatID, p, "val.persona", map[string]string{
			"input": rawID,
			"list":  personaIDList(),
		}, mainReplyKeyboard())
	}
	if err := h.db.SetPersona(ctx, userID, string(id)); err != nil {
		return err
	}
	return h.say(ctx, chatID, p, "persona.changed",
		map[string]string{"persona": persona.Labels()[id]}, personaKeyboard(id))
}

// cbPersona applies a persona button (PRD §4.12).
func (h *Handler) cbPersona(ctx context.Context, chatID, userID int64, p persona.ID, rawID string) error {
	return h.setPersona(ctx, chatID, userID, p, rawID)
}

// personaIDList is the {list} token of val.persona.
func personaIDList() string {
	ids := persona.IDs()
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, string(id))
	}
	return strings.Join(parts, ", ")
}

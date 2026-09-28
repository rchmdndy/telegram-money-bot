// Package report turns transactions into the aggregate numbers and the
// rendered messages behind /rekap, the daily reminder and /hari /terakhir
// (PRD §4.6, §4.8, §4.9). It is pure: no database, no network, no clock and
// no filesystem. Every user-facing string comes from package persona, so the
// numbers are the only thing this package contributes.
package report

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/dandy/telegram_money_bot/internal/money"
	"github.com/dandy/telegram_money_bot/internal/period"
	"github.com/dandy/telegram_money_bot/internal/persona"
)

// Transaction kinds. They mirror storage.Kind but are declared here so the
// pure reporting packages never depend on the SQLite driver.
const (
	Expense = "expense"
	Income  = "income"
)

// MaxMessageLen is Telegram's hard limit on sendMessage text (PRD §4.6).
// /rekap splits itself at this boundary; sendDocument is unaffected.
const MaxMessageLen = 4096

// nameWidth is the width of the category-name column in /rekap. It puts the
// amount column at byte offset 17, which is where the PRD §4.6 sample puts it.
const nameWidth = 15

// labelWidth is the width of the label column in the reminder body, so the
// amounts of "Pengeluaran", "Pemasukan", "Terpakai" and "Sisa" line up.
const labelWidth = 13

// minPctWidth keeps "(88,4%)" and "( 6,3%)" the same width (PRD §4.6).
const minPctWidth = 5

// Tx is the minimal transaction projection the renderers need.
type Tx struct {
	ID         int64
	OccurredOn string
	Kind       string
	Category   string
	Amount     int64
	Note       string
}

// CategoryTotal is one aggregated category inside a range.
type CategoryTotal struct {
	Name  string
	Total int64
	Count int
	Pct   float64
}

// Rekap is the aggregate behind /rekap (PRD §4.6).
type Rekap struct {
	PeriodName   string
	Range        period.Range
	Days         int
	Expense      []CategoryTotal
	ExpenseTotal int64
	ExpenseCount int
	Income       []CategoryTotal
	IncomeTotal  int64
	IncomeCount  int
	Last         *Tx
	DailyAverage int64
}

// Aggregate groups rows by category for the range r. Categories come back
// sorted by total descending (ties by name, so the output is deterministic);
// zero-valued categories never appear because they are never created.
func Aggregate(rows []Tx, r period.Range, periodName string) Rekap {
	re := Rekap{PeriodName: periodName, Range: r}
	if d, err := period.Days(r); err == nil {
		re.Days = d
	}
	expBy := map[string]*CategoryTotal{}
	incBy := map[string]*CategoryTotal{}
	var expOrder, incOrder []string
	for i := range rows {
		row := rows[i]
		if row.Kind == Income {
			re.IncomeTotal += row.Amount
			re.IncomeCount++
			addTotal(incBy, &incOrder, row.Category, row.Amount)
		} else {
			re.ExpenseTotal += row.Amount
			re.ExpenseCount++
			addTotal(expBy, &expOrder, row.Category, row.Amount)
		}
		if re.Last == nil || newer(row, *re.Last) {
			last := row
			re.Last = &last
		}
	}
	re.Expense = collect(expBy, expOrder, re.ExpenseTotal)
	re.Income = collect(incBy, incOrder, re.IncomeTotal)
	if re.Days > 0 {
		re.DailyAverage = int64(math.Round(float64(re.ExpenseTotal) / float64(re.Days)))
	}
	return re
}

func addTotal(by map[string]*CategoryTotal, order *[]string, name string, amount int64) {
	ct, ok := by[name]
	if !ok {
		ct = &CategoryTotal{Name: name}
		by[name] = ct
		*order = append(*order, name)
	}
	ct.Total += amount
	ct.Count++
}

func collect(by map[string]*CategoryTotal, order []string, total int64) []CategoryTotal {
	out := make([]CategoryTotal, 0, len(order))
	for _, name := range order {
		ct := *by[name]
		if total > 0 {
			ct.Pct = pct(ct.Total, total)
		}
		out = append(out, ct)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Total != out[j].Total {
			return out[i].Total > out[j].Total
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// newer reports whether a is later than b. Ties on the date are broken by ID
// so "Transaksi terakhir" is stable.
func newer(a, b Tx) bool {
	if a.OccurredOn != b.OccurredOn {
		return a.OccurredOn > b.OccurredOn
	}
	return a.ID > b.ID
}

// pct returns part/whole as a percentage in 0..100.
func pct(part, whole int64) float64 {
	if whole == 0 {
		return 0
	}
	return float64(part) / float64(whole) * 100
}

// FormatPct renders a percentage with one decimal, an Indonesian decimal
// comma and the percent sign (PRD §4.12 token {pct} = "13,8%"). Rounding can
// make a column sum to 99,9% or 100,1%; that is explicitly not a bug.
func FormatPct(p float64) string {
	s := strconv.FormatFloat(math.Round(p*10)/10, 'f', 1, 64)
	return strings.Replace(s, ".", ",", 1) + "%"
}

// RenderRekap renders /rekap and splits it into Telegram-sized messages.
// A continuation repeats the persona header plus a "(lanjutan)" marker, and
// no category line is ever cut in half (PRD §4.6).
func RenderRekap(id persona.ID, d Rekap) []string {
	header := persona.LabelRekapPrefix + " " +
		persona.Render(id, "rekap.header", map[string]string{"period": d.PeriodName})
	head := []string{header}
	if d.Days > 0 {
		head = append(head, fmt.Sprintf("%s (%d %s)", period.Format(d.Range), d.Days, persona.LabelDayUnit))
	} else {
		head = append(head, period.Format(d.Range))
	}
	if d.ExpenseCount == 0 && d.IncomeCount == 0 {
		lines := append(append([]string{}, head...), "", persona.Render(id, "rekap.empty", nil))
		return []string{strings.Join(lines, "\n")}
	}
	return packMessages(head, header+" "+persona.LabelContinuation, rekapBody(id, d))
}

func rekapBody(id persona.ID, d Rekap) []string {
	amountW := amountWidth(d.Expense, d.Income)
	pctW := pctWidth(d.Expense, d.Income)

	var body []string
	body = append(body, "")
	body = append(body, persona.LabelExpense+persona.LabelSep+money.Format(d.ExpenseTotal))
	body = append(body, categoryLines(d.Expense, amountW, pctW)...)
	body = append(body, "")
	body = append(body, persona.LabelIncome+persona.LabelSep+money.Format(d.IncomeTotal))
	body = append(body, categoryLines(d.Income, amountW, pctW)...)
	body = append(body, "")
	if d.IncomeTotal > 0 {
		body = append(body, persona.LabelSaldo+persona.LabelSep+money.Format(d.IncomeTotal-d.ExpenseTotal))
	} else {
		// PRD §4.6: with no income at all the SALDO line is replaced, so the
		// user never sees a misleading negative number.
		body = append(body, persona.Render(id, "rekap.no_income", nil))
	}
	if d.Days > 0 {
		body = append(body, persona.LabelAverage+persona.LabelSep+money.Format(d.DailyAverage))
	}
	if d.Last != nil {
		body = append(body, "")
		body = append(body, persona.LabelLastTx+": "+
			period.FormatDayMonth(d.Last.OccurredOn)+persona.LabelSep+
			noteOrDash(d.Last.Note)+" "+money.Format(d.Last.Amount))
	}
	return body
}

// categoryLines renders one aligned line per category, or the "(belum ada)"
// placeholder when the section is empty.
func categoryLines(ts []CategoryTotal, amountW, pctW int) []string {
	if len(ts) == 0 {
		return []string{"  " + persona.LabelEmptySection}
	}
	out := make([]string, 0, len(ts))
	for _, t := range ts {
		out = append(out, "  "+padRight(t.Name, nameWidth)+
			"Rp "+padLeft(money.FormatPlain(t.Total), amountW)+
			"  ("+padLeft(FormatPct(t.Pct), pctW)+")"+
			"  "+strconv.Itoa(t.Count)+" "+persona.LabelTxUnit)
	}
	return out
}

// amountWidth aligns both sections on the same numeric column: the amount
// itself is right-aligned, the "Rp " prefix is not padded, so the currency
// symbol stays put (PRD §4.6 sample).
func amountWidth(sections ...[]CategoryTotal) int {
	w := 0
	for _, ts := range sections {
		for _, t := range ts {
			if n := utf8.RuneCountInString(money.FormatPlain(t.Total)); n > w {
				w = n
			}
		}
	}
	return w
}

func pctWidth(sections ...[]CategoryTotal) int {
	w := minPctWidth
	for _, ts := range sections {
		for _, t := range ts {
			if n := utf8.RuneCountInString(FormatPct(t.Pct)); n > w {
				w = n
			}
		}
	}
	return w
}

// Reminder is the data behind the daily reminder (PRD §4.8).
type Reminder struct {
	Date            string // 'YYYY-MM-DD'
	DayExpense      int64
	DayExpenseCount int
	DayIncome       int64
	DayIncomeCount  int
	DayCategories   []CategoryTotal
	PeriodName      string
	PeriodRange     period.Range
	DayNo           int
	DayTotal        int
	PeriodIncome    int64
	PeriodExpense   int64
}

// RenderReminder renders the daily reminder. The reminder is always sent when
// enabled, so a day with no transactions still produces the reminder.empty
// message (PRD §4.8).
func RenderReminder(id persona.ID, d Reminder) string {
	dateLong := period.FormatDayMonthName(d.Date)
	if d.DayExpenseCount == 0 && d.DayIncomeCount == 0 {
		return persona.Render(id, "reminder.empty", map[string]string{"date": dateLong})
	}

	var b strings.Builder
	b.WriteString(persona.Render(id, "reminder.summary", map[string]string{"date": dateLong}))
	b.WriteString("\n")
	b.WriteString(labelLine(persona.LabelDayExpense, money.Format(d.DayExpense)))
	if d.DayExpenseCount > 0 {
		b.WriteString("  (" + strconv.Itoa(d.DayExpenseCount) + " " + persona.LabelTxUnit + ")")
	}
	b.WriteString("\n")
	b.WriteString(labelLine(persona.LabelDayIncome, money.Format(d.DayIncome)))
	if d.DayIncomeCount > 0 {
		b.WriteString("  (" + strconv.Itoa(d.DayIncomeCount) + " " + persona.LabelTxUnit + ")")
	}
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("%s %s (hari ke-%d dari %d)",
		persona.LabelPeriod, d.PeriodName, d.DayNo, d.DayTotal))
	b.WriteString("\n")
	if d.PeriodIncome > 0 {
		b.WriteString(labelLine(persona.LabelDayIncome, money.Format(d.PeriodIncome)))
		b.WriteString("\n")
		b.WriteString(labelLine(persona.LabelUsed, money.Format(d.PeriodExpense)))
		b.WriteString("  (" + FormatPct(pct(d.PeriodExpense, d.PeriodIncome)) + " " + persona.LabelFromIncome + ")")
		b.WriteString("\n")
		b.WriteString(labelLine(persona.LabelLeft, money.Format(d.PeriodIncome-d.PeriodExpense)))
	} else {
		// PRD §4.8: no income in the period means no misleading negative
		// "Sisa" — one rekap.no_income line instead.
		b.WriteString(persona.Render(id, "rekap.no_income", nil))
	}
	if len(d.DayCategories) > 0 {
		b.WriteString("\n\n")
		b.WriteString(persona.LabelDetail + ": ")
		parts := make([]string, 0, len(d.DayCategories))
		for _, c := range d.DayCategories {
			parts = append(parts, c.Name+" "+money.Format(c.Total))
		}
		b.WriteString(strings.Join(parts, ", "))
	}
	return b.String()
}

// List is the data behind /hari and /terakhir (PRD §4.9).
type List struct {
	Title    string
	Rows     []Tx
	Total    int64
	ShowDate bool
}

// RenderList renders the list header plus one line per transaction, in the
// same order the caller builds the inline keyboard rows.
func RenderList(id persona.ID, d List) string {
	header := persona.Render(id, "list.header", map[string]string{
		"title": d.Title,
		"count": strconv.Itoa(len(d.Rows)),
		"total": money.Format(d.Total),
	})
	if len(d.Rows) == 0 {
		return header
	}
	var b strings.Builder
	b.WriteString(header)
	for _, row := range d.Rows {
		b.WriteString("\n")
		b.WriteString(ListRow(row, d.ShowDate))
	}
	return b.String()
}

// ListRow renders one transaction line, matching the inline keyboard row the
// caller appends for it.
func ListRow(t Tx, showDate bool) string {
	line := persona.LabelItemPrefix
	if showDate {
		line += period.FormatDayMonth(t.OccurredOn) + " "
	}
	line += t.Category + " " + money.Format(t.Amount)
	if strings.TrimSpace(t.Note) != "" {
		line += persona.LabelSep + t.Note
	}
	return line
}

// packMessages splits head+body into Telegram-sized messages. head is only on
// the first message; every continuation starts with contHeader. Lines are
// atomic, so a category line is never cut in half (PRD §4.6).
func packMessages(head []string, contHeader string, body []string) []string {
	var msgs []string
	cur := append([]string{}, head...)
	curLen := joinedLen(cur)
	atHeader := true
	for _, line := range body {
		if !atHeader && curLen+1+len(line) > MaxMessageLen {
			msgs = append(msgs, strings.Join(cur, "\n"))
			cur = []string{contHeader}
			curLen = len(contHeader)
			atHeader = true
		}
		cur = append(cur, line)
		curLen += 1 + len(line)
		atHeader = false
	}
	return append(msgs, strings.Join(cur, "\n"))
}

func joinedLen(lines []string) int {
	n := 0
	for i, l := range lines {
		if i > 0 {
			n++
		}
		n += len(l)
	}
	return n
}

// labelLine pads a label to labelWidth so the amounts line up.
func labelLine(label, value string) string {
	return padRight(label, labelWidth) + value
}

func noteOrDash(note string) string {
	if strings.TrimSpace(note) == "" {
		return persona.LabelNoNote
	}
	return note
}

func padRight(s string, w int) string {
	if n := utf8.RuneCountInString(s); n < w {
		return s + strings.Repeat(" ", w-n)
	}
	return s
}

func padLeft(s string, w int) string {
	if n := utf8.RuneCountInString(s); n < w {
		return strings.Repeat(" ", w-n) + s
	}
	return s
}

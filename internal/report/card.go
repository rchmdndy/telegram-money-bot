package report

import (
	"strings"

	"github.com/dandy/telegram_money_bot/internal/persona"
)

// cardLabelWidth puts the colon of every card line in the same column, which
// is what makes PRD §4.2 step 5 and §4.9 step 3 line up:
//
//	Tanggal  : 21 Sep 2026 (Senin)
//	Kategori : Makan
const cardLabelWidth = 9

// Card is the confirmation card of PRD §4.2 step 5 and §4.9 step 3, and also
// the body of the edit field menu. The caller pre-formats every value (dates,
// money, notes) so the renderer stays a pure layout function; the labels are
// fixed strings that never follow the persona (PRD §4.2: "label field ... dan
// nilainya tidak pernah diubah persona").
type Card struct {
	// Header is the already-rendered first line (tx.confirm.header or
	// tx.edit.confirm.header). An empty Header emits no first line, which is
	// what the edit field menu uses.
	Header string
	// Date, Category, Amount and Note are the four field values. When a field
	// was changed, its value already carries the `old → new` text; the
	// renderer never rewrites a value.
	Date     string
	Category string
	Amount   string
	Note     string
}

// RenderCard renders the card body, one field per line.
func RenderCard(c Card) string {
	lines := make([]string, 0, 5)
	if c.Header != "" {
		lines = append(lines, c.Header)
	}
	lines = append(lines,
		cardLine(persona.LabelDate, c.Date),
		cardLine(persona.LabelCategory, c.Category),
		cardLine(persona.LabelAmount, c.Amount),
		cardLine(persona.LabelNote, c.Note),
	)
	return strings.Join(lines, "\n")
}

func cardLine(label, value string) string {
	return padRight(label, cardLabelWidth) + ": " + value
}

package bot

import (
	"context"
	"strings"
	"testing"

	"github.com/rchmdndy/telegram-money-bot/internal/money"
	"github.com/rchmdndy/telegram-money-bot/internal/period"
	"github.com/rchmdndy/telegram-money-bot/internal/persona"
	"github.com/rchmdndy/telegram-money-bot/internal/report"
	"github.com/rchmdndy/telegram-money-bot/internal/storage"
)

// rekapFor renders the /rekap reply the handler must produce for a range.
func rekapFor(t *testing.T, h *Handler, ctx context.Context, userID int64, r period.Range, name string) []string {
	t.Helper()
	rows, err := h.db.ListTransactionsInRange(ctx, userID, r.Start, r.End)
	if err != nil {
		t.Fatalf("ListTransactionsInRange: %v", err)
	}
	return report.RenderRekap(persona.Default(), report.Aggregate(reportRows(rows), r, name))
}

func assertMessages(t *testing.T, sender *mockSender, want []string) {
	t.Helper()
	if len(sender.messages) != len(want) {
		t.Fatalf("jumlah pesan = %d, want %d (%q)", len(sender.messages), len(want), sender.last(t))
	}
	for i, w := range want {
		if sender.messages[i].text != w {
			t.Fatalf("pesan[%d] =\n%s\nwant\n%s", i, sender.messages[i].text, w)
		}
	}
}

func TestRekapRangeVariants(t *testing.T) {
	h, sender, _, ctx := newTestHandler(t)
	addExpense(t, h, sender, ctx, 1, "Makan", "17000")
	mustCallback(t, h, ctx, 1, cbSave)

	yesterday, err := period.ShiftDays(testToday, -1)
	if err != nil {
		t.Fatalf("ShiftDays: %v", err)
	}
	week, err := period.WeekRange(testToday)
	if err != nil {
		t.Fatalf("WeekRange: %v", err)
	}
	month, err := period.MonthRange(testToday)
	if err != nil {
		t.Fatalf("MonthRange: %v", err)
	}

	cases := []struct {
		args string
		r    period.Range
		name string
	}{
		{"hari", period.Range{Start: testToday, End: testToday}, persona.LabelRangeToday},
		{"kemarin", period.Range{Start: yesterday, End: yesterday}, persona.LabelRangeYesterday},
		{"minggu", week, persona.LabelRangeWeek},
		{"bulan", month, persona.LabelRangeMonth},
		{"2026-09-21..2026-09-26", period.Range{Start: "2026-09-21", End: "2026-09-26"}, persona.LabelRangeCustom},
		{"", mustRange(t, testToday), persona.LabelDefaultPeriodName},
	}
	for _, tc := range cases {
		t.Run(tc.args, func(t *testing.T) {
			sender.reset()
			mustSend(t, h, ctx, 1, "/rekap "+tc.args)
			assertMessages(t, sender, rekapFor(t, h, ctx, 1, tc.r, tc.name))
		})
	}
}

func mustRange(t *testing.T, date string) period.Range {
	t.Helper()
	r, err := period.Resolve(date, 21)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	return r
}

func TestRekapRejectsUnknownAndReversedRanges(t *testing.T) {
	h, sender, _, ctx := newTestHandler(t)
	for _, args := range []string{"ngawur", "2026-09-26..2026-09-21", "21/09..26/09"} {
		t.Run(args, func(t *testing.T) {
			sender.reset()
			mustSend(t, h, ctx, 1, "/rekap "+args)
			if got, want := sender.last(t), valid("val.range", nil); got != want {
				t.Fatalf("/rekap %s = %q, want %q", args, got, want)
			}
		})
	}
}

func TestRekapEmptyRangeShowsHeaderOnly(t *testing.T) {
	h, sender, _, ctx := newTestHandler(t)
	mustSend(t, h, ctx, 1, "/rekap hari")
	want := rekapFor(t, h, ctx, 1, period.Range{Start: testToday, End: testToday}, persona.LabelRangeToday)
	assertMessages(t, sender, want)
	if len(want) != 1 || !strings.Contains(want[0], say("rekap.empty", nil)) {
		t.Fatalf("rekap kosong = %q", want)
	}
}

func TestPeriodeListShowsEveryLayer(t *testing.T) {
	h, sender, db, ctx := newTestHandler(t)
	mustSend(t, h, ctx, 1, "/periode set 25 Gajian")
	sender.reset()
	mustSend(t, h, ctx, 1, "/periode list")

	recs, err := db.ListPeriods(ctx, 1)
	if err != nil {
		t.Fatalf("ListPeriods: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("periode = %d, want 2", len(recs))
	}

	// Newest first: the schedule change the user just made, then the period
	// every user gets on registration. The second line is spelled out because
	// the initial period's effective_from is an internal sentinel, never a
	// date the user chose.
	want := persona.LabelItemPrefix + "Gajian 25" + persona.LabelDayRange + "24 " +
		persona.LabelEffectiveFrom + " " + testToday + "\n" +
		persona.LabelItemPrefix + "Gaji 21" + persona.LabelDayRange + "20 " +
		persona.LabelEffectiveFromStart
	if got := sender.last(t); got != want {
		t.Fatalf("/periode list =\n%s\nwant\n%s", got, want)
	}
	// Regression guard: the seed sentinel must never reach the user.
	if got := sender.last(t); strings.Contains(got, "1970-01-01") {
		t.Fatalf("/periode list membocorkan sentinel periode awal: %q", got)
	}
}

func TestHelpAndSettings(t *testing.T) {
	h, sender, _, ctx := newTestHandler(t)
	mustSend(t, h, ctx, 1, "/help")
	if got, want := sender.last(t), say("help.list", nil); got != want {
		t.Fatalf("/help = %q, want %q", got, want)
	}

	sender.reset()
	mustSend(t, h, ctx, 1, "/settings")
	settings, err := h.db.GetSettings(ctx, 1)
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	active, _, err := h.activePeriod(ctx, 1)
	if err != nil {
		t.Fatalf("activePeriod: %v", err)
	}
	cats, err := h.categoryListText(ctx, 1, persona.Default())
	if err != nil {
		t.Fatalf("categoryListText: %v", err)
	}
	want := say("settings.summary", map[string]string{
		"period":  active.Name,
		"time":    settings.ReminderTime,
		"persona": persona.Labels()[persona.Default()],
	}) + "\n\n" + cats
	if got := sender.last(t); got != want {
		t.Fatalf("/settings =\n%s\nwant\n%s", got, want)
	}
}

func TestEmptyLists(t *testing.T) {
	h, sender, _, ctx := newTestHandler(t)
	mustSend(t, h, ctx, 1, "/hari")
	want := report.RenderList(persona.Default(), report.List{Title: persona.TitleToday, Total: 0})
	if got := sender.messages[len(sender.messages)-1]; got.text != want || len(got.key.Inline) != 0 {
		t.Fatalf("/hari kosong = %q (keyboard %+v)", got.text, got.key.Inline)
	}

	mustSend(t, h, ctx, 1, "/terakhir")
	want = report.RenderList(persona.Default(), report.List{Title: persona.TitleRecent, Total: 0, ShowDate: true})
	if got := sender.last(t); got != want {
		t.Fatalf("/terakhir kosong = %q, want %q", got, want)
	}
}

func TestTerakhirShowsTheDateAndButtons(t *testing.T) {
	h, sender, db, ctx := newTestHandler(t)
	addExpense(t, h, sender, ctx, 1, "Makan", "17000")
	mustCallback(t, h, ctx, 1, cbSave)
	rows, err := db.ListRecentTransactions(ctx, 1, 1)
	if err != nil {
		t.Fatalf("ListRecentTransactions: %v", err)
	}

	sender.reset()
	mustSend(t, h, ctx, 1, "/terakhir")
	msg := sender.messages[len(sender.messages)-1]
	want := report.RenderList(persona.Default(), report.List{
		Title:    persona.TitleRecent,
		Rows:     []report.Tx{{ID: rows[0].ID, OccurredOn: testToday, Kind: "expense", Category: "Makan", Amount: 17000}},
		Total:    17000,
		ShowDate: true,
	})
	if msg.text != want {
		t.Fatalf("/terakhir =\n%s\nwant\n%s", msg.text, want)
	}
	if !strings.Contains(msg.text, period.FormatDayMonth(testToday)) {
		t.Fatalf("/terakhir tidak memuat tanggal: %q", msg.text)
	}
	if len(msg.key.Inline) != 1 || msg.key.Inline[0][0].Data != cbTxEditPrefix+itoa(rows[0].ID) {
		t.Fatalf("keyboard /terakhir = %+v", msg.key.Inline)
	}
}

func TestNoteButtonOnAddAndQuickCards(t *testing.T) {
	h, sender, db, ctx := newTestHandler(t)

	// Add flow: ✏️ Catatan replaces the note and returns to the card.
	mustSend(t, h, ctx, 1, persona.BtnExpense)
	mustCallback(t, h, ctx, 1, cbDateToday)
	mustCallback(t, h, ctx, 1, cbCatPickPrefix+itoa(catID(t, h.db, ctx, 1, storage.KindExpense, "Makan")))
	mustSend(t, h, ctx, 1, "17000")
	mustCallback(t, h, ctx, 1, cbNote)
	if got, want := sender.last(t), prompt("tx.prompt.note", nil); got != want {
		t.Fatalf("tombol catatan = %q, want %q", got, want)
	}
	if got := conversation(t, db, ctx, 1); got != StateTxNote {
		t.Fatalf("state = %q, want %q", got, StateTxNote)
	}
	mustSend(t, h, ctx, 1, "makan malam")
	wantCard := report.RenderCard(report.Card{
		Header:   say("tx.confirm.header", map[string]string{"kind": persona.KindExpenseWord}),
		Date:     period.FormatDayDate(testToday),
		Category: "Makan",
		Amount:   money.Format(17000),
		Note:     "makan malam",
	})
	if got := sender.last(t); got != wantCard {
		t.Fatalf("kartu setelah catatan =\n%s\nwant\n%s", got, wantCard)
	}
	mustCallback(t, h, ctx, 1, cbSave)

	rows, err := db.ListTransactionsByDate(ctx, 1, testToday)
	if err != nil {
		t.Fatalf("ListTransactionsByDate: %v", err)
	}
	if len(rows) != 1 || rows[0].Note != "makan malam" {
		t.Fatalf("catatan tersimpan = %+v", rows)
	}

	// Quick flow: the same button must keep the quick card, not fall back to
	// the add flow's confirmation.
	sender.reset()
	mustSend(t, h, ctx, 1, "o, 47k, makan")
	mustCallback(t, h, ctx, 1, cbNote)
	if got := conversation(t, db, ctx, 1); got != StateQuickNote {
		t.Fatalf("state quick note = %q, want %q", got, StateQuickNote)
	}
	mustSend(t, h, ctx, 1, "telur, saos, es krim")
	if got := sender.last(t); !strings.Contains(got, "telur, saos, es krim") {
		t.Fatalf("kartu quick setelah catatan = %q", got)
	}
	mustCallback(t, h, ctx, 1, cbSave)

	rows, err = db.ListRecentTransactions(ctx, 1, 1)
	if err != nil {
		t.Fatalf("ListRecentTransactions: %v", err)
	}
	if rows[0].Note != "telur, saos, es krim" || rows[0].Amount != 47000 {
		t.Fatalf("baris quick = %+v", rows[0].Transaction)
	}
}

func TestNoteTooLongIsRejected(t *testing.T) {
	h, sender, db, ctx := newTestHandler(t)
	mustSend(t, h, ctx, 1, "o, 47k, makan")
	mustCallback(t, h, ctx, 1, cbNote)
	mustSend(t, h, ctx, 1, strings.Repeat("a", 201))
	if got, want := sender.last(t), valid("val.note", nil); got != want {
		t.Fatalf("catatan 201 karakter = %q, want %q", got, want)
	}
	if got := conversation(t, db, ctx, 1); got != StateQuickNote {
		t.Fatalf("state = %q, want %q", got, StateQuickNote)
	}
}

func TestActiveCategoryLimit(t *testing.T) {
	h, sender, db, ctx := newTestHandler(t)
	// The expense seeds are six, so fourteen more reach the cap of twenty.
	for i := 0; i < maxActiveCategories-6; i++ {
		mustCallback(t, h, ctx, 1, cbCatMenuAdd)
		mustCallback(t, h, ctx, 1, cbKindPrefix+string(storage.KindExpense))
		mustSend(t, h, ctx, 1, "Ekstra "+itoa(int64(i)))
	}
	if n, err := db.CountActiveCategories(ctx, 1, storage.KindExpense); err != nil || n != maxActiveCategories {
		t.Fatalf("kategori aktif = %d (err=%v), want %d", n, err, maxActiveCategories)
	}

	sender.reset()
	mustCallback(t, h, ctx, 1, cbCatMenuAdd)
	mustCallback(t, h, ctx, 1, cbKindPrefix+string(storage.KindExpense))
	mustSend(t, h, ctx, 1, "Kelebihan")
	if got, want := sender.last(t), valid("val.cat_max", map[string]string{"max": itoa(maxActiveCategories)}); got != want {
		t.Fatalf("kategori ke-21 = %q, want %q", got, want)
	}
	// The income kind keeps its own budget.
	mustSend(t, h, ctx, 1, "/batal")
	mustCallback(t, h, ctx, 1, cbCatMenuAdd)
	mustCallback(t, h, ctx, 1, cbKindPrefix+string(storage.KindIncome))
	mustSend(t, h, ctx, 1, "Bonus")
	if got, want := sender.last(t), say("category.saved", map[string]string{"category": "Bonus"}); got != want {
		t.Fatalf("tambah kategori pemasukan = %q, want %q", got, want)
	}
}

func TestInactiveCategoryIsHiddenFromThePickerButListed(t *testing.T) {
	h, sender, db, ctx := newTestHandler(t)
	mustSend(t, h, ctx, 1, "/help")
	hiburan := catID(t, db, ctx, 1, storage.KindExpense, "Hiburan")
	mustCallback(t, h, ctx, 1, cbCatOffPrefix+itoa(hiburan))

	sender.reset()
	mustSend(t, h, ctx, 1, persona.BtnExpense)
	mustCallback(t, h, ctx, 1, cbDateToday)
	for _, row := range sender.messages[len(sender.messages)-1].key.Inline {
		for _, button := range row {
			if strings.Contains(button.Text, "Hiburan") {
				t.Fatalf("kategori nonaktif masih ditawarkan: %q", button.Text)
			}
		}
	}

	sender.reset()
	mustSend(t, h, ctx, 1, "/kategori")
	if got := sender.last(t); !strings.Contains(got, "Hiburan "+persona.LabelInactive) {
		t.Fatalf("/kategori tidak menandai kategori nonaktif: %q", got)
	}
}

func TestReminderUsesTheChosenPersona(t *testing.T) {
	h, sender, _, ctx := newTestHandler(t)
	mustSend(t, h, ctx, 1, "/persona set softboy")
	if err := h.SendReminder(ctx, 1); err != nil {
		t.Fatalf("SendReminder: %v", err)
	}
	want := persona.Render(persona.Softboy, "reminder.empty", map[string]string{
		"date": period.FormatDayMonthName(testToday),
	})
	if got := sender.last(t); got != want {
		t.Fatalf("reminder persona softboy = %q, want %q", got, want)
	}
	if got := persona.Render(persona.Netral, "reminder.empty", map[string]string{
		"date": period.FormatDayMonthName(testToday),
	}); got == want {
		t.Fatal("persona softboy harus berbeda dari netral")
	}
}

func TestExportWithExplicitRange(t *testing.T) {
	h, sender, _, ctx := newTestHandler(t)
	addExpense(t, h, sender, ctx, 1, "Makan", "17000")
	mustCallback(t, h, ctx, 1, cbSave)

	sender.reset()
	mustSend(t, h, ctx, 1, "/export 2026-09-01..2026-09-30")
	if len(sender.documents) != 1 {
		t.Fatalf("dokumen = %d, want 1", len(sender.documents))
	}
	doc := sender.documents[0]
	if want := "rekap-20260901-20260930.csv"; doc.filename != want {
		t.Fatalf("filename = %q, want %q", doc.filename, want)
	}
	if doc.caption != doc.filename {
		t.Fatalf("caption = %q, want %q", doc.caption, doc.filename)
	}
	if !strings.Contains(doc.content, "2026-09-27,expense,Makan,,17000") {
		t.Fatalf("csv = %q", doc.content)
	}

	// A range with no rows is a header-only file, not an error.
	sender.reset()
	mustSend(t, h, ctx, 1, "/export 2020-01-01..2020-01-31")
	if len(sender.documents) != 1 || strings.Count(sender.documents[0].content, "\n") != 1 {
		t.Fatalf("csv kosong = %+v", sender.documents)
	}
}

func TestQuickInputMissingAndExtraFields(t *testing.T) {
	h, sender, _, ctx := newTestHandler(t)
	// Fewer than three fields.
	mustSend(t, h, ctx, 1, "o, 47k")
	if got, want := sender.last(t), say("quick.format", nil); got != want {
		t.Fatalf("field kurang = %q, want %q", got, want)
	}
	// A line whose type is neither o nor i is not quick input at all, so it
	// falls through to the idle hint (PRD §4.13).
	mustSend(t, h, ctx, 1, "x, 47k, makan")
	if got, want := sender.last(t), say("idle.hint", nil); got != want {
		t.Fatalf("tipe bukan o/i = %q, want %q", got, want)
	}
	// Commas inside the note are kept.
	mustSend(t, h, ctx, 1, "o, 47k, makan, telur, saos, es krim")
	if got := sender.last(t); !strings.Contains(got, "telur, saos, es krim") {
		t.Fatalf("catatan ber-koma = %q", got)
	}
}

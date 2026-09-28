package report

import (
	"fmt"
	"strings"
	"testing"

	"github.com/dandy/telegram_money_bot/internal/period"
	"github.com/dandy/telegram_money_bot/internal/persona"
)

// sampleRows is the PRD §8 sample data (11 expense transactions in the
// 21 Sep – 20 Okt 2026 period), in insertion order.
func sampleRows() []Tx {
	rows := []Tx{
		{ID: 1, OccurredOn: "2026-09-21", Kind: Expense, Category: "Makan", Amount: 17000, Note: "maksi"},
		{ID: 2, OccurredOn: "2026-09-21", Kind: Expense, Category: "Makan", Amount: 15000, Note: "maklam"},
		{ID: 3, OccurredOn: "2026-09-21", Kind: Expense, Category: "Makan", Amount: 135000, Note: "catering"},
		{ID: 4, OccurredOn: "2026-09-22", Kind: Expense, Category: "Makan", Amount: 5000, Note: "roti sarapan"},
		{ID: 5, OccurredOn: "2026-09-22", Kind: Expense, Category: "Makan", Amount: 13500, Note: "roti tawar"},
		{ID: 6, OccurredOn: "2026-09-23", Kind: Expense, Category: "Rumah Tangga", Amount: 5000, Note: "sunlight"},
		{ID: 7, OccurredOn: "2026-09-24", Kind: Expense, Category: "Transport", Amount: 26000, Note: "bensin"},
		{ID: 8, OccurredOn: "2026-09-26", Kind: Expense, Category: "Rumah Tangga", Amount: 10700, Note: "telur, saos 5 sachet, 2 energen"},
		{ID: 9, OccurredOn: "2026-09-26", Kind: Expense, Category: "Makan", Amount: 165000, Note: "catering"},
		{ID: 10, OccurredOn: "2026-09-26", Kind: Expense, Category: "Makan", Amount: 16000, Note: "maklam"},
		{ID: 11, OccurredOn: "2026-09-26", Kind: Expense, Category: "Rumah Tangga", Amount: 6500, Note: "spons, es krim"},
	}
	return rows
}

var sampleRange = period.Range{Start: "2026-09-21", End: "2026-10-20"}

func TestAggregateSampleData(t *testing.T) {
	re := Aggregate(sampleRows(), sampleRange, "Gaji")
	if re.Days != 30 {
		t.Fatalf("Days = %d, want 30", re.Days)
	}
	if re.ExpenseTotal != 414700 || re.ExpenseCount != 11 {
		t.Fatalf("expense total/count = %d/%d, want 414700/11", re.ExpenseTotal, re.ExpenseCount)
	}
	if re.IncomeTotal != 0 || re.IncomeCount != 0 {
		t.Fatalf("income total/count = %d/%d, want 0/0", re.IncomeTotal, re.IncomeCount)
	}
	want := []struct {
		name  string
		total int64
		count int
		pct   string
	}{
		{"Makan", 366500, 7, "88,4%"},
		{"Transport", 26000, 1, "6,3%"},
		{"Rumah Tangga", 22200, 3, "5,4%"},
	}
	if len(re.Expense) != len(want) {
		t.Fatalf("got %d expense categories, want %d", len(re.Expense), len(want))
	}
	for i, w := range want {
		got := re.Expense[i]
		if got.Name != w.name || got.Total != w.total || got.Count != w.count {
			t.Errorf("expense[%d] = %s/%d/%d, want %s/%d/%d",
				i, got.Name, got.Total, got.Count, w.name, w.total, w.count)
		}
		if p := FormatPct(got.Pct); p != w.pct {
			t.Errorf("expense[%d] pct = %s, want %s", i, p, w.pct)
		}
	}
	if re.DailyAverage != 13823 {
		t.Errorf("DailyAverage = %d, want 13823", re.DailyAverage)
	}
	if re.Last == nil || re.Last.ID != 11 {
		t.Fatalf("Last = %+v, want the spons, es krim row (id 11)", re.Last)
	}
}

func TestAggregateOrderingAndTieBreak(t *testing.T) {
	rows := []Tx{
		{ID: 1, OccurredOn: "2026-09-21", Kind: Expense, Category: "Beta", Amount: 100},
		{ID: 2, OccurredOn: "2026-09-21", Kind: Expense, Category: "Alpha", Amount: 100},
		{ID: 3, OccurredOn: "2026-09-21", Kind: Expense, Category: "Gamma", Amount: 500},
	}
	re := Aggregate(rows, period.Range{Start: "2026-09-21", End: "2026-09-21"}, "Gaji")
	got := []string{re.Expense[0].Name, re.Expense[1].Name, re.Expense[2].Name}
	want := []string{"Gamma", "Alpha", "Beta"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

func TestAggregateEmptyRange(t *testing.T) {
	re := Aggregate(nil, sampleRange, "Gaji")
	if re.ExpenseTotal != 0 || re.IncomeTotal != 0 || len(re.Expense) != 0 {
		t.Fatalf("empty aggregate not empty: %+v", re)
	}
	if re.Last != nil {
		t.Fatalf("Last = %+v, want nil", re.Last)
	}
	if re.DailyAverage != 0 {
		t.Fatalf("DailyAverage = %d, want 0", re.DailyAverage)
	}
}

func TestAggregateBalance(t *testing.T) {
	rows := []Tx{
		{ID: 1, OccurredOn: "2026-09-21", Kind: Income, Category: "Gaji", Amount: 3000000},
		{ID: 2, OccurredOn: "2026-09-22", Kind: Expense, Category: "Makan", Amount: 414700},
	}
	re := Aggregate(rows, sampleRange, "Gaji")
	if got := re.IncomeTotal - re.ExpenseTotal; got != 2585300 {
		t.Fatalf("balance = %d, want 2585300", got)
	}
}

// TestRenderRekapSampleData pins the exact PRD §4.6 sample output byte for
// byte, including column alignment (acceptance criterion #4).
func TestRenderRekapSampleData(t *testing.T) {
	re := Aggregate(sampleRows(), sampleRange, "Gaji")
	msgs := RenderRekap(persona.Netral, re)
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}
	want := strings.Join([]string{
		"📊 Rekap Gaji",
		"21 Sep 2026 – 20 Okt 2026 (30 hari)",
		"",
		"PENGELUARAN — Rp 414.700",
		"  Makan          Rp 366.500  (88,4%)  7 transaksi",
		"  Transport      Rp  26.000  ( 6,3%)  1 transaksi",
		"  Rumah Tangga   Rp  22.200  ( 5,4%)  3 transaksi",
		"",
		"PEMASUKAN — Rp 0",
		"  (belum ada)",
		"",
		"Belum ada pemasukan dicatat di rentang ini.",
		"Rata-rata harian — Rp 13.823",
		"",
		// PRD §8 spells this note `spons, es krim`; the §4.6 sample output
		// capitalises it. Notes are user data and are never reworded
		// (PRD §4.12 rule 1), so the verbatim lowercase form is correct.
		"Transaksi terakhir: 26 Sep — spons, es krim Rp 6.500",
	}, "\n")
	if msgs[0] != want {
		t.Errorf("rekap render mismatch\n--- got ---\n%s\n--- want ---\n%s", msgs[0], want)
	}
}

func TestRenderRekapEmptyRange(t *testing.T) {
	re := Aggregate(nil, sampleRange, "Gaji")
	msgs := RenderRekap(persona.Netral, re)
	want := "📊 Rekap Gaji\n21 Sep 2026 – 20 Okt 2026 (30 hari)\n\nBelum ada transaksi di periode ini."
	if len(msgs) != 1 || msgs[0] != want {
		t.Fatalf("got %q, want %q", msgs, want)
	}
}

func TestRenderRekapWithIncome(t *testing.T) {
	rows := append(sampleRows(), Tx{
		ID: 12, OccurredOn: "2026-09-21", Kind: Income, Category: "Gaji", Amount: 3000000, Note: "gaji september",
	})
	re := Aggregate(rows, sampleRange, "Gaji")
	out := strings.Join(RenderRekap(persona.Netral, re), "\n")
	if !strings.Contains(out, "PEMASUKAN — Rp 3.000.000") {
		t.Errorf("missing income section:\n%s", out)
	}
	if !strings.Contains(out, "  Gaji           Rp 3.000.000  (100,0%)  1 transaksi") {
		t.Errorf("income category line misaligned:\n%s", out)
	}
	if !strings.Contains(out, "SALDO — Rp 2.585.300") {
		t.Errorf("missing balance line:\n%s", out)
	}
	if strings.Contains(out, "Belum ada pemasukan dicatat") {
		t.Errorf("no_income line must not appear when income exists:\n%s", out)
	}
}

// TestRenderRekapSplits verifies the 4096-char rule: no line is cut in half
// and every continuation repeats the header plus the (lanjutan) marker.
func TestRenderRekapSplits(t *testing.T) {
	var rows []Tx
	for i := 0; i < 400; i++ {
		rows = append(rows, Tx{
			ID:         int64(i + 1),
			OccurredOn: "2026-09-21",
			Kind:       Expense,
			Category:   fmt.Sprintf("Kategori %03d %s", i, strings.Repeat("x", 40)),
			Amount:     int64(1000 + i),
			Note:       "catatan",
		})
	}
	re := Aggregate(rows, sampleRange, "Gaji")
	msgs := RenderRekap(persona.Netral, re)
	if len(msgs) < 2 {
		t.Fatalf("expected a split, got %d message(s) of %d chars", len(msgs), len(msgs[0]))
	}
	for i, m := range msgs {
		if len(m) > MaxMessageLen {
			t.Errorf("message %d is %d chars, over the %d limit", i, len(m), MaxMessageLen)
		}
		for _, line := range strings.Split(m, "\n") {
			if line == "" {
				continue
			}
			if !strings.HasPrefix(line, "📊 Rekap Gaji") &&
				!strings.HasPrefix(line, "21 Sep 2026") &&
				!strings.Contains(line, "transaksi") &&
				!strings.HasPrefix(line, "PENGELUARAN") &&
				!strings.HasPrefix(line, "PEMASUKAN") &&
				!strings.HasPrefix(line, "SALDO") &&
				!strings.HasPrefix(line, "Belum ada pemasukan") &&
				!strings.HasPrefix(line, "Rata-rata harian") &&
				!strings.HasPrefix(line, "Transaksi terakhir") &&
				line != "(belum ada)" &&
				!strings.HasPrefix(line, "  ") {
				t.Errorf("message %d has an unexpected partial line %q", i, line)
			}
		}
	}
	if !strings.Contains(msgs[1], "(lanjutan)") {
		t.Errorf("continuation message lacks the marker: %q", msgs[1][:min(80, len(msgs[1]))])
	}
}

func TestRenderReminderEmpty(t *testing.T) {
	d := Reminder{Date: "2026-09-28"}
	got := RenderReminder(persona.Netral, d)
	if got != "Belum ada catatan hari ini (Senin, 28 Sep)." {
		t.Fatalf("got %q", got)
	}
}

// TestRenderReminderSummary pins the PRD §4.8 layout.
func TestRenderReminderSummary(t *testing.T) {
	d := Reminder{
		Date:            "2026-09-26",
		DayExpense:      198200,
		DayExpenseCount: 4,
		DayIncome:       0,
		DayCategories: []CategoryTotal{
			{Name: "Makan", Total: 181000},
			{Name: "Rumah Tangga", Total: 17200},
		},
		PeriodName:    "Gaji",
		PeriodRange:   sampleRange,
		DayNo:         6,
		DayTotal:      30,
		PeriodIncome:  3000000,
		PeriodExpense: 414700,
	}
	got := RenderReminder(persona.Netral, d)
	want := strings.Join([]string{
		"Ringkasan Sabtu, 26 Sep",
		"Pengeluaran  Rp 198.200  (4 transaksi)",
		"Pemasukan    Rp 0",
		"",
		"Periode Gaji (hari ke-6 dari 30)",
		"Pemasukan    Rp 3.000.000",
		"Terpakai     Rp 414.700  (13,8% dari pemasukan)",
		"Sisa         Rp 2.585.300",
		"",
		"Rincian: Makan Rp 181.000, Rumah Tangga Rp 17.200",
	}, "\n")
	if got != want {
		t.Errorf("reminder render mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestRenderReminderNoPeriodIncome(t *testing.T) {
	d := Reminder{
		Date:            "2026-09-26",
		DayExpense:      198200,
		DayExpenseCount: 4,
		PeriodName:      "Gaji",
		DayNo:           6,
		DayTotal:        30,
	}
	got := RenderReminder(persona.Netral, d)
	if !strings.Contains(got, "Belum ada pemasukan dicatat di rentang ini.") {
		t.Fatalf("missing no_income line:\n%s", got)
	}
	for _, bad := range []string{"Terpakai", "Sisa"} {
		if strings.Contains(got, bad) {
			t.Errorf("line %q must not appear without period income:\n%s", bad, got)
		}
	}
}

// TestNumbersIdenticalAcrossPersonas is acceptance criterion #21: switching
// persona may change the wrapper sentence but never a number.
func TestNumbersIdenticalAcrossPersonas(t *testing.T) {
	re := Aggregate(sampleRows(), sampleRange, "Gaji")
	for _, id := range persona.IDs() {
		out := strings.Join(RenderRekap(id, re), "\n")
		for _, want := range []string{
			"Rp 414.700", "Rp 366.500", "Rp  26.000", "Rp  22.200",
			"88,4%", "6,3%", "5,4%", "Rp 13.823", "26 Sep", "Rp 6.500",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("persona %s lost the value %q:\n%s", id, want, out)
			}
		}
		d := Reminder{
			Date:            "2026-09-26",
			DayExpense:      198200,
			DayExpenseCount: 4,
			PeriodName:      "Gaji",
			DayNo:           6,
			DayTotal:        30,
			PeriodIncome:    3000000,
			PeriodExpense:   414700,
		}
		rem := RenderReminder(id, d)
		for _, want := range []string{"Rp 198.200", "Rp 3.000.000", "Rp 414.700", "Rp 2.585.300", "13,8%", "Sabtu, 26 Sep"} {
			if !strings.Contains(rem, want) {
				t.Errorf("persona %s reminder lost the value %q:\n%s", id, want, rem)
			}
		}
	}
}

func TestRenderList(t *testing.T) {
	rows := []Tx{
		{ID: 11, OccurredOn: "2026-09-26", Kind: Expense, Category: "Rumah Tangga", Amount: 6500, Note: "spons, es krim"},
		{ID: 10, OccurredOn: "2026-09-26", Kind: Expense, Category: "Makan", Amount: 16000},
	}
	got := RenderList(persona.Netral, List{
		Title: persona.TitleRecent, Rows: rows, Total: 22500, ShowDate: true,
	})
	want := strings.Join([]string{
		"10 transaksi terakhir (2 transaksi, total Rp 22.500)",
		"• 26 Sep Rumah Tangga Rp 6.500 — spons, es krim",
		"• 26 Sep Makan Rp 16.000",
	}, "\n")
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestFormatPct(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "0,0%"},
		{88.44, "88,4%"},
		{6.25, "6,3%"},
		{100, "100,0%"},
		{13.75, "13,8%"},
	}
	for _, c := range cases {
		if got := FormatPct(c.in); got != c.want {
			t.Errorf("FormatPct(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

package persona

// labels.go holds every fixed user-facing string that is NOT part of the
// persona catalog: button captions, field labels and body words. PRD §4.12
// rule 2 requires all user-facing text to live in this package, and PRD §4.2 /
// §4.8 require these particular strings to be identical across personas
// ("label field ... dan nilainya tidak pernah diubah persona").
const (
	// Reply keyboard (PRD §4.2).
	BtnExpense  = "➕ Pengeluaran"
	BtnIncome   = "➕ Pemasukan"
	BtnRekap    = "📊 Rekap"
	BtnSettings = "⚙️ Pengaturan"

	// Inline date keyboard (PRD §4.2 step 2).
	BtnToday     = "Hari ini"
	BtnYesterday = "Kemarin"
	BtnPickDate  = "Pilih tanggal"

	// Confirmation card buttons (PRD §4.2 step 5, §4.9).
	BtnSave   = "✅ Simpan"
	BtnNote   = "✏️ Catatan"
	BtnCancel = "❌ Batal"
	BtnBack   = "⬅️ Kembali"

	// Edit field menu (PRD §4.9).
	BtnEditDate     = "📅 Tanggal"
	BtnEditCategory = "🏷 Kategori"
	BtnEditAmount   = "💰 Nominal"
	BtnEditNote     = "📝 Catatan"
	BtnEditKind     = "↔️ Tipe"
	BtnEditDone     = "⬅️ Selesai"

	// Per-row buttons in /hari and /terakhir (PRD §4.9).
	BtnEdit   = "✏️"
	BtnDelete = "🗑"

	// Category management buttons (PRD §4.4).
	BtnCatAdd    = "➕ Tambah"
	BtnCatRename = "✏️ Ganti nama"
	BtnCatOff    = "🗑 Nonaktifkan"
)

// Field labels and body words. These never change with the persona.
const (
	LabelDate     = "Tanggal"
	LabelCategory = "Kategori"
	LabelAmount   = "Nominal"
	LabelNote     = "Catatan"

	LabelExpense = "PENGELUARAN"
	LabelIncome  = "PEMASUKAN"
	LabelSaldo   = "SALDO"

	LabelEmptySection = "(belum ada)"
	LabelAverage      = "Rata-rata harian"
	LabelLastTx       = "Transaksi terakhir"
	LabelDetail       = "Rincian"
	LabelPeriod       = "Periode"

	LabelDayExpense = "Pengeluaran"
	LabelDayIncome  = "Pemasukan"
	LabelUsed       = "Terpakai"
	LabelLeft       = "Sisa"

	LabelDayUnit    = "hari"
	LabelTxUnit     = "transaksi"
	LabelFromIncome = "dari pemasukan"

	LabelRekapPrefix  = "📊"
	LabelContinuation = "(lanjutan)"
	LabelNoNote       = "-"
	LabelSep          = " — "

	LabelItemPrefix    = "• "
	LabelEffectiveFrom = "mulai"
	// LabelEffectiveFromStart replaces the `mulai <tanggal>` clause of the
	// /periode list for the initial period, whose effective_from is an
	// internal sentinel rather than a schedule change (PRD §4.5).
	LabelEffectiveFromStart = "sejak awal"
	// LabelDayRange joins start_day and end_day in the /periode list
	// (PRD §4.5: `21–20`), an en dash without surrounding spaces.
	LabelDayRange = "–"
	LabelInactive = "(nonaktif)"

	// Transaction kind words, used as the {kind} token value (PRD §4.12:
	// `{kind}` = `pengeluaran`/`pemasukan`). Lower case, unlike LabelExpense /
	// LabelIncome which are the PENGELUARAN / PEMASUKAN section headings.
	KindExpenseWord = "pengeluaran"
	KindIncomeWord  = "pemasukan"
	KindTxWord      = "transaksi"

	// Edit field words: the {kind} token of tx.edit.confirm.header is the
	// CHANGED FIELD (PRD §4.9: `Ubah nominal?`), not the transaction kind.
	FieldWordDate     = "tanggal"
	FieldWordCategory = "kategori"
	FieldWordAmount   = "nominal"
	FieldWordNote     = "catatan"
	FieldWordKind     = "tipe"

	// LabelDateFormats fills the {valid} token: the accepted date formats,
	// named verbatim as PRD §4.2 and §4.12 require (PRD:440).
	LabelDateFormats = "DD/MM atau DD-MM-YYYY"

	// Range names, used as the {period} token of rekap.header when the recap
	// is not the active period (PRD §4.6). The resolved dates always follow on
	// the next line, so the name stays generic.
	LabelRangeToday     = "hari ini"
	LabelRangeYesterday = "kemarin"
	LabelRangeWeek      = "minggu ini"
	LabelRangeMonth     = "bulan ini"
	LabelRangeCustom    = "rentang khusus"

	// LabelDefaultPeriodName matches the seeded period name in
	// internal/storage/users.go, used when /periode set has no name argument
	// and the user has no earlier period to inherit one from.
	LabelDefaultPeriodName = "Gaji"

	// LabelReminderOff fills the {time} token of settings.summary while the
	// reminder is disabled, so /settings never claims a reminder that is off.
	LabelReminderOff = "off"

	// LabelArrow separates the old and the new value in an edit card
	// (PRD §4.9: `Rp 17.000 → Rp 21.000`).
	LabelArrow = " → "
	// List titles supplied by the handler through the {title} token
	// (PRD §4.12 note on list.header).
	TitleToday  = "Transaksi hari ini"
	TitleRecent = "10 transaksi terakhir"
)

// FuzzySource marks a fuzzy category match on the confirmation card
// (PRD §10: the guess is shown as `(dari "makna")` so the user sees what was
// matched before pressing ✅ Simpan). The text lives here because PRD §4.12
// rule 2 keeps every user-facing string out of package bot.
func FuzzySource(input string) string {
	return "(dari \"" + input + "\")"
}

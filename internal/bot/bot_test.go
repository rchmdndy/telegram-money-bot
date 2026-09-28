package bot

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rchmdndy/telegram-money-bot/internal/money"
	"github.com/rchmdndy/telegram-money-bot/internal/period"
	"github.com/rchmdndy/telegram-money-bot/internal/persona"
	"github.com/rchmdndy/telegram-money-bot/internal/report"
	"github.com/rchmdndy/telegram-money-bot/internal/storage"
)

// testClock is the frozen "now" of every bot test: Sunday, 27 Sep 2026, 10:00
// UTC, which falls in the default 21→20 period.
var testClock = time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)

const testToday = "2026-09-27"

type sentMessage struct {
	chatID int64
	text   string
	key    Keyboard
}

type sentDocument struct {
	chatID   int64
	filename string
	content  string
	caption  string
}

// mockSender records everything the handler sends (PRD §6: no test may touch
// the Telegram API).
type mockSender struct {
	messages  []sentMessage
	documents []sentDocument
	answered  []string
	edited    []int
}

func (m *mockSender) SendMessage(_ context.Context, chatID int64, text string, k Keyboard) (int, error) {
	m.messages = append(m.messages, sentMessage{chatID: chatID, text: text, key: k})
	return len(m.messages), nil
}

func (m *mockSender) EditMessageKeyboard(_ context.Context, _ int64, messageID int, _ Keyboard) error {
	m.edited = append(m.edited, messageID)
	return nil
}

func (m *mockSender) SendDocument(_ context.Context, chatID int64, filename string, content []byte, caption string) error {
	m.documents = append(m.documents, sentDocument{chatID: chatID, filename: filename, content: string(content), caption: caption})
	return nil
}

func (m *mockSender) AnswerCallback(_ context.Context, callbackID string) error {
	m.answered = append(m.answered, callbackID)
	return nil
}

// last is the most recent message text, which is what almost every assertion
// checks.
func (m *mockSender) last(t *testing.T) string {
	t.Helper()
	if len(m.messages) == 0 {
		t.Fatal("tidak ada pesan yang dikirim")
	}
	return m.messages[len(m.messages)-1].text
}

func (m *mockSender) reset() {
	m.messages = nil
	m.documents = nil
	m.answered = nil
	m.edited = nil
}

func newTestHandler(t *testing.T) (*Handler, *mockSender, *storage.DB, context.Context) {
	t.Helper()
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	sender := &mockSender{}
	h := New(db, sender, time.UTC, slog.New(slog.NewTextHandler(io.Discard, nil)))
	h.SetClock(func() time.Time { return testClock })
	return h, sender, db, ctx
}

func catID(t *testing.T, db *storage.DB, ctx context.Context, userID int64, kind storage.Kind, name string) int64 {
	t.Helper()
	cats, err := db.ListCategories(ctx, userID, kind, false)
	if err != nil {
		t.Fatalf("ListCategories: %v", err)
	}
	for _, c := range cats {
		if c.Name == name {
			return c.ID
		}
	}
	t.Fatalf("kategori %q tidak ditemukan", name)
	return 0
}

// conversation returns the stored state, or "" when no conversation runs.
func conversation(t *testing.T, db *storage.DB, ctx context.Context, userID int64) string {
	t.Helper()
	c, err := db.GetConversation(ctx, userID)
	if errors.Is(err, storage.ErrNotFound) {
		return ""
	}
	if err != nil {
		t.Fatalf("GetConversation: %v", err)
	}
	return c.State
}

// say renders a catalog key in the persona every test user has by default.
func say(key string, vars map[string]string) string {
	return persona.Render(persona.Default(), key, vars)
}

func valid(key string, vars map[string]string) string {
	return persona.Validation(persona.Default(), key, vars)
}

func prompt(key string, vars map[string]string) string {
	return persona.Prompt(persona.Default(), key, vars)
}

// addExpense walks the full ➕ Pengeluaran flow and stops at the confirmation
// card, so tests that only care about the card do not repeat the four steps.
func addExpense(t *testing.T, h *Handler, sender *mockSender, ctx context.Context, userID int64, category string, amount string) {
	t.Helper()
	mustSend(t, h, ctx, userID, persona.BtnExpense)
	mustCallback(t, h, ctx, userID, cbDateToday)
	mustCallback(t, h, ctx, userID, cbCatPickPrefix+itoa(catID(t, h.db, ctx, userID, storage.KindExpense, category)))
	mustSend(t, h, ctx, userID, amount)
}

func mustSend(t *testing.T, h *Handler, ctx context.Context, userID int64, text string) {
	t.Helper()
	if err := h.HandleMessage(ctx, userID, userID, text); err != nil {
		t.Fatalf("HandleMessage(%q): %v", text, err)
	}
}

func mustCallback(t *testing.T, h *Handler, ctx context.Context, userID int64, data string) {
	t.Helper()
	if err := h.HandleCallback(ctx, userID, userID, 42, "cb-"+data, data); err != nil {
		t.Fatalf("HandleCallback(%q): %v", data, err)
	}
}

// itoa renders an ID the way the callback payloads carry it.
func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func TestAddFlowSavesTransaction(t *testing.T) {
	h, sender, db, ctx := newTestHandler(t)
	addExpense(t, h, sender, ctx, 1, "Makan", "17000")

	want := report.RenderCard(report.Card{
		Header:   say("tx.confirm.header", map[string]string{"kind": persona.KindExpenseWord}),
		Date:     period.FormatDayDate(testToday),
		Category: "Makan",
		Amount:   money.Format(17000),
		Note:     persona.LabelNoNote,
	})
	if got := sender.last(t); got != want {
		t.Fatalf("kartu konfirmasi =\n%s\nwant\n%s", got, want)
	}

	mustCallback(t, h, ctx, 1, cbSave)
	if got, want := sender.last(t), say("tx.saved", map[string]string{"category": "Makan", "amount": money.Format(17000)}); got != want {
		t.Fatalf("balasan simpan = %q, want %q", got, want)
	}
	rows, err := db.ListTransactionsByDate(ctx, 1, testToday)
	if err != nil {
		t.Fatalf("ListTransactionsByDate: %v", err)
	}
	if len(rows) != 1 || rows[0].Amount != 17000 || rows[0].Kind != storage.KindExpense {
		t.Fatalf("baris tersimpan = %+v", rows)
	}
	if got := conversation(t, db, ctx, 1); got != "" {
		t.Fatalf("percakapan harus selesai, masih %q", got)
	}
}

func TestAddFlowTotalIsPerCategoryPerDay(t *testing.T) {
	h, sender, db, ctx := newTestHandler(t)
	addExpense(t, h, sender, ctx, 1, "Makan", "17000")
	mustCallback(t, h, ctx, 1, cbSave)

	addExpense(t, h, sender, ctx, 1, "Makan", "15000")
	mustCallback(t, h, ctx, 1, cbSave)
	want := say("tx.saved", map[string]string{"category": "Makan", "amount": money.Format(32000)})
	if got := sender.last(t); got != want {
		t.Fatalf("balasan simpan kedua = %q, want %q", got, want)
	}

	// A different category keeps its own total.
	addExpense(t, h, sender, ctx, 1, "Transport", "26000")
	mustCallback(t, h, ctx, 1, cbSave)
	want = say("tx.saved", map[string]string{"category": "Transport", "amount": money.Format(26000)})
	if got := sender.last(t); got != want {
		t.Fatalf("balasan simpan kategori lain = %q, want %q", got, want)
	}
	_ = db
}

func TestAddFlowManualDateAndInvalidAmount(t *testing.T) {
	h, sender, _, ctx := newTestHandler(t)
	mustSend(t, h, ctx, 1, persona.BtnIncome)
	if got, want := sender.last(t), prompt("tx.prompt.date", map[string]string{"valid": persona.LabelDateFormats}); got != want {
		t.Fatalf("prompt tanggal = %q, want %q", got, want)
	}
	mustSend(t, h, ctx, 1, "26/09")
	if got, want := sender.last(t), say("category.list", nil); got != want {
		t.Fatalf("setelah tanggal manual = %q, want %q", got, want)
	}

	mustCallback(t, h, ctx, 1, cbCatPickPrefix+itoa(catID(t, h.db, ctx, 1, storage.KindIncome, "Gaji")))
	mustSend(t, h, ctx, 1, "abc")
	want := valid("val.amount", map[string]string{"limit": money.Format(money.MaxAmount)})
	if got := sender.last(t); got != want {
		t.Fatalf("nominal salah = %q, want %q", got, want)
	}
	// The state must be unchanged, so a valid amount still completes.
	if got := conversation(t, h.db, ctx, 1); got != StateTxAmount {
		t.Fatalf("state = %q, want %q", got, StateTxAmount)
	}
	mustSend(t, h, ctx, 1, "2.850.000")
	mustCallback(t, h, ctx, 1, cbSave)

	rows, err := h.db.ListRecentTransactions(ctx, 1, 5)
	if err != nil {
		t.Fatalf("ListRecentTransactions: %v", err)
	}
	if len(rows) != 1 || rows[0].OccurredOn != "2026-09-26" || rows[0].Amount != 2850000 || rows[0].Kind != storage.KindIncome {
		t.Fatalf("baris = %+v", rows)
	}
}

func TestAddFlowBackButtonCancels(t *testing.T) {
	h, sender, db, ctx := newTestHandler(t)
	mustSend(t, h, ctx, 1, persona.BtnExpense)
	mustCallback(t, h, ctx, 1, cbBack)
	if got, want := sender.last(t), say("tx.cancelled", nil); got != want {
		t.Fatalf("batal = %q, want %q", got, want)
	}
	if got := conversation(t, db, ctx, 1); got != "" {
		t.Fatalf("percakapan harus berakhir, masih %q", got)
	}
}

func TestBatalFromEveryState(t *testing.T) {
	for _, state := range States() {
		t.Run(state, func(t *testing.T) {
			h, sender, db, ctx := newTestHandler(t)
			if err := h.ensureUser(ctx, 1); err != nil {
				t.Fatalf("ensureUser: %v", err)
			}
			if err := h.saveConversation(ctx, 1, state, payload{Cur: &txData{Kind: string(storage.KindExpense)}}); err != nil {
				t.Fatalf("saveConversation: %v", err)
			}
			mustSend(t, h, ctx, 1, "/batal")
			if got, want := sender.last(t), say("tx.cancelled", nil); got != want {
				t.Fatalf("/batal dari %s = %q, want %q", state, got, want)
			}
			if got := conversation(t, db, ctx, 1); got != "" {
				t.Fatalf("state %s masih ada", got)
			}
		})
	}
}

func TestBatalWithoutConversation(t *testing.T) {
	h, sender, _, ctx := newTestHandler(t)
	mustSend(t, h, ctx, 1, "/batal")
	if got, want := sender.last(t), valid("val.no_conv", nil); got != want {
		t.Fatalf("batal tanpa percakapan = %q, want %q", got, want)
	}
}

func TestStateSurvivesRestart(t *testing.T) {
	h, sender, db, ctx := newTestHandler(t)
	mustSend(t, h, ctx, 1, persona.BtnExpense)
	mustCallback(t, h, ctx, 1, cbDateToday)
	mustCallback(t, h, ctx, 1, cbCatPickPrefix+itoa(catID(t, db, ctx, 1, storage.KindExpense, "Makan")))

	// A brand-new Handler on the same database is what a process restart is.
	restarted := New(db, sender, time.UTC, slog.New(slog.NewTextHandler(io.Discard, nil)))
	restarted.SetClock(func() time.Time { return testClock })
	mustSend(t, restarted, ctx, 1, "17000")
	mustCallback(t, restarted, ctx, 1, cbSave)

	rows, err := db.ListTransactionsByDate(ctx, 1, testToday)
	if err != nil {
		t.Fatalf("ListTransactionsByDate: %v", err)
	}
	if len(rows) != 1 || rows[0].Amount != 17000 {
		t.Fatalf("baris setelah restart = %+v", rows)
	}
}

func TestQuickInputGate(t *testing.T) {
	h, sender, db, ctx := newTestHandler(t)

	// Free text while idle is not quick input.
	mustSend(t, h, ctx, 1, "halo bot")
	if got, want := sender.last(t), say("idle.hint", nil); got != want {
		t.Fatalf("teks bebas = %q, want %q", got, want)
	}
	// A line that only looks like quick input is not: `oke` is not a type.
	mustSend(t, h, ctx, 1, "oke, 47k, makan")
	if got, want := sender.last(t), say("idle.hint", nil); got != want {
		t.Fatalf("oke = %q, want %q", got, want)
	}

	// With a conversation running, `o,` is ordinary state input.
	if err := h.saveConversation(ctx, 1, StateTxDate, payload{Cur: &txData{Kind: string(storage.KindExpense)}}); err != nil {
		t.Fatalf("saveConversation: %v", err)
	}
	mustSend(t, h, ctx, 1, "o, 47k, makan, makan malam")
	want := valid("val.date", map[string]string{"valid": persona.LabelDateFormats})
	if got := sender.last(t); got != want {
		t.Fatalf("quick saat percakapan aktif = %q, want %q", got, want)
	}
	if got := conversation(t, h.db, ctx, 1); got != StateTxDate {
		t.Fatalf("state = %q, want %q", got, StateTxDate)
	}
	if err := h.dropConversation(ctx, 1); err != nil {
		t.Fatalf("dropConversation: %v", err)
	}

	// A malformed quick input is an error, never the idle hint.
	mustSend(t, h, ctx, 1, "o, abc, makan, x")
	want = valid("val.amount", map[string]string{"limit": money.Format(money.MaxAmount)})
	if got := sender.last(t); got != want {
		t.Fatalf("nominal quick salah = %q, want %q", got, want)
	}
	_ = db
}

func TestQuickInputCategoryErrorNamesTheRightKind(t *testing.T) {
	h, sender, _, ctx := newTestHandler(t)
	// `i` asks for income, so the list must be the income categories even
	// though "makan" is an expense category.
	mustSend(t, h, ctx, 1, "i, 47k, makan, x")
	want := say("quick.category", map[string]string{
		"input": "makan",
		"list":  "Gaji, Lain-lain",
	})
	if got := sender.last(t); got != want {
		t.Fatalf("quick.category = %q, want %q", got, want)
	}
}

func TestQuickFuzzyMarksTheCardAndNeverSavesEarly(t *testing.T) {
	h, sender, db, ctx := newTestHandler(t)
	mustSend(t, h, ctx, 1, "o, 47k, makna, makan malam")

	want := report.RenderCard(report.Card{
		Header:   say("tx.confirm.header", map[string]string{"kind": persona.KindExpenseWord}),
		Date:     period.FormatDayDate(testToday),
		Category: "Makan " + persona.FuzzySource("makna"),
		Amount:   money.Format(47000),
		Note:     "makan malam",
	})
	if got := sender.last(t); got != want {
		t.Fatalf("kartu fuzzy =\n%s\nwant\n%s", got, want)
	}
	if n, err := db.CountTransactionsInRange(ctx, 1, "2026-01-01", "2026-12-31"); err != nil || n != 0 {
		t.Fatalf("fuzzy menyimpan lebih awal: n=%d err=%v", n, err)
	}
	mustCallback(t, h, ctx, 1, cbCancel)
	if n, err := db.CountTransactionsInRange(ctx, 1, "2026-01-01", "2026-12-31"); err != nil || n != 0 {
		t.Fatalf("batal tetap menyimpan: n=%d err=%v", n, err)
	}
}

func TestQuickExactMatchSavesOnConfirmation(t *testing.T) {
	h, _, db, ctx := newTestHandler(t)
	mustSend(t, h, ctx, 1, "i, 2.850.000, gaji, gaji maganghub periode september")
	mustCallback(t, h, ctx, 1, cbSave)

	rows, err := db.ListRecentTransactions(ctx, 1, 5)
	if err != nil {
		t.Fatalf("ListRecentTransactions: %v", err)
	}
	if len(rows) != 1 || rows[0].Kind != storage.KindIncome || rows[0].Amount != 2850000 {
		t.Fatalf("baris = %+v", rows)
	}
	if rows[0].Note != "gaji maganghub periode september" {
		t.Fatalf("catatan = %q", rows[0].Note)
	}
}

func TestMultiUserIsolation(t *testing.T) {
	h, sender, db, ctx := newTestHandler(t)
	addExpense(t, h, sender, ctx, 1, "Makan", "17000")
	mustCallback(t, h, ctx, 1, cbSave)

	rows, err := db.ListRecentTransactions(ctx, 1, 5)
	if err != nil {
		t.Fatalf("ListRecentTransactions: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("user 1 harus punya 1 baris, punya %d", len(rows))
	}
	victimID := rows[0].ID

	sender.reset()
	mustSend(t, h, ctx, 2, "/hari")
	if got, want := sender.last(t), report.RenderList(persona.Default(), report.List{Title: persona.TitleToday, Total: 0}); got != want {
		t.Fatalf("/hari user 2 = %q, want %q", got, want)
	}
	mustSend(t, h, ctx, 2, "/terakhir")
	if got := sender.last(t); strings.Contains(got, "Makan") {
		t.Fatalf("/terakhir user 2 membocorkan baris user 1: %q", got)
	}
	mustSend(t, h, ctx, 2, "/settings")
	if got := sender.last(t); strings.Contains(got, "Rp 17.000") {
		t.Fatalf("/settings user 2 membocorkan total user 1: %q", got)
	}

	// A callback carrying user 1's transaction ID must be rejected outright.
	sender.reset()
	mustCallback(t, h, ctx, 2, cbTxDeletePrefix+itoa(victimID))
	if got, want := sender.last(t), valid("val.tx_missing", nil); got != want {
		t.Fatalf("hapus lintas user = %q, want %q", got, want)
	}
	if n, err := db.CountTransactionsInRange(ctx, 1, "2026-01-01", "2026-12-31"); err != nil || n != 1 {
		t.Fatalf("baris user 1 terhapus: n=%d err=%v", n, err)
	}
	mustCallback(t, h, ctx, 2, cbTxEditPrefix+itoa(victimID))
	if got, want := sender.last(t), valid("val.tx_missing", nil); got != want {
		t.Fatalf("edit lintas user = %q, want %q", got, want)
	}
}

func TestEditChangesOnlyTheChosenField(t *testing.T) {
	h, sender, db, ctx := newTestHandler(t)
	addExpense(t, h, sender, ctx, 1, "Makan", "17000")
	mustCallback(t, h, ctx, 1, cbSave)

	rows, err := db.ListRecentTransactions(ctx, 1, 1)
	if err != nil {
		t.Fatalf("ListRecentTransactions: %v", err)
	}
	before := rows[0]

	mustCallback(t, h, ctx, 1, cbTxEditPrefix+itoa(before.ID))
	if got := sender.last(t); !strings.Contains(got, money.Format(17000)) {
		t.Fatalf("kartu edit = %q", got)
	}
	mustCallback(t, h, ctx, 1, cbEditPrefix+itoa(before.ID)+":"+fieldAmount)
	mustSend(t, h, ctx, 1, "21k")

	wantCard := report.RenderCard(report.Card{
		Header:   say("tx.edit.confirm.header", map[string]string{"kind": persona.FieldWordAmount}),
		Date:     period.FormatDayDate(before.OccurredOn),
		Category: before.CategoryName,
		Amount:   money.Format(17000) + persona.LabelArrow + money.Format(21000),
		Note:     noteOrDash(before.Note),
	})
	if got := sender.last(t); got != wantCard {
		t.Fatalf("kartu edit nominal =\n%s\nwant\n%s", got, wantCard)
	}
	mustCallback(t, h, ctx, 1, cbSave)

	after, err := db.GetTransaction(ctx, 1, before.ID)
	if err != nil {
		t.Fatalf("GetTransaction: %v", err)
	}
	if after.Amount != 21000 {
		t.Fatalf("amount = %d, want 21000", after.Amount)
	}
	if after.OccurredOn != before.OccurredOn || after.CategoryID != before.CategoryID || after.Note != before.Note {
		t.Fatalf("kolom lain berubah: %+v -> %+v", before, after)
	}
	if after.CreatedAt != before.CreatedAt {
		t.Fatalf("created_at berubah: %s -> %s", before.CreatedAt, after.CreatedAt)
	}
	// An edit must never insert a second row. (updated_at moving is asserted at
	// the storage layer, which can advance its clock past the RFC3339 second
	// boundary this Handler's real clock has.)
	if n, err := db.CountTransactionsInRange(ctx, 1, "2026-01-01", "2026-12-31"); err != nil || n != 1 {
		t.Fatalf("jumlah baris = %d (err=%v), want 1", n, err)
	}
}

func TestEditKindChangeRequiresANewCategory(t *testing.T) {
	h, sender, db, ctx := newTestHandler(t)
	addExpense(t, h, sender, ctx, 1, "Makan", "17000")
	mustCallback(t, h, ctx, 1, cbSave)

	rows, err := db.ListRecentTransactions(ctx, 1, 1)
	if err != nil {
		t.Fatalf("ListRecentTransactions: %v", err)
	}
	id := rows[0].ID

	mustCallback(t, h, ctx, 1, cbTxEditPrefix+itoa(id))
	mustCallback(t, h, ctx, 1, cbEditPrefix+itoa(id)+":"+fieldKind)
	if got, want := sender.last(t), say("category.list", nil); got != want {
		t.Fatalf("pilih tipe = %q, want %q", got, want)
	}
	// Choosing income must open the income picker, not save anything.
	mustCallback(t, h, ctx, 1, cbKindPrefix+string(storage.KindIncome))
	row, err := db.GetTransaction(ctx, 1, id)
	if err != nil {
		t.Fatalf("GetTransaction: %v", err)
	}
	if row.Kind != storage.KindExpense {
		t.Fatalf("jenis tersimpan sebelum kategori dipilih: %q", row.Kind)
	}
	mustCallback(t, h, ctx, 1, cbCatPickPrefix+itoa(catID(t, h.db, ctx, 1, storage.KindIncome, "Gaji")))
	if got := sender.last(t); !strings.Contains(got, "Gaji") {
		t.Fatalf("kartu setelah ganti jenis = %q", got)
	}
	mustCallback(t, h, ctx, 1, cbSave)

	row, err = db.GetTransaction(ctx, 1, id)
	if err != nil {
		t.Fatalf("GetTransaction: %v", err)
	}
	if row.Kind != storage.KindIncome || row.CategoryName != "Gaji" {
		t.Fatalf("baris setelah ganti jenis = %+v", row.Transaction)
	}
	if n, err := db.CountTransactionsInRange(ctx, 1, "2026-01-01", "2026-12-31"); err != nil || n != 1 {
		t.Fatalf("jumlah baris = %d (err=%v), want 1", n, err)
	}
}

func TestEditCategoryChangeReturnsToTheCard(t *testing.T) {
	h, sender, db, ctx := newTestHandler(t)
	addExpense(t, h, sender, ctx, 1, "Makan", "17000")
	mustCallback(t, h, ctx, 1, cbSave)

	rows, _ := db.ListRecentTransactions(ctx, 1, 1)
	id := rows[0].ID

	mustCallback(t, h, ctx, 1, cbTxEditPrefix+itoa(id))
	mustCallback(t, h, ctx, 1, cbEditPrefix+itoa(id)+":"+fieldCategory)
	mustCallback(t, h, ctx, 1, cbCatPickPrefix+itoa(catID(t, h.db, ctx, 1, storage.KindExpense, "Transport")))

	want := report.RenderCard(report.Card{
		Header:   say("tx.edit.confirm.header", map[string]string{"kind": persona.FieldWordCategory}),
		Date:     period.FormatDayDate(testToday),
		Category: "Makan" + persona.LabelArrow + "Transport",
		Amount:   money.Format(17000),
		Note:     persona.LabelNoNote,
	})
	if got := sender.last(t); got != want {
		t.Fatalf("kartu ganti kategori =\n%s\nwant\n%s", got, want)
	}
	mustCallback(t, h, ctx, 1, cbSave)
	row, err := db.GetTransaction(ctx, 1, id)
	if err != nil {
		t.Fatalf("GetTransaction: %v", err)
	}
	if row.CategoryName != "Transport" {
		t.Fatalf("kategori = %q, want Transport", row.CategoryName)
	}
}

func TestEditDoneEndsWithoutSaving(t *testing.T) {
	h, sender, db, ctx := newTestHandler(t)
	addExpense(t, h, sender, ctx, 1, "Makan", "17000")
	mustCallback(t, h, ctx, 1, cbSave)
	rows, _ := db.ListRecentTransactions(ctx, 1, 1)
	id := rows[0].ID

	mustCallback(t, h, ctx, 1, cbTxEditPrefix+itoa(id))
	mustCallback(t, h, ctx, 1, cbEditPrefix+itoa(id)+":"+fieldAmount)
	mustSend(t, h, ctx, 1, "99k")
	mustCallback(t, h, ctx, 1, cbEditPrefix+itoa(id)+":"+fieldDone)

	if got, want := sender.last(t), say("tx.cancelled", nil); got != want {
		t.Fatalf("selesai = %q, want %q", got, want)
	}
	row, err := db.GetTransaction(ctx, 1, id)
	if err != nil {
		t.Fatalf("GetTransaction: %v", err)
	}
	if row.Amount != 17000 {
		t.Fatalf("amount = %d, want 17000", row.Amount)
	}
	if got := conversation(t, db, ctx, 1); got != "" {
		t.Fatalf("percakapan masih %q", got)
	}
}

func TestDeleteTransaction(t *testing.T) {
	h, sender, db, ctx := newTestHandler(t)
	addExpense(t, h, sender, ctx, 1, "Makan", "17000")
	mustCallback(t, h, ctx, 1, cbSave)
	rows, _ := db.ListRecentTransactions(ctx, 1, 1)
	id := rows[0].ID

	sender.reset()
	mustCallback(t, h, ctx, 1, cbTxDeletePrefix+itoa(id))
	want := say("tx.deleted", map[string]string{"category": "Makan", "amount": money.Format(17000)})
	if got := sender.last(t); got != want {
		t.Fatalf("hapus = %q, want %q", got, want)
	}
	if _, err := db.GetTransaction(ctx, 1, id); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("baris masih ada: %v", err)
	}
}

func TestHariListsTodayWithButtons(t *testing.T) {
	h, sender, db, ctx := newTestHandler(t)
	addExpense(t, h, sender, ctx, 1, "Makan", "17000")
	mustCallback(t, h, ctx, 1, cbSave)
	rows, _ := db.ListRecentTransactions(ctx, 1, 1)

	sender.reset()
	mustSend(t, h, ctx, 1, "/hari")
	msg := sender.messages[len(sender.messages)-1]
	want := report.RenderList(persona.Default(), report.List{
		Title: persona.TitleToday,
		Rows:  []report.Tx{{ID: rows[0].ID, OccurredOn: testToday, Kind: "expense", Category: "Makan", Amount: 17000}},
		Total: 17000,
	})
	if msg.text != want {
		t.Fatalf("/hari =\n%s\nwant\n%s", msg.text, want)
	}
	if len(msg.key.Inline) != 1 || msg.key.Inline[0][0].Data != cbTxEditPrefix+itoa(rows[0].ID) {
		t.Fatalf("keyboard /hari = %+v", msg.key.Inline)
	}
}

func TestKategoriAddRenameAndDeactivate(t *testing.T) {
	h, sender, db, ctx := newTestHandler(t)
	mustSend(t, h, ctx, 1, "/kategori")
	mustCallback(t, h, ctx, 1, cbCatMenuAdd)
	if got, want := sender.last(t), prompt("prompt.cat_kind", nil); got != want {
		t.Fatalf("tambah kategori = %q, want %q", got, want)
	}
	mustCallback(t, h, ctx, 1, cbKindPrefix+string(storage.KindExpense))
	mustSend(t, h, ctx, 1, "Kopi")
	if got, want := sender.last(t), say("category.saved", map[string]string{"category": "Kopi"}); got != want {
		t.Fatalf("kategori tersimpan = %q, want %q", got, want)
	}
	kopi := catID(t, db, ctx, 1, storage.KindExpense, "Kopi")

	// A duplicate name is rejected without touching the state.
	mustCallback(t, h, ctx, 1, cbCatMenuAdd)
	mustCallback(t, h, ctx, 1, cbKindPrefix+string(storage.KindExpense))
	mustSend(t, h, ctx, 1, "kopi")
	if got, want := sender.last(t), valid("val.cat_dup", map[string]string{"category": "kopi"}); got != want {
		t.Fatalf("nama duplikat = %q, want %q", got, want)
	}
	if got := conversation(t, db, ctx, 1); got != StateCatName {
		t.Fatalf("state = %q, want %q", got, StateCatName)
	}

	mustCallback(t, h, ctx, 1, cbCatRenamePrefix+itoa(kopi))
	mustSend(t, h, ctx, 1, "Kopi Susu")
	if got, want := sender.last(t), say("category.renamed", map[string]string{"category": "Kopi Susu"}); got != want {
		t.Fatalf("ganti nama = %q, want %q", got, want)
	}
	mustCallback(t, h, ctx, 1, cbCatOffPrefix+itoa(kopi))
	if got, want := sender.last(t), say("category.deactivated", map[string]string{"category": "Kopi Susu"}); got != want {
		t.Fatalf("nonaktifkan = %q, want %q", got, want)
	}
	cats, err := db.ListCategories(ctx, 1, storage.KindExpense, true)
	if err != nil {
		t.Fatalf("ListCategories: %v", err)
	}
	for _, c := range cats {
		if c.Name == "Kopi Susu" {
			t.Fatal("kategori nonaktif masih muncul di daftar aktif")
		}
	}
}

func TestPeriodeStatusAndSet(t *testing.T) {
	h, sender, db, ctx := newTestHandler(t)
	mustSend(t, h, ctx, 1, "/periode")
	r, err := period.Resolve(testToday, 21)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	dayNo, _ := period.DayNumber(r, testToday)
	dayTotal, _ := period.Days(r)
	want := say("period.status", map[string]string{
		"period":    persona.LabelDefaultPeriodName,
		"start":     period.FormatShort(r.Start),
		"end":       period.FormatShort(r.End),
		"day_no":    itoa(int64(dayNo)),
		"day_total": itoa(int64(dayTotal)),
	})
	if got := sender.last(t); got != want {
		t.Fatalf("/periode = %q, want %q", got, want)
	}

	mustSend(t, h, ctx, 1, "/periode set 25 Gajian")
	newRange, err := period.Resolve(testToday, 25)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want = say("period.saved", map[string]string{"period": "Gajian", "start": period.FormatShort(newRange.Start)})
	if got := sender.last(t); got != want {
		t.Fatalf("/periode set = %q, want %q", got, want)
	}
	recs, err := db.ListPeriods(ctx, 1)
	if err != nil {
		t.Fatalf("ListPeriods: %v", err)
	}
	if len(recs) != 2 || recs[0].Name != "Gajian" || recs[0].StartDay != 25 {
		t.Fatalf("periode tersimpan = %+v", recs)
	}

	mustSend(t, h, ctx, 1, "/periode set 29")
	if got, want := sender.last(t), valid("val.start_day", nil); got != want {
		t.Fatalf("start day salah = %q, want %q", got, want)
	}
}

func TestReminderCommand(t *testing.T) {
	h, sender, db, ctx := newTestHandler(t)
	mustSend(t, h, ctx, 1, "/reminder off")
	if got, want := sender.last(t), say("reminder.off", nil); got != want {
		t.Fatalf("reminder off = %q, want %q", got, want)
	}
	s, err := db.GetSettings(ctx, 1)
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if s.ReminderEnabled {
		t.Fatal("reminder masih aktif")
	}
	mustSend(t, h, ctx, 1, "/reminder 07:30")
	if got, want := sender.last(t), say("reminder.past", map[string]string{"time": "07:30"}); got != want {
		t.Fatalf("reminder 07:30 = %q, want %q", got, want)
	}
	mustSend(t, h, ctx, 1, "/reminder 25:00")
	if got, want := sender.last(t), valid("val.time", nil); got != want {
		t.Fatalf("jam salah = %q, want %q", got, want)
	}
}

func TestPersonaCommand(t *testing.T) {
	h, sender, db, ctx := newTestHandler(t)
	mustSend(t, h, ctx, 1, "/persona")
	want := prompt("prompt.persona", map[string]string{"persona": persona.Labels()[persona.Default()]})
	if got := sender.last(t); got != want {
		t.Fatalf("/persona = %q, want %q", got, want)
	}
	mustSend(t, h, ctx, 1, "/persona set xxx")
	want = valid("val.persona", map[string]string{"input": "xxx", "list": personaIDList()})
	if got := sender.last(t); got != want {
		t.Fatalf("/persona set xxx = %q, want %q", got, want)
	}
	s, err := db.GetSettings(ctx, 1)
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if s.Persona != string(persona.Default()) {
		t.Fatalf("persona berubah menjadi %q", s.Persona)
	}

	mustSend(t, h, ctx, 1, "/persona set netral")
	if got, want := sender.last(t), say("persona.changed", map[string]string{"persona": persona.Labels()[persona.Netral]}); got != want {
		t.Fatalf("/persona set netral = %q, want %q", got, want)
	}
	// The next reply must already use the new persona.
	sender.reset()
	mustSend(t, h, ctx, 1, "/help")
	if got, want := persona.Render(persona.Netral, "help.list", nil), sender.last(t); got != want {
		t.Fatalf("help setelah ganti persona = %q, want %q", want, got)
	}
}

func TestRekapAndExportUseTheActivePeriod(t *testing.T) {
	h, sender, db, ctx := newTestHandler(t)
	addExpense(t, h, sender, ctx, 1, "Makan", "17000")
	mustCallback(t, h, ctx, 1, cbSave)

	r, err := period.Resolve(testToday, 21)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	rows, err := db.ListTransactionsInRange(ctx, 1, r.Start, r.End)
	if err != nil {
		t.Fatalf("ListTransactionsInRange: %v", err)
	}
	wantMsgs := report.RenderRekap(persona.Default(), report.Aggregate(reportRows(rows), r, persona.LabelDefaultPeriodName))

	sender.reset()
	mustSend(t, h, ctx, 1, "/rekap")
	if len(sender.messages) != len(wantMsgs) {
		t.Fatalf("jumlah pesan rekap = %d, want %d", len(sender.messages), len(wantMsgs))
	}
	for i, want := range wantMsgs {
		if sender.messages[i].text != want {
			t.Fatalf("rekap[%d] =\n%s\nwant\n%s", i, sender.messages[i].text, want)
		}
	}

	sender.reset()
	mustSend(t, h, ctx, 1, "/export")
	if len(sender.documents) != 1 {
		t.Fatalf("dokumen = %d, want 1", len(sender.documents))
	}
	doc := sender.documents[0]
	if want := "rekap-20260921-20261020.csv"; doc.filename != want {
		t.Fatalf("filename = %q, want %q", doc.filename, want)
	}
	if !strings.HasPrefix(doc.content, "tanggal,tipe,kategori,catatan,nominal\n") {
		t.Fatalf("csv = %q", doc.content)
	}
	if !strings.Contains(doc.content, "2026-09-27,expense,Makan,") {
		t.Fatalf("csv tidak memuat baris hari ini: %q", doc.content)
	}
}

func TestUnknownCommandAndCallbackAreHarmless(t *testing.T) {
	h, sender, _, ctx := newTestHandler(t)
	mustSend(t, h, ctx, 1, "/tidakada")
	if got, want := sender.last(t), say("idle.hint", nil); got != want {
		t.Fatalf("perintah asing = %q, want %q", got, want)
	}
	sender.reset()
	mustCallback(t, h, ctx, 1, "ngawur")
	if len(sender.messages) != 0 {
		t.Fatalf("callback asing mengirim pesan: %q", sender.last(t))
	}
}

func TestSendReminderEmptyAndWithData(t *testing.T) {
	h, sender, db, ctx := newTestHandler(t)
	if err := h.ensureUser(ctx, 1); err != nil {
		t.Fatalf("ensureUser: %v", err)
	}
	if err := h.SendReminder(ctx, 1); err != nil {
		t.Fatalf("SendReminder: %v", err)
	}
	want := say("reminder.empty", map[string]string{"date": period.FormatDayMonthName(testToday)})
	if got := sender.last(t); got != want {
		t.Fatalf("reminder kosong = %q, want %q", got, want)
	}
	if sender.messages[0].chatID != 1 {
		t.Fatalf("chatID = %d, want 1", sender.messages[0].chatID)
	}
	s, err := db.GetSettings(ctx, 1)
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if s.LastReminderDate != testToday {
		t.Fatalf("last_reminder_date = %q, want %q", s.LastReminderDate, testToday)
	}
	targets, err := db.ListReminderTargets(ctx, testToday)
	if err != nil {
		t.Fatalf("ListReminderTargets: %v", err)
	}
	if len(targets) != 0 {
		t.Fatalf("user masih terdaftar sebagai target: %+v", targets)
	}

	// A disabled reminder sends nothing at all.
	if err := db.SetReminderEnabled(ctx, 1, false); err != nil {
		t.Fatalf("SetReminderEnabled: %v", err)
	}
	sender.reset()
	if err := h.SendReminder(ctx, 1); err != nil {
		t.Fatalf("SendReminder: %v", err)
	}
	if len(sender.messages) != 0 {
		t.Fatalf("reminder mati tetap mengirim: %q", sender.last(t))
	}
}

func TestSendReminderSummarisesTheDay(t *testing.T) {
	h, sender, _, ctx := newTestHandler(t)
	addExpense(t, h, sender, ctx, 1, "Makan", "17000")
	mustCallback(t, h, ctx, 1, cbSave)
	addExpense(t, h, sender, ctx, 1, "Makan", "15000")
	mustCallback(t, h, ctx, 1, cbSave)
	sender.reset()

	if err := h.SendReminder(ctx, 1); err != nil {
		t.Fatalf("SendReminder: %v", err)
	}
	got := sender.last(t)
	if !strings.Contains(got, persona.LabelDayExpense) || !strings.Contains(got, money.Format(32000)) {
		t.Fatalf("reminder tidak memuat total hari ini: %q", got)
	}
	if !strings.Contains(got, "2 "+persona.LabelTxUnit) {
		t.Fatalf("reminder tidak memuat jumlah transaksi: %q", got)
	}
	if !strings.Contains(got, "Makan "+money.Format(32000)) {
		t.Fatalf("reminder tidak memuat rincian kategori: %q", got)
	}
}

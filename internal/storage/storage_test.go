package storage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func newTestDB(t *testing.T) (*DB, context.Context) {
	t.Helper()
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db, ctx
}

func mustUser(t *testing.T, db *DB, ctx context.Context, id int64) {
	t.Helper()
	if _, err := db.EnsureUser(ctx, id); err != nil {
		t.Fatalf("EnsureUser(%d): %v", id, err)
	}
}

func categoryID(t *testing.T, db *DB, ctx context.Context, userID int64, kind Kind, name string) int64 {
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

func TestMigrateIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m.db")
	ctx := context.Background()

	db1, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open pertama: %v", err)
	}
	want, err := LatestVersion()
	if err != nil {
		t.Fatalf("LatestVersion: %v", err)
	}
	var v int
	if err := db1.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v); err != nil {
		t.Fatalf("baca user_version: %v", err)
	}
	if v != want {
		t.Fatalf("user_version = %d, want %d", v, want)
	}
	if err := db1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	db2, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open kedua harus no-op: %v", err)
	}
	defer db2.Close()
	if err := db2.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v); err != nil {
		t.Fatalf("baca user_version: %v", err)
	}
	if v != want {
		t.Fatalf("user_version setelah Open kedua = %d, want %d", v, want)
	}
}

func TestEnsureUserSeeds(t *testing.T) {
	db, ctx := newTestDB(t)
	created, err := db.EnsureUser(ctx, 1001)
	if err != nil {
		t.Fatalf("EnsureUser: %v", err)
	}
	if !created {
		t.Fatal("EnsureUser pertama harus created=true")
	}

	exp, err := db.ListCategories(ctx, 1001, KindExpense, false)
	if err != nil {
		t.Fatalf("ListCategories: %v", err)
	}
	if len(exp) != 6 {
		t.Fatalf("kategori expense = %d, want 6", len(exp))
	}
	inc, err := db.ListCategories(ctx, 1001, KindIncome, false)
	if err != nil {
		t.Fatalf("ListCategories: %v", err)
	}
	if len(inc) != 2 {
		t.Fatalf("kategori income = %d, want 2", len(inc))
	}
	for i, want := range seedExpenseCategories {
		if exp[i].Name != want {
			t.Fatalf("expense[%d] = %q, want %q", i, exp[i].Name, want)
		}
		if !exp[i].Active {
			t.Fatalf("kategori %q harus aktif", exp[i].Name)
		}
	}

	s, err := db.GetSettings(ctx, 1001)
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if s.Persona != "penjaga" || s.ReminderTime != "19:00" || !s.ReminderEnabled || s.LastReminderDate != "" {
		t.Fatalf("settings awal = %+v", s)
	}

	periods, err := db.ListPeriods(ctx, 1001)
	if err != nil {
		t.Fatalf("ListPeriods: %v", err)
	}
	if len(periods) != 1 || periods[0].Name != "Gaji" || periods[0].StartDay != 21 || periods[0].EndDay != 20 {
		t.Fatalf("periode awal = %+v", periods)
	}

	created, err = db.EnsureUser(ctx, 1001)
	if err != nil {
		t.Fatalf("EnsureUser kedua: %v", err)
	}
	if created {
		t.Fatal("EnsureUser kedua harus created=false")
	}
	exp2, _ := db.ListCategories(ctx, 1001, KindExpense, false)
	if len(exp2) != 6 {
		t.Fatalf("kategori expense setelah EnsureUser kedua = %d, want 6", len(exp2))
	}
	periods2, _ := db.ListPeriods(ctx, 1001)
	if len(periods2) != 1 {
		t.Fatalf("periode setelah EnsureUser kedua = %d, want 1", len(periods2))
	}
}

func TestUserExists(t *testing.T) {
	db, ctx := newTestDB(t)
	ok, err := db.UserExists(ctx, 42)
	if err != nil {
		t.Fatalf("UserExists: %v", err)
	}
	if ok {
		t.Fatal("user belum terdaftar")
	}
	mustUser(t, db, ctx, 42)
	ok, err = db.UserExists(ctx, 42)
	if err != nil || !ok {
		t.Fatalf("UserExists setelah daftar = %v, %v", ok, err)
	}
}

func TestCategoryDuplicateAndKinds(t *testing.T) {
	db, ctx := newTestDB(t)
	mustUser(t, db, ctx, 1)

	if _, err := db.CreateCategory(ctx, 1, "makan", KindExpense); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("nama sama beda huruf besar/kecil harus ErrDuplicate, dapat %v", err)
	}
	if _, err := db.CreateCategory(ctx, 1, "MAKAN", KindExpense); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("harus ErrDuplicate, dapat %v", err)
	}
	id, err := db.CreateCategory(ctx, 1, "makan", KindIncome)
	if err != nil {
		t.Fatalf("nama sama pada tipe lain harus boleh: %v", err)
	}
	if id == 0 {
		t.Fatal("id kategori 0")
	}

	taken, err := db.CategoryNameTaken(ctx, 1, KindExpense, "makan", 0)
	if err != nil || !taken {
		t.Fatalf("CategoryNameTaken = %v, %v", taken, err)
	}
	taken, err = db.CategoryNameTaken(ctx, 1, KindExpense, "makan", categoryID(t, db, ctx, 1, KindExpense, "Makan"))
	if err != nil || taken {
		t.Fatalf("mengecualikan diri sendiri: %v, %v", taken, err)
	}
}

func TestMultiUserIsolation(t *testing.T) {
	db, ctx := newTestDB(t)
	mustUser(t, db, ctx, 1)
	mustUser(t, db, ctx, 2)

	catA := categoryID(t, db, ctx, 1, KindExpense, "Makan")
	txA, err := db.CreateTransaction(ctx, Transaction{
		UserID: 1, OccurredOn: "2026-09-21", Kind: KindExpense,
		CategoryID: catA, Amount: 17000, Note: "maksi",
	})
	if err != nil {
		t.Fatalf("CreateTransaction A: %v", err)
	}
	if _, err := db.CreatePeriod(ctx, 1, "GajiBaru", 25, "2026-09-25"); err != nil {
		t.Fatalf("CreatePeriod A: %v", err)
	}
	if err := db.SetPersona(ctx, 1, "posesif"); err != nil {
		t.Fatalf("SetPersona A: %v", err)
	}
	if err := db.SetReminderTime(ctx, 1, "07:30"); err != nil {
		t.Fatalf("SetReminderTime A: %v", err)
	}

	// Kategori user A tidak terlihat oleh B.
	if _, err := db.GetCategory(ctx, 2, catA); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetCategory lintas user = %v, want ErrNotFound", err)
	}
	if err := db.RenameCategory(ctx, 2, catA, "Bajakan"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("RenameCategory lintas user = %v", err)
	}
	if err := db.SetCategoryActive(ctx, 2, catA, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SetCategoryActive lintas user = %v", err)
	}

	// Transaksi user A tidak terlihat/diubah/dihapus oleh B.
	if _, err := db.GetTransaction(ctx, 2, txA); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetTransaction lintas user = %v", err)
	}
	if err := db.DeleteTransaction(ctx, 2, txA); !errors.Is(err, ErrNotFound) {
		t.Fatalf("DeleteTransaction lintas user = %v", err)
	}
	err = db.UpdateTransaction(ctx, Transaction{
		ID: txA, UserID: 2, OccurredOn: "2026-09-21", Kind: KindExpense,
		CategoryID: categoryID(t, db, ctx, 2, KindExpense, "Makan"), Amount: 1,
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdateTransaction lintas user = %v", err)
	}
	got, err := db.GetTransaction(ctx, 1, txA)
	if err != nil {
		t.Fatalf("GetTransaction A: %v", err)
	}
	if got.Amount != 17000 || got.Note != "maksi" {
		t.Fatalf("transaksi A berubah: %+v", got.Transaction)
	}

	// Daftar B kosong.
	if rows, err := db.ListTransactionsByDate(ctx, 2, "2026-09-21"); err != nil || len(rows) != 0 {
		t.Fatalf("ListTransactionsByDate B = %d, %v", len(rows), err)
	}
	if rows, err := db.ListRecentTransactions(ctx, 2, 10); err != nil || len(rows) != 0 {
		t.Fatalf("ListRecentTransactions B = %d, %v", len(rows), err)
	}
	if rows, err := db.ListTransactionsInRange(ctx, 2, "2026-09-01", "2026-09-30"); err != nil || len(rows) != 0 {
		t.Fatalf("ListTransactionsInRange B = %d, %v", len(rows), err)
	}
	if n, err := db.CountTransactionsInRange(ctx, 2, "2026-09-01", "2026-09-30"); err != nil || n != 0 {
		t.Fatalf("CountTransactionsInRange B = %d, %v", n, err)
	}

	// Kategori B hanya seed miliknya.
	expB, err := db.ListCategories(ctx, 2, KindExpense, false)
	if err != nil {
		t.Fatalf("ListCategories B: %v", err)
	}
	if len(expB) != 6 {
		t.Fatalf("kategori expense B = %d, want 6", len(expB))
	}

	// Settings B tidak terpengaruh.
	sB, err := db.GetSettings(ctx, 2)
	if err != nil {
		t.Fatalf("GetSettings B: %v", err)
	}
	if sB.ReminderTime != "19:00" || sB.Persona != "penjaga" {
		t.Fatalf("settings B = %+v", sB)
	}
	if err := db.SetPersona(ctx, 2, "softboy"); err != nil {
		t.Fatalf("SetPersona B: %v", err)
	}
	sA, _ := db.GetSettings(ctx, 1)
	if sA.Persona != "posesif" || sA.ReminderTime != "07:30" {
		t.Fatalf("settings A tercemar: %+v", sA)
	}

	// Periode B hanya seed.
	periodsB, err := db.ListPeriods(ctx, 2)
	if err != nil {
		t.Fatalf("ListPeriods B: %v", err)
	}
	if len(periodsB) != 1 || periodsB[0].EffectiveFrom != seedPeriodEffectiveFrom {
		t.Fatalf("periode B = %+v", periodsB)
	}
}

func TestUpdateTransactionFields(t *testing.T) {
	db, ctx := newTestDB(t)
	mustUser(t, db, ctx, 1)
	cat := categoryID(t, db, ctx, 1, KindExpense, "Makan")

	base := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	nowFunc = func() time.Time { return base }
	defer func() { nowFunc = func() time.Time { return time.Now().UTC() } }()

	id, err := db.CreateTransaction(ctx, Transaction{
		UserID: 1, OccurredOn: "2026-09-21", Kind: KindExpense, CategoryID: cat, Amount: 17000, Note: "maksi",
	})
	if err != nil {
		t.Fatalf("CreateTransaction: %v", err)
	}
	before, err := db.GetTransaction(ctx, 1, id)
	if err != nil {
		t.Fatalf("GetTransaction: %v", err)
	}
	if before.CreatedAt != before.UpdatedAt {
		t.Fatalf("created_at (%s) harus sama dengan updated_at saat insert (%s)", before.CreatedAt, before.UpdatedAt)
	}

	nowFunc = func() time.Time { return base.Add(2 * time.Hour) }
	if err := db.UpdateTransaction(ctx, Transaction{
		ID: id, UserID: 1, OccurredOn: "2026-09-22", Kind: KindExpense,
		CategoryID: cat, Amount: 21000, Note: "makan malam",
	}); err != nil {
		t.Fatalf("UpdateTransaction: %v", err)
	}
	after, err := db.GetTransaction(ctx, 1, id)
	if err != nil {
		t.Fatalf("GetTransaction: %v", err)
	}
	if after.Amount != 21000 || after.Note != "makan malam" || after.OccurredOn != "2026-09-22" {
		t.Fatalf("kolom tidak terupdate: %+v", after.Transaction)
	}
	if after.CreatedAt != before.CreatedAt {
		t.Fatalf("created_at berubah: %s -> %s", before.CreatedAt, after.CreatedAt)
	}
	if after.UpdatedAt == before.UpdatedAt {
		t.Fatal("updated_at harus berubah")
	}
	if after.UserID != 1 {
		t.Fatalf("user_id berubah: %d", after.UserID)
	}
}

func TestUpdateTransactionRejectsKindMismatch(t *testing.T) {
	db, ctx := newTestDB(t)
	mustUser(t, db, ctx, 1)
	catExpense := categoryID(t, db, ctx, 1, KindExpense, "Makan")
	id, err := db.CreateTransaction(ctx, Transaction{
		UserID: 1, OccurredOn: "2026-09-21", Kind: KindExpense, CategoryID: catExpense, Amount: 17000,
	})
	if err != nil {
		t.Fatalf("CreateTransaction: %v", err)
	}
	// Ganti tipe ke income tanpa mengganti kategori -> ditolak (PRD §4.9).
	err = db.UpdateTransaction(ctx, Transaction{
		ID: id, UserID: 1, OccurredOn: "2026-09-21", Kind: KindIncome, CategoryID: catExpense, Amount: 17000,
	})
	if err == nil {
		t.Fatal("tipe income dengan kategori expense harus ditolak")
	}
	if _, err := db.CreateTransaction(ctx, Transaction{
		UserID: 1, OccurredOn: "2026-09-21", Kind: KindIncome, CategoryID: catExpense, Amount: 1,
	}); err == nil {
		t.Fatal("INSERT dengan kategori beda tipe harus ditolak")
	}
}

func TestRangeBoundariesInclusive(t *testing.T) {
	db, ctx := newTestDB(t)
	mustUser(t, db, ctx, 1)
	cat := categoryID(t, db, ctx, 1, KindExpense, "Makan")
	for _, d := range []string{"2026-09-20", "2026-09-21", "2026-09-25", "2026-09-26", "2026-09-27"} {
		if _, err := db.CreateTransaction(ctx, Transaction{
			UserID: 1, OccurredOn: d, Kind: KindExpense, CategoryID: cat, Amount: 1000,
		}); err != nil {
			t.Fatalf("CreateTransaction(%s): %v", d, err)
		}
	}
	rows, err := db.ListTransactionsInRange(ctx, 1, "2026-09-21", "2026-09-26")
	if err != nil {
		t.Fatalf("ListTransactionsInRange: %v", err)
	}
	want := []string{"2026-09-21", "2026-09-25", "2026-09-26"}
	if len(rows) != len(want) {
		t.Fatalf("rows = %d, want %d (%+v)", len(rows), len(want), rows)
	}
	for i := range want {
		if rows[i].OccurredOn != want[i] {
			t.Fatalf("rows[%d] = %s, want %s", i, rows[i].OccurredOn, want[i])
		}
	}
}

func TestSumByCategoryOnDateScoping(t *testing.T) {
	db, ctx := newTestDB(t)
	mustUser(t, db, ctx, 1)
	mustUser(t, db, ctx, 2)
	makan := categoryID(t, db, ctx, 1, KindExpense, "Makan")
	transport := categoryID(t, db, ctx, 1, KindExpense, "Transport")

	for _, tx := range []Transaction{
		{UserID: 1, OccurredOn: "2026-09-21", Kind: KindExpense, CategoryID: makan, Amount: 17000},
		{UserID: 1, OccurredOn: "2026-09-21", Kind: KindExpense, CategoryID: makan, Amount: 15000},
		{UserID: 1, OccurredOn: "2026-09-21", Kind: KindExpense, CategoryID: transport, Amount: 26000},
		{UserID: 1, OccurredOn: "2026-09-22", Kind: KindExpense, CategoryID: makan, Amount: 5000},
	} {
		if _, err := db.CreateTransaction(ctx, tx); err != nil {
			t.Fatalf("CreateTransaction: %v", err)
		}
	}

	total, err := db.SumByCategoryOnDate(ctx, 1, makan, "2026-09-21")
	if err != nil {
		t.Fatalf("SumByCategoryOnDate: %v", err)
	}
	if total != 32000 {
		t.Fatalf("total = %d, want 32000", total)
	}
	if total, _ := db.SumByCategoryOnDate(ctx, 1, makan, "2026-09-23"); total != 0 {
		t.Fatalf("tanggal lain = %d, want 0", total)
	}
	if total, _ := db.SumByCategoryOnDate(ctx, 1, transport, "2026-09-22"); total != 0 {
		t.Fatalf("kategori lain = %d, want 0", total)
	}
	if total, _ := db.SumByCategoryOnDate(ctx, 2, makan, "2026-09-21"); total != 0 {
		t.Fatalf("user lain = %d, want 0", total)
	}

	exp, err := db.SumByKindInRange(ctx, 1, KindExpense, "2026-09-21", "2026-09-22")
	if err != nil {
		t.Fatalf("SumByKindInRange: %v", err)
	}
	if exp != 63000 {
		t.Fatalf("sum expense = %d, want 63000", exp)
	}
	if inc, _ := db.SumByKindInRange(ctx, 1, KindIncome, "2026-09-21", "2026-09-22"); inc != 0 {
		t.Fatalf("sum income = %d, want 0", inc)
	}
}

func TestListRecentTransactionsOrder(t *testing.T) {
	db, ctx := newTestDB(t)
	mustUser(t, db, ctx, 1)
	cat := categoryID(t, db, ctx, 1, KindExpense, "Makan")
	dates := []string{"2026-09-21", "2026-09-23", "2026-09-22", "2026-09-24"}
	for _, d := range dates {
		if _, err := db.CreateTransaction(ctx, Transaction{
			UserID: 1, OccurredOn: d, Kind: KindExpense, CategoryID: cat, Amount: 1000,
		}); err != nil {
			t.Fatalf("CreateTransaction: %v", err)
		}
	}
	rows, err := db.ListRecentTransactions(ctx, 1, 3)
	if err != nil {
		t.Fatalf("ListRecentTransactions: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	want := []string{"2026-09-24", "2026-09-23", "2026-09-22"}
	for i := range want {
		if rows[i].OccurredOn != want[i] {
			t.Fatalf("rows[%d] = %s, want %s", i, rows[i].OccurredOn, want[i])
		}
	}
	if rows[0].CategoryName != "Makan" {
		t.Fatalf("CategoryName = %q", rows[0].CategoryName)
	}
}

func TestListReminderTargets(t *testing.T) {
	db, ctx := newTestDB(t)
	mustUser(t, db, ctx, 1) // baru: last_reminder_date = ''
	mustUser(t, db, ctx, 2)
	mustUser(t, db, ctx, 3)

	if err := db.SetLastReminderDate(ctx, 2, "2026-09-27"); err != nil {
		t.Fatalf("SetLastReminderDate: %v", err)
	}
	if err := db.SetReminderEnabled(ctx, 3, false); err != nil {
		t.Fatalf("SetReminderEnabled: %v", err)
	}

	targets, err := db.ListReminderTargets(ctx, "2026-09-27")
	if err != nil {
		t.Fatalf("ListReminderTargets: %v", err)
	}
	if len(targets) != 1 || targets[0].UserID != 1 {
		t.Fatalf("targets = %+v, want hanya user 1", targets)
	}
	if targets[0].ReminderTime != "19:00" || targets[0].Persona != "penjaga" {
		t.Fatalf("target = %+v", targets[0])
	}
}

func TestConversationRoundTrip(t *testing.T) {
	db, ctx := newTestDB(t)
	mustUser(t, db, ctx, 1)

	if _, err := db.GetConversation(ctx, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetConversation kosong = %v, want ErrNotFound", err)
	}
	if err := db.SetConversation(ctx, Conversation{UserID: 1, State: "tx.amount", Payload: `{"kind":"expense"}`}); err != nil {
		t.Fatalf("SetConversation: %v", err)
	}
	if err := db.SetConversation(ctx, Conversation{UserID: 1, State: "tx.note", Payload: `{"amount":17000}`}); err != nil {
		t.Fatalf("SetConversation kedua: %v", err)
	}
	c, err := db.GetConversation(ctx, 1)
	if err != nil {
		t.Fatalf("GetConversation: %v", err)
	}
	if c.State != "tx.note" || c.Payload != `{"amount":17000}` {
		t.Fatalf("conversation = %+v", c)
	}

	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM conversations WHERE user_id = 1`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("baris conversation = %d, want 1", n)
	}

	if err := db.DeleteConversation(ctx, 1); err != nil {
		t.Fatalf("DeleteConversation: %v", err)
	}
	if _, err := db.GetConversation(ctx, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("setelah delete = %v", err)
	}
	if err := db.DeleteConversation(ctx, 1); err != nil {
		t.Fatalf("DeleteConversation pada baris hilang harus nil, dapat %v", err)
	}
}

func TestCreatePeriodValidation(t *testing.T) {
	db, ctx := newTestDB(t)
	mustUser(t, db, ctx, 1)

	for _, d := range []int{1, 29} {
		if _, err := db.CreatePeriod(ctx, 1, "X", d, "2026-10-01"); err == nil {
			t.Fatalf("startDay %d harus ditolak", d)
		}
	}
	id, err := db.CreatePeriod(ctx, 1, "GajiBaru", 25, "2026-10-01")
	if err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}
	if id == 0 {
		t.Fatal("id periode 0")
	}
	periods, err := db.ListPeriods(ctx, 1)
	if err != nil {
		t.Fatalf("ListPeriods: %v", err)
	}
	if len(periods) != 2 {
		t.Fatalf("periode = %d, want 2", len(periods))
	}
	if periods[0].EffectiveFrom != "2026-10-01" || periods[0].EndDay != 24 {
		t.Fatalf("periode terbaru = %+v", periods[0])
	}
	if _, err := db.CreatePeriod(ctx, 1, "GajiBaru", 25, "2026-10-01"); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("effective_from duplikat = %v, want ErrDuplicate", err)
	}
}

func TestForeignKeysEnabled(t *testing.T) {
	db, ctx := newTestDB(t)
	mustUser(t, db, ctx, 1)
	if _, err := db.CreateTransaction(ctx, Transaction{
		UserID: 1, OccurredOn: "2026-09-21", Kind: KindExpense, CategoryID: 999999, Amount: 1000,
	}); err == nil {
		t.Fatal("category_id tidak ada harus gagal")
	}
	// Kategori milik user lain juga ditolak oleh pemeriksaan kepemilikan.
	mustUser(t, db, ctx, 2)
	catB := categoryID(t, db, ctx, 2, KindExpense, "Makan")
	if _, err := db.CreateTransaction(ctx, Transaction{
		UserID: 1, OccurredOn: "2026-09-21", Kind: KindExpense, CategoryID: catB, Amount: 1000,
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("kategori user lain = %v, want ErrNotFound", err)
	}
}

func TestSettingsSettersMissingUser(t *testing.T) {
	db, ctx := newTestDB(t)
	if err := db.SetReminderEnabled(ctx, 777, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SetReminderEnabled user tak ada = %v", err)
	}
	if err := db.SetReminderTime(ctx, 777, "21:30"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SetReminderTime user tak ada = %v", err)
	}
	if err := db.SetPersona(ctx, 777, "netral"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SetPersona user tak ada = %v", err)
	}
}

func TestCategoryNamesForError(t *testing.T) {
	db, ctx := newTestDB(t)
	mustUser(t, db, ctx, 1)
	names, err := db.CategoryNames(ctx, 1, KindIncome)
	if err != nil {
		t.Fatalf("CategoryNames: %v", err)
	}
	if names != "Gaji, Lain-lain" {
		t.Fatalf("CategoryNames = %q", names)
	}
}

// TestOpenRelativePath pins the shipped default DB_PATH ("./moneybot.db"): a
// relative path must survive DSN building. url.URL{Path: "./x.db"} renders
// "file://./x.db", whose authority is "." — the driver rejects that with
// "invalid uri authority", which made the binary unable to start with its own
// documented default.
func TestOpenRelativePath(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	for _, path := range []string{"moneybot.db", "./moneybot.db"} {
		t.Run(path, func(t *testing.T) {
			ctx := context.Background()
			db, err := Open(ctx, path)
			if err != nil {
				t.Fatalf("Open(%q): %v", path, err)
			}
			defer db.Close()
			if _, err := db.EnsureUser(ctx, 1); err != nil {
				t.Fatalf("EnsureUser: %v", err)
			}
		})
	}
}

// A rename must survive the next call. EnsureUser used to re-run its category
// seeding on EVERY call with INSERT OR IGNORE; the seed rows are keyed
// UNIQUE (user_id, kind, name) and rename changes exactly that attribute, so
// the old name came back as a second category on the very next message.
func TestEnsureUserDoesNotResurrectRenamedSeedCategory(t *testing.T) {
	db, ctx := newTestDB(t)
	mustUser(t, db, ctx, 1)

	id := categoryID(t, db, ctx, 1, KindExpense, "Makan")
	if err := db.RenameCategory(ctx, 1, id, "Makan Besar"); err != nil {
		t.Fatalf("RenameCategory: %v", err)
	}

	if _, err := db.EnsureUser(ctx, 1); err != nil {
		t.Fatalf("EnsureUser kedua: %v", err)
	}

	exp, err := db.ListCategories(ctx, 1, KindExpense, false)
	if err != nil {
		t.Fatalf("ListCategories: %v", err)
	}
	if len(exp) != 6 {
		t.Fatalf("kategori expense = %d, want 6", len(exp))
	}
	for _, c := range exp {
		if c.Name == "Makan" {
			t.Fatal("EnsureUser menghidupkan kembali kategori seed yang sudah diganti nama")
		}
	}
}

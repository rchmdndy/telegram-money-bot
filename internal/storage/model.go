package storage

// Kind adalah tipe transaksi/kategori.
type Kind string

const (
	KindExpense Kind = "expense"
	KindIncome  Kind = "income"
)

// Valid melaporkan apakah Kind dikenal.
func (k Kind) Valid() bool { return k == KindExpense || k == KindIncome }

// Category adalah kategori transaksi milik satu user.
type Category struct {
	ID        int64
	UserID    int64
	Name      string
	Kind      Kind
	Active    bool
	SortOrder int
	CreatedAt string
}

// Transaction adalah satu baris transaksi.
type Transaction struct {
	ID         int64
	UserID     int64
	OccurredOn string // 'YYYY-MM-DD' WIB
	Kind       Kind
	CategoryID int64
	Amount     int64 // rupiah, integer
	Note       string
	CreatedAt  string
	UpdatedAt  string
}

// Settings adalah pengaturan per user.
type Settings struct {
	UserID           int64
	ReminderEnabled  bool
	ReminderTime     string // 'HH:MM'
	LastReminderDate string // 'YYYY-MM-DD', '' = belum pernah
	Persona          string
}

// Conversation adalah percakapan aktif satu user.
type Conversation struct {
	UserID    int64
	State     string
	Payload   string // JSON
	UpdatedAt string
}

// Package quick parses the one-line quick-input format and resolves the typed
// category name against the user's own categories (PRD §4.13). Pure: no
// database, no network, no clock — the handler loads the user's active
// categories of the matching kind and passes them in.
package quick

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/rchmdndy/telegram-money-bot/internal/money"
)

const (
	// MinFields is the smallest accepted field count: type, amount, category.
	// The note is optional — PRD §4.13 says it "Boleh kosong" and PRD §6 #13
	// accepts `O, 17.000, Makan` with an empty note, so three fields are
	// enough even though the error message shows four.
	MinFields = 3
	// MaxNoteLen is the note limit shared with the button flow (PRD §4.2).
	MaxNoteLen = 200
	// KindExpense and KindIncome mirror storage.Kind for the `o`/`i` field.
	KindExpense = "expense"
	KindIncome  = "income"
	// maxDistance is the largest Levenshtein distance still accepted as a
	// fuzzy category candidate (PRD §4.13.2).
	maxDistance = 2
	// prefixLen is the minimum common prefix that qualifies a candidate even
	// when the edit distance is larger than maxDistance (PRD §4.13.2, e.g.
	// `transportasi` → `Transport`).
	prefixLen = 4
)

var (
	// ErrFormat means the line does not have enough comma-separated fields or
	// the category field is blank.
	ErrFormat = errors.New("format quick input tidak sesuai")
	// ErrKind means the type field is neither `o` nor `i`.
	ErrKind = errors.New("tipe harus o atau i")
	// ErrAmount means money.Parse rejected the amount field.
	ErrAmount = errors.New("nominal tidak valid")
	// ErrNoteLong means the note exceeds MaxNoteLen characters.
	ErrNoteLong = errors.New("catatan terlalu panjang")
	// ErrCategory means no candidate matched the typed category name.
	ErrCategory = errors.New("kategori tidak ditemukan")
	// ErrAmbiguous means two or more candidates shared the smallest distance,
	// so guessing would be a coin flip (PRD §4.13.2).
	ErrAmbiguous = errors.New("kategori ambigu")
)

// Result is a parsed quick-input line. Category is still the raw user text;
// the caller resolves it with MatchCategory.
type Result struct {
	Kind     string // KindExpense or KindIncome
	Amount   int64
	Category string
	Note     string
}

// IsQuickInput reports whether text is a quick-input candidate: the first
// field (before the first comma, trimmed, lowercased) is exactly `o` or `i`
// (PRD §4.13.1 condition 2). `oke, 47k, makan` is not quick input and must be
// answered with idle.hint.
func IsQuickInput(text string) bool {
	first, _, ok := strings.Cut(text, ",")
	if !ok {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(first)) {
	case "o", "i":
		return true
	}
	return false
}

// Parse turns a quick-input line into a Result. It is only meaningful for a
// line IsQuickInput accepted, but it validates the type field too so a wrong
// type is reported rather than silently treated as expense.
func Parse(text string) (Result, error) {
	fields := strings.Split(text, ",")
	if len(fields) < MinFields {
		return Result{}, ErrFormat
	}

	var kind string
	switch strings.ToLower(strings.TrimSpace(fields[0])) {
	case "o":
		kind = KindExpense
	case "i":
		kind = KindIncome
	default:
		return Result{}, ErrKind
	}

	amountEnd := 1
	amount, err := money.Parse(fields[1])
	// A comma inside the amount (`13,5k`) doubles as the field separator, so
	// it arrives split across two fields. Rejoin the halves when the first is
	// plain digits and the second is digits plus `k`/`K` — per PRD §4.2 that
	// is the only amount form where the comma is not a separator.
	if len(fields) > 2 {
		if joined, ok := joinDecimalK(fields[1], fields[2]); ok {
			if v, jerr := money.Parse(joined); jerr == nil {
				amount, err, amountEnd = v, nil, 2
			}
		}
	}
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrAmount, err)
	}

	if len(fields) < amountEnd+2 {
		return Result{}, ErrFormat
	}
	category := strings.TrimSpace(fields[amountEnd+1])
	if category == "" {
		return Result{}, ErrFormat
	}

	// The note is everything after the third comma, joined back with the
	// commas it contained (PRD §4.13: "sisa field, boleh mengandung koma").
	note := strings.TrimSpace(strings.Join(fields[amountEnd+2:], ","))
	if utf8.RuneCountInString(note) > MaxNoteLen {
		return Result{}, ErrNoteLong
	}

	return Result{Kind: kind, Amount: amount, Category: category, Note: note}, nil
}

// joinDecimalK reports whether a and b are the two halves of a comma-decimal
// `k` amount, e.g. "13" + "5k" -> "13,5k".
func joinDecimalK(a, b string) (string, bool) {
	x := strings.TrimSpace(a)
	y := strings.TrimSpace(b)
	if !allDigits(x) || len(y) < 2 {
		return "", false
	}
	if y[len(y)-1] != 'k' && y[len(y)-1] != 'K' {
		return "", false
	}
	if !allDigits(y[:len(y)-1]) {
		return "", false
	}
	return x + "," + y, true
}

// allDigits reports whether s is non-empty and holds only ASCII digits.
func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// Category is one of the user's categories. The caller passes only active
// rows whose kind matches the parsed type.
type Category struct {
	ID   int64
	Name string
}

// Match is a resolved category.
type Match struct {
	ID       int64
	Name     string
	Fuzzy    bool
	Distance int
}

// MatchCategory resolves a typed category name (PRD §4.13.2): exact
// case-insensitive first, then fuzzy. A fuzzy result always reports
// Fuzzy true so the caller can mark it in the confirmation card — fuzzy never
// saves on its own. No candidate yields ErrCategory and two candidates at the
// smallest distance yield ErrAmbiguous; in both cases the conversation state
// must stay unchanged.
func MatchCategory(input string, cats []Category) (Match, error) {
	in := strings.TrimSpace(input)
	if in == "" {
		return Match{}, ErrCategory
	}
	for _, c := range cats {
		if strings.EqualFold(c.Name, in) {
			return Match{ID: c.ID, Name: c.Name}, nil
		}
	}

	best := -1
	var matches []Match
	for _, c := range cats {
		d := levenshtein(strings.ToLower(in), strings.ToLower(c.Name))
		if d > maxDistance && commonPrefix(strings.ToLower(in), strings.ToLower(c.Name)) < prefixLen {
			continue
		}
		if best == -1 || d < best {
			best = d
			matches = []Match{{ID: c.ID, Name: c.Name, Fuzzy: true, Distance: d}}
			continue
		}
		if d == best {
			matches = append(matches, Match{ID: c.ID, Name: c.Name, Fuzzy: true, Distance: d})
		}
	}

	switch len(matches) {
	case 0:
		return Match{}, ErrCategory
	case 1:
		return matches[0], nil
	default:
		return Match{}, ErrAmbiguous
	}
}

// levenshtein returns the edit distance between a and b.
func levenshtein(a, b string) int {
	ar, br := []rune(a), []rune(b)
	if len(ar) == 0 {
		return len(br)
	}
	prev := make([]int, len(br)+1)
	curr := make([]int, len(br)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		curr[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			curr[j] = min3(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(br)]
}

// commonPrefix returns the number of leading runes a and b share.
func commonPrefix(a, b string) int {
	ar, br := []rune(a), []rune(b)
	n := 0
	for n < len(ar) && n < len(br) && ar[n] == br[n] {
		n++
	}
	return n
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

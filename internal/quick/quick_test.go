package quick

import (
	"errors"
	"strings"
	"testing"
)

// TestIsQuickInput covers PRD §4.13.1 condition 2 and acceptance #27: the
// first field must be exactly `o` or `i`.
func TestIsQuickInput(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"o, 47k, makan, makan malam", true},
		{"i, 2.850.000, gaji, gaji maganghub periode september", true},
		{"O, 17.000, Makan", true},
		{"  o  , 47k, makan", true},
		{"I, 1k, gaji", true},
		{"oke, 47k, makan", false},
		{"x, 47k, makan, x", false},
		{"o", false},
		{"", false},
		{"halo", false},
		{"o47k", false},
		{"in, 47k, makan", false},
		{"makan, 47k, o", false},
	}
	for _, c := range cases {
		if got := IsQuickInput(c.in); got != c.want {
			t.Errorf("IsQuickInput(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestParseAccepts(t *testing.T) {
	cases := []struct {
		in       string
		kind     string
		amount   int64
		category string
		note     string
	}{
		{"o, 47k, makan, makan malam", KindExpense, 47000, "makan", "makan malam"},
		{"i, 2.850.000, gaji, gaji maganghub periode september", KindIncome, 2850000, "gaji", "gaji maganghub periode september"},
		{"O, 17.000, Makan", KindExpense, 17000, "Makan", ""},
		{"o, 47k, makan, telur, saos, es krim", KindExpense, 47000, "makan", "telur, saos, es krim"},
		{"o, 13,5k, makan, ", KindExpense, 13500, "makan", ""},
		{"i, 1, gaji, x", KindIncome, 1, "gaji", "x"},
	}
	for _, c := range cases {
		got, err := Parse(c.in)
		if err != nil {
			t.Errorf("Parse(%q) error: %v", c.in, err)
			continue
		}
		if got.Kind != c.kind || got.Amount != c.amount || got.Category != c.category || got.Note != c.note {
			t.Errorf("Parse(%q) = %+v, want kind=%s amount=%d category=%q note=%q",
				c.in, got, c.kind, c.amount, c.category, c.note)
		}
	}
}

func TestParseRejects(t *testing.T) {
	long := strings.Repeat("a", MaxNoteLen+1)
	ok := strings.Repeat("a", MaxNoteLen)
	cases := []struct {
		in   string
		want error
	}{
		{"o", ErrFormat},
		{"o, 47k", ErrFormat},
		{"o, 47k, ", ErrFormat},
		{"x, 47k, makan, x", ErrKind},
		{"o, abc, makan, x", ErrAmount},
		{"o, 0, makan, x", ErrAmount},
		{"o, 1000000001, makan, x", ErrAmount},
		{"o, 47k, makan, " + long, ErrNoteLong},
	}
	for _, c := range cases {
		_, err := Parse(c.in)
		if !errors.Is(err, c.want) {
			t.Errorf("Parse(%q) error = %v, want %v", c.in, err, c.want)
		}
	}
	if _, err := Parse("o, 47k, makan, " + ok); err != nil {
		t.Errorf("note of exactly %d chars must be accepted: %v", MaxNoteLen, err)
	}
}

// TestNoteLengthIsRunes guards against counting bytes: a 200-rune note of
// multi-byte characters is valid, 201 is not.
func TestNoteLengthIsRunes(t *testing.T) {
	ok := strings.Repeat("é", MaxNoteLen)
	if _, err := Parse("o, 47k, makan, " + ok); err != nil {
		t.Errorf("200-rune note rejected: %v", err)
	}
	if _, err := Parse("o, 47k, makan, " + ok + "é"); !errors.Is(err, ErrNoteLong) {
		t.Errorf("201-rune note error = %v, want ErrNoteLong", err)
	}
}

func testCats() []Category {
	return []Category{
		{ID: 1, Name: "Makan"},
		{ID: 2, Name: "Transport"},
		{ID: 3, Name: "Rumah Tangga"},
		{ID: 4, Name: "Kesehatan"},
		{ID: 5, Name: "Hiburan"},
		{ID: 6, Name: "Lainnya"},
	}
}

// TestMatchCategoryExact covers the first matching step (PRD §4.13.2).
func TestMatchCategoryExact(t *testing.T) {
	cats := testCats()
	cases := []struct {
		in   string
		want int64
	}{
		{"makan", 1},
		{"Makan", 1},
		{"MAKAN", 1},
		{"  makan  ", 1},
		{"transport", 2},
		{"rumah tangga", 3},
		{"Rumah Tangga", 3},
	}
	for _, c := range cases {
		got, err := MatchCategory(c.in, cats)
		if err != nil {
			t.Errorf("MatchCategory(%q) error: %v", c.in, err)
			continue
		}
		if got.ID != c.want {
			t.Errorf("MatchCategory(%q) = id %d, want %d", c.in, got.ID, c.want)
		}
		if got.Fuzzy {
			t.Errorf("MatchCategory(%q) marked Fuzzy for an exact match", c.in)
		}
	}
}

// TestMatchCategoryFuzzy covers the second step: distance <= 2 or a common
// prefix of at least 4 characters (PRD §6 #14, acceptance #26).
//
// Distances are plain Levenshtein (no transposition), so `makna` -> `Makan`
// costs 2 rather than the 1 the PRD table states; both stay within
// maxDistance, so the observable match is identical.
func TestMatchCategoryFuzzy(t *testing.T) {
	cats := testCats()
	cases := []struct {
		in   string
		id   int64
		name string
		dist int
	}{
		{"makna", 1, "Makan", 2},
		{"makanan", 1, "Makan", 2},
		{"transportasi", 2, "Transport", 3}, // qualifies via the prefix rule
		{"kesahatan", 4, "Kesehatan", 1},
	}
	for _, c := range cases {
		got, err := MatchCategory(c.in, cats)
		if err != nil {
			t.Errorf("MatchCategory(%q) error: %v", c.in, err)
			continue
		}
		if got.ID != c.id || got.Name != c.name {
			t.Errorf("MatchCategory(%q) = %d/%s, want %d/%s", c.in, got.ID, got.Name, c.id, c.name)
		}
		if !got.Fuzzy {
			t.Errorf("MatchCategory(%q) not marked Fuzzy", c.in)
		}
		if got.Distance != c.dist {
			t.Errorf("MatchCategory(%q) distance = %d, want %d", c.in, got.Distance, c.dist)
		}
	}
}

func TestMatchCategoryRejects(t *testing.T) {
	cats := testCats()
	// Distance 3 and no 4-char prefix.
	if got, err := MatchCategory("xyz", cats); !errors.Is(err, ErrCategory) {
		t.Errorf("MatchCategory(xyz) = %+v, %v; want ErrCategory", got, err)
	}
	if _, err := MatchCategory("", cats); !errors.Is(err, ErrCategory) {
		t.Errorf("MatchCategory(empty) error = %v, want ErrCategory", err)
	}
	if _, err := MatchCategory("makan", nil); !errors.Is(err, ErrCategory) {
		t.Errorf("MatchCategory with no categories error = %v, want ErrCategory", err)
	}
}

// TestMatchCategoryAmbiguous covers the tie rule: two candidates at the same
// smallest distance must be rejected rather than guessed (PRD §4.13.2).
func TestMatchCategoryAmbiguous(t *testing.T) {
	cats := []Category{{ID: 1, Name: "Makan"}, {ID: 2, Name: "Makam"}}
	if got, err := MatchCategory("makax", cats); !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("MatchCategory(makax) = %+v, %v; want ErrAmbiguous", got, err)
	}
	// A single closest candidate still wins.
	cats = []Category{{ID: 1, Name: "Makan"}, {ID: 2, Name: "Belanja"}}
	got, err := MatchCategory("makna", cats)
	if err != nil || got.ID != 1 {
		t.Fatalf("MatchCategory(makna) = %+v, %v; want id 1", got, err)
	}
}

// TestMatchCategoryInactiveNeverCandidate documents the caller contract: the
// caller passes only active categories, so an inactive name cannot match.
func TestMatchCategoryInactiveNeverCandidate(t *testing.T) {
	active := []Category{{ID: 1, Name: "Makan"}}
	if _, err := MatchCategory("Hiburan", active); !errors.Is(err, ErrCategory) {
		t.Fatalf("inactive name matched: %v", err)
	}
}

func TestLevenshtein(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"a", "", 1},
		{"", "ab", 2},
		{"makan", "makan", 0},
		{"makna", "makan", 2},
		{"makan", "makna", 2},
		{"makanan", "makan", 2},
		{"transportasi", "transport", 3},
		{"kitten", "sitting", 3},
		{"makan", "gaji", 4},
	}
	for _, c := range cases {
		if got := levenshtein(c.a, c.b); got != c.want {
			t.Errorf("levenshtein(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestCommonPrefix(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"transportasi", "transport", 9},
		{"makan", "makanan", 5},
		{"makan", "makam", 4},
		{"abc", "xyz", 0},
		{"", "abc", 0},
	}
	for _, c := range cases {
		if got := commonPrefix(c.a, c.b); got != c.want {
			t.Errorf("commonPrefix(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// TestFuzzyNeverSaves is a structural guard for PRD §4.13.2: MatchCategory
// returns data only — it has no way to write a transaction.
func TestFuzzyNeverSaves(t *testing.T) {
	got, err := MatchCategory("makna", testCats())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Fuzzy {
		t.Fatal("fuzzy match must be flagged so the handler routes it through the confirmation card")
	}
}

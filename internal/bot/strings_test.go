package bot

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

// allowedLiterals is the complete set of string literals package bot may hold
// that contain a space or a non-ASCII rune. Every entry is a separator, a log
// message, or an error/format string — never something a user reads.
//
// PRD §4.12 rule 2: all user-facing text lives in internal/persona. A new
// literal that looks like a sentence fails this test, which forces the author
// either to move the text into internal/persona or to justify it here.
var allowedLiterals = map[string]string{
	" ":                              "separator",
	" \t\n":                          "separator (perintah dan argumen)",
	", ":                             "separator (daftar)",
	"bot: state %q":                  "error internal",
	"callback tidak dikenal":         "log",
	"gagal memproses update":         "log",
	"gagal mengirim pesan error":     "log",
	"gagal menjawab callback":        "log",
	"panic saat memproses update":    "log",
	"panic: %v":                      "log",
	"payload percakapan rusak":       "log",
	"rentang %q":                     "error internal",
	"rentang terbalik %q":            "error internal",
	"state percakapan tidak dikenal": "log",
}

// TestNoUserFacingLiterals walks the package's own source and rejects any
// string literal that looks like user-facing prose (PRD §4.12 rule 2).
func TestNoUserFacingLiterals(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("ParseFile(%s): %v", name, err)
		}

		// Struct tags (json:"cur,omitempty") are string literals too, but they
		// are wire format, not text.
		tags := map[token.Pos]bool{}
		ast.Inspect(file, func(n ast.Node) bool {
			if field, ok := n.(*ast.Field); ok && field.Tag != nil {
				tags[field.Tag.Pos()] = true
			}
			return true
		})

		ast.Inspect(file, func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.ImportSpec:
				return false
			case *ast.BasicLit:
				if v.Kind != token.STRING || tags[v.Pos()] {
					return true
				}
				text, err := strconv.Unquote(v.Value)
				if err != nil {
					return true
				}
				if !looksLikeProse(text) {
					return true
				}
				if _, ok := allowedLiterals[text]; !ok {
					t.Errorf("%s: literal %q terlihat seperti teks untuk pengguna; "+
						"pindahkan ke internal/persona, atau tambahkan ke allowedLiterals dengan alasan",
						fset.Position(v.Pos()), text)
				}
			}
			return true
		})
	}
}

// looksLikeProse reports whether a literal could be read by a user: it holds a
// space or a rune outside ASCII. Keys ("tx.saved"), callback payloads
// ("dt:today"), state names and layout strings hold neither.
func looksLikeProse(s string) bool {
	return strings.ContainsFunc(s, func(r rune) bool {
		return r == ' ' || r > unicode.MaxASCII
	})
}

// TestAllowedLiteralsAreStillPresent keeps the allowlist honest: a stale entry
// would silently permit a future literal to slip back in.
func TestAllowedLiteralsAreStillPresent(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	fset := token.NewFileSet()
	present := map[string]bool{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("ParseFile(%s): %v", name, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				if text, err := strconv.Unquote(lit.Value); err == nil {
					present[text] = true
				}
			}
			return true
		})
	}
	for literal, reason := range allowedLiterals {
		if !present[literal] {
			t.Errorf("allowedLiterals memuat %q (%s) yang sudah tidak dipakai", literal, reason)
		}
	}
}

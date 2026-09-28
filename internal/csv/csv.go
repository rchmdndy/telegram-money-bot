// Package csv renders transactions as the export document described in
// PRD §4.7.
//
// The package is pure: it formats values and returns bytes. It performs no
// I/O and knows nothing about Telegram or the database.
package csv

import (
	"strconv"
	"strings"
)

// Row is one transaction in the export.
type Row struct {
	Date     string // YYYY-MM-DD
	Kind     string // "expense" or "income"
	Category string
	Note     string
	Amount   int64
}

const headerRow = "tanggal,tipe,kategori,catatan,nominal"

// formulaPrefixes are the leading characters a spreadsheet would interpret
// as a formula. User text starting with one of them is escaped as data
// (PRD §4.7).
const formulaPrefixes = "=+-@"

// Export renders the CSV document. The header row is always present, so an
// empty slice yields a header-only document rather than an error (PRD §4.7).
func Export(rows []Row) []byte {
	var b strings.Builder
	b.WriteString(headerRow)
	b.WriteByte('\n')
	for _, r := range rows {
		b.WriteString(escapeField(r.Date))
		b.WriteByte(',')
		b.WriteString(escapeField(r.Kind))
		b.WriteByte(',')
		b.WriteString(escapeField(r.Category))
		b.WriteByte(',')
		b.WriteString(escapeField(r.Note))
		b.WriteByte(',')
		b.WriteString(strconv.FormatInt(r.Amount, 10))
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

// Filename builds `rekap-YYYYMMDD-YYYYMMDD.csv` from two `YYYY-MM-DD` dates.
func Filename(start, end string) string {
	return "rekap-" + compact(start) + "-" + compact(end) + ".csv"
}

func compact(date string) string {
	return strings.ReplaceAll(date, "-", "")
}

// escapeField applies the RFC 4180 quoting rules and the CSV-injection guard.
func escapeField(s string) string {
	if s != "" && strings.IndexByte(formulaPrefixes, s[0]) >= 0 {
		s = "'" + s
	}
	if strings.ContainsAny(s, ",\"\r\n") {
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	return s
}

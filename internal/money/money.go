// Package money parses and formats integer rupiah amounts.
//
// The package is pure: no DB, no Telegram, no network, no logging.
package money

import (
	"fmt"
	"strings"
)

const (
	// MinAmount is the smallest acceptable amount (rupiah).
	MinAmount int64 = 1
	// MaxAmount is the largest acceptable amount (rupiah).
	MaxAmount int64 = 1_000_000_000
)

// Parse converts user input into integer rupiah.
//
// Accepted forms: "17000", "17.000", "17k", "17K", "17,5k", "17.5k",
// "13,5k", "1.234.567", "17 000". Rules (PRD §4.2):
//
//   - Spaces (including U+00A0) are ignored.
//   - Trailing "k"/"K" multiplies by 1000; the numeric body may then hold at
//     most one "." or "," as decimal separator.
//   - Without "k", "." is the thousands separator (well-formed groups of 3)
//     and "," is the decimal separator whose fractional part must be all zeros.
//   - Result must be an integer within [MinAmount, MaxAmount].
//
// Everything else returns an error.
func Parse(s string) (int64, error) {
	orig := s
	// Remove all spaces (regular and non-breaking).
	s = strings.Map(func(r rune) rune {
		if r == ' ' || r == ' ' {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
	if s == "" {
		return 0, fmt.Errorf("nominal tidak valid: %q", orig)
	}

	k := false
	if n := len(s); strings.EqualFold(s[n-1:], "k") {
		k = true
		s = s[:n-1]
		if s == "" {
			return 0, fmt.Errorf("nominal tidak valid: %q", orig)
		}
	}

	var n int64
	var err error
	if k {
		n, err = parseWithSuffix(s)
	} else {
		n, err = parsePlain(s)
	}
	if err != nil {
		return 0, fmt.Errorf("nominal tidak valid: %q", orig)
	}
	if n < MinAmount || n > MaxAmount {
		return 0, fmt.Errorf("nominal di luar batas 1–1.000.000.000: %q", orig)
	}
	return n, nil
}

// parseWithSuffix parses the numeric body of an input ending in "k" and
// returns body × 1000. The body may contain at most one "." or "," as
// decimal separator, e.g. "17.5" → 17500, "17,25" → 17250.
func parseWithSuffix(body string) (int64, error) {
	idx := -1
	for i := 0; i < len(body); i++ {
		c := body[i]
		switch {
		case c >= '0' && c <= '9':
		case c == '.' || c == ',':
			if idx >= 0 { // second separator
				return 0, fmt.Errorf("lebih dari satu pemisah desimal")
			}
			idx = i
		default:
			return 0, fmt.Errorf("karakter tidak valid %q", c)
		}
	}

	var intPart, fracPart string
	if idx < 0 {
		intPart = body
	} else {
		intPart, fracPart = body[:idx], body[idx+1:]
	}
	if intPart == "" || fracPart == "" && idx >= 0 {
		return 0, fmt.Errorf("bagian desimal kosong")
	}
	// Scale fractional part to 3 decimal digits ("5" → 500, "25" → 250);
	// anything beyond 3 digits must be zeros, else ×1000 is non-integer.
	switch {
	case len(fracPart) > 3:
		if !allZeros(fracPart[3:]) {
			return 0, fmt.Errorf("hasil bukan bilangan bulat")
		}
		fracPart = fracPart[:3]
	case len(fracPart) < 3:
		fracPart += strings.Repeat("0", 3-len(fracPart))
	}
	digits := intPart + fracPart
	n, ok := accumulate(digits)
	if !ok {
		return 0, fmt.Errorf("overflow")
	}
	return n, nil
}

// parsePlain parses an input without the "k" suffix. "." is the thousands
// separator (must group digits in threes after the first group) and "," is
// the decimal separator whose fractional part must be all zeros.
func parsePlain(s string) (int64, error) {
	intBody := s
	if idx := strings.IndexByte(s, ','); idx >= 0 {
		if strings.IndexByte(s[idx+1:], ',') >= 0 {
			return 0, fmt.Errorf("lebih dari satu pemisah desimal")
		}
		intBody = s[:idx]
		if !allZeros(s[idx+1:]) || s[idx+1:] == "" {
			return 0, fmt.Errorf("hasil bukan bilangan bulat")
		}
	}

	if !strings.Contains(intBody, ".") {
		if intBody == "" {
			return 0, fmt.Errorf("kosong")
		}
		for i := 0; i < len(intBody); i++ {
			if intBody[i] < '0' || intBody[i] > '9' {
				return 0, fmt.Errorf("karakter tidak valid %q", intBody[i])
			}
		}
		n, ok := accumulate(intBody)
		if !ok {
			return 0, fmt.Errorf("overflow")
		}
		return n, nil
	}

	// Thousands separators: groups of 3 digits after the first group.
	groups := strings.Split(intBody, ".")
	if len(groups[0]) == 0 || len(groups[0]) > 3 || !allDigits(groups[0]) {
		return 0, fmt.Errorf("pengelompokan ribuan tidak valid")
	}
	for _, g := range groups[1:] {
		if len(g) != 3 || !allDigits(g) {
			return 0, fmt.Errorf("pengelompokan ribuan tidak valid")
		}
	}
	n, ok := accumulate(strings.Join(groups, ""))
	if !ok {
		return 0, fmt.Errorf("overflow")
	}
	return n, nil
}

// accumulate converts a digit string to int64, rejecting overflow,
// values above math.MaxInt64, and leading signs.
func accumulate(digits string) (int64, bool) {
	// Strip leading zeros so length bounds the magnitude.
	digits = strings.TrimLeft(digits, "0")
	if len(digits) > 18 { // always fits int64 below 19 digits
		// 19 digits only valid if <= MaxInt64
		if len(digits) > 19 {
			return 0, false
		}
	}
	var n int64
	for i := 0; i < len(digits); i++ {
		c := digits[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		// MaxAmount is 1e9; reject anything obviously beyond int64 headroom.
		if n > (1<<62)/10 {
			return 0, false
		}
		n = n*10 + int64(c-'0')
	}
	return n, true
}

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

func allZeros(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != '0' {
			return false
		}
	}
	return s != ""
}

// Format renders integer rupiah in Indonesian convention:
// 17000 → "Rp 17.000", 1000000 → "Rp 1.000.000", 0 → "Rp 0".
// Negative amounts render as "Rp -1.500".
func Format(amount int64) string {
	return "Rp " + FormatPlain(amount)
}

// FormatPlain renders without the "Rp " prefix: 17000 → "17.000", 0 → "0".
// Negative amounts render as "-1.500".
func FormatPlain(amount int64) string {
	neg := amount < 0
	if neg {
		amount = -amount // safe for all values except MinInt64, which we still handle digit-wise below
	}
	var buf [20]byte
	i := len(buf)
	n := amount
	if n == 0 {
		i--
		buf[i] = '0'
	}
	for digits := 0; n > 0; digits++ {
		if digits > 0 && digits%3 == 0 {
			i--
			buf[i] = '.'
		}
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	s := string(buf[i:])
	if s == "" {
		s = "0"
	}
	if neg {
		return "-" + s
	}
	return s
}

// cache probe

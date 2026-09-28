package period

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var monthShort = [12]string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}

var weekdayName = [7]string{"Minggu", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu"}

// Format renders an inclusive range, e.g. `21 Sep 2026 – 20 Okt 2026`
// (U+2013 EN DASH, single spaces). Invalid dates are returned verbatim.
func Format(r Range) string {
	return FormatShort(r.Start) + " \u2013 " + FormatShort(r.End)
}

// FormatShort renders one date as `21 Sep 2026`.
func FormatShort(date string) string {
	t, err := parseDate(date)
	if err != nil {
		return date
	}
	return fmt.Sprintf("%d %s %d", t.Day(), monthShort[t.Month()-1], t.Year())
}

// FormatDayMonth renders one date as `26 Sep` (no year). Used by the daily
// reminder, whose date is always the current one (PRD §4.8).
func FormatDayMonth(date string) string {
	t, err := parseDate(date)
	if err != nil {
		return date
	}
	return fmt.Sprintf("%d %s", t.Day(), monthShort[t.Month()-1])
}

// FormatDayMonthName renders `Sabtu, 26 Sep`, the shape PRD §4.8 uses for the
// {date} token in the reminder strings.
func FormatDayMonthName(date string) string {
	if _, err := parseDate(date); err != nil {
		return date
	}
	return Weekday(date) + ", " + FormatDayMonth(date)
}

// Weekday returns the Indonesian day name, e.g. `Senin`.
func Weekday(date string) string {
	t, err := parseDate(date)
	if err != nil {
		return ""
	}
	return weekdayName[t.Weekday()]
}

// FormatDayDate renders `21 Sep 2026 (Senin)`.
func FormatDayDate(date string) string {
	if _, err := parseDate(date); err != nil {
		return date
	}
	return FormatShort(date) + " (" + Weekday(date) + ")"
}

// ParseDate normalises user-typed dates (PRD §4.2, §11 decision 1):
// `DD-MM-YYYY` is taken verbatim, `DD/MM` uses the current year unless the
// result would be in the future, in which case the previous year is used
// (records are always past spending). Anything else — including ISO
// `YYYY-MM-DD` and impossible calendar dates such as `31/02` — is rejected.
func ParseDate(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", errors.New("tanggal tidak boleh kosong")
	}
	if _, err := time.Parse(dateLayout, s); err == nil {
		return "", fmt.Errorf("tanggal tidak valid: %q (gunakan DD/MM atau DD-MM-YYYY)", s)
	}
	if strings.Contains(s, "-") {
		t, err := time.Parse("02-01-2006", s)
		if err != nil {
			return "", fmt.Errorf("tanggal tidak valid: %q (gunakan DD/MM atau DD-MM-YYYY)", s)
		}
		return formatDate(t), nil
	}
	parts := strings.Split(s, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", fmt.Errorf("tanggal tidak valid: %q (gunakan DD/MM atau DD-MM-YYYY)", s)
	}
	day, errD := strconv.Atoi(strings.TrimSpace(parts[0]))
	month, errM := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errD != nil || errM != nil {
		return "", fmt.Errorf("tanggal tidak valid: %q (gunakan DD/MM atau DD-MM-YYYY)", s)
	}
	now := nowFunc()
	candidate := time.Date(now.Year(), time.Month(month), day, 0, 0, 0, 0, time.UTC)
	if candidate.Month() != time.Month(month) || candidate.Day() != day {
		return "", fmt.Errorf("tanggal tidak valid: %q (gunakan DD/MM atau DD-MM-YYYY)", s)
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if candidate.After(today) {
		candidate = time.Date(now.Year()-1, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	}
	return formatDate(candidate), nil
}

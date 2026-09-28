// Package period implements the pure calendar arithmetic behind reporting
// periods and report ranges (PRD §4.5, §4.6). Dates are plain `YYYY-MM-DD`
// strings compared lexicographically; nothing here touches a database, the
// network, or the filesystem.
package period

import (
	"errors"
	"fmt"
	"time"
)

const dateLayout = "2006-01-02"

// Valid bounds for a period start day. The upper bound of 28 guarantees that
// `startDay-1` (1..27) exists in every month, so no end-of-month special case
// is ever needed.
const (
	MinStartDay = 2
	MaxStartDay = 28
)

// Period is one row of the `periods` table (PRD §5.3).
type Period struct {
	Name          string
	StartDay      int
	EndDay        int
	EffectiveFrom string
}

// Range is an inclusive date range.
type Range struct {
	Start string
	End   string
}

// nowFunc is the clock used by ParseDate to infer the current year. Tests
// override it; production never does.
var nowFunc = func() time.Time { return time.Now().UTC() }

func parseDate(s string) (time.Time, error) {
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("tanggal tidak valid: %q", s)
	}
	return t, nil
}

func formatDate(t time.Time) string { return t.Format(dateLayout) }

// ValidStartDay reports whether d may be used as a period start day.
func ValidStartDay(d int) bool { return d >= MinStartDay && d <= MaxStartDay }

// Resolve returns the period window containing date for a period that starts
// on day startDay of every month. The window starts on startDay and ends on
// startDay-1 of the following month.
func Resolve(date string, startDay int) (Range, error) {
	if !ValidStartDay(startDay) {
		return Range{}, fmt.Errorf("tanggal awal periode harus %d–%d, bukan %d", MinStartDay, MaxStartDay, startDay)
	}
	t, err := parseDate(date)
	if err != nil {
		return Range{}, err
	}
	y, m, d := t.Date()
	if d >= startDay {
		return Range{
			Start: formatDate(time.Date(y, m, startDay, 0, 0, 0, 0, time.UTC)),
			End:   formatDate(time.Date(y, m+1, startDay-1, 0, 0, 0, 0, time.UTC)),
		}, nil
	}
	return Range{
		Start: formatDate(time.Date(y, m-1, startDay, 0, 0, 0, 0, time.UTC)),
		End:   formatDate(time.Date(y, m, startDay-1, 0, 0, 0, 0, time.UTC)),
	}, nil
}

// ResolveActive picks the period whose EffectiveFrom is the greatest one not
// after date, then resolves that period's window for date (PRD §4.5: a
// schedule change adds a row, so older ranges keep the older schedule).
func ResolveActive(date string, periods []Period) (Period, Range, error) {
	if _, err := parseDate(date); err != nil {
		return Period{}, Range{}, err
	}
	best := -1
	for i := range periods {
		if periods[i].EffectiveFrom > date {
			continue
		}
		if best < 0 || periods[i].EffectiveFrom > periods[best].EffectiveFrom {
			best = i
		}
	}
	if best < 0 {
		return Period{}, Range{}, errors.New("tidak ada periode aktif untuk tanggal ini")
	}
	r, err := Resolve(date, periods[best].StartDay)
	if err != nil {
		return Period{}, Range{}, err
	}
	return periods[best], r, nil
}

// Days returns the inclusive day count of r.
func Days(r Range) (int, error) {
	s, err := parseDate(r.Start)
	if err != nil {
		return 0, err
	}
	e, err := parseDate(r.End)
	if err != nil {
		return 0, err
	}
	if e.Before(s) {
		return 0, fmt.Errorf("rentang tidak valid: %s melewati %s", r.Start, r.End)
	}
	return int(e.Sub(s).Hours()/24) + 1, nil
}

// DayNumber returns the 1-based index of date inside r.
func DayNumber(r Range, date string) (int, error) {
	s, err := parseDate(r.Start)
	if err != nil {
		return 0, err
	}
	e, err := parseDate(r.End)
	if err != nil {
		return 0, err
	}
	d, err := parseDate(date)
	if err != nil {
		return 0, err
	}
	if d.Before(s) || d.After(e) {
		return 0, fmt.Errorf("tanggal %s di luar rentang %s..%s", date, r.Start, r.End)
	}
	return int(d.Sub(s).Hours()/24) + 1, nil
}

// AllDates returns every date in r, ascending.
func AllDates(r Range) ([]string, error) {
	n, err := Days(r)
	if err != nil {
		return nil, err
	}
	s, err := parseDate(r.Start)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, formatDate(s.AddDate(0, 0, i)))
	}
	return out, nil
}

// WeekRange returns Monday..Sunday of the week containing date.
func WeekRange(date string) (Range, error) {
	t, err := parseDate(date)
	if err != nil {
		return Range{}, err
	}
	off := (int(t.Weekday()) + 6) % 7 // Monday = 0
	start := t.AddDate(0, 0, -off)
	return Range{Start: formatDate(start), End: formatDate(start.AddDate(0, 0, 6))}, nil
}

// MonthRange returns the calendar month containing date.
func MonthRange(date string) (Range, error) {
	t, err := parseDate(date)
	if err != nil {
		return Range{}, err
	}
	start := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	return Range{Start: formatDate(start), End: formatDate(start.AddDate(0, 1, -1))}, nil
}

// ShiftDays moves date by n days (n may be negative).
func ShiftDays(date string, n int) (string, error) {
	t, err := parseDate(date)
	if err != nil {
		return "", err
	}
	return formatDate(t.AddDate(0, 0, n)), nil
}

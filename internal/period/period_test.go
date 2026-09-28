package period

import (
	"testing"
	"time"
)

func TestResolve(t *testing.T) {
	cases := []struct {
		name     string
		date     string
		startDay int
		want     Range
	}{
		{"sehari sebelum tanggal 21", "2026-09-20", 21, Range{"2026-08-21", "2026-09-20"}},
		{"tepat tanggal 21", "2026-09-21", 21, Range{"2026-09-21", "2026-10-20"}},
		{"tengah periode", "2026-09-27", 21, Range{"2026-09-21", "2026-10-20"}},
		{"lintas tahun", "2026-12-25", 21, Range{"2026-12-21", "2027-01-20"}},
		{"januari", "2027-01-05", 21, Range{"2026-12-21", "2027-01-20"}},
		{"februari start 28", "2027-02-27", 28, Range{"2027-01-28", "2027-02-27"}},
		{"februari tepat start 28", "2027-02-28", 28, Range{"2027-02-28", "2027-03-27"}},
		{"februari non-kabisat", "2026-02-10", 21, Range{"2026-01-21", "2026-02-20"}},
		{"start 2", "2026-03-01", 2, Range{"2026-02-02", "2026-03-01"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Resolve(c.date, c.startDay)
			if err != nil {
				t.Fatalf("Resolve(%q, %d) error: %v", c.date, c.startDay, err)
			}
			if got != c.want {
				t.Fatalf("Resolve(%q, %d) = %+v, want %+v", c.date, c.startDay, got, c.want)
			}
		})
	}
}

func TestResolveInvalidStartDay(t *testing.T) {
	for _, d := range []int{-1, 0, 1, 29, 31} {
		if _, err := Resolve("2026-09-21", d); err == nil {
			t.Fatalf("Resolve dengan startDay %d harus error", d)
		}
	}
}

func TestResolveInvalidDate(t *testing.T) {
	for _, s := range []string{"", "abc", "2026-13-01", "21/09"} {
		if _, err := Resolve(s, 21); err == nil {
			t.Fatalf("Resolve(%q) harus error", s)
		}
	}
}

func TestValidStartDay(t *testing.T) {
	for d := MinStartDay; d <= MaxStartDay; d++ {
		if !ValidStartDay(d) {
			t.Fatalf("ValidStartDay(%d) harus true", d)
		}
	}
	for _, d := range []int{MinStartDay - 1, MaxStartDay + 1} {
		if ValidStartDay(d) {
			t.Fatalf("ValidStartDay(%d) harus false", d)
		}
	}
}

func TestResolveActive(t *testing.T) {
	periods := []Period{
		{Name: "Gaji", StartDay: 21, EndDay: 20, EffectiveFrom: "1970-01-01"},
		{Name: "GajiBaru", StartDay: 25, EndDay: 24, EffectiveFrom: "2026-09-25"},
	}
	// Sebelum effective_from periode baru -> periode lama.
	p, r, err := ResolveActive("2026-09-24", periods)
	if err != nil {
		t.Fatalf("ResolveActive error: %v", err)
	}
	if p.Name != "Gaji" || r != (Range{"2026-09-21", "2026-10-20"}) {
		t.Fatalf("got %q %+v", p.Name, r)
	}
	// Tepat pada effective_from -> periode baru.
	p, r, err = ResolveActive("2026-09-25", periods)
	if err != nil {
		t.Fatalf("ResolveActive error: %v", err)
	}
	if p.Name != "GajiBaru" || r != (Range{"2026-09-25", "2026-10-24"}) {
		t.Fatalf("got %q %+v", p.Name, r)
	}
}

func TestResolveActiveErrors(t *testing.T) {
	if _, _, err := ResolveActive("2026-09-21", nil); err == nil {
		t.Fatal("slice kosong harus error")
	}
	only := []Period{{Name: "Gaji", StartDay: 21, EndDay: 20, EffectiveFrom: "2026-01-01"}}
	if _, _, err := ResolveActive("2025-12-31", only); err == nil {
		t.Fatal("tidak ada periode yang berlaku harus error")
	}
}

func TestDays(t *testing.T) {
	n, err := Days(Range{"2026-09-21", "2026-10-20"})
	if err != nil {
		t.Fatalf("Days error: %v", err)
	}
	if n != 30 {
		t.Fatalf("Days = %d, want 30", n)
	}
	n, err = Days(Range{"2026-09-21", "2026-09-21"})
	if err != nil || n != 1 {
		t.Fatalf("Days satu hari = %d, %v", n, err)
	}
	if _, err := Days(Range{"2026-10-20", "2026-09-21"}); err == nil {
		t.Fatal("rentang terbalik harus error")
	}
}

func TestDayNumber(t *testing.T) {
	r := Range{"2026-09-21", "2026-10-20"}
	n, err := DayNumber(r, "2026-09-27")
	if err != nil {
		t.Fatalf("DayNumber error: %v", err)
	}
	if n != 7 {
		t.Fatalf("DayNumber = %d, want 7", n)
	}
	if n, err := DayNumber(r, "2026-09-21"); err != nil || n != 1 {
		t.Fatalf("hari pertama = %d, %v", n, err)
	}
	if n, err := DayNumber(r, "2026-10-20"); err != nil || n != 30 {
		t.Fatalf("hari terakhir = %d, %v", n, err)
	}
	for _, d := range []string{"2026-09-20", "2026-10-21"} {
		if _, err := DayNumber(r, d); err == nil {
			t.Fatalf("DayNumber(%q) di luar rentang harus error", d)
		}
	}
}

func TestAllDates(t *testing.T) {
	got, err := AllDates(Range{"2026-09-21", "2026-09-23"})
	if err != nil {
		t.Fatalf("AllDates error: %v", err)
	}
	want := []string{"2026-09-21", "2026-09-22", "2026-09-23"}
	if len(got) != len(want) {
		t.Fatalf("AllDates = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("AllDates[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if _, err := AllDates(Range{"2026-09-23", "2026-09-21"}); err == nil {
		t.Fatal("rentang terbalik harus error")
	}
}

func TestWeekRange(t *testing.T) {
	// 2026-09-27 adalah Minggu.
	got, err := WeekRange("2026-09-27")
	if err != nil {
		t.Fatalf("WeekRange error: %v", err)
	}
	if got != (Range{"2026-09-21", "2026-09-27"}) {
		t.Fatalf("WeekRange = %+v", got)
	}
	// Senin menghasilkan rentang yang sama.
	got, err = WeekRange("2026-09-21")
	if err != nil {
		t.Fatalf("WeekRange error: %v", err)
	}
	if got != (Range{"2026-09-21", "2026-09-27"}) {
		t.Fatalf("WeekRange(Senin) = %+v", got)
	}
}

func TestMonthRange(t *testing.T) {
	cases := []struct {
		date string
		want Range
	}{
		{"2026-09-15", Range{"2026-09-01", "2026-09-30"}},
		{"2026-02-10", Range{"2026-02-01", "2026-02-28"}},
		{"2028-02-10", Range{"2028-02-01", "2028-02-29"}},
		{"2026-12-31", Range{"2026-12-01", "2026-12-31"}},
	}
	for _, c := range cases {
		t.Run(c.date, func(t *testing.T) {
			got, err := MonthRange(c.date)
			if err != nil {
				t.Fatalf("MonthRange error: %v", err)
			}
			if got != c.want {
				t.Fatalf("MonthRange(%q) = %+v, want %+v", c.date, got, c.want)
			}
		})
	}
}

func TestShiftDays(t *testing.T) {
	cases := []struct {
		date string
		n    int
		want string
	}{
		{"2026-12-31", 1, "2027-01-01"},
		{"2026-01-01", -1, "2025-12-31"},
		{"2026-09-21", 0, "2026-09-21"},
		{"2028-02-28", 1, "2028-02-29"},
	}
	for _, c := range cases {
		got, err := ShiftDays(c.date, c.n)
		if err != nil {
			t.Fatalf("ShiftDays(%q, %d) error: %v", c.date, c.n, err)
		}
		if got != c.want {
			t.Fatalf("ShiftDays(%q, %d) = %q, want %q", c.date, c.n, got, c.want)
		}
	}
	if _, err := ShiftDays("abc", 1); err == nil {
		t.Fatal("ShiftDays dengan tanggal invalid harus error")
	}
}

func TestFormat(t *testing.T) {
	if got := Format(Range{"2026-09-21", "2026-10-20"}); got != "21 Sep 2026 \u2013 20 Okt 2026" {
		t.Fatalf("Format = %q", got)
	}
}

func TestFormatShort(t *testing.T) {
	cases := map[string]string{
		"2026-09-21": "21 Sep 2026",
		"2026-01-01": "1 Jan 2026",
		"2026-12-31": "31 Des 2026",
		"2026-05-09": "9 Mei 2026",
		"2026-08-17": "17 Agu 2026",
	}
	for in, want := range cases {
		if got := FormatShort(in); got != want {
			t.Fatalf("FormatShort(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWeekday(t *testing.T) {
	// 21..27 Sep 2026 = Senin..Minggu.
	want := []string{"Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu", "Minggu"}
	for i, w := range want {
		d := time.Date(2026, 9, 21+i, 0, 0, 0, 0, time.UTC).Format(dateLayout)
		if got := Weekday(d); got != w {
			t.Fatalf("Weekday(%q) = %q, want %q", d, got, w)
		}
	}
}

func TestFormatDayDate(t *testing.T) {
	if got := FormatDayDate("2026-09-21"); got != "21 Sep 2026 (Senin)" {
		t.Fatalf("FormatDayDate = %q", got)
	}
}

func TestParseDate(t *testing.T) {
	restore := nowFunc
	nowFunc = func() time.Time { return time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC) }
	defer func() { nowFunc = restore }()

	ok := []struct {
		in   string
		want string
	}{
		{"21/09", "2026-09-21"},
		{"27/09", "2026-09-27"},
		{"30/12", "2025-12-30"}, // masih di masa depan pada 2026 -> tahun sebelumnya
		{"01/01", "2026-01-01"},
		{"21-09-2026", "2026-09-21"},
		{" 21/09 ", "2026-09-21"},
		{"9/9", "2026-09-09"},
	}
	for _, c := range ok {
		got, err := ParseDate(c.in)
		if err != nil {
			t.Fatalf("ParseDate(%q) error: %v", c.in, err)
		}
		if got != c.want {
			t.Fatalf("ParseDate(%q) = %q, want %q", c.in, got, c.want)
		}
	}

	bad := []string{"", "abc", "31/02", "32/01", "00/01", "21/13", "2026-09-21", "21-09", "21/", "/09", "21/09/2026", "1.2"}
	for _, in := range bad {
		if got, err := ParseDate(in); err == nil {
			t.Fatalf("ParseDate(%q) harus error, dapat %q", in, got)
		}
	}
}

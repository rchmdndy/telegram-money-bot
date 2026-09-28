package money

import (
	"fmt"
	"testing"
)

func TestParseAccept(t *testing.T) {
	cases := []struct {
		input string
		want  int64
	}{
		{"17000", 17000},
		{"17.000", 17000},
		{"1.234.567", 1234567},
		{"17k", 17000},
		{"17K", 17000},
		{"17,5k", 17500},
		{"17.5k", 17500},
		{"17,25k", 17250},
		{"0,5k", 500},
		{"1", 1},
		{"1000000000", 1000000000},
		{"17 000", 17000},
		{"17 000", 17000},
		{"17,0", 17},
		{"1.000", 1000},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got, err := Parse(tc.input)
			if err != nil {
				t.Fatalf("Parse(%q) returned error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("Parse(%q) = %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}

func TestParseReject(t *testing.T) {
	inputs := []string{
		"",
		"0",
		"-5",
		"+5",
		"17,5",
		"1000000001",
		"99999999999999999999",
		"17.00",
		"1.2345",
		"17,,5k",
		"1.234,5k",
		"17..5",
		"abc",
		"17x",
		"k",
		"17.",
		",5",
		"1,2,3",
	}
	for _, input := range inputs {
		name := input
		if name == "" {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			got, err := Parse(input)
			if err == nil {
				t.Fatalf("Parse(%q) = %d, want error", input, got)
			}
		})
	}
}

func TestParseBoundaries(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"MinAmount", fmt.Sprintf("%d", MinAmount), false},
		{"BelowMinAmount", fmt.Sprintf("%d", MinAmount-1), true},
		{"MaxAmount", fmt.Sprintf("%d", MaxAmount), false},
		{"AboveMaxAmount", fmt.Sprintf("%d", MaxAmount+1), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Parse(%q) = %d, want error", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%q) returned error: %v", tc.input, err)
			}
		})
	}
}

func TestFormat(t *testing.T) {
	cases := []struct {
		amount int64
		want   string
	}{
		{17000, "Rp 17.000"},
		{1000000, "Rp 1.000.000"},
		{0, "Rp 0"},
		{1000000000, "Rp 1.000.000.000"},
		{999, "Rp 999"},
		{-1500, "Rp -1.500"},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.amount), func(t *testing.T) {
			if got := Format(tc.amount); got != tc.want {
				t.Fatalf("Format(%d) = %q, want %q", tc.amount, got, tc.want)
			}
		})
	}
}

func TestFormatPlain(t *testing.T) {
	cases := []struct {
		amount int64
		want   string
	}{
		{17000, "17.000"},
		{0, "0"},
		{1000000000, "1.000.000.000"},
		{5, "5"},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.amount), func(t *testing.T) {
			if got := FormatPlain(tc.amount); got != tc.want {
				t.Fatalf("FormatPlain(%d) = %q, want %q", tc.amount, got, tc.want)
			}
		})
	}
}

func TestParseFormatRoundTrip(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"17000", "Rp 17.000"},
		{"17.000", "Rp 17.000"},
		{"17k", "Rp 17.000"},
		{"17,5k", "Rp 17.500"},
		{"17.5k", "Rp 17.500"},
		{"17,25k", "Rp 17.250"},
		{"0,5k", "Rp 500"},
		{"1.234.567", "Rp 1.234.567"},
		{"1", "Rp 1"},
		{"1000000000", "Rp 1.000.000.000"},
		{"17 000", "Rp 17.000"},
		{"17,0", "Rp 17"},
		{"1.000", "Rp 1.000"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got, err := Parse(tc.input)
			if err != nil {
				t.Fatalf("Parse(%q) returned error: %v", tc.input, err)
			}
			if rendered := Format(got); rendered != tc.want {
				t.Fatalf("Format(Parse(%q)) = %q, want %q", tc.input, rendered, tc.want)
			}
		})
	}
}

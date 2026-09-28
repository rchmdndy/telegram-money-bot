package csv

import (
	"strings"
	"testing"
)

func TestExportHeaderOnlyForEmptyRange(t *testing.T) {
	got := string(Export(nil))
	want := "tanggal,tipe,kategori,catatan,nominal\n"
	if got != want {
		t.Fatalf("Export(nil) = %q, want %q", got, want)
	}
	if got := string(Export([]Row{})); got != want {
		t.Fatalf("Export(empty slice) = %q, want %q", got, want)
	}
}

func TestExportSampleRows(t *testing.T) {
	got := string(Export([]Row{
		{Date: "2026-09-21", Kind: "expense", Category: "Makan", Note: "maksi", Amount: 17000},
		{Date: "2026-09-21", Kind: "expense", Category: "Makan", Note: "maklam", Amount: 15000},
		{Date: "2026-09-22", Kind: "income", Category: "Gaji", Note: "gaji september", Amount: 3000000},
	}))
	want := "tanggal,tipe,kategori,catatan,nominal\n" +
		"2026-09-21,expense,Makan,maksi,17000\n" +
		"2026-09-21,expense,Makan,maklam,15000\n" +
		"2026-09-22,income,Gaji,gaji september,3000000\n"
	if got != want {
		t.Fatalf("Export = %q, want %q", got, want)
	}
}

func TestExportEscaping(t *testing.T) {
	tests := []struct {
		name string
		row  Row
		want string
	}{
		{
			name: "comma in note",
			row:  Row{Date: "2026-09-26", Kind: "expense", Category: "Rumah Tangga", Note: "spons, es krim", Amount: 6500},
			want: `2026-09-26,expense,Rumah Tangga,"spons, es krim",6500`,
		},
		{
			name: "quote in note",
			row:  Row{Date: "2026-09-26", Kind: "expense", Category: "Makan", Note: `nasi "goreng"`, Amount: 1000},
			want: `2026-09-26,expense,Makan,"nasi ""goreng""",1000`,
		},
		{
			name: "newline in note",
			row:  Row{Date: "2026-09-26", Kind: "expense", Category: "Makan", Note: "baris satu\nbaris dua", Amount: 1000},
			want: "2026-09-26,expense,Makan,\"baris satu\nbaris dua\",1000",
		},
		{
			name: "comma in category",
			row:  Row{Date: "2026-09-26", Kind: "expense", Category: "Makan, Minum", Note: "", Amount: 1000},
			want: `2026-09-26,expense,"Makan, Minum",,1000`,
		},
		{
			name: "formula in note",
			row:  Row{Date: "2026-09-26", Kind: "expense", Category: "Makan", Note: "=1+1", Amount: 1000},
			want: `2026-09-26,expense,Makan,'=1+1,1000`,
		},
		{
			name: "plus prefix",
			row:  Row{Date: "2026-09-26", Kind: "expense", Category: "Makan", Note: "+62", Amount: 1000},
			want: `2026-09-26,expense,Makan,'+62,1000`,
		},
		{
			name: "minus prefix",
			row:  Row{Date: "2026-09-26", Kind: "expense", Category: "Makan", Note: "-diskon", Amount: 1000},
			want: `2026-09-26,expense,Makan,'-diskon,1000`,
		},
		{
			name: "at prefix",
			row:  Row{Date: "2026-09-26", Kind: "expense", Category: "Makan", Note: "@warung", Amount: 1000},
			want: `2026-09-26,expense,Makan,'@warung,1000`,
		},
		{
			name: "formula and comma",
			row:  Row{Date: "2026-09-26", Kind: "expense", Category: "Makan", Note: "=SUM(A1,A2)", Amount: 1000},
			want: `2026-09-26,expense,Makan,"'=SUM(A1,A2)",1000`,
		},
		{
			name: "empty note stays empty",
			row:  Row{Date: "2026-09-26", Kind: "expense", Category: "Makan", Note: "", Amount: 1000},
			want: `2026-09-26,expense,Makan,,1000`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := string(Export([]Row{tc.row}))
			lines := strings.SplitN(out, "\n", 2)
			if len(lines) != 2 {
				t.Fatalf("output = %q", out)
			}
			got := strings.TrimSuffix(lines[1], "\n")
			if got != tc.want {
				t.Fatalf("baris = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestExportNoBOM(t *testing.T) {
	out := Export(nil)
	if strings.HasPrefix(string(out), "\ufeff") {
		t.Fatal("CSV tidak boleh memakai BOM")
	}
}

func TestFilename(t *testing.T) {
	got := Filename("2026-09-21", "2026-10-20")
	if got != "rekap-20260921-20261020.csv" {
		t.Fatalf("Filename = %q", got)
	}
}

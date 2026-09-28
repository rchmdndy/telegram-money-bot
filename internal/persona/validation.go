package persona

import "sort"

// validation holds the persona-wrapped validation and error messages
// (PRD §4.12 rule 4: "Error dan validasi boleh memakai persona, tapi alasan
// dan batasnya harus tetap disebut verbatim"). Only the wrapper sentence
// follows the persona; the reason and every limit are rendered from the same
// tokens the catalog uses, so a number can never be reworded.
//
// It is a separate table from catalog on purpose: catalog is pinned to the 35
// handler keys of the PRD §4.12 key table, while these messages are the
// state-machine rejections of PRD §4.11 ("pesan spesifik") and the field
// validations of §4.2, §4.4, §4.5, §4.8 and §4.13.4.
var validation = map[ID]map[string]string{
	Netral: {
		"val.amount":     "Nominal tidak valid. Masukkan angka 1–{limit}.",
		"val.date":       "Tanggal tidak valid. Pakai format {valid}.",
		"val.cat_name":   "Nama kategori harus 1–32 karakter.",
		"val.cat_dup":    "Kategori {category} sudah ada.",
		"val.cat_max":    "Maksimal {max} kategori aktif per tipe.",
		"val.cat_empty":  "Belum ada kategori aktif. Tambah dulu lewat /kategori.",
		"val.time":       "Jam harus format HH:MM, 00:00–23:59.",
		"val.start_day":  "Tanggal awal periode harus 2–28.",
		"val.persona":    "Persona \"{input}\" tidak ada. Pilihan: {list}.",
		"val.range":      "Rentang tidak dikenal. Pakai: periode, hari, kemarin, minggu, bulan, atau YYYY-MM-DD..YYYY-MM-DD.",
		"val.tx_missing": "Transaksi tidak ditemukan.",
		"val.no_conv":    "Tidak ada percakapan aktif.",
		"val.note":       "Catatan maks 200 karakter.",
	},
	Penjaga: {
		"val.amount":     "Peti tidak mengenal angka itu, Paduka — masukkan angka 1–{limit}.",
		"val.date":       "Tanggal itu tak termuat di buku agung, Paduka. Pakai format {valid}.",
		"val.cat_name":   "Nama kategori harus 1–32 karakter, Paduka.",
		"val.cat_dup":    "Kategori {category} sudah tercatat di buku agung, Paduka.",
		"val.cat_max":    "Peti hanya menampung {max} kategori aktif per tipe, Paduka.",
		"val.cat_empty":  "Belum ada kategori aktif di peti, Paduka. Tambah lewat /kategori.",
		"val.time":       "Jamnya harus HH:MM, Paduka, antara 00:00–23:59.",
		"val.start_day":  "Tanggal awal periode harus 2–28, Paduka.",
		"val.persona":    "Persona \"{input}\" tak ada di katalog, Paduka. Pilihan: {list}.",
		"val.range":      "Rentang itu tak ada di buku agung, Paduka. Pakai: periode, hari, kemarin, minggu, bulan, atau YYYY-MM-DD..YYYY-MM-DD.",
		"val.tx_missing": "Catatan itu tak ada di peti, Paduka.",
		"val.no_conv":    "Tak ada percakapan yang sedang berjalan, Paduka.",
		"val.note":       "Catatan tak boleh melebihi 200 karakter, Paduka.",
	},
	Posesif: {
		"val.amount":     "Nominalnya nggak valid. Masukkan angka 1–{limit}. Ulang.",
		"val.date":       "Tanggalnya salah. Pakai {valid}. Jangan ngawur.",
		"val.cat_name":   "Nama kategori 1–32 karakter. Itu aja.",
		"val.cat_dup":    "Kategori {category} sudah ada. Jangan dobel.",
		"val.cat_max":    "Maksimal {max} kategori aktif per tipe. Sudah penuh.",
		"val.cat_empty":  "Belum ada kategori aktif. Tambah lewat /kategori dulu.",
		"val.time":       "Formatnya HH:MM, 00:00–23:59. Ulang.",
		"val.start_day":  "Tanggal awalnya 2–28. Itu aja.",
		"val.persona":    "\"{input}\" bukan persona. Pilihannya cuma {list}.",
		"val.range":      "Rentang apa itu? Pakai: periode, hari, kemarin, minggu, bulan, atau YYYY-MM-DD..YYYY-MM-DD.",
		"val.tx_missing": "Transaksinya nggak ada. Mungkin sudah dihapus.",
		"val.no_conv":    "Nggak ada percakapan aktif.",
		"val.note":       "Catatan maks 200 karakter. Potong.",
	},
	Softboy: {
		"val.amount":     "Nominalnya belum aku mengerti. Coba angka 1–{limit} ya!",
		"val.date":       "Tanggalnya belum aku kenal. Pakai {valid} ya!",
		"val.cat_name":   "Nama kategorinya 1–32 karakter ya!",
		"val.cat_dup":    "Kategori {category} sudah ada, kok. Pakai yang itu aja ya!",
		"val.cat_max":    "Kategori aktifnya sudah {max} per tipe, ya. Batasnya segitu!",
		"val.cat_empty":  "Belum ada kategori aktif, ya. Tambah dulu lewat /kategori!",
		"val.time":       "Jamnya pakai format HH:MM ya, 00:00–23:59!",
		"val.start_day":  "Tanggal awal periodenya 2–28 ya!",
		"val.persona":    "Persona \"{input}\" belum ada, ya. Pilihannya {list}!",
		"val.range":      "Rentangnya belum aku kenal. Pakai: periode, hari, kemarin, minggu, bulan, atau YYYY-MM-DD..YYYY-MM-DD!",
		"val.tx_missing": "Transaksinya nggak ketemu, ya.",
		"val.no_conv":    "Nggak ada percakapan yang jalan, kok.",
		"val.note":       "Catatannya maks 200 karakter, ya!",
	},
}

// ValidationKeys returns every validation message key, sorted.
func ValidationKeys() []string {
	set := map[string]bool{}
	for _, texts := range validation {
		for k := range texts {
			set[k] = true
		}
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// HasValidation reports whether key is a validation message key.
func HasValidation(key string) bool {
	_, ok := validation[Netral][key]
	return ok
}

// Validation renders the validation message key in persona id. Same contract
// as Render: an unknown persona falls back to Netral, an unknown key returns
// "", and a missing variable stays literal so the bug is visible.
func Validation(id ID, key string, vars map[string]string) string {
	return render(validation, id, key, vars)
}

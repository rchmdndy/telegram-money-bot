package persona

import "sort"

// prompts holds the "type a value" prompts the conversation machine sends
// while it waits for input that no button can supply (a new category name, a
// period start day, a reminder time).
//
// It is a third table, next to catalog and validation, for one reason: PRD
// §4.12 rule 2 requires every user-facing string to live in this package (a
// test fails if package bot writes one), while the catalog is pinned to the 35
// handler keys of the PRD §4.12 table. A prompt is neither a catalog reply nor
// a validation error, so it gets its own table instead of widening either.
var prompts = map[ID]map[string]string{
	Netral: {
		"prompt.cat_name":         "Ketik nama kategori baru (1–32 karakter), atau /batal.",
		"prompt.cat_rename_pick":  "Pilih kategori yang ingin diganti namanya:",
		"prompt.cat_rename_name":  "Ketik nama baru untuk {category} (1–32 karakter), atau /batal.",
		"prompt.cat_off_pick":     "Pilih kategori yang ingin dinonaktifkan:",
		"prompt.period_start_day": "Ketik tanggal awal periode (2–28), atau /batal.",
		"prompt.reminder_time":    "Ketik jam reminder (HH:MM), atau /batal.",
		"prompt.persona":          "Persona aktif: {persona}.",
		"prompt.cat_kind":         "Pilih tipe kategori baru:",
	},
	Penjaga: {
		"prompt.cat_name":         "Sebutkan nama kategori baru, Paduka (1–32 karakter), atau /batal.",
		"prompt.cat_rename_pick":  "Kategori manakah yang hendak Paduka ganti namanya?",
		"prompt.cat_rename_name":  "Sebutkan nama baru untuk {category}, Paduka (1–32 karakter), atau /batal.",
		"prompt.cat_off_pick":     "Kategori manakah yang hendak Paduka nonaktifkan?",
		"prompt.period_start_day": "Sebutkan tanggal awal periode, Paduka (2–28), atau /batal.",
		"prompt.reminder_time":    "Sebutkan pukul berapa pengingat kerajaan dibunyikan (HH:MM), atau /batal.",
		"prompt.persona":          "Persona yang kujunjung saat ini, Paduka: {persona}.",
		"prompt.cat_kind":         "Kategori ini untuk pengeluaran atau pemasukan, Paduka?",
	},
	Posesif: {
		"prompt.cat_name":         "Nama kategori barunya apa? 1–32 karakter. Atau /batal.",
		"prompt.cat_rename_pick":  "Mau ganti nama kategori yang mana?",
		"prompt.cat_rename_name":  "Nama baru buat {category}? 1–32 karakter. Atau /batal.",
		"prompt.cat_off_pick":     "Kategori mana yang mau dimatiin?",
		"prompt.period_start_day": "Tanggal awal periodenya berapa? 2–28. Atau /batal.",
		"prompt.reminder_time":    "Jam berapa reminder-nya? Format HH:MM. Atau /batal.",
		"prompt.persona":          "Persona aktifku sekarang: {persona}.",
		"prompt.cat_kind":         "Kategori barunya buat pengeluaran atau pemasukan?",
	},
	Softboy: {
		"prompt.cat_name":         "Nama kategori barunya apa ya? 1–32 karakter. Atau /batal!",
		"prompt.cat_rename_pick":  "Mau ganti nama kategori yang mana nih?",
		"prompt.cat_rename_name":  "Nama baru untuk {category} apa ya? 1–32 karakter. Atau /batal!",
		"prompt.cat_off_pick":     "Kategori mana yang mau aku nonaktifkan?",
		"prompt.period_start_day": "Tanggal awal periodenya berapa ya? 2–28. Atau /batal!",
		"prompt.reminder_time":    "Jam reminder-nya berapa? Format HH:MM ya. Atau /batal!",
		"prompt.persona":          "Sekarang aku jadi {persona}, ya!",
		"prompt.cat_kind":         "Kategorinya buat pengeluaran atau pemasukan ya?",
	},
}

// PromptKeys returns every prompt key, sorted.
func PromptKeys() []string {
	set := map[string]bool{}
	for _, texts := range prompts {
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

// HasPrompt reports whether key is a prompt key.
func HasPrompt(key string) bool {
	_, ok := prompts[Netral][key]
	return ok
}

// Prompt renders the prompt key in persona id. Same contract as Render.
//
// The prompts table is consulted first; a key it does not define falls back to
// the catalog. The fallback is required because the PRD §4.12 ownership table
// pins three prompts (tx.prompt.date, tx.prompt.amount, tx.prompt.note) to the
// catalog, and h.ask renders every prompt through this function. Without it
// those three render as "" and Telegram rejects the message with
// "Bad Request: message text is empty".
func Prompt(id ID, key string, vars map[string]string) string {
	if HasPrompt(key) {
		return render(prompts, id, key, vars)
	}
	return Render(id, key, vars)
}

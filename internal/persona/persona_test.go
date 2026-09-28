package persona

import (
	"os"
	"strings"
	"testing"
)

// requiredKeys is the PRD §4.12 key ownership table: every persona must
// define all of these, and every one must be non-empty (PRD §6 #10, §10 risk
// "katalog persona tidak lengkap → pesan kosong di produksi").
var requiredKeys = []string{
	"welcome",
	"tx.saved",
	"tx.updated",
	"tx.deleted",
	"tx.cancelled",
	"tx.prompt.amount",
	"tx.prompt.note",
	"tx.prompt.date",
	"tx.confirm.header",
	"tx.edit.confirm.header",
	"reminder.empty",
	"reminder.summary",
	"rekap.header",
	"rekap.empty",
	"rekap.no_income",
	"category.saved",
	"category.renamed",
	"category.deactivated",
	"period.status",
	"period.saved",
	"settings.summary",
	"reminder.set",
	"reminder.off",
	"reminder.on",
	"reminder.past",
	"persona.changed",
	"list.header",
	"help.list",
	"category.list",
	"error.generic",
	"idle.hint",
	"quick.format",
	"quick.kind",
	"quick.category",
	"quick.note_long",
}

func TestCatalogHasEveryRequiredKey(t *testing.T) {
	if len(requiredKeys) != 35 {
		t.Fatalf("requiredKeys has %d entries, the PRD table lists 35", len(requiredKeys))
	}
	for _, id := range IDs() {
		texts, ok := catalog[id]
		if !ok {
			t.Errorf("persona %q missing from catalog", id)
			continue
		}
		for _, key := range requiredKeys {
			v, ok := texts[key]
			if !ok {
				t.Errorf("persona %q missing key %q", id, key)
				continue
			}
			if strings.TrimSpace(v) == "" {
				t.Errorf("persona %q key %q is empty", id, key)
			}
		}
	}
}

// TestCatalogHasNoExtraKeys keeps the catalog exactly as wide as the PRD
// table: an undeclared key would be dead weight no handler ever renders.
func TestCatalogHasNoExtraKeys(t *testing.T) {
	want := map[string]bool{}
	for _, k := range requiredKeys {
		want[k] = true
	}
	for _, id := range IDs() {
		for k := range catalog[id] {
			if !want[k] {
				t.Errorf("persona %q defines undeclared key %q", id, k)
			}
		}
	}
}

func TestCatalogCoversEveryPersonaID(t *testing.T) {
	if len(catalog) != len(IDs()) {
		t.Fatalf("catalog has %d personas, IDs() lists %d", len(catalog), len(IDs()))
	}
	for _, id := range IDs() {
		if _, ok := catalog[id]; !ok {
			t.Errorf("IDs() lists %q but the catalog does not define it", id)
		}
	}
}

func TestValidAndDefault(t *testing.T) {
	for _, id := range IDs() {
		if !Valid(id) {
			t.Errorf("Valid(%q) = false", id)
		}
	}
	for _, bad := range []ID{"", "xxx", "Netral", "penjaga ", "admin"} {
		if Valid(bad) {
			t.Errorf("Valid(%q) = true, want false", bad)
		}
	}
	if Default() != Penjaga {
		t.Errorf("Default() = %q, want penjaga", Default())
	}
	if !Valid(Default()) {
		t.Errorf("Default() = %q is not a valid persona", Default())
	}
}

func TestLabelsCoverEveryID(t *testing.T) {
	labels := Labels()
	for _, id := range IDs() {
		if strings.TrimSpace(labels[id]) == "" {
			t.Errorf("persona %q has no label", id)
		}
	}
	if len(labels) != len(IDs()) {
		t.Errorf("Labels() has %d entries, want %d", len(labels), len(IDs()))
	}
}

func TestKeysMatchesCatalog(t *testing.T) {
	keys := Keys()
	if len(keys) != len(requiredKeys) {
		t.Fatalf("Keys() returned %d keys, want %d", len(keys), len(requiredKeys))
	}
	for i := 1; i < len(keys); i++ {
		if keys[i-1] >= keys[i] {
			t.Fatalf("Keys() not sorted at %d: %q >= %q", i, keys[i-1], keys[i])
		}
	}
	for _, k := range requiredKeys {
		if !Has(k) {
			t.Errorf("Has(%q) = false", k)
		}
	}
	if Has("nope.nope") {
		t.Error(`Has("nope.nope") = true`)
	}
}

// TestEveryPlaceholderIsDeclared enforces PRD §4.12: an undeclared token is a
// program error, not raw output to the user.
func TestEveryPlaceholderIsDeclared(t *testing.T) {
	declared := map[string]bool{}
	for _, name := range Tokens() {
		declared[name] = true
	}
	for _, id := range IDs() {
		for key, tpl := range catalog[id] {
			for _, m := range tokenRe.FindAllString(tpl, -1) {
				name := m[1 : len(m)-1]
				if !declared[name] {
					t.Errorf("persona %q key %q uses undeclared token %s", id, key, m)
				}
			}
		}
	}
}

// TestValidationTokensAreDeclared documents PRD §4.12: the validation tokens
// ({limit}, {max}, {start_day}, {list}, {valid}, {note}, {pct}, {remaining},
// {day}, {date_short}) are used by persona-wrapped validation messages and are
// deliberately absent from the catalog table, but they must still be declared
// so a handler can never invent a token the renderer would leave literal.
func TestValidationTokensAreDeclared(t *testing.T) {
	declared := map[string]bool{}
	for _, name := range Tokens() {
		declared[name] = true
	}
	for _, name := range []string{
		"limit", "max", "start_day", "list", "valid",
		"note", "pct", "remaining", "day", "date_short",
	} {
		if !declared[name] {
			t.Errorf("validation token %q is not declared in tokens.go", name)
		}
	}
}

func TestTokensReturnsCopy(t *testing.T) {
	got := Tokens()
	if len(got) == 0 {
		t.Fatal("Tokens() returned nothing")
	}
	got[0] = "mutated"
	if Tokens()[0] == "mutated" {
		t.Fatal("Tokens() leaks the internal slice")
	}
}

// TestRenderLeavesNoPlaceholder renders every key with a value for every
// declared token and asserts nothing in braces survives (PRD §6 #11).
func TestRenderLeavesNoPlaceholder(t *testing.T) {
	vars := map[string]string{}
	for _, name := range Tokens() {
		vars[name] = "X" + name
	}
	for _, id := range IDs() {
		for _, key := range Keys() {
			out := Render(id, key, vars)
			if out == "" {
				t.Errorf("Render(%q, %q) returned empty", id, key)
				continue
			}
			if strings.Contains(out, "{") || strings.Contains(out, "}") {
				t.Errorf("Render(%q, %q) left a placeholder: %q", id, key, out)
			}
		}
	}
}

// TestRenderUnknownIDFallsBackToNetral covers PRD §6 #12: a hand-edited DB
// holding an unknown persona value must render netral, never panic.
func TestRenderUnknownIDFallsBackToNetral(t *testing.T) {
	for _, bad := range []ID{"", "xxx", "Netral"} {
		got := Render(bad, "tx.cancelled", nil)
		want := catalog[Netral]["tx.cancelled"]
		if got != want {
			t.Errorf("Render(%q, tx.cancelled) = %q, want the netral text %q", bad, got, want)
		}
	}
	// Validation keys too: the fallback must cover every key, not a subset.
	for _, key := range Keys() {
		if got := Render("nope", key, nil); got == "" {
			t.Errorf("Render with unknown id returned empty for key %q", key)
		}
	}
}

func TestRenderUnknownKeyReturnsEmpty(t *testing.T) {
	if got := Render(Netral, "does.not.exist", nil); got != "" {
		t.Fatalf("Render unknown key = %q, want empty", got)
	}
}

// TestRenderMissingTokenStaysLiteral documents the debug behaviour: a missing
// variable is left as the literal {token} so the bug is visible in a test.
func TestRenderMissingTokenStaysLiteral(t *testing.T) {
	got := Render(Netral, "tx.saved", map[string]string{"category": "Makan"})
	if !strings.Contains(got, "{amount}") {
		t.Fatalf("missing token not left literal: %q", got)
	}
	if !strings.Contains(got, "Makan") {
		t.Fatalf("supplied token not substituted: %q", got)
	}
}

// TestNumbersAreVerbatimAcrossPersonas is acceptance criterion #21 at the
// catalog level: a value substituted for a token must survive every persona
// byte for byte, never reworded (PRD §4.12 rule 1). Only the tokens the
// template actually references are asserted, since each key uses a subset.
func TestNumbersAreVerbatimAcrossPersonas(t *testing.T) {
	vars := map[string]string{
		"amount": "Rp 17.000",
		"date":   "Sabtu, 26 Sep 2026",
		"pct":    "13,8%",
		"total":  "Rp 414.700",
		"count":  "4",
		"period": "Gaji",
		"start":  "21 Sep 2026",
		"end":    "20 Okt 2026",
		"time":   "19:00",
	}
	for _, id := range IDs() {
		for _, key := range Keys() {
			tpl := catalog[id][key]
			out := Render(id, key, vars)
			for _, m := range tokenRe.FindAllString(tpl, -1) {
				name := m[1 : len(m)-1]
				want, ok := vars[name]
				if !ok {
					continue
				}
				if !strings.Contains(out, want) {
					t.Errorf("persona %q key %q reworded %s: got %q, want it to contain %q",
						id, key, m, out, want)
				}
			}
		}
	}
}

// TestReminderDateIsVerbatim pins the §4.12 tone samples: only the wrapper
// sentence changes with the persona, {date} stays exactly as supplied.
func TestReminderDateIsVerbatim(t *testing.T) {
	vars := map[string]string{"date": "Sabtu, 26 Sep"}
	for _, id := range IDs() {
		if got := Render(id, "reminder.summary", vars); !strings.Contains(got, "Sabtu, 26 Sep") {
			t.Errorf("persona %q reminder.summary dropped the date: %q", id, got)
		}
	}
}

// TestPersonaIDsMatchDatabaseCheck parses the migration that constrains
// settings.persona and asserts it lists exactly the catalog IDs (PRD §5.3
// note: "Katalog di kode dan daftar di CHECK harus sama").
func TestPersonaIDsMatchDatabaseCheck(t *testing.T) {
	const path = "../storage/migrations/0001_init.sql"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	sql := string(data)
	const marker = "CHECK (persona IN ("
	i := strings.Index(sql, marker)
	if i < 0 {
		t.Fatalf("%s has no persona CHECK constraint", path)
	}
	rest := sql[i+len(marker):]
	j := strings.Index(rest, ")")
	if j < 0 {
		t.Fatalf("unterminated persona CHECK in %s", path)
	}
	var dbIDs []string
	for _, part := range strings.Split(rest[:j], ",") {
		dbIDs = append(dbIDs, strings.Trim(strings.TrimSpace(part), "'"))
	}
	if len(dbIDs) != len(IDs()) {
		t.Fatalf("DB CHECK lists %v, catalog lists %v", dbIDs, IDs())
	}
	for i, id := range IDs() {
		if dbIDs[i] != string(id) {
			t.Errorf("DB CHECK[%d] = %q, catalog ID = %q", i, dbIDs[i], id)
		}
	}
}

// promptsTableKeys is every key the prompts table itself must define.
var promptsTableKeys = []string{
	"prompt.cat_kind",
	"prompt.cat_name",
	"prompt.cat_off_pick",
	"prompt.cat_rename_name",
	"prompt.cat_rename_pick",
	"prompt.period_start_day",
	"prompt.persona",
	"prompt.reminder_time",
}

// promptKeys is every key h.ask passes to Prompt: the prompts-table keys plus
// the three tx.prompt.* keys the PRD §4.12 ownership table pins to the catalog.
var promptKeys = append(append([]string{}, promptsTableKeys...),
	"tx.prompt.amount",
	"tx.prompt.date",
	"tx.prompt.note",
)

// validationKeys is every key the state machine (PRD §4.11) and the field
// validations (PRD §4.2, §4.4, §4.5, §4.8, §4.13.4) pass to Validation.
var validationKeys = []string{
	"val.amount",
	"val.cat_dup",
	"val.cat_empty",
	"val.cat_max",
	"val.cat_name",
	"val.date",
	"val.no_conv",
	"val.note",
	"val.persona",
	"val.range",
	"val.start_day",
	"val.time",
	"val.tx_missing",
}

// assertTableComplete checks one persona table is exactly as wide as the keys
// the handlers use: every persona defines every key, non-empty, and no persona
// carries a key no handler renders. A key present in Netral but missing from
// one persona is the empty-message failure class, since render falls back per
// table, never per key.
func assertTableComplete(t *testing.T, name string, table map[ID]map[string]string, keys []string) {
	t.Helper()
	want := map[string]bool{}
	for _, k := range keys {
		want[k] = true
	}
	for _, id := range IDs() {
		texts, ok := table[id]
		if !ok {
			t.Errorf("%s has no entry for persona %q", name, id)
			continue
		}
		for _, key := range keys {
			v, ok := texts[key]
			if !ok {
				t.Errorf("%s persona %q missing key %q", name, id, key)
				continue
			}
			if strings.TrimSpace(v) == "" {
				t.Errorf("%s persona %q key %q is empty", name, id, key)
			}
		}
		for key := range texts {
			if !want[key] {
				t.Errorf("%s persona %q defines undeclared key %q", name, id, key)
			}
		}
	}
}

// TestPromptsTableIsComplete covers the table whose incompleteness shipped as
// a production outage: every persona must define every prompt the handlers
// send, or the bot calls Telegram with an empty message text.
func TestPromptsTableIsComplete(t *testing.T) {
	assertTableComplete(t, "prompts", prompts, promptsTableKeys)
}

// TestValidationTableIsComplete is the same guard for the rejection messages.
func TestValidationTableIsComplete(t *testing.T) {
	assertTableComplete(t, "validation", validation, validationKeys)
}

// TestPromptKeysMatchTable keeps PromptKeys() and HasPrompt() in step with the
// keys the handlers actually use.
func TestPromptKeysMatchTable(t *testing.T) {
	got := PromptKeys()
	if len(got) != len(promptsTableKeys) {
		t.Fatalf("PromptKeys() returned %d keys, want %d: %v", len(got), len(promptsTableKeys), got)
	}
	for i := 1; i < len(got); i++ {
		if got[i-1] >= got[i] {
			t.Fatalf("PromptKeys() not sorted at %d: %q >= %q", i, got[i-1], got[i])
		}
	}
	for _, k := range promptsTableKeys {
		if !HasPrompt(k) {
			t.Errorf("HasPrompt(%q) = false", k)
		}
	}
	if HasPrompt("nope.nope") {
		t.Error(`HasPrompt("nope.nope") = true`)
	}
}

// TestValidationKeysMatchTable is the same guard for the validation table.
func TestValidationKeysMatchTable(t *testing.T) {
	got := ValidationKeys()
	if len(got) != len(validationKeys) {
		t.Fatalf("ValidationKeys() returned %d keys, want %d: %v", len(got), len(validationKeys), got)
	}
	for i := 1; i < len(got); i++ {
		if got[i-1] >= got[i] {
			t.Fatalf("ValidationKeys() not sorted at %d: %q >= %q", i, got[i-1], got[i])
		}
	}
	for _, k := range validationKeys {
		if !HasValidation(k) {
			t.Errorf("HasValidation(%q) = false", k)
		}
	}
	if HasValidation("nope.nope") {
		t.Error(`HasValidation("nope.nope") = true`)
	}
}

// TestPromptFallsBackToCatalog pins the fix for the outage: h.ask renders every
// prompt through Prompt, and the PRD §4.12 ownership table pins three prompts
// to the catalog. When Prompt consulted only the prompts table those three
// rendered "" and Telegram rejected the send with
// "Bad Request: message text is empty", killing the whole add-transaction flow.
func TestPromptFallsBackToCatalog(t *testing.T) {
	for _, key := range []string{"tx.prompt.date", "tx.prompt.amount", "tx.prompt.note"} {
		if HasPrompt(key) {
			t.Fatalf("%q now lives in the prompts table; move it out of this test's assumptions", key)
		}
		if !Has(key) {
			t.Fatalf("%q is not a catalog key either", key)
		}
		for _, id := range IDs() {
			if got := Prompt(id, key, nil); got == "" {
				t.Errorf("Prompt(%q, %q) = \"\", want the catalog text", id, key)
			}
		}
	}
	// Every key a handler can ask for must render, for every persona.
	for _, id := range IDs() {
		for _, key := range promptKeys {
			if got := Prompt(id, key, nil); strings.TrimSpace(got) == "" {
				t.Errorf("Prompt(%q, %q) = %q", id, key, got)
			}
		}
	}
}

// TestPromptAndValidationLeaveNoPlaceholder is the prompts/validation half of
// TestRenderLeavesNoPlaceholder.
func TestPromptAndValidationLeaveNoPlaceholder(t *testing.T) {
	vars := map[string]string{}
	for _, name := range Tokens() {
		vars[name] = "X" + name
	}
	for _, id := range IDs() {
		for _, key := range promptKeys {
			out := Prompt(id, key, vars)
			if strings.Contains(out, "{") || strings.Contains(out, "}") {
				t.Errorf("Prompt(%q, %q) left a placeholder: %q", id, key, out)
			}
		}
		for _, key := range validationKeys {
			out := Validation(id, key, vars)
			if strings.Contains(out, "{") || strings.Contains(out, "}") {
				t.Errorf("Validation(%q, %q) left a placeholder: %q", id, key, out)
			}
		}
	}
}

// TestPromptAndValidationDeclareEveryToken extends
// TestEveryPlaceholderIsDeclared to the two tables that test does not read.
func TestPromptAndValidationDeclareEveryToken(t *testing.T) {
	declared := map[string]bool{}
	for _, name := range Tokens() {
		declared[name] = true
	}
	for tableName, table := range map[string]map[ID]map[string]string{"prompts": prompts, "validation": validation} {
		for id, texts := range table {
			for key, tpl := range texts {
				for _, m := range tokenRe.FindAllString(tpl, -1) {
					if token := m[1 : len(m)-1]; !declared[token] {
						t.Errorf("%s persona %q key %q uses undeclared token %s", tableName, id, key, m)
					}
				}
			}
		}
	}
}

// TestPromptAndValidationFallBackToNetral covers PRD §6 #12 for the two tables
// TestRenderUnknownIDFallsBackToNetral does not read.
func TestPromptAndValidationFallBackToNetral(t *testing.T) {
	for _, bad := range []ID{"", "xxx", "Netral"} {
		for _, key := range promptKeys {
			want := Prompt(Netral, key, nil)
			if got := Prompt(bad, key, nil); got != want {
				t.Errorf("Prompt(%q, %q) = %q, want the netral text %q", bad, key, got, want)
			}
		}
		for _, key := range validationKeys {
			want := Validation(Netral, key, nil)
			if got := Validation(bad, key, nil); got != want {
				t.Errorf("Validation(%q, %q) = %q, want the netral text %q", bad, key, got, want)
			}
		}
	}
}

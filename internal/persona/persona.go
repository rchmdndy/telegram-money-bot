// Package persona holds the hardcoded persona catalog (PRD §4.12) and
// renders user-facing strings with {token} placeholders. Pure data +
// rendering: no database, no Telegram, no filesystem, no network.
package persona

import "sort"

// ID identifies one persona in the catalog.
type ID string

const (
	Netral  ID = "netral"
	Penjaga ID = "penjaga"
	Posesif ID = "posesif"
	Softboy ID = "softboy"
)

// IDs returns the 4 valid persona IDs in display order.
func IDs() []ID {
	return []ID{Netral, Penjaga, Posesif, Softboy}
}

// Valid reports whether id is one of the 4 catalog IDs.
func Valid(id ID) bool {
	switch id {
	case Netral, Penjaga, Posesif, Softboy:
		return true
	}
	return false
}

// Default is the default persona for new users.
func Default() ID { return Penjaga }

// Labels returns the human-readable name of a persona.
func Labels() map[ID]string {
	return map[ID]string{
		Netral:  "Netral",
		Penjaga: "Penjaga Harta Karun",
		Posesif: "Cowok Posesif",
		Softboy: "Softboy Manja",
	}
}

// Keys returns all required message keys, sorted.
func Keys() []string {
	set := map[string]bool{}
	for _, texts := range catalog {
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

// Has reports whether key exists in the catalog.
func Has(key string) bool {
	_, ok := catalog[Netral][key]
	return ok
}

// Render returns the text for key rendered in persona id, with every
// {token} replaced by vars[token]. Unknown id falls back to Netral
// (never panics). Unknown key returns "" (callers must not hit this;
// a test asserts key existence).
func Render(id ID, key string, vars map[string]string) string {
	return render(catalog, id, key, vars)
}

// render is the shared body of Render and Validation: table lookup with
// Netral fallback, then one pass of {token} substitution.
func render(table map[ID]map[string]string, id ID, key string, vars map[string]string) string {
	texts, ok := table[id]
	if !ok {
		texts = table[Netral]
	}
	tpl, ok := texts[key]
	if !ok {
		return ""
	}
	return tokenRe.ReplaceAllStringFunc(tpl, func(m string) string {
		name := m[1 : len(m)-1]
		if v, ok := vars[name]; ok {
			return v
		}
		return m
	})
}

package persona

import "regexp"

// tokenRe matches {placeholder} names in catalog templates.
var tokenRe = regexp.MustCompile(`\{([a-z_]+)\}`)

// tokens is every placeholder name allowed anywhere in the catalog
// (PRD §4.12 token table: catalog tokens + validation tokens).
var tokens = []string{
	"amount",
	"date",
	"date_short",
	"day",
	"category",
	"note",
	"kind",
	"count",
	"total",
	"period",
	"start",
	"end",
	"day_no",
	"day_total",
	"pct",
	"remaining",
	"start_day",
	"time",
	"persona",
	"list",
	"input",
	"limit",
	"max",
	"valid",
	"title",
}

// Tokens returns every {...} placeholder name allowed anywhere in the catalog.
func Tokens() []string {
	out := make([]string, len(tokens))
	copy(out, tokens)
	return out
}

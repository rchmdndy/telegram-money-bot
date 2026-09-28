// Package config loads bot configuration from the environment (PRD §5.5).
package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	defaultDBPath   = "./moneybot.db"
	defaultTimezone = "Asia/Jakarta"
	defaultLogLevel = "info"
)

var validLogLevels = []string{"debug", "info", "warn", "error"}

// Config holds the runtime configuration of the bot.
type Config struct {
	BotToken string
	DBPath   string
	Timezone string
	LogLevel string
	Location *time.Location
}

// ValidLogLevels returns the accepted LOG_LEVEL values.
func ValidLogLevels() []string {
	out := make([]string, len(validLogLevels))
	copy(out, validLogLevels)
	return out
}

// Load reads configuration from the environment, applying defaults, and
// validates it. Returns an error for a missing/invalid bot token, an unknown
// timezone, or an invalid log level.
func Load() (*Config, error) {
	return LoadFrom(os.LookupEnv)
}

// LoadFrom is Load with an injectable lookup func (for tests): get(key) (string, bool).
func LoadFrom(get func(string) (string, bool)) (*Config, error) {
	token, ok := get("TELEGRAM_BOT_TOKEN")
	// The error message must never contain the token value itself.
	if !ok || strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("config: TELEGRAM_BOT_TOKEN wajib diisi (dari @BotFather)")
	}

	dbPath := defaultDBPath
	if v, ok := get("DB_PATH"); ok && strings.TrimSpace(v) != "" {
		dbPath = v
	}

	tz := defaultTimezone
	if v, ok := get("TZ"); ok && strings.TrimSpace(v) != "" {
		tz = v
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, fmt.Errorf("config: zona waktu TZ %q tidak dikenal: %w", tz, err)
	}

	logLevel := defaultLogLevel
	if v, ok := get("LOG_LEVEL"); ok && strings.TrimSpace(v) != "" {
		logLevel = strings.ToLower(v)
	}
	if !validLogLevel(logLevel) {
		return nil, fmt.Errorf("config: LOG_LEVEL %q tidak valid (pilihan: %s)",
			logLevel, strings.Join(validLogLevels, ", "))
	}

	return &Config{
		BotToken: token,
		DBPath:   dbPath,
		Timezone: tz,
		LogLevel: logLevel,
		Location: loc,
	}, nil
}

func validLogLevel(level string) bool {
	for _, v := range validLogLevels {
		if v == level {
			return true
		}
	}
	return false
}

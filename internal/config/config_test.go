package config

import (
	"strings"
	"testing"
)

func fakeEnv(vars map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		v, ok := vars[key]
		return v, ok
	}
}

func TestLoadFromDefaults(t *testing.T) {
	cfg, err := LoadFrom(fakeEnv(map[string]string{
		"TELEGRAM_BOT_TOKEN": "123:ABC",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BotToken != "123:ABC" {
		t.Errorf("BotToken = %q, want %q", cfg.BotToken, "123:ABC")
	}
	if cfg.DBPath != "./moneybot.db" {
		t.Errorf("DBPath = %q, want %q", cfg.DBPath, "./moneybot.db")
	}
	if cfg.Timezone != "Asia/Jakarta" {
		t.Errorf("Timezone = %q, want %q", cfg.Timezone, "Asia/Jakarta")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
	if cfg.Location == nil {
		t.Fatal("Location is nil")
	}
	if cfg.Location.String() != "Asia/Jakarta" {
		t.Errorf("Location = %q, want %q", cfg.Location.String(), "Asia/Jakarta")
	}
}

func TestLoadFromMissingToken(t *testing.T) {
	tokenValue := "SUPER-SECRET-TOKEN"

	// Missing entirely.
	if _, err := LoadFrom(fakeEnv(map[string]string{})); err == nil {
		t.Fatal("expected error for missing token, got nil")
	}

	// Empty value: error must not leak the token value.
	cfg, err := LoadFrom(fakeEnv(map[string]string{
		"TELEGRAM_BOT_TOKEN": "",
	}))
	if err == nil {
		t.Fatal("expected error for empty token, got nil")
	}
	if cfg != nil {
		t.Errorf("expected nil config on error, got %+v", cfg)
	}
	if strings.Contains(err.Error(), tokenValue) {
		t.Errorf("error message must not contain token value: %q", err.Error())
	}

	// Whitespace-only token is rejected too.
	if _, err := LoadFrom(fakeEnv(map[string]string{
		"TELEGRAM_BOT_TOKEN": "   ",
	})); err == nil {
		t.Fatal("expected error for whitespace token, got nil")
	}
}

func TestLoadFromCustomValues(t *testing.T) {
	cfg, err := LoadFrom(fakeEnv(map[string]string{
		"TELEGRAM_BOT_TOKEN": "123:ABC",
		"DB_PATH":            "/tmp/bot.db",
		"TZ":                 "UTC",
		"LOG_LEVEL":          "warn",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DBPath != "/tmp/bot.db" {
		t.Errorf("DBPath = %q, want %q", cfg.DBPath, "/tmp/bot.db")
	}
	if cfg.Timezone != "UTC" {
		t.Errorf("Timezone = %q, want %q", cfg.Timezone, "UTC")
	}
	if cfg.LogLevel != "warn" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "warn")
	}
}

func TestLoadFromInvalidTZ(t *testing.T) {
	_, err := LoadFrom(fakeEnv(map[string]string{
		"TELEGRAM_BOT_TOKEN": "123:ABC",
		"TZ":                 "Not/AZone",
	}))
	if err == nil {
		t.Fatal("expected error for invalid TZ, got nil")
	}
}

func TestLoadFromInvalidLogLevel(t *testing.T) {
	_, err := LoadFrom(fakeEnv(map[string]string{
		"TELEGRAM_BOT_TOKEN": "123:ABC",
		"LOG_LEVEL":          "verbose",
	}))
	if err == nil {
		t.Fatal("expected error for invalid LOG_LEVEL, got nil")
	}
}

func TestLoadFromLogLevelCaseInsensitive(t *testing.T) {
	cfg, err := LoadFrom(fakeEnv(map[string]string{
		"TELEGRAM_BOT_TOKEN": "123:ABC",
		"LOG_LEVEL":          "DEBUG",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}
}

func TestValidLogLevels(t *testing.T) {
	levels := ValidLogLevels()
	want := []string{"debug", "info", "warn", "error"}
	if len(levels) != len(want) {
		t.Fatalf("ValidLogLevels() = %v, want %v", levels, want)
	}
	for i := range want {
		if levels[i] != want[i] {
			t.Fatalf("ValidLogLevels() = %v, want %v", levels, want)
		}
	}
}

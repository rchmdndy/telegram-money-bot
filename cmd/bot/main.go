// Command bot runs the Telegram money bot: long polling on one goroutine and
// the reminder scheduler on another, both stopped by SIGINT/SIGTERM.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	// Embed the timezone database so TZ resolution works in images without
	// /usr/share/zoneinfo (PRD §5.5).
	_ "time/tzdata"

	telegram "github.com/go-telegram/bot"

	"github.com/rchmdndy/telegram-money-bot/internal/bot"
	"github.com/rchmdndy/telegram-money-bot/internal/config"
	"github.com/rchmdndy/telegram-money-bot/internal/scheduler"
	"github.com/rchmdndy/telegram-money-bot/internal/storage"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "moneybot: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := newLogger(cfg.LogLevel)
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := storage.Open(ctx, cfg.DBPath)
	if err != nil {
		return fmt.Errorf("membuka database %s: %w", cfg.DBPath, err)
	}
	defer db.Close()
	log.Info("database siap", "path", cfg.DBPath)

	// bot.New calls getMe, so an invalid token fails here instead of on the
	// first user message.
	tg, err := telegram.New(cfg.BotToken)
	if err != nil {
		return fmt.Errorf("token bot ditolak: %w", err)
	}

	handler := bot.New(db, &bot.TelegramSender{Bot: tg}, cfg.Location, log)
	bot.Register(tg, handler)

	sched := scheduler.New(db, handler, cfg.Location, log)

	// The scheduler runs beside the long poll; whichever side fails first
	// cancels the shared context and stops the other.
	errs := make(chan error, 2)
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		log.Info("scheduler berjalan", "interval", scheduler.DefaultInterval.String(), "tz", cfg.Timezone)
		sched.Run(runCtx)
		errs <- nil
	}()
	go func() {
		log.Info("bot mulai long polling")
		tg.Start(runCtx)
		errs <- nil
	}()

	<-runCtx.Done()
	log.Info("berhenti", "sebab", context.Cause(runCtx))
	// Wait for both goroutines to unwind before the deferred db.Close.
	<-errs
	<-errs
	return nil
}

// newLogger builds the process logger at the configured level.
func newLogger(level string) *slog.Logger {
	var lv slog.Level
	switch level {
	case "debug":
		lv = slog.LevelDebug
	case "warn":
		lv = slog.LevelWarn
	case "error":
		lv = slog.LevelError
	default:
		lv = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lv}))
}

package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"cron-weather/internal/config"
	"cron-weather/internal/transport"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// TelegramBot implements both Consumer and Producer using Telegram Bot API.
type TelegramBot struct {
	bot *tgbotapi.BotAPI
	log *slog.Logger

	updates tgbotapi.UpdatesChannel
}

// NewTelegramBot initializes Telegram bot client using provided config.
func NewTelegramBot(cfg *config.Config, log *slog.Logger) (*TelegramBot, error) {
	bot, err := tgbotapi.NewBotAPI(cfg.TgBot.BotToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create tg bot: %v", err)
	}

	// Telegram client debug prints raw JSON responses (with \uXXXX for non-ASCII).
	// Keep it opt-in via env TG_DEBUG=true.
	bot.Debug = cfg.TgBot.Debug

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30
	u.AllowedUpdates = []string{"message", "channel_post"}
	updates := bot.GetUpdatesChan(u)

	return &TelegramBot{bot: bot, log: log, updates: updates}, nil
}

// Get starts receiving incoming commands and returns a channel of parsed CronJob events.
func (t *TelegramBot) Get(ctx context.Context) (<-chan transport.CronJob, error) {
	out := make(chan transport.CronJob)

	t.log.Info("telegram bot started", slog.String("username", t.bot.Self.UserName))

	go func() {
		defer close(out)
		defer t.bot.StopReceivingUpdates()

		for {
			select {
			case <-ctx.Done():
				return
			case upd, ok := <-t.updates:
				if !ok {
					return
				}
				job, ok := t.toCronJob(upd)
				if !ok {
					continue
				}
				out <- job
			}
		}
	}()

	return out, nil
}

// Send delivers a message to Telegram.
func (t *TelegramBot) Send(ctx context.Context, msg transport.Message) error {
	m := tgbotapi.NewMessage(msg.ChatID, msg.Text)
	_, err := t.bot.Send(m)
	if err != nil {
		return fmt.Errorf("tg send: %w", err)
	}
	return nil
}

func (t *TelegramBot) toCronJob(upd tgbotapi.Update) (transport.CronJob, bool) {
	msg := upd.Message
	if msg == nil {
		msg = upd.ChannelPost
	}
	if msg == nil {
		return transport.CronJob{}, false
	}

	if !msg.IsCommand() {
		return transport.CronJob{}, false
	}

	cmd := strings.ToLower(msg.Command())
	chatID := msg.Chat.ID

	args := strings.TrimSpace(msg.CommandArguments())

	return transport.CronJob{ChatID: chatID, Command: cmd, Args: args}, true
}

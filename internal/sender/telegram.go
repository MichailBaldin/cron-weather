package sender

import (
	"context"
	"fmt"
	"strings"

	"cron-weather/internal/config"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type TelegramSender struct {
	bot    *tgbotapi.BotAPI
	chatID int64
}

func NewTelegramSender(cfg config.Telegram) (*TelegramSender, error) {
	bot, err := tgbotapi.NewBotAPI(cfg.Token)
	if err != nil {
		return nil, fmt.Errorf("failed to create bot: %w", err)
	}
	return &TelegramSender{
		bot:    bot,
		chatID: cfg.ChatID,
	}, nil
}

func (t *TelegramSender) Send(ctx context.Context, messages []string) error {
	if len(messages) == 0 {
		return nil
	}
	text := strings.Join(messages, "\n\n")
	msg := tgbotapi.NewMessage(t.chatID, text)
	msg.ParseMode = "HTML" // можно использовать HTML-форматирование

	_, err := t.bot.Send(msg)
	if err != nil {
		return fmt.Errorf("send telegram message: %w", err)
	}
	return nil
}

package sender

import (
	"context"
	"fmt"
	"strings"

	"cron-weather/internal/config"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	// Telegram message hard limit is 4096 chars; keep some safety margin.
	telegramMsgLimit = 4000
)

// TelegramSender sends alerts to a single Telegram chat.
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

// Send joins messages and sends them to Telegram.
// It escapes HTML and splits the payload into multiple messages if needed.
func (t *TelegramSender) Send(ctx context.Context, messages []string) error {
	if len(messages) == 0 {
		return nil
	}

	// Escape user/API-provided text to avoid breaking HTML parse mode.
	escaped := make([]string, 0, len(messages))
	for _, m := range messages {
		escaped = append(escaped, escapeTelegramHTML(m))
	}

	text := strings.Join(messages, "\n\n")
	
	// Telegram has a message length limit, so we may need to split.
	parts := splitByLimit(text, telegramMsgLimit)

	for _, part := range parts {
		if err := t.sendOne(ctx, part); err != nil {
			return err
		}
	}

	return nil
}

// sendOne sends a single Telegram message part.
func (t *TelegramSender) sendOne(ctx context.Context, text string) error {
	msg := tgbotapi.NewMessage(t.chatID, text)
	msg.ParseMode = "HTML"

	// Respect cancellation between parts (tgbotapi doesn't accept ctx directly).
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	_, err := t.bot.Send(msg)
	if err != nil {
		return fmt.Errorf("send telegram message: %w", err)
	}
	return nil
}

// escapeTelegramHTML escapes minimal set of chars used by Telegram HTML parse mode.
func escapeTelegramHTML(s string) string {
	// Order matters: escape '&' first.
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// splitByLimit splits text into chunks not exceeding limit.
// It tries to split by paragraphs, then by lines, then falls back to hard split.
func splitByLimit(text string, limit int) []string {
	if len(text) <= limit {
		return []string{text}
	}

	// First try to split by double newlines (paragraphs).
	paras := strings.Split(text, "\n\n")
	var out []string
	var cur strings.Builder

	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}

	for _, p := range paras {
		// +2 for the paragraph separator we add back
		addLen := len(p)
		if cur.Len() > 0 {
			addLen += 2
		}

		if cur.Len()+addLen <= limit {
			if cur.Len() > 0 {
				cur.WriteString("\n\n")
			}
			cur.WriteString(p)
			continue
		}

		// If paragraph itself is too big, split it further.
		if cur.Len() > 0 {
			flush()
		}
		if len(p) <= limit {
			cur.WriteString(p)
			continue
		}

		// Try splitting big paragraph by lines.
		lines := strings.Split(p, "\n")
		for _, ln := range lines {
			add := len(ln)
			if cur.Len() > 0 {
				add += 1
			}

			if cur.Len()+add <= limit {
				if cur.Len() > 0 {
					cur.WriteString("\n")
				}
				cur.WriteString(ln)
				continue
			}

			if cur.Len() > 0 {
				flush()
			}

			// Hard split a very long single line.
			for len(ln) > limit {
				out = append(out, ln[:limit])
				ln = ln[limit:]
			}
			if len(ln) > 0 {
				cur.WriteString(ln)
			}
		}
	}

	flush()
	return out
}
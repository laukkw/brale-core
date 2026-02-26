// 本文件主要内容：实现 Telegram 发送通道。
package notify

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type TelegramSender struct {
	token   string
	chatID  int64
	client  *http.Client
	apiBase string
}

func NewTelegramSender(cfg TelegramConfig) (Sender, error) {
	if strings.TrimSpace(cfg.Token) == "" {
		return nil, fmt.Errorf("telegram token is required")
	}
	if cfg.ChatID == 0 {
		return nil, fmt.Errorf("telegram chat_id is required")
	}
	client := &http.Client{Timeout: 15 * time.Second}
	return &TelegramSender{
		token:   cfg.Token,
		chatID:  cfg.ChatID,
		client:  client,
		apiBase: "https://api.telegram.org",
	}, nil
}

func (s *TelegramSender) Send(ctx context.Context, msg Message) error {
	text, parseMode := formatTelegramMessage(msg)
	endpoint := fmt.Sprintf("%s/bot%s/sendMessage", s.apiBase, s.token)
	form := url.Values{}
	form.Set("chat_id", strconv.FormatInt(s.chatID, 10))
	form.Set("text", text)
	if parseMode != "" {
		form.Set("parse_mode", parseMode)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			return
		}
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram send failed: %s", strings.TrimSpace(string(body)))
	}
	return nil
}

func formatTelegramMessage(msg Message) (string, string) {
	html := strings.TrimSpace(msg.HTML)
	if html != "" {
		return formatTelegramHTML(html), "HTML"
	}
	text := strings.TrimSpace(msg.Plain)
	if text != "" {
		return text, ""
	}
	text = strings.TrimSpace(msg.Markdown)
	if text != "" {
		return text, ""
	}
	return msg.Title, ""
}

func formatTelegramHTML(input string) string {
	text := strings.TrimSpace(input)
	if text == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		"<h3>", "<b>",
		"</h3>", "</b>\n",
		"<h4>", "<b>",
		"</h4>", "</b>\n",
		"<p>", "",
		"</p>", "\n",
	)
	return strings.TrimSpace(replacer.Replace(text))
}

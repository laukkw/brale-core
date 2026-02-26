// 本文件主要内容：发送邮件通知消息。
package notify

import (
	"context"
	"fmt"
	"strings"

	"github.com/wneessen/go-mail"
)

type EmailSender struct {
	client *mail.Client
	from   string
	to     []string
}

func NewEmailSender(cfg EmailConfig) (Sender, error) {
	if strings.TrimSpace(cfg.SMTPHost) == "" {
		return nil, fmt.Errorf("smtp_host is required")
	}
	if cfg.SMTPPort <= 0 {
		return nil, fmt.Errorf("smtp_port is required")
	}
	if strings.TrimSpace(cfg.SMTPUser) == "" {
		return nil, fmt.Errorf("smtp_user is required")
	}
	if strings.TrimSpace(cfg.SMTPPass) == "" {
		return nil, fmt.Errorf("smtp_pass is required")
	}
	if strings.TrimSpace(cfg.From) == "" {
		return nil, fmt.Errorf("email from is required")
	}
	if len(cfg.To) == 0 {
		return nil, fmt.Errorf("email to is required")
	}
	client, err := mail.NewClient(
		cfg.SMTPHost,
		mail.WithPort(cfg.SMTPPort),
		mail.WithTLSPortPolicy(mail.TLSMandatory),
		mail.WithSMTPAuth(mail.SMTPAuthAutoDiscover),
		mail.WithUsername(cfg.SMTPUser),
		mail.WithPassword(cfg.SMTPPass),
	)
	if err != nil {
		return nil, err
	}
	return &EmailSender{
		client: client,
		from:   cfg.From,
		to:     append([]string{}, cfg.To...),
	}, nil
}

func (s *EmailSender) Send(ctx context.Context, msg Message) error {
	m := mail.NewMsg()
	if err := m.From(s.from); err != nil {
		return err
	}
	for _, addr := range s.to {
		if err := m.To(addr); err != nil {
			return err
		}
	}
	if msg.Title != "" {
		m.Subject(msg.Title)
	}
	body := strings.TrimSpace(msg.HTML)
	bodyType := mail.TypeTextHTML
	if body == "" {
		body = strings.TrimSpace(msg.Markdown)
		bodyType = mail.TypeTextPlain
	}
	if body == "" {
		body = strings.TrimSpace(msg.Plain)
	}
	if body == "" {
		body = msg.Title
	}
	m.SetBodyString(bodyType, body)
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.client.DialAndSend(m)
}

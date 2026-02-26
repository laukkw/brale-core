// 本文件主要内容：定义通知接口与消息结构。
package notify

import (
	"context"

	"brale-core/internal/config"
	"brale-core/internal/decision/decisionfmt"
	"brale-core/internal/notifyport"
)

type Notifier interface {
	SendGate(ctx context.Context, report decisionfmt.DecisionReport) error
	SendPositionOpen(ctx context.Context, notice PositionOpenNotice) error
	SendPositionClose(ctx context.Context, notice PositionCloseNotice) error
	SendPositionCloseSummary(ctx context.Context, notice PositionCloseSummaryNotice) error
	SendRiskPlanUpdate(ctx context.Context, notice RiskPlanUpdateNotice) error
	SendTradeOpen(ctx context.Context, notice TradeOpenNotice) error
	SendTradePartialClose(ctx context.Context, notice TradePartialCloseNotice) error
	SendTradeCloseSummary(ctx context.Context, notice TradeCloseSummaryNotice) error
	SendError(ctx context.Context, message string) error
}

type PositionOpenNotice = notifyport.PositionOpenNotice

type PositionCloseNotice = notifyport.PositionCloseNotice

type PositionCloseSummaryNotice = notifyport.PositionCloseSummaryNotice

type RiskPlanUpdateNotice = notifyport.RiskPlanUpdateNotice

type RiskPlanUpdateScoreItem = notifyport.RiskPlanUpdateScoreItem

type TradeOpenNotice struct {
	TradeID       int
	Pair          string
	Amount        float64
	StakeAmount   float64
	IsShort       bool
	OpenRate      float64
	OpenTimestamp int64
	Leverage      float64
	EnterTag      string
}

type TradePartialCloseNotice struct {
	TradeID             int
	Pair                string
	IsShort             bool
	OpenRate            float64
	CloseRate           float64
	Amount              float64
	StakeAmount         float64
	RealizedProfit      float64
	RealizedProfitRatio float64
	ExitReason          string
	ExitType            string
}

type TradeCloseSummaryNotice struct {
	TradeID        int
	Pair           string
	IsShort        bool
	OpenRate       float64
	CloseRate      float64
	Amount         float64
	StakeAmount    float64
	CloseProfitAbs float64
	CloseProfitPct float64
	ProfitAbs      float64
	ProfitPct      float64
	TradeDuration  int
	TradeDurationS int64
	ExitReason     string
	ExitType       string
	Leverage       float64
}

type Sender interface {
	Send(ctx context.Context, msg Message) error
}

type Message struct {
	Title    string
	Markdown string
	HTML     string
	Plain    string
}

type NotificationConfig struct {
	Enabled  bool
	Telegram TelegramConfig
	Email    EmailConfig
}

type TelegramConfig struct {
	Enabled bool
	Token   string
	ChatID  int64
}

type EmailConfig struct {
	Enabled  bool
	SMTPHost string
	SMTPPort int
	SMTPUser string
	SMTPPass string
	From     string
	To       []string
}

func FromConfig(cfg config.NotificationConfig) NotificationConfig {
	return NotificationConfig{
		Enabled: cfg.Enabled,
		Telegram: TelegramConfig{
			Enabled: cfg.Telegram.Enabled,
			Token:   cfg.Telegram.Token,
			ChatID:  cfg.Telegram.ChatID,
		},
		Email: EmailConfig{
			Enabled:  cfg.Email.Enabled,
			SMTPHost: cfg.Email.SMTPHost,
			SMTPPort: cfg.Email.SMTPPort,
			SMTPUser: cfg.Email.SMTPUser,
			SMTPPass: cfg.Email.SMTPPass,
			From:     cfg.Email.From,
			To:       cfg.Email.To,
		},
	}
}

package decision

import (
	"context"

	"brale-core/internal/decision/decisionfmt"
	"brale-core/internal/notifyport"
)

type Notifier interface {
	SendGate(ctx context.Context, input decisionfmt.DecisionInput, report decisionfmt.DecisionReport) error
	SendRiskPlanUpdate(ctx context.Context, notice RiskPlanUpdateNotice) error
	SendError(ctx context.Context, notice ErrorNotice) error
}

type RiskPlanUpdateNotice = notifyport.RiskPlanUpdateNotice

type RiskPlanUpdateScoreItem = notifyport.RiskPlanUpdateScoreItem

type ErrorNotice = notifyport.ErrorNotice

type NopNotifier struct{}

func (NopNotifier) SendGate(ctx context.Context, input decisionfmt.DecisionInput, report decisionfmt.DecisionReport) error {
	return nil
}

func (NopNotifier) SendRiskPlanUpdate(ctx context.Context, notice RiskPlanUpdateNotice) error {
	return nil
}

func (NopNotifier) SendError(ctx context.Context, notice ErrorNotice) error {
	return nil
}

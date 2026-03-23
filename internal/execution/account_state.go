package execution

import "fmt"

func AccountStateFromBalance(data map[string]any) (AccountState, error) {
	if data == nil {
		return AccountState{}, fmt.Errorf("balance not available")
	}
	equity, ok := ExtractUSDTBalance(data)
	if !ok || equity <= 0 {
		return AccountState{}, fmt.Errorf("balance not available")
	}
	available, ok := ExtractUSDTAvailable(data)
	if !ok || available <= 0 {
		available = equity
	}
	return AccountState{
		Equity:    equity,
		Available: available,
		Currency:  ResolveStakeCurrency(data),
	}, nil
}

func BalanceEquity(data map[string]any) float64 {
	acct, err := AccountStateFromBalance(data)
	if err != nil {
		return 0
	}
	return acct.Equity
}

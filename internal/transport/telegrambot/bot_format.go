package telegrambot

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

func formatMonitorStatus(resp MonitorStatusResponse) string {
	if len(resp.Symbols) == 0 {
		return "暂无监控币种。"
	}
	b := &strings.Builder{}
	for _, sym := range resp.Symbols {
		b.WriteString("【")
		b.WriteString(sym.Symbol)
		b.WriteString("】\n")
		b.WriteString("下一次运行: ")
		b.WriteString(formatTime(sym.NextRun))
		b.WriteString("\n")
		b.WriteString("K线周期: ")
		b.WriteString(sym.KlineInterval)
		b.WriteString("\n")
		fmt.Fprintf(b, "单笔风险: %.4f (≈ %.2f USDT)\n", sym.RiskPct, sym.RiskAmount)
		fmt.Fprintf(b, "最大杠杆: %.2f\n", sym.MaxLeverage)
		fmt.Fprintf(b, "止盈倍数: %.2f\n", sym.TakeProfitMultiple)
		fmt.Fprintf(b, "初始止损倍数: %.2f\n", sym.InitialStopMultiple)
		b.WriteString("入场定价: ")
		b.WriteString(sym.EntryPricingMode)
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String())
}

func formatPositions(items []PositionStatusItem) string {
	if len(items) == 0 {
		return "暂无持仓。"
	}
	b := &strings.Builder{}
	for _, pos := range items {
		b.WriteString("【")
		b.WriteString(pos.Symbol)
		b.WriteString("】\n")
		fmt.Fprintf(b, "方向: %s\n", pos.Side)
		fmt.Fprintf(b, "数量: %.6f (原始 %.6f)\n", pos.Amount, pos.AmountRequested)
		fmt.Fprintf(b, "保证金: %.4f\n", pos.MarginAmount)
		fmt.Fprintf(b, "开仓价: %.4f\n", pos.EntryPrice)
		fmt.Fprintf(b, "当前价: %.4f\n", pos.CurrentPrice)
		fmt.Fprintf(b, "收益: %.4f (已实现 %.4f / 未实现 %.4f)\n", pos.ProfitTotal, pos.ProfitRealized, pos.ProfitUnrealized)
		b.WriteString("开仓时间: ")
		b.WriteString(formatOpenedAt(pos.OpenedAt))
		b.WriteString("\n")
		fmt.Fprintf(b, "持仓时长: %dm\n", pos.DurationMin)
		if len(pos.TakeProfits) > 0 {
			b.WriteString("止盈: ")
			b.WriteString(formatFloatList(pos.TakeProfits))
			b.WriteString("\n")
		} else {
			b.WriteString("止盈: —\n")
		}
		if pos.StopLoss > 0 {
			fmt.Fprintf(b, "止损: %.4f\n", pos.StopLoss)
		} else {
			b.WriteString("止损: —\n")
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func formatTradeHistory(items []TradeHistoryItem) string {
	if len(items) == 0 {
		return "暂无历史仓位。"
	}
	b := &strings.Builder{}
	for _, tr := range items {
		b.WriteString("【")
		b.WriteString(tr.Symbol)
		b.WriteString("】\n")
		fmt.Fprintf(b, "方向: %s\n", tr.Side)
		fmt.Fprintf(b, "数量: %.6f\n", tr.Amount)
		fmt.Fprintf(b, "保证金: %.4f\n", tr.MarginAmount)
		b.WriteString("开仓时间: ")
		b.WriteString(formatTime(tr.OpenedAt))
		b.WriteString("\n")
		fmt.Fprintf(b, "持仓时长: %ds\n", tr.DurationSec)
		fmt.Fprintf(b, "收益: %.4f\n\n", tr.Profit)
	}
	return strings.TrimSpace(b.String())
}

func latestTradeHistory(items []TradeHistoryItem, limit int) []TradeHistoryItem {
	if len(items) == 0 || limit <= 0 {
		return nil
	}
	sorted := append([]TradeHistoryItem(nil), items...)
	sort.SliceStable(sorted, func(i, j int) bool {
		left := sorted[i].OpenedAt
		right := sorted[j].OpenedAt
		if left.Equal(right) {
			return sorted[i].Symbol < sorted[j].Symbol
		}
		if left.IsZero() {
			return false
		}
		if right.IsZero() {
			return true
		}
		return left.After(right)
	})
	if len(sorted) > limit {
		sorted = sorted[:limit]
	}
	return sorted
}

func formatScheduleResponse(resp ScheduleResponse) string {
	b := &strings.Builder{}
	if strings.TrimSpace(resp.Summary) != "" {
		b.WriteString(resp.Summary)
		b.WriteString("\n")
	}
	if resp.LLMScheduled {
		if len(resp.NextRuns) > 0 {
			b.WriteString("下一次运行：\n")
			for _, item := range resp.NextRuns {
				b.WriteString("- ")
				b.WriteString(item.Symbol)
				b.WriteString(" ")
				b.WriteString(item.NextExecution)
				if item.BarInterval != "" {
					b.WriteString(" (")
					b.WriteString(item.BarInterval)
					b.WriteString(")")
				}
				b.WriteString("\n")
			}
		}
		return strings.TrimSpace(b.String())
	}
	if len(resp.Positions) > 0 {
		b.WriteString("\n")
		b.WriteString(formatPositions(resp.Positions))
	}
	return strings.TrimSpace(b.String())
}
func splitMessageChunks(text string, maxRunes int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if maxRunes <= 0 {
		maxRunes = 3500
	}
	lines := strings.Split(text, "\n")
	chunks := make([]string, 0, len(lines)/8+1)
	current := &strings.Builder{}
	currentLen := 0
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		lineLen := len([]rune(line))
		extra := lineLen
		if currentLen > 0 {
			extra++
		}
		if currentLen > 0 && currentLen+extra > maxRunes {
			chunks = append(chunks, current.String())
			current.Reset()
			currentLen = 0
		}
		if lineLen > maxRunes {
			runes := []rune(line)
			for len(runes) > maxRunes {
				chunks = append(chunks, string(runes[:maxRunes]))
				runes = runes[maxRunes:]
			}
			if len(runes) > 0 {
				current.WriteString(string(runes))
				currentLen = len(runes)
			}
			continue
		}
		if currentLen > 0 {
			current.WriteString("\n")
			currentLen++
		}
		current.WriteString(line)
		currentLen += lineLen
	}
	if currentLen > 0 {
		chunks = append(chunks, current.String())
	}
	return chunks
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format("2006-01-02 15:04:05")
}

func formatOpenedAt(value string) string {
	if strings.TrimSpace(value) == "" {
		return "—"
	}
	return value
}

func formatFloatList(items []float64) string {
	if len(items) == 0 {
		return "—"
	}
	parts := make([]string, 0, len(items))
	for _, v := range items {
		parts = append(parts, fmt.Sprintf("%.4f", v))
	}
	return strings.Join(parts, ", ")
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

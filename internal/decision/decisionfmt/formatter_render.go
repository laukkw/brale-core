package decisionfmt

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"brale-core/internal/pkg/parseutil"
)

func (f DefaultFormatter) RenderGateText(report GateReport) string {
	lines := []string{
		fmt.Sprintf("全局可交易: %s", report.Overall.TradeableText),
	}
	decisionText := report.Overall.DecisionText
	if strings.TrimSpace(decisionText) == "" {
		decisionText = report.Overall.DecisionAction
	}
	if strings.TrimSpace(decisionText) != "" {
		if strings.TrimSpace(report.Overall.DecisionAction) != "" && report.Overall.DecisionAction != decisionText {
			lines = append(lines, fmt.Sprintf("决策: %s (%s)", decisionText, report.Overall.DecisionAction))
		} else {
			lines = append(lines, fmt.Sprintf("决策: %s", decisionText))
		}
	}
	lines = append(lines, fmt.Sprintf("Grade: %d", report.Overall.Grade))
	reason := report.Overall.Reason
	if strings.TrimSpace(reason) == "" {
		reason = "—"
	}
	if strings.TrimSpace(report.Overall.ReasonCode) != "" {
		lines = append(lines, fmt.Sprintf("逻辑解释: %s (原因码: %s)", reason, report.Overall.ReasonCode))
	} else {
		lines = append(lines, fmt.Sprintf("逻辑解释: %s", reason))
	}
	if strings.TrimSpace(report.Overall.Direction) != "" && report.Overall.Direction != "—" {
		lines = append(lines, fmt.Sprintf("方向: %s", report.Overall.Direction))
	}
	if report.RuleHit != nil && strings.TrimSpace(report.RuleHit.Name) != "" {
		ruleText := fmt.Sprintf("命中规则: %s (priority %d)", displayGateReasonCode(report.RuleHit.Name), report.RuleHit.Priority)
		if strings.TrimSpace(report.RuleHit.Action) != "" {
			ruleText = fmt.Sprintf("%s, action=%s", ruleText, translateDecisionAction(report.RuleHit.Action))
		}
		if strings.TrimSpace(report.RuleHit.Reason) != "" {
			ruleText = fmt.Sprintf("%s, reason=%s", ruleText, displayGateReasonCode(report.RuleHit.Reason))
		}
		lines = append(lines, ruleText)
	}
	if trace := formatGateTrace(report.Derived); trace != "" {
		lines = append(lines, fmt.Sprintf("Gate 过程: %s", trace))
	}
	if summary := formatDerivedSummary(report.Derived); summary != "" {
		lines = append(lines, fmt.Sprintf("Derived: %s", summary))
	}
	for _, p := range report.Providers {
		label := providerRoleLabel(p.Role)
		line := fmt.Sprintf("%s Provider: %s", label, p.TradeableText)
		if len(p.Factors) > 0 {
			parts := make([]string, 0, len(p.Factors))
			for _, f := range p.Factors {
				parts = append(parts, fmt.Sprintf("%s=%s", f.Label, f.Status))
			}
			line = fmt.Sprintf("%s | %s", line, strings.Join(parts, "；"))
		}
		lines = append(lines, line)
	}
	if report.Overall.ExpectedSnapID > 0 {
		lines = append(lines, fmt.Sprintf("说明: Gate 事件缺失 (snapshot %d)", report.Overall.ExpectedSnapID))
	}
	return strings.Join(lines, "\n")
}

func (f DefaultFormatter) RenderDecisionMarkdown(report DecisionReport) string {
	var b strings.Builder
	gateLabel := "Gate"
	gateText := report.Gate.Overall.TradeableText
	if label, text, ok := resolveHoldStatusLine(report); ok {
		gateLabel = label
		gateText = text
	}
	fmt.Fprintf(&b, "[%s][snapshot:%d] %s: %s\n", report.Symbol, report.SnapshotID, gateLabel, gateText)
	fmt.Fprintf(&b, "时间: %s | 价格: %s\n\n", formatReportTime(), formatCurrentPrice(report))
	decisionText := report.Gate.Overall.DecisionText
	if strings.TrimSpace(decisionText) == "" {
		decisionText = report.Gate.Overall.DecisionAction
	}
	if strings.TrimSpace(decisionText) != "" {
		if strings.TrimSpace(report.Gate.Overall.DecisionAction) != "" && report.Gate.Overall.DecisionAction != decisionText {
			fmt.Fprintf(&b, "Gate 决策: %s (%s)\n", decisionText, report.Gate.Overall.DecisionAction)
		} else {
			fmt.Fprintf(&b, "Gate 决策: %s\n", decisionText)
		}
	}
	fmt.Fprintf(&b, "Gate Grade: %d\n", report.Gate.Overall.Grade)
	if report.Gate.Overall.Reason != "" {
		if report.Gate.Overall.ReasonCode != "" {
			fmt.Fprintf(&b, "Gate 原因: %s (原因码: %s)\n", report.Gate.Overall.Reason, report.Gate.Overall.ReasonCode)
		} else {
			fmt.Fprintf(&b, "Gate 原因: %s\n", report.Gate.Overall.Reason)
		}
	}
	if report.Gate.RuleHit != nil && strings.TrimSpace(report.Gate.RuleHit.Name) != "" {
		ruleText := fmt.Sprintf("Gate 命中规则: %s (priority %d)", displayGateReasonCode(report.Gate.RuleHit.Name), report.Gate.RuleHit.Priority)
		if strings.TrimSpace(report.Gate.RuleHit.Action) != "" {
			ruleText = fmt.Sprintf("%s, action=%s", ruleText, translateDecisionAction(report.Gate.RuleHit.Action))
		}
		if strings.TrimSpace(report.Gate.RuleHit.Reason) != "" {
			ruleText = fmt.Sprintf("%s, reason=%s", ruleText, displayGateReasonCode(report.Gate.RuleHit.Reason))
		}
		fmt.Fprintf(&b, "%s\n", ruleText)
	}
	if trace := formatGateTrace(report.Gate.Derived); trace != "" {
		fmt.Fprintf(&b, "Gate 过程: %s\n", trace)
	}
	if summary := formatDerivedSummary(report.Gate.Derived); summary != "" {
		fmt.Fprintf(&b, "Derived: %s\n", summary)
	}
	if report.Gate.Overall.Reason != "" || report.Gate.RuleHit != nil || len(report.Gate.Derived) > 0 {
		fmt.Fprint(&b, "\n")
	}
	if monitor := renderMonitorMarkdown(report); strings.TrimSpace(monitor) != "" {
		fmt.Fprintf(&b, "%s\n\n", monitor)
	}
	writeStageMarkdown(&b, "Provider", report.Providers)
	writeStageMarkdown(&b, "Agent", report.Agents)
	return strings.TrimSpace(b.String())
}

func (f DefaultFormatter) RenderDecisionHTML(report DecisionReport) string {
	const telegramHTMLLimit = 4096
	baseSections := []string{
		renderHTMLHeader(report),
		renderHTMLNarrative(report),
	}
	metricsSection := renderHTMLMetrics(report)
	monitorSection := renderHTMLMonitorDetail(report)
	riskSection := renderHTMLRiskDetail(report)
	sections := append([]string{}, baseSections...)
	sections = append(sections, metricsSection, monitorSection, riskSection)
	assembled := joinHTMLSections(sections)
	if utf8.RuneCountInString(assembled) > telegramHTMLLimit {
		if riskSection != "" {
			sections = append([]string{}, baseSections...)
			sections = append(sections, metricsSection)
			assembled = joinHTMLSections(sections)
		}
	}
	if utf8.RuneCountInString(assembled) > telegramHTMLLimit {
		if metricsSection != "" {
			sections = append([]string{}, baseSections...)
			assembled = joinHTMLSections(sections)
		}
	}
	if utf8.RuneCountInString(assembled) > telegramHTMLLimit {
		assembled = trimHTMLRunes(assembled, telegramHTMLLimit)
	}
	return assembled
}

func writeStageMarkdown(b *strings.Builder, label string, stages []StageOutput) {
	if len(stages) == 0 {
		return
	}
	fmt.Fprintf(b, "%s:\n", label)
	for _, item := range stages {
		roleLabel := providerRoleLabel(item.Role)
		if roleLabel == "" {
			roleLabel = item.Role
		}
		fmt.Fprintf(b, "- %s\n", roleLabel)
		if item.Summary != "" {
			lines := strings.Split(item.Summary, "\n")
			for _, line := range lines {
				fmt.Fprintf(b, "  %s\n", line)
			}
		}
	}
	b.WriteString("\n")
}

func escapeHTML(text string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&#39;",
	)
	return replacer.Replace(text)
}

func renderHTMLHeader(report DecisionReport) string {
	decisionText := strings.TrimSpace(report.Gate.Overall.DecisionText)
	if execTitle := resolveExecutionTitle(report); execTitle != "" {
		decisionText = execTitle
	}
	if decisionText == "" {
		decisionText = strings.TrimSpace(report.Gate.Overall.DecisionAction)
	}
	if decisionText == "" {
		decisionText = "—"
	}
	line1 := fmt.Sprintf("%s %s 决策报告 (Snapshot: %d)", decisionText, report.Symbol, report.SnapshotID)
	statusLabel := "可交易"
	gateText := strings.TrimSpace(report.Gate.Overall.TradeableText)
	if label, text, ok := resolveHoldStatusLine(report); ok {
		statusLabel = label
		gateText = text
	}
	line2Parts := []string{fmt.Sprintf("%s: %s", statusLabel, gateText)}
	if gateText == "" {
		line2Parts = line2Parts[:0]
	}
	action := strings.TrimSpace(report.Gate.Overall.DecisionAction)
	if action != "" && action != decisionText {
		line2Parts = append(line2Parts, fmt.Sprintf("动作: %s", action))
	}
	direction := strings.TrimSpace(report.Gate.Overall.Direction)
	if direction != "" && direction != "无方向" {
		line2Parts = append(line2Parts, fmt.Sprintf("方向: %s", direction))
	}
	line2 := strings.Join(line2Parts, " | ")
	line3 := fmt.Sprintf("时间: %s | 价格: %s", formatReportTime(), formatCurrentPrice(report))
	content := escapeHTML(line1)
	if strings.TrimSpace(line2) != "" {
		content = fmt.Sprintf("%s\n%s", content, escapeHTML(line2))
	}
	if strings.TrimSpace(line3) != "" {
		content = fmt.Sprintf("%s\n%s", content, escapeHTML(line3))
	}
	return fmt.Sprintf("<b>标题区</b>\n%s", wrapPre(content))
}

func parseStringValue(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func parseBoolValue(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true")
	case float64:
		return v != 0
	case float32:
		return v != 0
	case int:
		return v != 0
	case int64:
		return v != 0
	case uint64:
		return v != 0
	default:
		return false
	}
}

func parseFloatValue(value any) float64 {
	return parseutil.Float(value)
}

func parseFloatValueOK(value any) (float64, bool) {
	return parseutil.FloatOK(value)
}

func parseStringList(value any) []string {
	switch v := value.(type) {
	case []string:
		return v
	case []any:
		items := make([]string, 0, len(v))
		for _, raw := range v {
			text := parseStringValue(raw)
			if text == "" {
				continue
			}
			items = append(items, text)
		}
		return items
	default:
		return nil
	}
}

func parseMapValue(value any) map[string]any {
	if value == nil {
		return nil
	}
	if v, ok := value.(map[string]any); ok {
		return v
	}
	return nil
}

type directionConsensusMetrics struct {
	Score                 float64
	ScoreOK               bool
	ScoreThreshold        float64
	ScoreThresholdOK      bool
	Confidence            float64
	ConfidenceOK          bool
	ConfidenceThreshold   float64
	ConfidenceThresholdOK bool
	IndicatorScore        float64
	IndicatorScoreOK      bool
	StructureScore        float64
	StructureScoreOK      bool
	MechanicsScore        float64
	MechanicsScoreOK      bool
}

func parseDirectionConsensusMetrics(derived map[string]any) *directionConsensusMetrics {
	if len(derived) == 0 {
		return nil
	}
	raw, ok := derived["direction_consensus"]
	if !ok {
		return nil
	}
	consensus := parseMapValue(raw)
	if len(consensus) == 0 {
		return nil
	}
	out := &directionConsensusMetrics{}
	out.Score, out.ScoreOK = parseFloatValueOK(consensus["score"])
	out.ScoreThreshold, out.ScoreThresholdOK = parseFloatValueOK(consensus["score_threshold"])
	out.Confidence, out.ConfidenceOK = parseFloatValueOK(consensus["confidence"])
	out.ConfidenceThreshold, out.ConfidenceThresholdOK = parseFloatValueOK(consensus["confidence_threshold"])
	sources := parseMapValue(consensus["sources"])
	if len(sources) > 0 {
		indicator := parseMapValue(sources["indicator"])
		out.IndicatorScore, out.IndicatorScoreOK = parseFloatValueOK(indicator["score"])
		structure := parseMapValue(sources["structure"])
		out.StructureScore, out.StructureScoreOK = parseFloatValueOK(structure["score"])
		mechanics := parseMapValue(sources["mechanics"])
		out.MechanicsScore, out.MechanicsScoreOK = parseFloatValueOK(mechanics["score"])
	}
	return out
}

func formatReportTime() string {
	now := time.Now()
	return now.Format("2006-01-02 15:04:05 MST -0700")
}

func formatCurrentPrice(report DecisionReport) string {
	price := extractCurrentPrice(report)
	if price <= 0 {
		return "—"
	}
	text := strconv.FormatFloat(price, 'f', 4, 64)
	text = strings.TrimRight(text, "0")
	text = strings.TrimRight(text, ".")
	if text == "" {
		return "—"
	}
	return text
}

func extractCurrentPrice(report DecisionReport) float64 {
	if report.Gate.Derived == nil {
		return 0
	}
	val, ok := report.Gate.Derived["current_price"]
	if !ok || val == nil {
		return 0
	}
	return parseutil.Float(val)
}

func renderHTMLNarrative(report DecisionReport) string {
	summary := pickNarrativeSummary(report)
	if strings.TrimSpace(summary) == "" {
		summary = "—"
	}
	sections := splitNarrativeSections(summary)
	labels := []struct {
		Key   string
		Label string
	}{
		{Key: "状态", Label: "🧭 状态"},
		{Key: "动作", Label: "⚙️ 动作"},
		{Key: "冲突", Label: "⚔️ 冲突"},
		{Key: "风险", Label: "🛡️ 风险"},
	}
	parts := make([]string, 0, 1+len(labels)*2)
	parts = append(parts, "<b>叙事分析</b>")
	for idx, label := range labels {
		value := strings.TrimSpace(sections[label.Key])
		if label.Key == "冲突" && isPlaceholderValue(value) {
			continue
		}
		if isPlaceholderValue(value) {
			value = "—"
		}
		if idx > 0 && len(parts) > 1 {
			parts = append(parts, "")
		}
		parts = append(parts, fmt.Sprintf("<b>%s</b>", label.Label))
		parts = append(parts, wrapPre(escapeHTML(value)))
	}
	return strings.Join(parts, "\n")
}

func renderHTMLMetrics(report DecisionReport) string {
	consensus := parseDirectionConsensusMetrics(report.Gate.Derived)
	metrics := make([]string, 0, 10)
	metrics = appendBaseHTMLMetrics(metrics, report, consensus)
	metrics = appendTightenHTMLMetrics(metrics, report)
	metrics = append(metrics, "可交易判定: 指标=动量扩张&趋势一致&无噪音; 结构=结构清晰&完整; 力学=无清算压力")
	metrics = append(metrics, "方向权重: 结构1.0 / 指标0.7 / 力学0.5")
	metrics = appendRuleAndStopHTMLMetrics(metrics, report)
	metrics = appendConsensusHTMLMetrics(metrics, report, consensus)
	return renderHTMLMetricsBlock(metrics)
}

func appendBaseHTMLMetrics(metrics []string, report DecisionReport, consensus *directionConsensusMetrics) []string {
	if consensus != nil && consensus.ConfidenceOK {
		metrics = append(metrics, fmt.Sprintf("信心指数: %s / 100", formatExecutionFloat(consensus.Confidence*100)))
	} else {
		metrics = append(metrics, fmt.Sprintf("信心指数: %d", report.Gate.Overall.Grade))
	}
	metrics = append(metrics, fmt.Sprintf("Gate等级: %d", report.Gate.Overall.Grade))
	statusLabel, gateText := resolveStatusHTMLMetric(report)
	if gateText != "" {
		metrics = append(metrics, fmt.Sprintf("%s: %s", statusLabel, gateText))
	}
	reason := strings.TrimSpace(report.Gate.Overall.Reason)
	if reason != "" {
		metrics = append(metrics, fmt.Sprintf("主要原因: %s", reason))
	}
	direction := strings.TrimSpace(report.Gate.Overall.Direction)
	if direction != "" {
		metrics = append(metrics, fmt.Sprintf("趋势方向: %s", direction))
	}
	return metrics
}

func resolveStatusHTMLMetric(report DecisionReport) (statusLabel, gateText string) {
	statusLabel = "可交易"
	gateText = strings.TrimSpace(report.Gate.Overall.TradeableText)
	if label, text, ok := resolveHoldStatusLine(report); ok {
		statusLabel = label
		gateText = text
	}
	return statusLabel, gateText
}

func appendTightenHTMLMetrics(metrics []string, report DecisionReport) []string {
	exec := parseExecutionSummary(report.Gate.Derived)
	if exec == nil || !strings.EqualFold(exec.Action, "tighten") {
		return metrics
	}
	metrics = append(metrics, fmt.Sprintf("执行结论: %s", formatExecutionSummary(*exec)))
	if exec.ScoreThreshold > 0 || exec.ScoreParseOK {
		metrics = append(metrics, fmt.Sprintf("收紧评分: %s / %s", formatExecutionFloat(exec.ScoreTotal), formatExecutionFloat(exec.ScoreThreshold)))
	}
	if len(exec.BlockedBy) > 0 {
		metrics = append(metrics, fmt.Sprintf("阻断原因: %s", formatExecutionBlockedReasons(exec.BlockedBy)))
		if blockedStage := formatExecutionBlockedStages(exec.BlockedBy); blockedStage != "" {
			metrics = append(metrics, fmt.Sprintf("收紧阻隔环节: %s", blockedStage))
		}
	}
	if exec.ATRChangePctOK || exec.MonitorGateHit || exec.ATRThreshold > 0 {
		atrChangeText := "—"
		if exec.ATRChangePctOK {
			atrChangeText = formatExecutionFloat(math.Abs(exec.ATRChangePct))
		}
		metrics = append(metrics, fmt.Sprintf("收紧门槛: 监控收紧=%s; |ATR变化|=%s (>=%s)", translateBoolStatus(exec.MonitorGateHit), atrChangeText, formatExecutionFloat(exec.ATRThreshold)))
	}
	if strings.TrimSpace(exec.NewsGateReasonZH) != "" {
		metrics = append(metrics, fmt.Sprintf("舆情门槛: %s", exec.NewsGateReasonZH))
	}
	return metrics
}

func appendRuleAndStopHTMLMetrics(metrics []string, report DecisionReport) []string {
	if report.Gate.RuleHit != nil {
		ruleName := strings.TrimSpace(report.Gate.RuleHit.Name)
		if ruleName != "" {
			metrics = append(metrics, fmt.Sprintf("命中规则: %s", displayGateReasonCode(ruleName)))
		}
	}
	stopStep := strings.TrimSpace(fmt.Sprint(report.Gate.Derived["gate_stop_step"]))
	if stopStep != "" {
		metrics = append(metrics, fmt.Sprintf("停在步骤: %s", displayGateStep(stopStep)))
	}
	return metrics
}

func appendConsensusHTMLMetrics(metrics []string, report DecisionReport, consensus *directionConsensusMetrics) []string {
	if !strings.EqualFold(strings.TrimSpace(report.Gate.Overall.ReasonCode), "CONSENSUS_NOT_PASSED") || consensus == nil {
		return metrics
	}
	sourceScores := make([]string, 0, 3)
	if consensus.StructureScoreOK {
		sourceScores = append(sourceScores, fmt.Sprintf("结构=%s", formatExecutionFloat(consensus.StructureScore)))
	}
	if consensus.IndicatorScoreOK {
		sourceScores = append(sourceScores, fmt.Sprintf("指标=%s", formatExecutionFloat(consensus.IndicatorScore)))
	}
	if consensus.MechanicsScoreOK {
		sourceScores = append(sourceScores, fmt.Sprintf("力学=%s", formatExecutionFloat(consensus.MechanicsScore)))
	}
	if len(sourceScores) > 0 {
		metrics = append(metrics, fmt.Sprintf("三路得分: %s", strings.Join(sourceScores, " / ")))
	}
	if consensus.ScoreOK {
		if consensus.ScoreThresholdOK {
			metrics = append(metrics, fmt.Sprintf("共识总分: %s (需达到 |score| >= %s)", formatExecutionFloat(consensus.Score), formatExecutionFloat(consensus.ScoreThreshold)))
		} else {
			metrics = append(metrics, fmt.Sprintf("共识总分: %s", formatExecutionFloat(consensus.Score)))
		}
	}
	if consensus.ConfidenceOK && consensus.ConfidenceThresholdOK {
		metrics = append(metrics, fmt.Sprintf("共识置信: %s (需达到 >= %s)", formatExecutionFloat(consensus.Confidence), formatExecutionFloat(consensus.ConfidenceThreshold)))
	}
	return metrics
}

func renderHTMLMetricsBlock(metrics []string) string {
	lines := make([]string, 0, len(metrics))
	for _, item := range metrics {
		lines = append(lines, escapeHTML(item))
	}
	return fmt.Sprintf("<b>关键仪表</b>\n%s", wrapPre(strings.Join(lines, "\n")))
}

func renderHTMLSignals(report DecisionReport) string {
	items := make([]string, 0, 4)
	for _, p := range report.Gate.Providers {
		label := providerRoleLabel(p.Role)
		if strings.TrimSpace(label) == "" {
			label = p.Role
		}
		entry := fmt.Sprintf("%s: %s", label, p.TradeableText)
		if p.Tradeable {
			items = append(items, fmt.Sprintf("🟢 %s", entry))
			continue
		}
		items = append(items, fmt.Sprintf("🔴 %s", entry))
	}
	if len(items) == 0 {
		items = append(items, collectStageSummaries(report)...)
	}
	content := formatBulletLines(items)
	if strings.TrimSpace(content) == "" {
		return ""
	}
	return fmt.Sprintf("<b>多空博弈</b>\n%s", wrapPre(content))
}

func renderHTMLRiskDetail(report DecisionReport) string {
	if !shouldRenderRiskDetail(report) {
		return ""
	}
	action := strings.TrimSpace(fmt.Sprint(report.Gate.Derived["sieve_action"]))
	reasonCode := strings.TrimSpace(fmt.Sprint(report.Gate.Derived["sieve_reason"]))
	if action == "" && reasonCode == "" {
		return ""
	}
	lines := make([]string, 0, 3)
	if action != "" {
		actionLabel := translateDecisionAction(action)
		if strings.TrimSpace(actionLabel) == "" {
			actionLabel = action
		}
		lines = append(lines, escapeHTML(fmt.Sprintf("拦截动作: %s", actionLabel)))
	}
	if reasonCode != "" {
		lines = append(lines, fmt.Sprintf("拦截代码: <code>%s</code>", escapeHTML(reasonCode)))
		reasonLabel := translateSieveReasonCode(reasonCode)
		if strings.TrimSpace(reasonLabel) != "" {
			lines = append(lines, escapeHTML(fmt.Sprintf("人话解释: %s", reasonLabel)))
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return fmt.Sprintf("<b>风控拦截详情</b>\n%s", wrapPre(strings.Join(lines, "\n")))
}

func renderHTMLMonitorDetail(report DecisionReport) string {
	if !isHoldDecision(report.Gate.Overall.DecisionAction) {
		return ""
	}
	if report.Gate.Derived == nil {
		return ""
	}
	mode := strings.ToLower(strings.TrimSpace(fmt.Sprint(report.Gate.Derived["gate_trace_mode"])))
	if mode != "monitor" {
		return ""
	}
	steps := collectMonitorTrace(report.Gate.Derived)
	if len(steps) == 0 {
		return ""
	}
	lines := []string{"当前状态: 持仓中，未放行新开仓", "详细展开:"}
	blocked := make([]string, 0, len(steps))
	for _, step := range steps {
		label := strings.TrimSpace(step.label)
		if label == "" {
			label = step.step
		}
		detail := formatMonitorTag(step.tag, step.reason)
		lines = append(lines, fmt.Sprintf("%s: %s", label, detail))
		if strings.EqualFold(step.tag, "keep") {
			blocked = append(blocked, fmt.Sprintf("%s=%s", label, detail))
		}
	}
	if len(blocked) > 0 {
		lines = append(lines, fmt.Sprintf("阻断步骤: %s", strings.Join(blocked, "；")))
	}
	return fmt.Sprintf("<b>持仓监控</b>\n%s", wrapPre(escapeHTML(strings.Join(lines, "\n"))))
}

type monitorTraceStep struct {
	step   string
	label  string
	tag    string
	reason string
}

func collectMonitorTrace(derived map[string]any) []monitorTraceStep {
	stepsRaw, ok := derived["gate_trace"].([]any)
	if !ok || len(stepsRaw) == 0 {
		return nil
	}
	steps := make([]monitorTraceStep, 0, len(stepsRaw))
	for _, raw := range stepsRaw {
		stepMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		stepKey := strings.TrimSpace(fmt.Sprint(stepMap["step"]))
		if stepKey == "" || stepKey == "<nil>" {
			continue
		}
		label := translateGateStep(stepKey)
		if strings.TrimSpace(label) == "" {
			label = stepKey
		}
		tag := strings.TrimSpace(fmt.Sprint(stepMap["tag"]))
		if tag == "<nil>" {
			tag = ""
		}
		reason := strings.TrimSpace(fmt.Sprint(stepMap["reason"]))
		if reason == "<nil>" {
			reason = ""
		}
		steps = append(steps, monitorTraceStep{
			step:   stepKey,
			label:  label,
			tag:    tag,
			reason: reason,
		})
	}
	return steps
}

func formatMonitorTag(tag, reason string) string {
	label := strings.TrimSpace(translateDecisionAction(tag))
	if label == "" {
		label = tag
	}
	if strings.TrimSpace(reason) == "" {
		return label
	}
	return fmt.Sprintf("%s（原因：%s）", label, reason)
}

func renderMonitorMarkdown(report DecisionReport) string {
	if !isHoldDecision(report.Gate.Overall.DecisionAction) {
		return ""
	}
	if report.Gate.Derived == nil {
		return ""
	}
	mode := strings.ToLower(strings.TrimSpace(fmt.Sprint(report.Gate.Derived["gate_trace_mode"])))
	if mode != "monitor" {
		return ""
	}
	steps := collectMonitorTrace(report.Gate.Derived)
	if len(steps) == 0 {
		return ""
	}
	lines := []string{"持仓监控", "当前状态: 持仓中，未放行新开仓", "详细展开:"}
	blocked := make([]string, 0, len(steps))
	for _, step := range steps {
		label := strings.TrimSpace(step.label)
		if label == "" {
			label = step.step
		}
		detail := formatMonitorTag(step.tag, step.reason)
		lines = append(lines, fmt.Sprintf("- %s: %s", label, detail))
		if strings.EqualFold(step.tag, "keep") {
			blocked = append(blocked, fmt.Sprintf("%s=%s", label, detail))
		}
	}
	if len(blocked) > 0 {
		lines = append(lines, fmt.Sprintf("阻断步骤: %s", strings.Join(blocked, "；")))
	}
	return strings.Join(lines, "\n")
}

func shouldRenderRiskDetail(report DecisionReport) bool {
	if len(report.Gate.Derived) == 0 {
		return false
	}
	action := strings.ToUpper(strings.TrimSpace(report.Gate.Overall.DecisionAction))
	if action != "WAIT" && action != "VETO" {
		return false
	}
	if strings.TrimSpace(fmt.Sprint(report.Gate.Derived["sieve_action"])) != "" {
		return true
	}
	if strings.TrimSpace(fmt.Sprint(report.Gate.Derived["sieve_reason"])) != "" {
		return true
	}
	return false
}

func pickNarrativeSummary(report DecisionReport) string {
	if merged := mergeNarrativeSummary(report); strings.TrimSpace(merged) != "" {
		return merged
	}
	var fallback string
	for _, item := range append(report.Agents, report.Providers...) {
		summary := strings.TrimSpace(item.Summary)
		if summary == "" {
			continue
		}
		if fallback == "" {
			fallback = summary
		}
		if strings.Contains(summary, "状态：") || strings.Contains(summary, "动作：") || strings.Contains(summary, "冲突：") || strings.Contains(summary, "风险：") || strings.Contains(summary, "状态:") {
			return summary
		}
	}
	return fallback
}

func mergeNarrativeSummary(report DecisionReport) string {
	sections := map[string][]string{
		"状态": {},
		"动作": {},
		"冲突": {},
		"风险": {},
	}
	seen := map[string]map[string]struct{}{
		"状态": {},
		"动作": {},
		"冲突": {},
		"风险": {},
	}
	hasValue := false
	for _, stage := range orderedNarrativeStages(report) {
		summary := strings.TrimSpace(stage.Summary)
		if summary == "" {
			continue
		}
		if mergeNarrativeFromSummary(summary, sections, seen) {
			hasValue = true
		}
	}
	if !hasValue {
		return ""
	}
	labels := []string{"状态", "动作", "冲突", "风险"}
	lines := make([]string, 0, len(labels))
	for _, label := range labels {
		values := sections[label]
		if len(values) == 0 {
			lines = append(lines, fmt.Sprintf("%s：—", label))
			continue
		}
		lines = append(lines, fmt.Sprintf("%s：%s", label, strings.Join(values, "；")))
	}
	return strings.Join(lines, "\n")
}

func orderedNarrativeStages(report DecisionReport) []StageOutput {
	stages := append([]StageOutput{}, report.Agents...)
	stages = append(stages, report.Providers...)
	ordered := make([]StageOutput, 0, len(stages))
	used := make([]bool, len(stages))
	roleOrder := []string{"structure", "indicator", "mechanics"}
	for _, role := range roleOrder {
		for idx, stage := range stages {
			if used[idx] {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(stage.Role), role) {
				ordered = append(ordered, stage)
				used[idx] = true
			}
		}
	}
	for idx, stage := range stages {
		if used[idx] {
			continue
		}
		ordered = append(ordered, stage)
	}
	return ordered
}

func mergeNarrativeFromSummary(summary string, sections map[string][]string, seen map[string]map[string]struct{}) bool {
	lines := strings.Split(summary, "\n")
	hasValue := false
	for _, line := range lines {
		label, values, ok := parseNarrativeLine(line)
		if !ok {
			continue
		}
		if len(values) == 0 {
			continue
		}
		if _, ok := seen[label]; !ok {
			seen[label] = map[string]struct{}{}
		}
		for _, value := range values {
			trimmed := strings.TrimSpace(value)
			if isPlaceholderValue(trimmed) {
				continue
			}
			if _, exists := seen[label][trimmed]; exists {
				continue
			}
			seen[label][trimmed] = struct{}{}
			sections[label] = append(sections[label], trimmed)
			hasValue = true
		}
	}
	return hasValue
}

func parseNarrativeLine(line string) (string, []string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return "", nil, false
	}
	labels := []string{"状态", "动作", "冲突", "风险"}
	for _, label := range labels {
		if strings.HasPrefix(trimmed, label+"：") {
			value := strings.TrimSpace(strings.TrimPrefix(trimmed, label+"："))
			return label, splitNarrativeValues(value), true
		}
		if strings.HasPrefix(trimmed, label+":") {
			value := strings.TrimSpace(strings.TrimPrefix(trimmed, label+":"))
			return label, splitNarrativeValues(value), true
		}
	}
	return "", nil, false
}

func splitNarrativeValues(value string) []string {
	trimmed := strings.TrimSpace(value)
	if isPlaceholderValue(trimmed) {
		return nil
	}
	parts := strings.FieldsFunc(trimmed, func(r rune) bool {
		return r == '；' || r == ';'
	})
	if len(parts) == 0 {
		return nil
	}
	return parts
}

func splitNarrativeSections(summary string) map[string]string {
	sections := map[string][]string{
		"状态": {},
		"动作": {},
		"冲突": {},
		"风险": {},
	}
	matched := false
	for _, line := range strings.Split(summary, "\n") {
		label, values, ok := parseNarrativeLine(line)
		if !ok {
			continue
		}
		matched = true
		for _, value := range values {
			trimmed := strings.TrimSpace(value)
			if isPlaceholderValue(trimmed) {
				continue
			}
			sections[label] = append(sections[label], trimmed)
		}
	}
	output := make(map[string]string, 4)
	if !matched {
		output["状态"] = strings.TrimSpace(summary)
		return output
	}
	for label, values := range sections {
		output[label] = strings.Join(values, "；")
	}
	return output
}

func isPlaceholderValue(value string) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed == "" || trimmed == "—" || trimmed == "-"
}

func collectStageSummaries(report DecisionReport) []string {
	items := make([]string, 0, len(report.Agents)+len(report.Providers))
	for _, stage := range report.Agents {
		if strings.TrimSpace(stage.Summary) != "" {
			items = append(items, stage.Summary)
		}
	}
	for _, stage := range report.Providers {
		if strings.TrimSpace(stage.Summary) != "" {
			items = append(items, stage.Summary)
		}
	}
	return items
}

func formatBulletLines(lines []string) string {
	output := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		trimmed = strings.TrimPrefix(trimmed, "- ")
		trimmed = strings.TrimPrefix(trimmed, "• ")
		if trimmed == "" || trimmed == "-" || trimmed == "—" {
			continue
		}
		output = append(output, fmt.Sprintf("• %s", escapeHTML(trimmed)))
	}
	if len(output) == 0 {
		return ""
	}
	return strings.Join(output, "\n")
}

func wrapPre(content string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		trimmed = "—"
	}
	return fmt.Sprintf("<pre>%s</pre>", trimmed)
}

func joinHTMLSections(sections []string) string {
	parts := make([]string, 0, len(sections))
	for _, section := range sections {
		trimmed := strings.TrimSpace(section)
		if trimmed == "" {
			continue
		}
		parts = append(parts, trimmed)
	}
	return strings.Join(parts, "\n\n")
}

func trimHTMLRunes(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	runes := []rune(text)
	return string(runes[:limit])
}

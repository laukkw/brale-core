import fs from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import React from 'react';
import satori from 'satori';
import resvgJs from '@resvg/resvg-js';

const { Resvg } = resvgJs;

const __dirname = path.dirname(fileURLToPath(import.meta.url));

const DEFAULT_INPUT = path.resolve(__dirname, './sample-input.json');
const DEFAULT_OUTPUT = path.resolve(__dirname, 'ethusdt-og-card.png');
const CONFIG_PATH = path.resolve(__dirname, '../../configs/system.toml');
const AUTHOR_AVATAR_PATH = path.resolve(__dirname, 'auth.jpg');
const BRALE_LOGO_PATH = path.resolve(__dirname, './brale-icon-only.png');
const CARD_WIDTH = 640;
const CANVAS_WIDTH = CARD_WIDTH;
const DEFAULT_OUTPUT_WIDTH = 2048;
const DEFAULT_RENDER_HEIGHT = 1440;
const MAX_RENDER_HEIGHT = 4096;
const PAPER_TEXTURE_DATA_URI = "data:image/svg+xml,%3Csvg viewBox='0 0 200 200' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='noiseFilter'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.85' numOctaves='3' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23noiseFilter)' opacity='0.9'/%3E%3C/svg%3E";

const h = React.createElement;

const valueMap = new Map([
  ['ALLOW', '通过'],
  ['allow', '通过'],
  ['WAIT', '等待'],
  ['wait', '等待'],
  ['VETO', '否决'],
  ['veto', '否决'],
  ['none', '无方向'],
  ['CONSENSUS_NOT_PASSED', '三路共识未通过'],
  ['DIRECTION_UNCLEAR', '方向不明确'],
  ['DIRECTION_MISSING', '方向缺失'],
  ['DATA_MISSING', '数据不足'],
  ['STRUCT_BREAK', '结构失效'],
  ['STRUCT_HARD_INVALIDATION', '结构硬失效'],
  ['MECH_RISK', '清算风险过高'],
  ['LIQUIDATION_CASCADE', '连锁清算风险'],
  ['INDICATOR_NOISE', '指标噪音'],
  ['INDICATOR_MIXED', '指标混乱'],
  ['QUALITY_TOO_LOW', '建仓质量不足'],
  ['EDGE_TOO_LOW', '执行价值不足'],
  ['ALLOW', '通过'],
  ['PASS_STRONG', '强通过'],
  ['SIEVE_POLICY', '风控覆写'],
  ['GATE_MISSING', 'Gate 事件缺失'],
  ['direction', '方向'],
  ['data', '数据完整性'],
  ['structure', '结构完整性'],
  ['liquidation_cascade', '清算风险检查'],
  ['quality', '建仓质量'],
  ['edge', '执行价值'],
  ['mech_risk', '清算风险检查'],
  ['indicator_noise', '指标噪音'],
  ['structure_clear', '结构清晰度'],
  ['tag_consistency', '标签一致性'],
  ['script_select', '脚本选择'],
  ['script_allowed', '脚本条件'],
  ['gate_allow', 'Gate 放行'],
  ['indicator', '指标'],
  ['mechanics', '市场机制'],
  ['trend_up', '上行趋势'],
  ['trend_down', '下行趋势'],
  ['contracting', '收敛'],
  ['expanding', '扩张'],
  ['aligned', '一致'],
  ['divergent', '分歧'],
  ['mixed', '混合/分歧'],
  ['messy', '结构杂乱'],
  ['clean', '结构清晰'],
  ['unclear', '不明确'],
  ['unknown', '无法判断'],
  ['low', '低'],
  ['divergence_reversal', '背离反转风险'],
  ['pullback_entry', '回踩入场'],
  ['trend_surge', '趋势加速'],
  ['momentum_weak', '动能偏弱'],
  ['neutral', '中性/无明显倾向'],
  ['fuel_ready', '条件具备'],
  ['stable', '稳定'],
  ['increasing', '杠杆升温'],
  ['overheated', '过热'],
  ['balanced', '多空均衡'],
  ['long_crowded', '多头拥挤'],
  ['crowded_long', '多头拥挤'],
  ['short_crowded', '空头拥挤'],
  ['crowded_short', '空头拥挤'],
  ['liquidation_cascade', '连环清算风险'],
  ['breakout_confirmed', '突破确认'],
  ['support_retest', '回踩确认'],
  ['fakeout_rejection', '假突破回落'],
  ['structure_broken', '结构失效'],
  ['bos_up', '向上突破(BOS)'],
  ['bos_down', '向下突破(BOS)'],
  ['choch_up', '向上转折(CHoCH)'],
  ['choch_down', '向下转折(CHoCH)'],
  ['double_top', '双顶形态'],
  ['double_bottom', '双底形态'],
  ['head_shoulders', '头肩形态'],
  ['inv_head_shoulders', '反头肩形态'],
  ['triangle_sym', '对称三角形'],
  ['triangle_asc', '上升三角形'],
  ['triangle_desc', '下降三角形'],
  ['wedge_rising', '上升楔形'],
  ['wedge_falling', '下降楔形'],
  ['flag', '旗形整理'],
  ['pennant', '三角旗形'],
  ['channel_up', '上行通道'],
  ['channel_down', '下行通道'],
  ['medium', '中'],
  ['high', '高'],
  ['range', '区间震荡'],
  ['否决', '否决'],
]);

const sentenceMap = new Map([
  // general terms
  ['stable', '稳定'],
  ['medium', '中'],
  ['low', '低'],
  ['high', '高'],
  ['mixed', '混合'],
  ['positive', '正向'],
  ['negative', '负向'],
  ['bullish', '看多'],
  ['bearish', '看空'],
  ['neutral', '中性'],
  ['overbought', '超买'],
  ['oversold', '超卖'],
  ['slightly', '小幅'],
  ['significantly', '大幅'],
  ['versus', '对比'],
  ['but', '但'],
  ['and', '且'],
  // EMA / indicator terms
  ['ema_fast', '快线EMA'],
  ['ema_mid', '中线EMA'],
  ['ema_slow', '慢线EMA'],
  ['delta_pct', '变化率'],
  ['BB', '布林带'],
  ['CHOP', '震荡指数'],
  ['Aroon', '阿隆指标'],
  ['StochRSI', '随机RSI'],
  // OI / funding / mechanics
  ['OI increased', '持仓量上升'],
  ['OI decreased', '持仓量下降'],
  ['OI declined', '持仓量回落'],
  ['OI stable', '持仓量稳定'],
  ['increased', '上升'],
  ['decreased', '下降'],
  ['declined', '回落'],
  ['funding rate negative', '资金费率为负'],
  ['funding rate positive', '资金费率为正'],
  ['funding rate', '资金费率'],
  ['negative funding', '负资金费率'],
  ['open interest', '持仓量'],
  // crowding / anomaly
  ['long crowding', '多头拥挤'],
  ['short crowding', '空头拥挤'],
  ['fear_greed', '恐贪指数'],
  // time frames
  ['in 15m', '在15分钟内'],
  ['in 1h', '在1小时内'],
  ['in 4h', '在4小时内'],
  ['over 4h', '4小时以上'],
  ['over 1h', '1小时以上'],
  // structure
  ['expanding', '扩张'],
  ['contracting', '收缩'],
]);

function mapValue(value) {
  const key = String(value ?? '').trim();
  if (!key) return '';
  return valueMap.get(key) ?? valueMap.get(key.toLowerCase()) ?? key;
}

function mapSentence(text) {
  let output = String(text ?? '').trim();
  if (!output) return '—';
  for (const [source, target] of sentenceMap.entries()) {
    output = output.replaceAll(source, target);
  }
  return output;
}

function emptyDash(value) {
  const text = String(value ?? '').trim();
  return text || '—';
}

function parseNumber(value, fallback = 0) {
  const n = Number(value);
  return Number.isFinite(n) ? n : fallback;
}

function parseBool(value, fallback = false) {
  if (typeof value === 'boolean') return value;
  if (typeof value === 'number') return value !== 0;
  if (typeof value === 'string') {
    const normalized = value.trim().toLowerCase();
    if (normalized === 'true' || normalized === '1') return true;
    if (normalized === 'false' || normalized === '0') return false;
  }
  return fallback;
}

function parseKnownBool(value) {
  if (typeof value === 'boolean' || typeof value === 'number') {
    return { known: true, value: parseBool(value, false) };
  }
  if (typeof value === 'string') {
    const normalized = value.trim().toLowerCase();
    if (['true', 'false', '1', '0'].includes(normalized)) {
      return { known: true, value: parseBool(value, false) };
    }
  }
  return { known: false, value: false };
}

function trimFloat(value, digits = 4) {
  const n = Number(value);
  if (!Number.isFinite(n)) return '—';
  return n.toFixed(digits).replace(/\.0+$/, '').replace(/(\.\d*?)0+$/, '$1');
}

function translateExecutionBlockedReason(reason) {
  switch (String(reason ?? '').trim().toLowerCase()) {
    case 'monitor_gate':
      return '收紧监控门槛未满足';
    case 'atr_missing':
      return 'ATR 数据缺失';
    case 'atr_gate':
      return 'ATR 门槛未满足';
    case 'atr_value_missing':
      return 'ATR 数值缺失';
    case 'score_threshold':
      return '评分未达标';
    case 'score_parse':
      return '评分解析失败';
    case 'risk_plan_missing':
      return '风控计划缺失';
    case 'risk_plan_disabled':
      return '风控更新未启用';
    case 'price_unavailable':
      return '价格不可用';
    case 'price_source_missing':
      return '价格源缺失';
    case 'binding_missing':
      return '策略绑定缺失';
    case 'no_tighten_needed':
      return '执行阶段未发现更优止损';
    case 'not_evaluated':
      return '未完成评估';
    case 'tighten_debounce':
      return '收紧更新冷却中';
    default:
      return String(reason ?? '').trim();
  }
}

function formatPositionDirection(direction) {
  const normalized = String(direction ?? '').trim().toLowerCase();
  switch (normalized) {
    case 'long':
    case '多头':
      return '多头';
    case 'short':
    case '空头':
      return '空头';
    case 'conflict':
    case '信号冲突':
      return '信号冲突';
    default:
      return String(direction ?? '').trim() || '持仓中';
  }
}

function formatTakeProfitList(levels) {
  if (!Array.isArray(levels) || levels.length === 0) return '—';
  const values = levels
    .map((value) => trimFloat(value))
    .filter((value) => value !== '—');
  return values.length > 0 ? values.join(' / ') : '—';
}

function clamp(value, min, max) {
  return Math.max(min, Math.min(max, value));
}

function resolveOutputWidth(value) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) {
    return DEFAULT_OUTPUT_WIDTH;
  }
  return Math.round(clamp(parsed, CANVAS_WIDTH, 4096));
}

const OUTPUT_WIDTH = resolveOutputWidth(
  process.env.BRALE_NOTIFY_OG_OUTPUT_WIDTH
  ?? process.env.OG_OUTPUT_WIDTH
  ?? process.env.BRALE_NOTIFY_OG_EXPORT_WIDTH
  ?? process.env.OG_EXPORT_WIDTH
  ?? DEFAULT_OUTPUT_WIDTH,
);
const EXPORT_SCALE = Math.round((OUTPUT_WIDTH / CANVAS_WIDTH) * 100) / 100;

function ratioToPercent(value) {
  return clamp(Math.round(value * 100), 0, 100);
}

function withPercent(value) {
  return `${ratioToPercent(value)}%`;
}

function normalizeBaseSymbol(symbol) {
  const raw = String(symbol ?? '').trim().toUpperCase();
  if (!raw) return 'UNKNOWN';
  const quotes = ['USDT', 'USDC', 'BUSD', 'USD'];
  for (const quote of quotes) {
    if (raw.endsWith(quote) && raw.length > quote.length) {
      return raw.slice(0, -quote.length) || raw;
    }
  }
  return raw;
}

function formatTitlePrice(value) {
  const n = Number(value);
  if (!Number.isFinite(n)) return '';
  if (n === 0) return '0.00';
  if (Math.abs(n) < 0.005) {
    return n.toExponential(2).replace('e+', 'e');
  }
  return n.toFixed(2);
}

function resolveMarkPrice(raw) {
  const candidates = [
    raw?.current_price,
    raw?.mark_price,
    raw?.raw_blocks?.gate?.derived?.current_price,
    raw?.raw_blocks?.gate?.current_price,
    raw?.raw_blocks?.position?.mark_price,
  ];
  for (const candidate of candidates) {
    const n = Number(candidate);
    if (Number.isFinite(n)) {
      return n;
    }
  }
  return NaN;
}

function formatReportTime(date = new Date()) {
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
    timeZone: 'Asia/Shanghai',
  }).format(date).replace(/\//g, '-');
}

function resolveScale(model) {
  const analysisChars = model.analysisItems.reduce((sum, item) => sum + item.text.length, 0);
  const sourceChars = model.sourceCard.lines.reduce((sum, line) => sum + line.text.length, 0);
  const total = analysisChars + sourceChars;
  let factor = 1;
  if (total > 1800) {
    factor = 0.82;
  } else if (total > 1450) {
    factor = 0.88;
  } else if (total > 1200) {
    factor = 0.93;
  } else if (total > 1000) {
    factor = 0.96;
  }
  const scaled = (v, min) => Math.max(min, Math.round(v * factor));
  return {
    title: scaled(58, 42),
    subtitle: scaled(26, 20),
    body: scaled(20, 15),
    small: scaled(17, 13),
    tiny: scaled(13, 10),
    cardPad: scaled(26, 18),
    gap: scaled(18, 12),
    tagWidth: scaled(118, 92),
    progressHeight: scaled(14, 10),
    headerHeight: scaled(88, 74),
    avatar: scaled(42, 36),
  };
}

function estimateRenderHeight(model) {
  const titleChars = model.title.length;
  const sourceChars = model.sourceCard.lines.reduce((sum, line) => sum + line.text.length, 0);
  const progressChars = model.progressCards.reduce((sum, card) => sum + card.title.length + card.value.length + card.status.length, 0);
  const analysisChars = model.analysisItems.reduce((sum, item) => sum + item.tag.length + item.text.length, 0);
  const analysisGroupCount = model.analysisItems.filter((item) => item.isCategory).length;
  const analysisSeparatorCount = Math.max(0, analysisGroupCount - 1);
  const totalChars = titleChars + sourceChars + progressChars + analysisChars;

  return clamp(
    1024
      + Math.ceil(totalChars * 0.88)
      + model.sourceCard.lines.length * 24
      + model.progressCards.length * 28
      + model.analysisItems.length * 32
      + analysisSeparatorCount * 24,
    DEFAULT_RENDER_HEIGHT,
    MAX_RENDER_HEIGHT,
  );
}

export function buildModel(raw) {
  const cardType = String(raw?.card_type || 'decision').trim();
  switch (cardType) {
    case 'position_open':
      return buildPositionOpenModel(raw);
    case 'position_close':
      return buildPositionCloseModel(raw);
    case 'risk_update':
      return buildRiskUpdateModel(raw);
    case 'startup':
      return buildStartupModel(raw);
    case 'partial_close':
      return buildPartialCloseModel(raw);
    default:
      return buildDecisionModel(raw);
  }
}

function buildDecisionModel(raw) {
  const gate = raw?.raw_blocks?.gate ?? {};
  const agent = raw?.raw_blocks?.agent ?? {};
  const indicator = agent.indicator ?? {};
  const mechanics = agent.mechanics ?? {};
  const structure = agent.structure ?? {};
  const symbol = normalizeBaseSymbol(raw?.symbol || 'UNKNOWN');
  const markPrice = resolveMarkPrice(raw);
  const titlePrice = formatTitlePrice(markPrice);

  const consensusRaw = gate.direction_consensus ?? {};
  const fallbackConsensusScore = Math.max(
    parseNumber(indicator.movement_score, 0),
    parseNumber(mechanics.movement_score, 0),
    parseNumber(structure.movement_score, 0),
  );
  const fallbackConsensusConfidence = (
    parseNumber(indicator.movement_confidence, 0) +
    parseNumber(mechanics.movement_confidence, 0) +
    parseNumber(structure.movement_confidence, 0)
  ) / 3;

  const consensusScore = parseNumber(consensusRaw.score, fallbackConsensusScore);
  const consensusConfidence = parseNumber(consensusRaw.confidence, fallbackConsensusConfidence);
  const consensusScoreThreshold = parseNumber(consensusRaw.score_threshold, 0.5);
  const consensusConfidenceThreshold = parseNumber(consensusRaw.confidence_threshold, 0.6);

  const scoreRate = consensusScoreThreshold > 0 ? Math.abs(consensusScore) / consensusScoreThreshold : 1;
  const confidenceRate = consensusConfidenceThreshold > 0 ? consensusConfidence / consensusConfidenceThreshold : 1;

  const scorePassed = Object.hasOwn(consensusRaw, 'score_passed')
    ? parseBool(consensusRaw.score_passed, Math.abs(consensusScore) >= consensusScoreThreshold)
    : Math.abs(consensusScore) >= consensusScoreThreshold;
  const confidencePassed = Object.hasOwn(consensusRaw, 'confidence_passed')
    ? parseBool(consensusRaw.confidence_passed, consensusConfidence >= consensusConfidenceThreshold)
    : consensusConfidence >= consensusConfidenceThreshold;

  const summaryCards = [
    {
      key: 'score',
      title: '共识总分',
      value: `当前 ${consensusScore.toFixed(3)} / 达成率 ${withPercent(scoreRate)}`,
      progressPct: ratioToPercent(Math.abs(consensusScore)),
      thresholdPct: ratioToPercent(consensusScoreThreshold),
      thresholdLabel: '方向阈值',
      thresholdText: consensusScoreThreshold.toFixed(3),
      status: scorePassed ? '达到方向门槛' : '未达方向门槛',
      tone: scorePassed ? 'emerald' : 'rose',
      isSuccess: scorePassed,
    },
    {
      key: 'confidence',
      title: '共识置信度',
      value: `当前 ${consensusConfidence.toFixed(3)} / 达成率 ${withPercent(confidenceRate)}`,
      progressPct: ratioToPercent(consensusConfidence),
      thresholdPct: ratioToPercent(consensusConfidenceThreshold),
      thresholdLabel: '置信阈值',
      thresholdText: consensusConfidenceThreshold.toFixed(3),
      status: confidencePassed ? '达到置信门槛' : '未达置信门槛',
      tone: confidencePassed ? 'emerald' : 'amber',
      isSuccess: confidencePassed,
    },
  ];

  const setupQuality = parseNumber(gate.setup_quality, 0);
  const entryEdge = parseNumber(gate.entry_edge, 0);
  const qualityThreshold = parseNumber(gate.quality_threshold, 0.35);
  const edgeThreshold = parseNumber(gate.edge_threshold, 0.10);

  const qualityRate = qualityThreshold > 0 ? setupQuality / qualityThreshold : 1;
  const edgeRate = edgeThreshold > 0 ? entryEdge / edgeThreshold : 1;
  const qualityPassed = setupQuality >= qualityThreshold;
  const edgePassed = entryEdge >= edgeThreshold;

  const hasQualityData = setupQuality > 0 || entryEdge > 0;
  const execution = gate.execution && typeof gate.execution === 'object' ? gate.execution : null;
  const hasEntryPlan = execution && (parseNumber(execution.stop_loss, 0) > 0 || Array.isArray(execution.take_profits));

  let evidenceCards;
  if (hasQualityData) {
    evidenceCards = [
      {
        key: 'setup_quality',
        title: '建仓质量',
        value: `当前 ${setupQuality.toFixed(3)} / 达成率 ${withPercent(qualityRate)}`,
        progressPct: ratioToPercent(setupQuality),
        thresholdPct: ratioToPercent(qualityThreshold),
        thresholdLabel: '质量阈值',
        thresholdText: qualityThreshold.toFixed(2),
        status: qualityPassed ? '达到质量门槛' : '质量不足',
        tone: qualityPassed ? 'emerald' : 'amber',
        isSuccess: qualityPassed,
      },
      {
        key: 'entry_edge',
        title: '执行价值',
        value: `当前 ${entryEdge.toFixed(3)} / 达成率 ${withPercent(edgeRate)}`,
        progressPct: ratioToPercent(entryEdge),
        thresholdPct: ratioToPercent(edgeThreshold),
        thresholdLabel: '执行阈值',
        thresholdText: edgeThreshold.toFixed(2),
        status: edgePassed ? '达到执行门槛' : '执行价值不足',
        tone: edgePassed ? 'emerald' : 'amber',
        isSuccess: edgePassed,
      },
    ];
  } else {
    // Replace empty quality/edge with agent signal strength
    const indicatorScore = parseNumber(indicator.movement_score, 0);
    const mechanicsScore = parseNumber(mechanics.movement_score, 0);
    const structureScore = parseNumber(structure.movement_score, 0);
    const maxAgentScore = Math.max(Math.abs(indicatorScore), Math.abs(mechanicsScore), Math.abs(structureScore));
    const maxAgentName = Math.abs(indicatorScore) >= Math.abs(mechanicsScore) && Math.abs(indicatorScore) >= Math.abs(structureScore)
      ? '指标' : Math.abs(mechanicsScore) >= Math.abs(structureScore) ? '市场机制' : '结构';
    evidenceCards = [
      {
        key: 'max_signal',
        title: '最强信号',
        value: `${maxAgentName} ${maxAgentScore.toFixed(3)}`,
        progressPct: ratioToPercent(Math.abs(maxAgentScore)),
        thresholdPct: ratioToPercent(consensusScoreThreshold),
        thresholdLabel: '方向阈值',
        thresholdText: consensusScoreThreshold.toFixed(3),
        status: Math.abs(maxAgentScore) >= consensusScoreThreshold ? '信号有效' : '信号不足',
        tone: Math.abs(maxAgentScore) >= consensusScoreThreshold ? 'emerald' : 'rose',
        isSuccess: Math.abs(maxAgentScore) >= consensusScoreThreshold,
      },
      {
        key: 'risk_penalty',
        title: '风控惩罚',
        value: `${parseNumber(gate.risk_penalty, 0).toFixed(3)}`,
        progressPct: ratioToPercent(Math.abs(parseNumber(gate.risk_penalty, 0))),
        thresholdPct: 50,
        thresholdLabel: '中性线',
        thresholdText: '0.00',
        status: parseNumber(gate.risk_penalty, 0) <= 0 ? '无惩罚' : '扣分中',
        tone: parseNumber(gate.risk_penalty, 0) <= 0 ? 'emerald' : 'amber',
        isSuccess: parseNumber(gate.risk_penalty, 0) <= 0,
      },
    ];
  }

  const trace = Array.isArray(gate.trace)
    ? gate.trace.filter((item) => item && typeof item === 'object' && String(item.step ?? '').trim())
    : [];
  const failedTrace = trace.find((item) => {
    const status = parseKnownBool(item.ok);
    return status.known && status.value === false;
  }) || null;

  const stopStep = emptyDash(mapValue(gate.stop_step || failedTrace?.step));
  const finalDecision = String(gate.decision_action || '').trim().toUpperCase();
  const ruleName = String(gate.rule_name || '').trim();
  const sieveAction = String(gate.sieve_action || '').trim();
  const sieveReason = String(gate.sieve_reason || '').trim();
  const actionBefore = String(gate.action_before || '').trim();
  const sieveTriggered = Boolean(sieveAction || sieveReason);
  const tradeable = parseBool(gate.tradeable, false);
  const sieveOutcome = String(sieveAction || finalDecision || '').trim().toUpperCase();
  const sieveChanged = Boolean(sieveTriggered && actionBefore && sieveOutcome && actionBefore.toUpperCase() !== sieveOutcome);

  const sourceLabel = (sieveTriggered && (finalDecision === 'VETO' || sieveChanged))
    ? '风控覆写'
    : failedTrace
      ? 'Gate 主流程'
      : 'Gate 总结';

  const sourceLines = [
    {
      text: failedTrace
        ? `停止步骤：${stopStep}${failedTrace?.reason ? ` · 命中 ${emptyDash(mapValue(failedTrace.reason))}` : ''}`
        : sieveTriggered
          ? '停止步骤：Gate 未中断'
          : tradeable
            ? '停止步骤：无（Gate 放行）'
            : `停止步骤：${stopStep}`,
      note: false,
      kind: 'danger',
    },
  ];
  const blockedBy = Array.isArray(execution?.blocked_by)
    ? execution.blocked_by.map((item) => translateExecutionBlockedReason(item)).filter(Boolean)
    : [];
  const positionText = formatPositionDirection(gate.direction);
  const isTightenContext = String(execution?.action ?? '').trim().toLowerCase() === 'tighten';
  if (isTightenContext) {
    if (parseBool(execution.executed, false)) {
      sourceLines.push({
        text: '持仓处理：已执行收紧',
        note: false,
        kind: 'default',
      });
    } else if (blockedBy.length > 0) {
      sourceLines.push({
        text: `持仓处理：收紧未执行 · 原因：${blockedBy.join(' / ')}`,
        note: false,
        kind: 'danger',
      });
    } else if (parseBool(execution.evaluated, false)) {
      sourceLines.push({
        text: '持仓处理：收紧未触发',
        note: false,
        kind: 'default',
      });
    }
    sourceLines.push({
      text: `当前仓位：${positionText}`,
      note: false,
      kind: 'default',
    });
    if (parseBool(execution.executed, false)) {
      sourceLines.push({
        text: `止损：${trimFloat(execution.stop_loss)} · 止盈：${formatTakeProfitList(execution.take_profits)}`,
        note: false,
        kind: 'default',
      });
    }
  }
  if (ruleName) {
    sourceLines.push({
      text: `命中规则：${emptyDash(mapValue(ruleName))} (${ruleName})`,
      note: false,
      kind: 'default',
    });
  }
  if (sieveTriggered) {
    sourceLines.push({
      text: `风控筛选：${emptyDash(mapValue(actionBefore || '—'))} → ${emptyDash(mapValue(sieveAction || finalDecision || '—'))}${sieveReason ? ` · ${emptyDash(mapValue(sieveReason))}` : ''}`,
      note: false,
      kind: 'default',
    });
  }
  // Show entry plan when gate allows
  if (tradeable && hasEntryPlan) {
    const slText = trimFloat(execution.stop_loss);
    const tpList = formatTakeProfitList(execution.take_profits);
    sourceLines.push({
      text: `📌 开仓计划 — 止损：${slText} · 止盈：${tpList}`,
      note: false,
      kind: 'success',
    });
  }
  sourceLines.push({
    text: '说明：共识卡展示方向/置信门槛；结构与清算风险卡展示判断可靠度阈值。',
    note: true,
    kind: 'note',
  });

  const sourceCard = {
    title: '判定来源说明',
    tradeable,
    sourceLabel,
    lines: sourceLines,
    verdictText: tradeable ? '可交易' : '不可交易',
  };

  const analysisItems = [
    {
      tag: '指标综合',
      text: `扩张状态=${emptyDash(mapValue(indicator.expansion))} 一致性=${emptyDash(mapValue(indicator.alignment))} 噪音=${emptyDash(mapValue(indicator.noise))}`,
      variant: 'indicator',
      isCategory: true,
    },
    {
      tag: '动能细节',
      text: mapSentence(indicator.momentum_detail),
      variant: 'indicator',
      isCategory: false,
    },
    {
      tag: '冲突细节',
      text: emptyDash(mapSentence(indicator.conflict_detail)),
      variant: 'indicator',
      isCategory: false,
    },
    {
      tag: '市场机制',
      text: `杠杆=${emptyDash(mapValue(mechanics.leverage_state))} 拥挤度=${emptyDash(mapValue(mechanics.crowding))} 风险等级=${emptyDash(mapValue(mechanics.risk_level))}`,
      variant: 'mechanics',
      isCategory: true,
    },
    {
      tag: '持仓量背景',
      text: mapSentence(mechanics.open_interest_context),
      variant: 'mechanics',
      isCategory: false,
    },
    {
      tag: '异常细节',
      text: mapSentence(mechanics.anomaly_detail),
      variant: 'mechanics',
      isCategory: false,
    },
    {
      tag: '结构分析',
      text: `结构状态=${emptyDash(mapValue(structure.regime))} 最近突破=${emptyDash(mapValue(structure.last_break))} 形态=${emptyDash(mapValue(structure.pattern))} 质量=${emptyDash(mapValue(structure.quality))}`,
      variant: 'structure',
      isCategory: true,
    },
    {
      tag: '结构细节',
      text: `量能配合=${mapSentence(structure.volume_action)} K线反应=${mapSentence(structure.candle_reaction)}`,
      variant: 'structure',
      isCategory: false,
    },
  ];

  return {
    symbol,
    title: `${symbol} 的决策报告`,
    titlePrice,
    reportTimeCN: formatReportTime(),
    sourceCard,
    progressCards: [...summaryCards, ...evidenceCards],
    analysisItems,
  };
}

// ===== Position Open Card =====
function buildPositionOpenModel(raw) {
  const d = raw?.data ?? {};
  const symbol = normalizeBaseSymbol(raw?.symbol || 'UNKNOWN');
  const direction = String(d.direction || '-').trim();
  const directionCN = direction === 'long' ? '做多' : direction === 'short' ? '做空' : direction;
  const entryPrice = parseNumber(d.entry_price, 0);
  const stopPrice = parseNumber(d.stop_price, 0);
  const riskPct = parseNumber(d.risk_pct, 0);
  const leverage = parseNumber(d.leverage, 0);
  const qty = parseNumber(d.qty, 0);
  const tpList = Array.isArray(d.take_profits) ? d.take_profits.filter((v) => v > 0) : [];

  const sourceCard = {
    title: '开仓详情',
    tradeable: true,
    sourceLabel: '仓位管理',
    verdictText: `${directionCN}开仓`,
    lines: [
      { text: `方向：${directionCN} · 数量：${trimFloat(qty)}`, note: false, kind: 'default' },
      { text: `开仓价格：${trimFloat(entryPrice)}`, note: false, kind: 'default' },
      { text: `止损：${stopPrice > 0 ? trimFloat(stopPrice) : '—'} · 止盈：${formatTakeProfitList(tpList)}`, note: false, kind: 'success' },
      ...(String(d.stop_reason || '').trim() ? [{ text: `止损策略：${d.stop_reason}`, note: false, kind: 'default' }] : []),
    ],
  };

  const progressCards = [
    buildSimpleInfoCard('开仓价格', trimFloat(entryPrice), 'emerald'),
    buildSimpleInfoCard('止损价格', stopPrice > 0 ? trimFloat(stopPrice) : '—', stopPrice > 0 ? 'amber' : 'rose'),
    buildSimpleInfoCard('风险比例', riskPct > 0 ? `${(riskPct * 100).toFixed(1)}%` : '—', 'amber'),
    buildSimpleInfoCard('杠杆倍数', leverage > 0 ? `${leverage}x` : '—', 'emerald'),
  ];

  return {
    symbol,
    title: `${symbol} 开仓通知`,
    titlePrice: entryPrice > 0 ? trimFloat(entryPrice) : '',
    reportTimeCN: formatReportTime(),
    sourceCard,
    progressCards,
    analysisItems: tpList.length > 0 ? tpList.map((tp, i) => ({
      tag: `止盈 ${i + 1}`,
      text: trimFloat(tp),
      variant: 'indicator',
      isCategory: i === 0,
    })) : [],
  };
}

// ===== Position Close Card =====
function buildPositionCloseModel(raw) {
  const d = raw?.data ?? {};
  const symbol = normalizeBaseSymbol(raw?.symbol || 'UNKNOWN');
  const direction = String(d.direction || '-').trim();
  const directionCN = direction === 'long' ? '做多' : direction === 'short' ? '做空' : direction;
  const closeType = String(d.close_type || 'full').trim();
  const isFullClose = closeType === 'full';
  const entryPrice = parseNumber(d.entry_price, 0);
  const exitPrice = parseNumber(d.exit_price, 0) || parseNumber(d.trigger_price, 0);
  const pnlAmount = parseNumber(d.pnl_amount, 0);
  const pnlPct = parseNumber(d.pnl_pct, 0);
  const reason = String(d.reason || d.exit_reason || '-').trim();
  const qty = parseNumber(d.qty, 0);
  const leverage = parseNumber(d.leverage, 0);
  const tradeDuration = parseNumber(d.trade_duration_s, 0);

  const isProfit = pnlAmount > 0;
  const headerEmoji = isProfit ? '📈' : '📉';
  const headerText = isFullClose ? '全部平仓' : '仓位关闭';
  const pnlDisplay = pnlAmount !== 0 ? `${isProfit ? '+' : ''}${trimFloat(pnlAmount)} (${isProfit ? '+' : ''}${(pnlPct * 100).toFixed(2)}%)` : '—';

  const sourceLines = [
    { text: `方向：${directionCN} · 数量：${trimFloat(qty)}`, note: false, kind: 'default' },
  ];
  if (entryPrice > 0) sourceLines.push({ text: `入场价：${trimFloat(entryPrice)}`, note: false, kind: 'default' });
  if (exitPrice > 0) sourceLines.push({ text: `出场价：${trimFloat(exitPrice)}`, note: false, kind: 'default' });
  if (pnlAmount !== 0) sourceLines.push({ text: `${headerEmoji} 盈亏：${pnlDisplay}`, note: false, kind: isProfit ? 'success' : 'danger' });
  sourceLines.push({ text: `原因：${mapValue(reason)}`, note: false, kind: 'default' });
  if (tradeDuration > 0) sourceLines.push({ text: `持仓时长：${formatDuration(tradeDuration)}`, note: false, kind: 'default' });

  const sourceCard = {
    title: `${headerText}详情`,
    tradeable: false,
    sourceLabel: isFullClose ? '全部平仓' : '部分关闭',
    verdictText: `${directionCN}${headerText}`,
    lines: sourceLines,
  };

  const progressCards = [];
  if (entryPrice > 0) progressCards.push(buildSimpleInfoCard('入场价', trimFloat(entryPrice), 'emerald'));
  if (exitPrice > 0) progressCards.push(buildSimpleInfoCard('出场价', trimFloat(exitPrice), isProfit ? 'emerald' : 'rose'));
  progressCards.push(buildSimpleInfoCard('盈亏', pnlDisplay, isProfit ? 'emerald' : 'rose'));
  progressCards.push(buildSimpleInfoCard('杠杆', leverage > 0 ? `${leverage}x` : '—', 'amber'));

  return {
    symbol,
    title: `${symbol} ${headerText}`,
    titlePrice: exitPrice > 0 ? trimFloat(exitPrice) : '',
    reportTimeCN: formatReportTime(),
    sourceCard,
    progressCards,
    analysisItems: [],
  };
}

// ===== Risk Update Card =====
function buildRiskUpdateModel(raw) {
  const d = raw?.data ?? {};
  const symbol = normalizeBaseSymbol(raw?.symbol || 'UNKNOWN');
  const direction = String(d.direction || '-').trim();
  const directionCN = direction === 'long' ? '做多' : direction === 'short' ? '做空' : direction;
  const entryPrice = parseNumber(d.entry_price, 0);
  const oldStop = parseNumber(d.old_stop, 0);
  const newStop = parseNumber(d.new_stop, 0);
  const markPrice = parseNumber(d.mark_price, 0);
  const source = String(d.source || '-').trim();
  const sourceMap = { tighten: '收紧', initial: '初始', manual: '手动', native: '原生策略', llm: 'LLM 策略' };
  const sourceCN = sourceMap[source.toLowerCase()] || source;
  const tpList = Array.isArray(d.take_profits) ? d.take_profits.filter((v) => v > 0) : [];
  const gateSatisfied = parseBool(d.gate_satisfied, false);
  const scoreTotal = parseNumber(d.score_total, 0);
  const scoreThreshold = parseNumber(d.score_threshold, 0);
  const tightenReason = String(d.tighten_reason || '-').trim();
  const stopReason = String(d.stop_reason || d.reason || '-').trim();

  const sourceLines = [
    { text: `方向：${directionCN} · 来源：${sourceCN}`, note: false, kind: 'default' },
    { text: `旧止损：${oldStop > 0 ? trimFloat(oldStop) : '—'} → 新止损：${newStop > 0 ? trimFloat(newStop) : '—'}`, note: false, kind: oldStop !== newStop ? 'success' : 'default' },
    { text: `止盈：${formatTakeProfitList(tpList)}`, note: false, kind: 'default' },
    { text: `止损原因：${mapValue(stopReason)}`, note: false, kind: 'default' },
  ];
  if (tightenReason !== '-') sourceLines.push({ text: `收紧原因：${mapValue(tightenReason)}`, note: false, kind: 'default' });
  if (scoreTotal !== 0) sourceLines.push({ text: `Gate 得分：${scoreTotal.toFixed(3)} / 阈值 ${scoreThreshold.toFixed(3)} · ${gateSatisfied ? '✅ 满足' : '❌ 不满足'}`, note: false, kind: gateSatisfied ? 'success' : 'danger' });

  const sourceCard = {
    title: '风控计划更新',
    tradeable: gateSatisfied,
    sourceLabel: sourceCN,
    verdictText: `${directionCN} · ${sourceCN}`,
    lines: sourceLines,
  };

  const progressCards = [
    buildSimpleInfoCard('入场价', entryPrice > 0 ? trimFloat(entryPrice) : '—', 'emerald'),
    buildSimpleInfoCard('新止损', newStop > 0 ? trimFloat(newStop) : '—', 'amber'),
    buildSimpleInfoCard('标记价', markPrice > 0 ? trimFloat(markPrice) : '—', 'emerald'),
    buildSimpleInfoCard('Gate', gateSatisfied ? '满足' : '不满足', gateSatisfied ? 'emerald' : 'rose'),
  ];

  return {
    symbol,
    title: `${symbol} 风控更新`,
    titlePrice: markPrice > 0 ? trimFloat(markPrice) : '',
    reportTimeCN: formatReportTime(),
    sourceCard,
    progressCards,
    analysisItems: tpList.length > 0 ? tpList.map((tp, i) => ({
      tag: `止盈 ${i + 1}`,
      text: trimFloat(tp),
      variant: 'indicator',
      isCategory: i === 0,
    })) : [],
  };
}

// ===== Startup Card =====
function buildStartupModel(raw) {
  const d = raw?.data ?? {};
  const symbols = Array.isArray(d.symbols) ? d.symbols : [];
  const intervals = Array.isArray(d.intervals) ? d.intervals : [];
  const barInterval = String(d.bar_interval || '-').trim();
  const balance = parseNumber(d.balance, 0);
  const currency = String(d.currency || 'USDT').trim();
  const scheduleMode = String(d.schedule_mode || '自动调度').trim();

  const sourceCard = {
    title: '系统启动',
    tradeable: true,
    sourceLabel: '启动完成',
    verdictText: '🚀 Brale 已启动',
    lines: [
      { text: `监控币种：${symbols.length > 0 ? symbols.join(', ') : '—'}`, note: false, kind: 'default' },
      { text: `分析周期：${intervals.length > 0 ? intervals.join(', ') : '—'}`, note: false, kind: 'default' },
      { text: `调度模式：${scheduleMode} · 决策间隔：${barInterval}`, note: false, kind: 'default' },
      ...(balance > 0 ? [{ text: `账户余额：${trimFloat(balance)} ${currency}`, note: false, kind: 'success' }] : []),
    ],
  };

  const progressCards = [
    buildSimpleInfoCard('币种数量', `${symbols.length}`, 'emerald'),
    buildSimpleInfoCard('分析周期', `${intervals.length}`, 'emerald'),
    buildSimpleInfoCard('决策间隔', barInterval, 'amber'),
    buildSimpleInfoCard('余额', balance > 0 ? `${trimFloat(balance)} ${currency}` : '—', 'emerald'),
  ];

  return {
    symbol: 'BRALE',
    title: 'Brale 系统启动',
    titlePrice: '',
    reportTimeCN: formatReportTime(),
    sourceCard,
    progressCards,
    analysisItems: symbols.map((s, i) => ({
      tag: `币种 ${i + 1}`,
      text: s,
      variant: 'indicator',
      isCategory: i === 0,
    })),
  };
}

// ===== Partial Close Card =====
function buildPartialCloseModel(raw) {
  const d = raw?.data ?? {};
  const symbol = normalizeBaseSymbol(raw?.symbol || 'UNKNOWN');
  const direction = String(d.direction || '-').trim();
  const directionCN = direction === 'long' ? '做多' : direction === 'short' ? '做空' : direction;
  const openRate = parseNumber(d.open_rate, 0);
  const closeRate = parseNumber(d.close_rate, 0);
  const amount = parseNumber(d.amount, 0);
  const realizedProfit = parseNumber(d.realized_profit, 0);
  const realizedProfitRatio = parseNumber(d.realized_profit_ratio, 0);
  const exitReason = String(d.exit_reason || '-').trim();
  const exitType = String(d.exit_type || '-').trim();
  const isProfit = realizedProfit > 0;

  const sourceCard = {
    title: '部分平仓详情',
    tradeable: false,
    sourceLabel: '部分平仓',
    verdictText: `${directionCN} · 部分平仓`,
    lines: [
      { text: `方向：${directionCN} · 数量：${trimFloat(amount)}`, note: false, kind: 'default' },
      { text: `开仓价：${trimFloat(openRate)} → 平仓价：${trimFloat(closeRate)}`, note: false, kind: 'default' },
      { text: `${isProfit ? '📈' : '📉'} 已实现盈亏：${isProfit ? '+' : ''}${trimFloat(realizedProfit)} (${isProfit ? '+' : ''}${(realizedProfitRatio * 100).toFixed(2)}%)`, note: false, kind: isProfit ? 'success' : 'danger' },
      { text: `退出原因：${mapValue(exitReason)} · 类型：${mapValue(exitType)}`, note: false, kind: 'default' },
    ],
  };

  const progressCards = [
    buildSimpleInfoCard('开仓价', trimFloat(openRate), 'emerald'),
    buildSimpleInfoCard('平仓价', trimFloat(closeRate), isProfit ? 'emerald' : 'rose'),
    buildSimpleInfoCard('已实现盈亏', `${isProfit ? '+' : ''}${trimFloat(realizedProfit)}`, isProfit ? 'emerald' : 'rose'),
    buildSimpleInfoCard('盈亏比率', `${(realizedProfitRatio * 100).toFixed(2)}%`, isProfit ? 'emerald' : 'rose'),
  ];

  return {
    symbol,
    title: `${symbol} 部分平仓`,
    titlePrice: closeRate > 0 ? trimFloat(closeRate) : '',
    reportTimeCN: formatReportTime(),
    sourceCard,
    progressCards,
    analysisItems: [],
  };
}

// ===== Shared Helpers for Card Models =====
function buildSimpleInfoCard(title, value, tone) {
  return {
    key: title,
    title,
    value,
    progressPct: 100,
    thresholdPct: 0,
    thresholdLabel: '',
    thresholdText: '',
    status: '',
    tone,
    isSuccess: tone === 'emerald',
    isSimple: true,
  };
}

function formatDuration(seconds) {
  if (seconds <= 0) return '—';
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (h > 0) return `${h}小时${m > 0 ? ` ${m}分钟` : ''}`;
  if (m > 0) return `${m}分钟`;
  return `${seconds}秒`;
}

function tonePalette(tone) {
  if (tone === 'emerald') {
    return {
      fill: '#34d399',
      badgeBg: '#ecfdf5',
      badgeText: '#059669',
      badgeBorder: '#a7f3d0',
    };
  }
  if (tone === 'amber') {
    return {
      fill: '#fbbf24',
      badgeBg: '#fffbeb',
      badgeText: '#b45309',
      badgeBorder: '#fde68a',
    };
  }
  return {
    fill: '#fb7185',
    badgeBg: '#fff1f2',
    badgeText: '#e11d48',
    badgeBorder: '#fecdd3',
  };
}

function categoryTagStyle(variant) {
  const map = {
    indicator: { bg: '#dcfce7', fg: '#166534', border: '#86efac' },
    mechanics: { bg: '#dcfce7', fg: '#166534', border: '#86efac' },
    structure: { bg: '#dcfce7', fg: '#166534', border: '#86efac' },
    default: { bg: '#e5e7eb', fg: '#111827', border: '#9ca3af' },
  };
  return map[variant] ?? map.default;
}

function normalTagStyle(variant) {
  const map = {
    indicator: { bg: '#f1f5f9', fg: '#475569', border: '#cbd5e1' },
    mechanics: { bg: '#f1f5f9', fg: '#475569', border: '#cbd5e1' },
    structure: { bg: '#f1f5f9', fg: '#475569', border: '#cbd5e1' },
    default: { bg: '#f8fafc', fg: '#4b5563', border: '#d1d5db' },
  };
  return map[variant] ?? map.default;
}

function progressCard(card, scale) {
  const tone = tonePalette(card.tone);
  const thresholdColor = '#2563eb';

  // Simple info card: large value display without progress bar
  if (card.isSimple) {
    return h(
      'div',
      {
        style: {
          display: 'flex',
          flexDirection: 'column',
          justifyContent: 'center',
          gap: Math.max(6, scale.gap - 12),
          width: '100%',
          minWidth: 0,
          background: 'rgba(255,255,255,0.72)',
          border: '1px solid rgba(203,213,225,0.9)',
          borderRadius: 18,
          padding: `${Math.max(12, scale.cardPad - 10)}px ${Math.max(14, scale.cardPad - 8)}px`,
        },
      },
      h(
        'div',
        { style: { display: 'flex', fontSize: scale.small, color: '#6b7280', fontWeight: 600 } },
        card.title,
      ),
      h(
        'div',
        {
          style: {
            display: 'flex',
            fontSize: Math.max(22, scale.heading - 2),
            color: tone.badgeText,
            fontWeight: 800,
            fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
          },
        },
        card.value,
      ),
    );
  }

  return h(
    'div',
    {
      style: {
        display: 'flex',
        flexDirection: 'column',
        justifyContent: 'space-between',
        gap: Math.max(8, scale.gap - 10),
        width: '100%',
        minWidth: 0,
        background: 'rgba(255,255,255,0.72)',
        border: '1px solid rgba(203,213,225,0.9)',
        borderRadius: 18,
        padding: `${Math.max(12, scale.cardPad - 10)}px ${Math.max(14, scale.cardPad - 8)}px`,
      },
    },
    h(
      'div',
      {
        style: {
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'flex-start',
          gap: 10,
        },
      },
      h(
        'div',
        {
          style: {
            display: 'flex',
            flexDirection: 'column',
            gap: 4,
            flex: 1,
            minWidth: 0,
          },
        },
        h(
          'div',
          {
            style: {
              display: 'flex',
              fontSize: scale.small,
              color: '#1f2937',
              fontWeight: 700,
              whiteSpace: 'pre-wrap',
              wordBreak: 'break-word',
            },
          },
          card.title,
        ),
        h(
          'div',
          {
            style: {
              display: 'flex',
              fontSize: scale.tiny,
              color: '#6b7280',
              whiteSpace: 'pre-wrap',
              wordBreak: 'break-word',
              lineHeight: 1.35,
              fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
            },
          },
          card.value,
        ),
      ),
      h(
        'div',
        {
          style: {
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            padding: '4px 8px',
            borderRadius: 8,
            border: `1px solid ${tone.badgeBorder}`,
            background: tone.badgeBg,
            color: tone.badgeText,
            fontSize: Math.max(10, scale.tiny - 2),
            fontWeight: 700,
            whiteSpace: 'nowrap',
          },
        },
        card.status,
      ),
    ),
    h(
      'div',
      {
        style: {
          display: 'flex',
          position: 'relative',
          width: '100%',
          height: scale.progressHeight,
          borderRadius: 999,
          background: '#e5e7eb',
          overflow: 'hidden',
        },
      },
      h('div', {
        style: {
          display: 'flex',
          position: 'absolute',
          left: 0,
          top: 0,
          bottom: 0,
          width: `${card.progressPct}%`,
          background: tone.fill,
          borderRadius: 999,
        },
      }),
      h('div', {
        style: {
          display: 'flex',
          position: 'absolute',
          top: -1,
          bottom: -1,
          left: `${card.thresholdPct}%`,
          width: 2,
          transform: 'translateX(-50%)',
          background: thresholdColor,
        },
      }),
    ),
    h(
      'div',
      {
        style: {
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          fontSize: Math.max(10, scale.tiny - 2),
          color: '#9ca3af',
          fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
        },
      },
      h('div', { style: { display: 'flex' } }, `当前 ${card.progressPct}%`),
      h('div', { style: { display: 'flex', color: thresholdColor, fontWeight: 700 } }, `${card.thresholdLabel || '阈值'} ${card.thresholdText}`),
    ),
  );
}

function analysisItem(item, scale) {
  const tagStyle = item.isCategory ? categoryTagStyle(item.variant) : normalTagStyle(item.variant);
  return h(
    'div',
    {
      style: {
        display: 'flex',
        gap: Math.max(10, scale.gap - 8),
        alignItems: 'center',
        width: '100%',
      },
    },
    h(
      'div',
      {
          style: {
            display: 'flex',
            width: scale.tagWidth,
            flexShrink: 0,
            marginTop: 0,
            alignSelf: 'center',
          },
        },
      h(
        'div',
        {
          style: {
            display: 'flex',
            width: '100%',
            alignItems: 'center',
            justifyContent: 'center',
            padding: '6px 8px',
            borderRadius: 10,
            border: `1px solid ${tagStyle.border}`,
            background: tagStyle.bg,
            color: tagStyle.fg,
            fontSize: Math.max(9, scale.tiny - 2),
            letterSpacing: '0.04em',
            fontWeight: item.isCategory ? 800 : 600,
            textTransform: 'uppercase',
            textAlign: 'center',
            lineHeight: 1.2,
          },
        },
        item.tag,
      ),
    ),
    h(
      'div',
      {
        style: {
            display: 'flex',
            flex: 1,
            minWidth: 0,
            fontSize: Math.max(14, scale.small - 1),
            color: '#475569',
            lineHeight: 1.55,
            whiteSpace: 'pre-wrap',
            wordBreak: 'break-word',
          },
      },
      item.text,
    ),
  );
}

function analysisSeparator(scale) {
  const segmentCount = 18;
  return h(
    'div',
    {
      style: {
        display: 'flex',
        width: '100%',
        padding: `${Math.max(6, scale.gap - 10)}px 6px ${Math.max(2, scale.gap - 12)}px`,
      },
    },
    h(
      'div',
      {
        style: {
          display: 'flex',
          alignItems: 'center',
          width: '100%',
          gap: 8,
          opacity: 0.9,
        },
      },
      ...Array.from({ length: segmentCount }, (_, index) => h('div', {
        key: `analysis-separator-${index}`,
        style: {
          display: 'flex',
          flex: 1,
          minWidth: 0,
          height: 2,
          borderRadius: 999,
          background: index % 3 === 1 ? 'rgba(148,163,184,0.18)' : 'rgba(148,163,184,0.42)',
        },
      })),
    ),
  );
}

function sourceCard(card, scale) {
  const tradeable = card.tradeable;
  return h(
    'div',
    {
      style: {
        display: 'flex',
        flexDirection: 'column',
        gap: Math.max(10, scale.gap - 8),
        marginTop: Math.max(4, scale.gap - 12),
      },
    },
    h(
      'div',
      {
        style: {
          display: 'flex',
          flexDirection: 'column',
          gap: 0,
          borderRadius: 14,
          border: '1px solid rgba(251,113,133,0.16)',
          background: 'linear-gradient(135deg, rgba(255,241,242,0.92), rgba(255,255,255,0.66))',
          boxShadow: '0 2px 8px rgba(15,23,42,0.04)',
          position: 'relative',
          overflow: 'hidden',
        },
      },
      h('div', {
        style: {
          display: 'flex',
          position: 'absolute',
          left: 0,
          top: 0,
          bottom: 0,
          width: 4,
          background: '#fb7185',
        },
      }),
      h(
        'div',
        {
          style: {
            display: 'flex',
            alignItems: 'center',
            gap: 12,
            padding: '18px 18px 14px 22px',
          },
        },
        h(
          'div',
          {
            style: {
              display: 'flex',
              width: 28,
              height: 28,
              borderRadius: 999,
              alignItems: 'center',
              justifyContent: 'center',
              background: '#ffe4e6',
              color: '#f43f5e',
              flexShrink: 0,
            },
          },
          '!',
        ),
        h(
          'div',
          {
            style: {
              display: 'flex',
              flexDirection: 'column',
              gap: 12,
              flex: 1,
              minWidth: 0,
            },
          },
          h(
            'div',
            {
              style: {
                display: 'flex',
                alignItems: 'center',
                gap: 10,
              },
            },
            h('div', {
              style: {
                display: 'flex',
                fontSize: Math.max(18, scale.small),
                color: '#1f2937',
                fontWeight: 800,
              },
            }, '总判定结果'),
            h('div', {
              style: {
                display: 'flex',
                marginLeft: 'auto',
                padding: '6px 12px',
                borderRadius: 8,
                background: tradeable ? '#10b981' : '#f43f5e',
                color: '#ffffff',
                fontWeight: 900,
                fontSize: Math.max(11, scale.tiny + 1),
                letterSpacing: '0.08em',
              },
            }, card.verdictText),
          ),
          h('div', {
            style: {
              display: 'flex',
              paddingLeft: 0,
              fontSize: Math.max(15, scale.small - 1),
              color: '#334155',
              fontWeight: 700,
            },
          }, `最终否决来源: ${card.sourceLabel}`),
        ),
      ),
    ),
    h(
      'div',
      {
        style: {
          display: 'flex',
          flexDirection: 'column',
          gap: Math.max(8, scale.gap - 12),
          borderRadius: 14,
          border: '1px solid rgba(203,213,225,0.8)',
          background: 'rgba(248,250,252,0.72)',
          boxShadow: '0 2px 8px rgba(15,23,42,0.03)',
          padding: '16px 18px',
        },
      },
      ...card.lines.map((line) => {
        const dotColor = line.kind === 'danger'
          ? '#ef4444'
          : line.kind === 'success'
            ? '#10b981'
            : line.note
              ? '#3b82f6'
              : '#94a3b8';
        const textColor = line.kind === 'danger'
          ? '#dc2626'
          : line.kind === 'success'
            ? '#059669'
            : line.note
              ? '#2563eb'
              : '#475569';
        const textWeight = line.kind === 'danger'
          ? 700
          : line.kind === 'success'
            ? 700
            : line.note
              ? 700
              : 500;
        return h(
        'div',
        {
          style: {
            display: 'flex',
            alignItems: 'flex-start',
            gap: 8,
            width: '100%',
          },
        },
        h('div', {
          style: {
            display: 'flex',
            width: 6,
            height: 6,
            marginTop: 7,
            borderRadius: 999,
            flexShrink: 0,
            background: dotColor,
          },
        }),
        h(
          'div',
          {
            style: {
              display: 'flex',
              flex: 1,
              minWidth: 0,
              fontSize: Math.max(12, scale.tiny),
              color: textColor,
              fontWeight: textWeight,
              lineHeight: 1.45,
              whiteSpace: 'pre-wrap',
              wordBreak: 'break-word',
            },
          },
          line.text,
        ),
      );
      }),
    ),
  );
}

function buildTree(model, meta, canvasHeight) {
  const scale = resolveScale(model);

  return h(
    'div',
    {
      style: {
        width: `${CANVAS_WIDTH}px`,
        height: `${canvasHeight}px`,
        display: 'flex',
        alignItems: 'flex-start',
        justifyContent: 'flex-start',
        padding: '0',
        boxSizing: 'border-box',
        fontFamily: 'Noto Sans SC',
        color: '#1f2937',
      },
    },
    h(
      'div',
      {
        style: {
          display: 'flex',
          width: `${CARD_WIDTH}px`,
          flexDirection: 'column',
          borderRadius: 24,
          background: '#E8E6E1',
          position: 'relative',
          overflow: 'hidden',
        },
      },
      h('div', {
        style: {
          display: 'flex',
          position: 'absolute',
          inset: 0,
          backgroundImage: `url(${PAPER_TEXTURE_DATA_URI})`,
          backgroundSize: '200px 200px',
          opacity: 0.1,
        },
      }),
      h('div', {
        style: {
          display: 'flex',
          position: 'absolute',
          inset: 0,
          background: 'linear-gradient(135deg, rgba(0,0,0,0.03), rgba(255,255,255,0), rgba(0,0,0,0.015))',
          opacity: 0.65,
        },
      }),
      h('div', {
        style: {
          display: 'flex',
          position: 'absolute',
          width: 260,
          height: 1,
          left: -36,
          top: 220,
          background: 'rgba(15,23,42,0.05)',
          transform: 'rotate(15deg)',
        },
      }),
      h('div', {
        style: {
          display: 'flex',
          position: 'absolute',
          width: 320,
          height: 1,
          right: 40,
          top: 540,
          background: 'rgba(255,255,255,0.8)',
          transform: 'rotate(-12deg)',
        },
      }),
      h('div', {
        style: {
          display: 'flex',
          position: 'absolute',
          width: 320,
          height: 1,
          right: 40,
          top: 542,
          background: 'rgba(15,23,42,0.04)',
          transform: 'rotate(-12deg)',
        },
      }),
      h(
        'div',
        {
          style: {
            display: 'flex',
            position: 'relative',
            alignItems: 'center',
            justifyContent: 'space-between',
            padding: '0 38px',
            height: scale.headerHeight,
            borderBottom: '1px solid rgba(203,213,225,0.7)',
          },
        },
        h(
          'div',
          { style: { display: 'flex', alignItems: 'center', gap: 12 } },
          meta.logoDataUri
            ? h('img', {
              src: meta.logoDataUri,
              width: 38,
              height: 38,
              style: { display: 'flex', width: 38, height: 38, objectFit: 'contain' },
            })
            : h('div', {
              style: {
                display: 'flex', width: 38, height: 38, borderRadius: 10, alignItems: 'center', justifyContent: 'center', background: '#cbd5e1', color: '#0f172a', fontWeight: 800,
              },
            }, 'B'),
          h('div', {
            style: {
              display: 'flex', fontSize: Math.max(18, scale.small), letterSpacing: '0.12em', fontWeight: 800, color: '#0f172a',
            },
          }, 'BRALE'),
        ),
        h('div', {
          style: {
            display: 'flex',
            fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
            color: '#64748b',
            fontSize: Math.max(12, scale.tiny),
            fontWeight: 500,
            letterSpacing: '0.08em',
            textTransform: 'uppercase',
          },
        }, meta.runner),
        h(
          'div',
          { style: { display: 'flex', alignItems: 'center', gap: 12 } },
          h('div', {
            style: {
              display: 'flex', fontSize: Math.max(13, scale.small - 2), color: '#334155', fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace', fontWeight: 700,
            },
          }, '@lauk_liu'),
          meta.avatarDataUri
            ? h('img', {
              src: meta.avatarDataUri,
              width: scale.avatar,
              height: scale.avatar,
              style: { display: 'flex', width: scale.avatar, height: scale.avatar, borderRadius: 999, border: '2px solid #ffffff', objectFit: 'cover', background: '#f3f4f6' },
            })
            : h('div', {
              style: {
                display: 'flex', width: scale.avatar, height: scale.avatar, borderRadius: 999, alignItems: 'center', justifyContent: 'center', background: '#e5e7eb', color: '#374151', fontWeight: 700,
              },
            }, 'L'),
        ),
      ),
      h(
        'div',
        {
          style: {
            display: 'flex',
            position: 'relative',
            flexDirection: 'column',
            padding: '24px 34px 34px',
            gap: Math.max(22, scale.gap),
          },
        },
        h(
          'div',
          { style: { display: 'flex', flexDirection: 'column', gap: Math.max(12, scale.gap - 10) } },
          h(
            'div',
            {
              style: {
                display: 'flex',
                alignItems: 'flex-start',
                justifyContent: 'space-between',
                gap: 18,
                width: '100%',
              },
            },
            h(
              'div',
              { style: { display: 'flex', flexDirection: 'column', gap: 10, minWidth: 0, alignItems: 'flex-start', flex: 1 } },
              h('div', {
                style: {
                  display: 'flex', fontSize: Math.max(32, scale.title - 12), lineHeight: 1.05, letterSpacing: '-0.045em', color: '#0f172a', fontWeight: 900, whiteSpace: 'nowrap', wordBreak: 'normal', overflow: 'hidden',
                },
              }, model.title),
              h(
                'div',
                {
                  style: {
                    display: 'flex', alignItems: 'center', gap: 8, color: '#475569', fontSize: Math.max(14, scale.small - 3), fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace', fontWeight: 500,
                  },
                },
                h('div', { style: { display: 'flex', width: 8, height: 8, borderRadius: 999, background: '#94a3b8', flexShrink: 0 } }),
                h('div', { style: { display: 'flex' } }, model.reportTimeCN),
              ),
            ),
            model.titlePrice
              ? h(
                'div',
                {
                  style: {
                    display: 'flex',
                    flexDirection: 'column',
                    flexShrink: 0,
                    alignSelf: 'flex-start',
                    alignItems: 'flex-end',
                    gap: 6,
                    marginTop: 8,
                    minWidth: 140,
                  },
                },
                h(
                  'div',
                  {
                    style: {
                      display: 'flex',
                      color: '#94a3b8',
                      fontSize: Math.max(11, scale.tiny - 1),
                      lineHeight: 1,
                      fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
                      fontWeight: 700,
                      whiteSpace: 'nowrap',
                    },
                  },
                  '当前标记价格',
                ),
                h(
                  'div',
                  {
                    style: {
                      display: 'flex',
                      color: '#64748b',
                      fontSize: Math.max(16, scale.small - 1),
                      lineHeight: 1,
                      fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
                      fontWeight: 700,
                      whiteSpace: 'nowrap',
                    },
                  },
                  model.titlePrice,
                ),
              )
              : null,
          ),
          sourceCard(model.sourceCard, scale),
        ),
        h(
          'div',
          { style: { display: 'flex', flexDirection: 'column', gap: Math.max(18, scale.gap - 6) } },
          h(
            'div',
            { style: { display: 'flex', flexDirection: 'column', gap: Math.max(10, scale.gap - 12) } },
            h(
              'div',
              { style: { display: 'flex', alignItems: 'center', gap: 8, fontSize: Math.max(16, scale.small - 2), color: '#334155', letterSpacing: '0.05em', fontWeight: 800, textTransform: 'uppercase' } },
              h('div', { style: { display: 'flex', width: 10, height: 10, borderRadius: 999, background: '#64748b' } }),
    h('div', { style: { display: 'flex' } }, '局部证据快照（非最终放行）'),
            ),
            h(
              'div',
              { style: { display: 'flex', flexWrap: 'wrap', width: '100%', gap: Math.max(12, scale.gap - 10) } },
              ...model.progressCards.map((card) => h(
                'div',
                { style: { display: 'flex', width: '48.8%', minWidth: 0 } },
                progressCard(card, scale),
              )),
            ),
          ),
          h(
            'div',
            { style: { display: 'flex', flexDirection: 'column', gap: Math.max(10, scale.gap - 12) } },
            h(
              'div',
              { style: { display: 'flex', alignItems: 'center', gap: 8, fontSize: Math.max(16, scale.small - 2), color: '#334155', letterSpacing: '0.05em', fontWeight: 800, textTransform: 'uppercase' } },
              h('div', { style: { display: 'flex', width: 10, height: 10, borderRadius: 999, background: '#64748b' } }),
              h('div', { style: { display: 'flex' } }, '分析报告'),
            ),
            h(
              'div',
              { style: { display: 'flex', flexDirection: 'column', gap: Math.max(8, scale.gap - 12) } },
              ...model.analysisItems.flatMap((item, index) => {
                if (item.isCategory && index > 0) {
                  return [analysisSeparator(scale), analysisItem(item, scale)];
                }
                return [analysisItem(item, scale)];
              }),
            ),
          ),
        ),
      ),
    ),
  );
}

async function loadAuthorAvatar() {
  try {
    await fs.access(AUTHOR_AVATAR_PATH);
    const raw = await fs.readFile(AUTHOR_AVATAR_PATH);
    return `data:image/jpeg;base64,${raw.toString('base64')}`;
  } catch {
    return '';
  }
}

async function loadBraleLogo() {
  try {
    await fs.access(BRALE_LOGO_PATH);
    const raw = await fs.readFile(BRALE_LOGO_PATH);
    return `data:image/png;base64,${raw.toString('base64')}`;
  } catch {
    return '';
  }
}

async function resolveRunner() {
  try {
    const config = await fs.readFile(CONFIG_PATH, 'utf8');
    const match = config.match(/^exec_api_key\s*=\s*"(.+)"$/m);
    if (!match) return 'UNKNOWN';

    const raw = match[1].trim();
    const envRef = raw.match(/^\$\{(.+)\}$/);
    if (!envRef) {
      return raw.toUpperCase();
    }
    return (process.env[envRef[1]] || envRef[1]).toUpperCase();
  } catch {
    return 'UNKNOWN';
  }
}

async function loadFonts() {
  const fontsDir = path.resolve(__dirname, 'fonts');
  const [regular, bold] = await Promise.all([
    fs.readFile(path.join(fontsDir, 'NotoSansCJKsc-Regular.otf')),
    fs.readFile(path.join(fontsDir, 'NotoSansCJKsc-Bold.otf')),
  ]);
  return [
    { name: 'Noto Sans SC', data: regular, weight: 400, style: 'normal' },
    { name: 'Noto Sans SC', data: bold, weight: 700, style: 'normal' },
  ];
}

export async function renderCard({ inputPath, outputPath }) {
  await fs.access(inputPath);
  await fs.access(path.resolve(__dirname, 'fonts/NotoSansCJKsc-Regular.otf'));
  await fs.access(path.resolve(__dirname, 'fonts/NotoSansCJKsc-Bold.otf'));

  const raw = JSON.parse(await fs.readFile(inputPath, 'utf8'));
  const model = buildModel(raw);
  const renderHeight = estimateRenderHeight(model);
  const [runner, avatarDataUri, logoDataUri, fonts] = await Promise.all([
    resolveRunner(),
    loadAuthorAvatar(),
    loadBraleLogo(),
    loadFonts(),
  ]);

  const svg = await satori(buildTree(model, {
    runner,
    avatarDataUri,
    logoDataUri,
  }, renderHeight), {
    width: CANVAS_WIDTH,
    height: renderHeight,
    fonts,
  });

  let cropBox = null;
  const cropProbe = new Resvg(svg);
  const bbox = cropProbe.getBBox();
  if (bbox) {
    const cropHeight = clamp(
      Math.ceil(bbox.y + bbox.height + 2),
      1,
      renderHeight,
    );
    bbox.x = 0;
    bbox.y = 0;
    bbox.width = CANVAS_WIDTH;
    bbox.height = cropHeight;
    cropBox = bbox;
  }

  const resvg = new Resvg(svg, {
    fitTo: { mode: 'width', value: OUTPUT_WIDTH },
  });

  if (cropBox) {
    resvg.cropByBBox(cropBox);
  }

  const rendered = resvg.render();

  await fs.writeFile(outputPath, rendered.asPng());
  return {
    outputPath,
    width: rendered.width,
    height: rendered.height,
    estimatedHeight: renderHeight,
    logicalWidth: CANVAS_WIDTH,
    exportScale: EXPORT_SCALE,
  };
}

async function main() {
  const inputPath = path.resolve(process.env.OG_INPUT ?? process.argv[2] ?? DEFAULT_INPUT);
  const outputPath = path.resolve(process.env.OG_OUTPUT ?? process.argv[3] ?? DEFAULT_OUTPUT);
  const result = await renderCard({ inputPath, outputPath });
  console.log(result.outputPath);
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main().catch((error) => {
    console.error(error);
    process.exitCode = 1;
  });
}

export {
  CANVAS_WIDTH,
  CARD_WIDTH,
  OUTPUT_WIDTH,
  EXPORT_SCALE,
  DEFAULT_RENDER_HEIGHT,
  estimateRenderHeight,
};

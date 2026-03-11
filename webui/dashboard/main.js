const dashboardBase = String(window.__DASHBOARD_BASE__ || "/dashboard").replace(/\/$/, "");
const apiBase = "/api/runtime/dashboard";
const runtimeBase = "/api/runtime";
const refreshMs = 60000;
const fetchTimeoutMs = 12000;

const state = {
  symbol: "",
  symbols: [],
  overviewCards: [],
  intervalsBySymbol: {},
  klineIntervals: [],
  klineInterval: "1h",
  klineLimit: 120,
  lastFlow: null,
  lastHistoryItems: [],
  lastDecisionItems: [],
  selectedDecisionSnapshotID: 0,
  selectedDecisionSnapshotSymbol: "",
  lastPositionHistory: [],
  historyRequestSeq: 0,
  reportCollapsed: true,
  refreshing: false,
  pendingRefreshMode: ""
};

const els = {
  heroTitle: document.getElementById("hero-title"),
  status: document.getElementById("runtime-status"),
  clock: document.getElementById("clock-chip"),
  symbolSelect: document.getElementById("symbol-select"),
  intervalSelect: document.getElementById("interval-select"),
  klineRange: document.getElementById("kline-range"),
  klinePriceTags: document.getElementById("kline-price-tags"),
  livePositionList: document.getElementById("live-position-list"),
  acctTotal: document.getElementById("acct-total"),
  acctFree: document.getElementById("acct-free"),
  acctUsed: document.getElementById("acct-used"),
  acctSymbols: document.getElementById("acct-symbols"),
  acctProfitClosed: document.getElementById("acct-profit-closed"),
  acctProfitAll: document.getElementById("acct-profit-all"),
  flowGraph: document.getElementById("flow-graph"),
  flowMeta: document.getElementById("flow-meta"),
  positionHistoryBody: document.getElementById("position-history-body"),
  decisionHistoryBody: document.getElementById("decision-history-body"),
  decisionDetail: document.getElementById("decision-detail"),
  decisionViewLink: document.getElementById("decision-view-link")
};

let chart = null;
let positionHoldTimer = 0;
let activeHeldPositionCard = null;

function startHeroTitleTyping() {
  const el = els.heroTitle;
  if (!el) {
    return;
  }
  const fullText = String(el.getAttribute("data-text") || el.textContent || "");
  if (!fullText) {
    return;
  }
  el.textContent = "";
  el.classList.add("typing");
  let index = 0;

  function tick() {
    index += 1;
    el.textContent = fullText.slice(0, index);
    if (index >= fullText.length) {
      el.classList.remove("typing");
      el.classList.add("typed");
      return;
    }
    const delay = fullText[index - 1] === " " ? 36 : 54;
    window.setTimeout(tick, delay);
  }

  window.setTimeout(tick, 180);
}

function fmtNumber(value) {
  if (!Number.isFinite(value)) {
    return "--";
  }
  return value.toLocaleString("en-US", { maximumFractionDigits: 4 });
}

function fmtUsd(value) {
  if (!Number.isFinite(value)) {
    return "--";
  }
  const sign = value > 0 ? "+" : "";
  return `${sign}${value.toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 })} USDT`;
}

function fmtPercent(value, digits) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) {
    return "--";
  }
  const fractionDigits = Number.isFinite(Number(digits)) ? Number(digits) : 2;
  return `${(parsed * 100).toFixed(fractionDigits)}%`;
}

function fmtSignedDelta(value) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) {
    return "--";
  }
  const sign = parsed > 0 ? "+" : "";
  return `${sign}${parsed.toFixed(4)}`;
}

function fmtConsensusValue(value) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) {
    return "--";
  }
  return parsed.toFixed(3);
}

function fmtConsensusPassed(value) {
  if (value === true) {
    return "达标";
  }
  if (value === false) {
    return "未达标";
  }
  return "--";
}

function decisionBoolClass(value) {
  if (value === true) {
    return "pass";
  }
  if (value === false) {
    return "fail";
  }
  return "neutral";
}

function decisionActionClass(action) {
  const text = String(action || "").trim().toUpperCase();
  if (!text) {
    return "neutral";
  }
  if (["ALLOW", "OPEN", "ENTRY", "BUY", "LONG"].includes(text)) {
    return "pass";
  }
  if (["VETO", "TIGHTEN", "WAIT", "SKIP", "REJECT", "BLOCK"].includes(text)) {
    return "fail";
  }
  return "neutral";
}

function actionLabel(action) {
  const text = String(action || "").trim().toUpperCase();
  const labels = {
    ALLOW: "放行",
    OPEN: "开仓",
    ENTRY: "入场",
    LONG: "做多",
    SHORT: "做空",
    WAIT: "等待",
    VETO: "否决",
    BLOCK: "阻断",
    TIGHTEN: "收紧",
    HOLD: "持有",
    KEEP: "保持"
  };
  return labels[text] || (text || "--");
}

function flowStatusLabel(status) {
  if (status === "blocked") {
    return "阻拦";
  }
  if (status === "ok") {
    return "通过";
  }
  return "观察中";
}

function translateSieveReason(reason) {
  const code = String(reason || "").trim().toUpperCase();
  const labels = {
    CROWD_ALIGN_LOW_BLOCK: "拥挤方向与当前机会不匹配，风险筛网拒绝放行。",
    CROWD_ALIGN_HIGH_BLOCK: "拥挤度过高且与方向一致，系统选择保守处理。",
    CROWD_ALIGN_LOW_WAIT: "方向尚可，但拥挤确认不足，先等待下一轮确认。",
    LIQ_LOW_WAIT: "清算强度不足，先不激活更高仓位。",
    LIQ_HIGH_ALLOW: "清算与力学标签匹配，允许按筛网规则推进。"
  };
  return labels[code] || (code ? `筛网命中 ${code}` : "当前未命中额外筛网原因");
}

function calcRR(entry, target, stop) {
  const baseEntry = Number(entry);
  const baseTarget = Number(target);
  const baseStop = Number(stop);
  if (!Number.isFinite(baseEntry) || !Number.isFinite(baseTarget) || !Number.isFinite(baseStop)) {
    return null;
  }
  const riskDistance = Math.abs(baseEntry - baseStop);
  if (!Number.isFinite(riskDistance) || riskDistance <= 0) {
    return null;
  }
  return Math.abs(baseTarget - baseEntry) / riskDistance;
}

function decisionMetricProgress(value, threshold, useAbs) {
  const current = Number(value);
  const limit = Number(threshold);
  if (!Number.isFinite(current) || !Number.isFinite(limit) || limit <= 0) {
    return null;
  }
  const base = useAbs ? Math.abs(current) : current;
  const ratio = (base / limit) * 100;
  if (!Number.isFinite(ratio)) {
    return null;
  }
  const normalized = Math.max(0, ratio);
  return {
    ratio: normalized,
    width: Math.max(0, Math.min(100, normalized))
  };
}

function escapeHtml(input) {
  return String(input || "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

function updateClock() {
  const now = new Date();
  els.clock.textContent = `CN ${now.toLocaleTimeString("zh-CN", { hour12: false })}`;
}

async function fetchJSON(path, params) {
  const url = new URL(path, window.location.origin);
  if (params) {
    Object.entries(params).forEach(([key, value]) => {
      if (value !== undefined && value !== null && value !== "") {
        url.searchParams.set(key, value);
      }
    });
  }
  const controller = new AbortController();
  const timeoutID = setTimeout(() => controller.abort(), fetchTimeoutMs);
  let resp;
  try {
    resp = await fetch(url.toString(), {
      headers: { Accept: "application/json" },
      signal: controller.signal
    });
  } catch (err) {
    if (err && typeof err === "object" && "name" in err && err.name === "AbortError") {
      throw new Error("请求超时");
    }
    throw err;
  } finally {
    clearTimeout(timeoutID);
  }
  const contentType = String(resp.headers.get("content-type") || "").toLowerCase();
  let data;
  if (contentType.includes("application/json")) {
    data = await resp.json();
  } else {
    const textBody = await resp.text();
    data = { msg: textBody || `HTTP ${resp.status}` };
  }
  if (!resp.ok) {
    const msg = data && data.msg ? data.msg : `HTTP ${resp.status}`;
    throw new Error(msg);
  }
  return data;
}

function renderGlobalError(err) {
  const msg = err instanceof Error ? err.message : String(err);
  els.flowGraph.innerHTML = "";
  els.flowMeta.innerHTML = `<div>加载失败: ${escapeHtml(msg)}</div>`;
  els.decisionDetail.textContent = `加载失败: ${msg}`;
}

function renderSymbolSelect() {
  const options = state.symbols
    .map((symbol) => `<option value="${escapeHtml(symbol)}" ${symbol === state.symbol ? "selected" : ""}>${escapeHtml(symbol)}</option>`)
    .join("");
  els.symbolSelect.innerHTML = options;
  els.symbolSelect.disabled = state.symbols.length === 0;
}

function fmtShortTime(value) {
  const text = String(value || "").trim();
  if (!text) {
    return "--";
  }
  const ts = Date.parse(text);
  if (!Number.isFinite(ts)) {
    return text;
  }
  return new Date(ts).toLocaleString("zh-CN", {
    hour12: false,
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit"
  });
}

const INITIAL_RISK_TIMELINE_SOURCES = new Set(["entry-fill", "open-fill", "init", "init_from_plan"]);

function riskValuesEqual(left, right) {
  const a = Number(left);
  const b = Number(right);
  if (!Number.isFinite(a) && !Number.isFinite(b)) {
    return true;
  }
  if (!Number.isFinite(a) || !Number.isFinite(b)) {
    return false;
  }
  return Math.abs(a - b) < 1e-8;
}

function isSameRiskPlanVersion(left, right) {
  if (!riskValuesEqual(left && left.stop_loss, right && right.stop_loss)) {
    return false;
  }
  const leftTPs = Array.isArray(left && left.take_profits) ? left.take_profits : [];
  const rightTPs = Array.isArray(right && right.take_profits) ? right.take_profits : [];
  if (leftTPs.length !== rightTPs.length) {
    return false;
  }
  for (let i = 0; i < leftTPs.length; i += 1) {
    if (!riskValuesEqual(leftTPs[i], rightTPs[i])) {
      return false;
    }
  }
  return true;
}

function normalizeRiskTimelineItems(items) {
  const rows = Array.isArray(items) ? items : [];
  if (rows.length === 0) {
    return [];
  }
  const normalized = rows.map((item, index) => {
    const createdAt = String(item && item.created_at ? item.created_at : "").trim();
    const ts = Date.parse(createdAt);
    return {
      ...item,
      __idx: index,
      __ts: Number.isFinite(ts) ? ts : Number.NaN
    };
  });
  normalized.sort((a, b) => {
    const aValid = Number.isFinite(a.__ts);
    const bValid = Number.isFinite(b.__ts);
    if (aValid && bValid && a.__ts !== b.__ts) {
      return a.__ts - b.__ts;
    }
    if (aValid && !bValid) {
      return -1;
    }
    if (!aValid && bValid) {
      return 1;
    }
    return a.__idx - b.__idx;
  });
  const deduped = [];
  normalized.forEach((item) => {
    if (deduped.length === 0 || !isSameRiskPlanVersion(deduped[deduped.length - 1], item)) {
      deduped.push(item);
    }
  });
  return deduped;
}

function isInitialRiskTimelineItem(item) {
  const source = String(item && item.source ? item.source : "").trim().toLowerCase();
  return INITIAL_RISK_TIMELINE_SOURCES.has(source);
}

function renderRiskTakeProfitPills(values) {
  const takeProfits = Array.isArray(values) ? values : [];
  if (takeProfits.length === 0) {
    return `<span class="risk-version-empty">--</span>`;
  }
  return takeProfits
    .map((price, index) => `<span class="risk-tp-pill">TP${index + 1} ${fmtNumber(Number(price))}</span>`)
    .join("");
}

function renderRiskPlanTimeline(items) {
  const versions = normalizeRiskTimelineItems(items);
  if (versions.length === 0) {
    return "";
  }
  if (versions.length === 1 && isInitialRiskTimelineItem(versions[0])) {
    return "";
  }
  return `<div class="position-hold-panel" data-hold-panel>
    <div class="position-hold-head">
      <strong>止盈止损变化</strong>
      <span>${versions.length} 个版本，从旧到新</span>
    </div>
    <div class="position-hold-timeline">
      ${versions.map((item, index) => {
        const isCurrent = index === versions.length - 1;
        const card = `<article class="risk-version-card ${isCurrent ? "current" : ""}">
          <div class="risk-version-top">
            <span class="risk-version-label">${escapeHtml(isCurrent ? "当前版本" : `版本 ${index + 1}`)}</span>
            <em>${escapeHtml(fmtShortTime(item.created_at))}</em>
          </div>
          <div class="risk-version-values">
            <div class="risk-version-metric stop">
              <span>止损 SL</span>
              <strong>${fmtNumber(Number(item.stop_loss))}</strong>
            </div>
            <div class="risk-version-metric tp">
              <span>止盈 TP</span>
              <div class="risk-version-tp-list">${renderRiskTakeProfitPills(item.take_profits)}</div>
            </div>
          </div>
          <div class="risk-version-foot">${escapeHtml(String(item.label || item.source || "risk_update"))}</div>
        </article>`;
        if (index === versions.length - 1) {
          return card;
        }
        return `${card}<span class="risk-version-link" aria-hidden="true"></span>`;
      }).join("")}
    </div>
  </div>`;
}

function clearPositionHold(card) {
  if (positionHoldTimer) {
    window.clearTimeout(positionHoldTimer);
    positionHoldTimer = 0;
  }
  const target = card || activeHeldPositionCard;
  if (target) {
    target.classList.remove("pressing", "holding");
  }
  if (!card || card === activeHeldPositionCard) {
    activeHeldPositionCard = null;
  }
}

function bindLivePositionHold() {
  const cards = els.livePositionList.querySelectorAll("[data-position-card][data-holdable='true']");
  cards.forEach((card) => {
    const startHold = () => {
      clearPositionHold(activeHeldPositionCard);
      card.classList.add("pressing");
      positionHoldTimer = window.setTimeout(() => {
        card.classList.remove("pressing");
        card.classList.add("holding");
        activeHeldPositionCard = card;
        positionHoldTimer = 0;
      }, 320);
    };
    const endHold = () => clearPositionHold(card);
    card.addEventListener("pointerdown", startHold);
    card.addEventListener("pointerup", endHold);
    card.addEventListener("pointerleave", endHold);
    card.addEventListener("pointercancel", endHold);
  });
}

function renderLivePositions(cards) {
  if (!Array.isArray(cards) || cards.length === 0) {
    els.livePositionList.innerHTML = `<div class="position-empty">当前无实时持仓</div>`;
    clearPositionHold();
    return;
  }

  els.livePositionList.innerHTML = cards
    .map((card) => {
      const position = card.position || {};
      const timeline = Array.isArray(position.risk_plan_timeline) ? position.risk_plan_timeline : [];
      const timelineMarkup = renderRiskPlanTimeline(timeline);
      const holdable = timelineMarkup !== "";
      const pnl = card.pnl || {};
      const realizedClass = Number(pnl.realized) >= 0 ? "positive" : "negative";
      const unrealizedClass = Number(pnl.unrealized) >= 0 ? "positive" : "negative";
      return `<article class="position-card ${holdable ? "holdable" : ""}" data-position-card data-holdable="${holdable ? "true" : "false"}">
        <div class="position-head">
          <div>
            <div class="position-symbol">${escapeHtml(card.symbol)}</div>
            ${holdable ? `<div class="position-hold-hint">按住查看止盈止损版本</div>` : ""}
          </div>
          <span class="side-chip">${escapeHtml(position.side || "--")}</span>
        </div>
        <div class="metrics-grid">
          <div class="metric"><span class="k">仓位规模</span><span class="v">${fmtNumber(position.amount)}</span></div>
          <div class="metric"><span class="k">入场价</span><span class="v">${fmtNumber(position.entry_price)}</span></div>
          <div class="metric"><span class="k">当前价</span><span class="v">${fmtNumber(position.current_price)}</span></div>
          <div class="metric"><span class="k">止盈 TP</span><span class="v positive">${(position.take_profits || []).map((v) => fmtNumber(Number(v))).join(" / ") || "--"}</span></div>
          <div class="metric"><span class="k">止损 SL</span><span class="v negative">${fmtNumber(position.stop_loss)}</span></div>
          <div class="metric"><span class="k">已实现盈亏</span><span class="v ${realizedClass}">${fmtUsd(Number(pnl.realized))}</span></div>
          <div class="metric"><span class="k">未实现盈亏</span><span class="v ${unrealizedClass}">${fmtUsd(Number(pnl.unrealized))}</span></div>
          <div class="metric"><span class="k">合计盈亏</span><span class="v ${Number(pnl.total) >= 0 ? "positive" : "negative"}">${fmtUsd(Number(pnl.total || 0))}</span></div>
        </div>
        ${timelineMarkup}
      </article>`;
    })
    .join("");
  bindLivePositionHold();
}

function renderIntervalSelect() {
  const intervals = Array.isArray(state.klineIntervals) ? state.klineIntervals : [];
  const options = intervals
    .map((intervalValue) => {
      const selected = intervalValue === state.klineInterval ? "selected" : "";
      return `<option value="${escapeHtml(intervalValue)}" ${selected}>${escapeHtml(intervalValue.toUpperCase())}</option>`;
    })
    .join("");
  els.intervalSelect.innerHTML = options;
  els.intervalSelect.disabled = intervals.length === 0;
}

function renderAccountSummary(payload) {
  const balance = payload && payload.balance ? payload.balance : {};
  const profit = payload && payload.profit ? payload.profit : {};
  const monitored = Array.isArray(balance.monitored_symbols) ? balance.monitored_symbols : [];
  const currency = String(balance.currency || "USDT");
  const total = Number(balance.total || 0);
  const available = Number(balance.available || 0);
  const used = Number(balance.used || 0);

  els.acctTotal.textContent = `${total.toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 })} ${currency}`;
  els.acctFree.textContent = `${available.toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 })} ${currency}`;
  els.acctUsed.textContent = `${used.toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 })} ${currency}`;
  els.acctSymbols.textContent = monitored.length > 0 ? `${monitored.length} (${monitored.join(" / ")})` : "--";
  els.acctProfitClosed.textContent = fmtUsd(Number(profit.closed_profit || 0));
  els.acctProfitAll.textContent = fmtUsd(Number(profit.all_profit || 0));
}

function renderFlow(flow) {
  const nodes = flow && Array.isArray(flow.nodes) ? flow.nodes : [];
  const trace = flow && flow.trace ? flow.trace : {};
  if (nodes.length === 0) {
    els.flowGraph.innerHTML = "";
    els.flowMeta.innerHTML = "";
    return;
  }

  const gate = trace && trace.gate ? trace.gate : null;
  const inPosition = trace && trace.in_position ? trace.in_position : null;
  const tighten = flow && flow.tighten ? flow.tighten : null;
  const action = String(gate && gate.action ? gate.action : "").toUpperCase();
  const resultNode = nodes.find((item) => String(item && item.stage || "").toLowerCase() === "result") || null;

  function node(id, posClass, status, title, desc, detail, reason, stageType) {
    return { id, posClass, status, title, desc, detail, reason, stageType };
  }

  function summarizeStageValues(values) {
    if (!Array.isArray(values) || values.length === 0) {
      return "--";
    }
    const pass = values.filter((field) => String(field && field.state || "") === "pass").length;
    const block = values.filter((field) => String(field && field.state || "") === "block").length;
    const enumCount = values.length - pass - block;
    return `pass:${pass} block:${block} enum:${enumCount}`;
  }

  function findStage(items, stageName) {
    const list = Array.isArray(items) ? items : [];
    return list.find((item) => String(item && item.stage || "").toLowerCase() === stageName) || null;
  }

  const agentIndicator = findStage(trace.agents, "indicator");
  const agentStructure = findStage(trace.agents, "structure");
  const agentMechanics = findStage(trace.agents, "mechanics");
  const providerIndicator = findStage(trace.providers, "indicator");
  const providerStructure = findStage(trace.providers, "structure");
  const providerMechanics = findStage(trace.providers, "mechanics");
  const providerMode = String((providerIndicator && providerIndicator.mode) || (providerStructure && providerStructure.mode) || (providerMechanics && providerMechanics.mode) || "standard").toLowerCase();
  const providerTitlePrefix = providerMode === "in_position" ? "InPositionProvider" : "Provider";

  function stageStatus(stage, fallbackStatus) {
    const normalized = String(stage && stage.status || "").trim().toLowerCase();
    if (normalized === "ok" || normalized === "blocked") {
      return normalized;
    }
    return String(fallbackStatus || "ok");
  }

  function stageReason(stage, fallbackReason) {
    const reason = String(stage && stage.reason || "").trim();
    if (reason) {
      return reason;
    }
    return String(fallbackReason || "");
  }

  function stageSummary(stage, fallbackSummary) {
    const summary = String(stage && stage.summary || "").trim();
    if (summary) {
      return summary;
    }
    return String(fallbackSummary || "--");
  }

  const graphNodes = [
    node("agent-indicator", "flow-pos-agent-indicator", stageStatus(agentIndicator, "ok"), "Agent/indicator", stageSummary(agentIndicator, summarizeStageValues(agentIndicator && agentIndicator.values)), agentIndicator && agentIndicator.values, stageReason(agentIndicator, ""), "agent"),
    node("agent-structure", "flow-pos-agent-structure", stageStatus(agentStructure, "ok"), "Agent/structure", stageSummary(agentStructure, summarizeStageValues(agentStructure && agentStructure.values)), agentStructure && agentStructure.values, stageReason(agentStructure, ""), "agent"),
    node("agent-mechanics", "flow-pos-agent-mechanics", stageStatus(agentMechanics, "ok"), "Agent/mechanics", stageSummary(agentMechanics, summarizeStageValues(agentMechanics && agentMechanics.values)), agentMechanics && agentMechanics.values, stageReason(agentMechanics, ""), "agent"),
    node("provider-indicator", "flow-pos-provider-indicator", stageStatus(providerIndicator, "ok"), `${providerTitlePrefix}/indicator`, stageSummary(providerIndicator, summarizeStageValues(providerIndicator && providerIndicator.values)), providerIndicator && providerIndicator.values, stageReason(providerIndicator, ""), "provider"),
    node("provider-structure", "flow-pos-provider-structure", stageStatus(providerStructure, "ok"), `${providerTitlePrefix}/structure`, stageSummary(providerStructure, summarizeStageValues(providerStructure && providerStructure.values)), providerStructure && providerStructure.values, stageReason(providerStructure, ""), "provider"),
    node("provider-mechanics", "flow-pos-provider-mechanics", stageStatus(providerMechanics, "ok"), `${providerTitlePrefix}/mechanics`, stageSummary(providerMechanics, summarizeStageValues(providerMechanics && providerMechanics.values)), providerMechanics && providerMechanics.values, stageReason(providerMechanics, ""), "provider")
  ];

  if (inPosition && inPosition.active) {
    graphNodes.push(node("inposition", "flow-pos-inposition", stageStatus(inPosition, "ok"), "InPosition", String(inPosition.side || "open"), [{ key: "active", value: "true", state: "pass" }, { key: "side", value: String(inPosition.side || "-") }], stageReason(inPosition, "已有持仓，链路进入监控/管理路径"), "monitor"));
  }

  const gateStatus = stageStatus(gate, "ok");
  const gateReason = stageReason(gate, "");
  graphNodes.push(node("gate", "flow-pos-gate", gateStatus, "Gate", `${actionLabel(gate && gate.action ? gate.action : "--")}`, gate && Array.isArray(gate.rules) ? gate.rules : [], gateReason, "gate"));

  const resultStatus = String(resultNode && resultNode.status || "").trim().toLowerCase() || "ok";
  const resultReason = String(resultNode && resultNode.reason || "").trim();
  const resultDesc = resultNode ? String(resultNode.outcome || "-") : "-";
  const resultValues = resultNode && Array.isArray(resultNode.values) ? resultNode.values : [];
  graphNodes.push(node("result", "flow-pos-result", resultStatus, "Result", resultDesc, resultValues, resultReason, "result"));

  function renderFieldList(fields) {
    if (!Array.isArray(fields) || fields.length === 0) {
      return `<span class="flow-chip">--</span>`;
    }
    return fields.map((field) => {
      const stateValue = String(field && field.state || "");
      const stateClass = stateValue === "block" ? "block" : (stateValue === "pass" ? "pass" : "");
      return `<span class="flow-chip ${stateClass}">${escapeHtml(String(field && field.key || ""))}:${escapeHtml(String(field && field.value || ""))}</span>`;
    }).join("");
  }

  const nodesHTML = graphNodes.map((item, index) => {
    const reasonAttr = item.reason ? ` title="${escapeHtml(item.reason)}" data-reason="${escapeHtml(item.reason)}"` : "";
    return `<article id="flow-node-${escapeHtml(item.id)}" class="flow-node ${escapeHtml(item.status)} ${escapeHtml(item.posClass)}" style="--flow-index:${index}"${reasonAttr}>
      <div class="flow-node-topline">
        <span class="flow-node-kind">${escapeHtml(item.stageType || "stage")}</span>
        <span class="flow-node-state ${escapeHtml(item.status)}">${escapeHtml(flowStatusLabel(item.status))}</span>
      </div>
      <div class="flow-node-title">${escapeHtml(item.title)}</div>
      <div class="flow-node-desc">${escapeHtml(item.desc)}</div>
      ${Array.isArray(item.detail) && item.detail.length > 0 ? `<div class="flow-node-values">${renderFieldList(item.detail)}</div>` : ""}
    </article>`;
  }).join("");

  els.flowGraph.innerHTML = `<div class="flow-backdrop"></div><svg class="flow-links" id="flow-links" aria-hidden="true"></svg><div class="flow-dag">${nodesHTML}</div>`;

  function drawFlowLinks() {
    const svg = document.getElementById("flow-links");
    if (!svg) {
      return;
    }
    const graphRect = els.flowGraph.getBoundingClientRect();

    function pointOf(id, side) {
      const nodeEl = document.getElementById(`flow-node-${id}`);
      if (!nodeEl) {
        return null;
      }
      const rect = nodeEl.getBoundingClientRect();
      const x = side === "left" ? rect.left - graphRect.left : rect.right - graphRect.left;
      const y = rect.top - graphRect.top + (rect.height / 2);
      return { x, y };
    }

    const defs = `<defs><marker id="flow-arrow-head" markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto"><path d="M0,0 L8,4 L0,8 z" fill="#6db9ff"></path></marker></defs>`;
    const links = [];

    function pushLink(fromID, toID, status) {
      const start = pointOf(fromID, "right");
      const end = pointOf(toID, "left");
      if (!start || !end) {
        return;
      }
      const bend = Math.max(26, Math.abs(end.x - start.x) * 0.36);
      const path = `M ${start.x} ${start.y} C ${start.x + bend} ${start.y}, ${end.x - bend} ${end.y}, ${end.x} ${end.y}`;
      links.push(`<path class="flow-link-base ${status}" d="${path}" marker-end="url(#flow-arrow-head)" />`);
      links.push(`<path class="flow-link-pulse ${status}" d="${path}" marker-end="url(#flow-arrow-head)" />`);
    }

    const rows = ["indicator", "structure", "mechanics"];
    rows.forEach((row) => {
      pushLink(`agent-${row}`, `provider-${row}`, stageStatus(findStage(trace.providers, row), "ok"));
      pushLink(`provider-${row}`, "gate", gateStatus === "blocked" ? "blocked" : "ok");
    });
    if (inPosition && inPosition.active) {
      pushLink("inposition", "gate", gateStatus === "blocked" ? "blocked" : "ok");
    }
    pushLink("gate", "result", resultStatus === "blocked" ? "blocked" : "ok");

    svg.setAttribute("viewBox", `0 0 ${Math.max(1, Math.round(graphRect.width))} ${Math.max(1, Math.round(graphRect.height))}`);
    svg.innerHTML = `${defs}${links.join("")}`;
  }

  function scheduleFlowLinksDraw() {
    window.requestAnimationFrame(() => {
      drawFlowLinks();
      const svg = document.getElementById("flow-links");
      if (!svg) {
        return;
      }
      window.requestAnimationFrame(() => {
        svg.classList.add("ready");
      });
    });
  }

  scheduleFlowLinksDraw();
  window.setTimeout(scheduleFlowLinksDraw, 240);

  els.flowMeta.innerHTML = "";
}

function reportToggleLabel(collapsed) {
  return collapsed ? "展开报告" : "收起报告";
}

function syncDecisionReportCollapse(root) {
  const container = root || els.decisionDetail;
  if (!container) {
    return;
  }
  const shell = container.querySelector("[data-report-shell]");
  if (!shell) {
    return;
  }
  const collapsed = Boolean(state.reportCollapsed);
  shell.classList.toggle("collapsed", collapsed);
  shell.setAttribute("data-collapsed", collapsed ? "true" : "false");
  shell.querySelectorAll("[data-report-toggle]").forEach((button) => {
    button.textContent = reportToggleLabel(collapsed);
    button.setAttribute("aria-expanded", collapsed ? "false" : "true");
  });
}

function bindDecisionReportToggle() {
  const shell = els.decisionDetail.querySelector("[data-report-shell]");
  if (!shell) {
    return;
  }
  shell.querySelectorAll("[data-report-toggle]").forEach((button) => {
    button.addEventListener("click", () => {
      state.reportCollapsed = !state.reportCollapsed;
      syncDecisionReportCollapse();
    });
  });
  syncDecisionReportCollapse();
}

function pickMiddleUpperInterval(intervals) {
  if (!Array.isArray(intervals) || intervals.length === 0) {
    return "";
  }
  const values = intervals.map((item) => String(item || "").trim().toLowerCase()).filter(Boolean);
  if (values.length === 0) {
    return "";
  }
  const index = Math.floor(values.length / 2);
  return values[index] || values[0] || "";
}

function ensureChart() {
  if (chart) {
    return chart;
  }
  const dom = document.getElementById("kline-chart");
  chart = window.echarts.init(dom);
  return chart;
}

function renderKline(payload, flow) {
  const candles = Array.isArray(payload && payload.candles) ? payload.candles : [];
  els.klineRange.textContent = `${String(payload && payload.interval ? payload.interval : state.klineInterval).toUpperCase()} Window`;
  const c = ensureChart();

  if (candles.length === 0) {
    els.klinePriceTags.innerHTML = "";
    c.setOption({ title: { text: "No Kline Data", left: "center", top: "middle", textStyle: { color: "#9caec6", fontSize: 14 } }, xAxis: { show: false }, yAxis: { show: false }, series: [] });
    return;
  }

  const labels = candles.map((item) => Number(item.open_time || 0));
  const values = candles.map((item) => [Number(item.open || 0), Number(item.close || 0), Number(item.low || 0), Number(item.high || 0)]);
  const selectedCard = Array.isArray(state.overviewCards)
    ? state.overviewCards.find((item) => String(item && item.symbol || "").toUpperCase() === String(state.symbol || "").toUpperCase())
    : null;
  const selectedPosition = selectedCard && selectedCard.position ? selectedCard.position : null;

  function nearestPointByTime(ts) {
    if (!Number.isFinite(ts) || labels.length === 0) {
      return null;
    }
    let idx = 0;
    let best = Math.abs(labels[0] - ts);
    for (let i = 1; i < labels.length; i += 1) {
      const delta = Math.abs(labels[i] - ts);
      if (delta < best) {
        best = delta;
        idx = i;
      }
    }
    return { x: labels[idx], y: values[idx][1] };
  }

  const markerName = flow && flow.anchor ? `${flow.anchor.type || "anchor"}#${flow.anchor.snapshot_id || 0}` : "Anchor";
  const markerData = [
    {
      name: markerName,
      coord: [labels[labels.length - 1], values[values.length - 1][1]],
      value: `锚点: ${markerName}`,
      itemStyle: { color: "#f6bd43" }
    }
  ];

  state.lastPositionHistory.forEach((trade) => {
    const openedAt = trade && trade.opened_at ? Date.parse(trade.opened_at) : 0;
    const closedAt = trade && trade.closed_at ? Date.parse(trade.closed_at) : 0;
    const openPoint = nearestPointByTime(openedAt);
    if (openPoint) {
      markerData.push({
        name: "开仓",
        coord: [openPoint.x, openPoint.y],
        value: `开仓 ${trade.symbol || ""} ${trade.side || ""} 数量:${fmtNumber(Number(trade.amount || 0))}`,
        itemStyle: { color: "#3ad6a3" }
      });
    }
    const closePoint = nearestPointByTime(closedAt);
    if (closePoint) {
      markerData.push({
        name: "平仓",
        coord: [closePoint.x, closePoint.y],
        value: `平仓 ${trade.symbol || ""} 收益:${fmtUsd(Number(trade.profit || 0))}`,
        itemStyle: { color: "#ff6a78" }
      });
    }
  });

  state.lastDecisionItems.forEach((item) => {
    const action = String(item && item.action ? item.action : "").toUpperCase();
    if (!action.includes("TIGHTEN")) {
      return;
    }
    const ts = item && item.at ? Date.parse(item.at) : 0;
    const point = nearestPointByTime(ts);
    if (!point) {
      return;
    }
    markerData.push({
      name: "收紧",
      coord: [point.x, point.y],
      value: `收紧止损: ${item.reason || "-"}`,
      itemStyle: { color: "#f6bd43" }
    });
  });

  const currentPrice = selectedPosition && Number.isFinite(Number(selectedPosition.current_price))
    ? Number(selectedPosition.current_price)
    : Number(values[values.length - 1][1]);
  const stopLoss = selectedPosition && Number.isFinite(Number(selectedPosition.stop_loss))
    ? Number(selectedPosition.stop_loss)
    : NaN;
  const takeProfits = selectedPosition && Array.isArray(selectedPosition.take_profits)
    ? selectedPosition.take_profits.map((value) => Number(value)).filter((value) => Number.isFinite(value))
    : [];

  function priceLine(name, value, color, lineType) {
    if (!Number.isFinite(value)) {
      return null;
    }
    return {
      name,
      yAxis: value,
      lineStyle: {
        color,
        type: lineType || "dashed",
        width: 1.5,
        opacity: 0.95
      }
    };
  }

  const lineData = [];
  const currentLine = priceLine("当前", currentPrice, "#5bc0ff", "solid");
  if (currentLine) {
    lineData.push(currentLine);
  }
  const stopLine = priceLine("止损", stopLoss, "#ff6a78", "dashed");
  if (stopLine) {
    lineData.push(stopLine);
  }
  takeProfits.forEach((value, index) => {
    const tpLine = priceLine(`止盈${index + 1}`, value, "#3ad6a3", "dashed");
    if (tpLine) {
      lineData.push(tpLine);
    }
  });

  function measurePriceTagColumn(lines, compactViewport) {
    const validLines = Array.isArray(lines)
      ? lines.filter((line) => Number.isFinite(Number(line && line.yAxis)))
      : [];
    const dense = validLines.length >= 4;
    const fontSize = dense ? 10 : 11;
    const canvas = measurePriceTagColumn.canvas || document.createElement("canvas");
    if (!measurePriceTagColumn.canvas) {
      measurePriceTagColumn.canvas = canvas;
    }
    const ctx = canvas.getContext("2d");
    let maxTextWidth = 0;
    if (ctx) {
      ctx.font = `${fontSize}px "IBM Plex Sans SC", "PingFang SC", "Microsoft YaHei", sans-serif`;
      validLines.forEach((line) => {
        const text = `${String(line.name || "价格")} ${Number(line.yAxis).toFixed(2)}`;
        const measured = Number(ctx.measureText(text).width || 0);
        if (measured > maxTextWidth) {
          maxTextWidth = measured;
        }
      });
    }
    if (maxTextWidth <= 0) {
      validLines.forEach((line) => {
        const text = `${String(line.name || "价格")} ${Number(line.yAxis).toFixed(2)}`;
        const fallback = text.length * (dense ? 5.2 : 5.7);
        if (fallback > maxTextWidth) {
          maxTextWidth = fallback;
        }
      });
    }
    const minWidth = compactViewport ? 86 : 96;
    const maxWidth = compactViewport ? 122 : 144;
    const horizontalPadding = dense ? 16 : 20;
    const tagColumnWidth = Math.max(minWidth, Math.min(maxWidth, Math.ceil(maxTextWidth + horizontalPadding)));
    const tagColumnRight = compactViewport ? 8 : 10;
    const klineGridRight = tagColumnWidth + tagColumnRight + 8;
    return { tagColumnWidth, tagColumnRight, klineGridRight, dense };
  }

  const compactViewport = window.matchMedia && window.matchMedia("(max-width: 1080px)").matches;
  const tagMetrics = measurePriceTagColumn(lineData, compactViewport);
  els.klinePriceTags.style.width = `${tagMetrics.tagColumnWidth}px`;
  els.klinePriceTags.style.right = `${tagMetrics.tagColumnRight}px`;

  const signalData = markerData.map((item) => ({
    name: item.name,
    value: [item.coord[0], item.coord[1]],
    detail: String(item.value || ""),
    itemStyle: item.itemStyle || {}
  }));

  function renderKlinePriceTags(lines) {
    const dom = c.getDom();
    const chartHeight = Number(dom && dom.clientHeight ? dom.clientHeight : 360);
    const top = 58;
    const bottom = 34;
    const minY = top;
    const maxY = Math.max(top + 10, chartHeight - bottom);
    const tagItems = [];
    (lines || []).forEach((line) => {
      const y = Number(line && line.yAxis);
      if (!Number.isFinite(y)) {
        return;
      }
      let pixel = Number(c.convertToPixel({ yAxisIndex: 0 }, y));
      if (!Number.isFinite(pixel)) {
        return;
      }
      if (pixel < minY) {
        pixel = minY;
      }
      if (pixel > maxY) {
        pixel = maxY;
      }
      const color = line && line.lineStyle ? line.lineStyle.color : "#5bc0ff";
      tagItems.push({
        name: String(line.name || "价格"),
        y,
        pixel,
        color: String(color)
      });
    });

    tagItems.sort((a, b) => a.pixel - b.pixel);
    const availableHeight = Math.max(1, maxY - minY);
    const desiredGap = tagItems.length >= 4 ? 24 : 28;
    let minGap = desiredGap;
    if (tagItems.length > 1) {
      const maxGapWithoutOverflow = Math.floor(availableHeight / (tagItems.length - 1));
      minGap = Math.max(18, Math.min(desiredGap, maxGapWithoutOverflow));
    }
    for (let i = 1; i < tagItems.length; i += 1) {
      if (tagItems[i].pixel - tagItems[i - 1].pixel < minGap) {
        tagItems[i].pixel = tagItems[i - 1].pixel + minGap;
      }
    }
    if (tagItems.length > 0 && tagItems[tagItems.length - 1].pixel > maxY) {
      tagItems[tagItems.length - 1].pixel = maxY;
      for (let i = tagItems.length - 2; i >= 0; i -= 1) {
        const allowed = tagItems[i + 1].pixel - minGap;
        if (tagItems[i].pixel > allowed) {
          tagItems[i].pixel = allowed;
        }
      }
    }
    if (tagItems.length > 0 && tagItems[0].pixel < minY) {
      tagItems[0].pixel = minY;
      for (let i = 1; i < tagItems.length; i += 1) {
        const allowed = tagItems[i - 1].pixel + minGap;
        if (tagItems[i].pixel < allowed) {
          tagItems[i].pixel = allowed;
        }
      }
    }

    const denseClass = tagMetrics.dense || tagItems.length >= 4 ? " dense" : "";

    const tags = tagItems.map((item) => `<div class="price-tag${denseClass}" style="top:${item.pixel}px;background:${escapeHtml(item.color)}">${escapeHtml(item.name)} ${item.y.toFixed(2)}</div>`);
    els.klinePriceTags.innerHTML = tags.join("");
  }

  function formatBeijingTime(value) {
    const ts = Number(value);
    if (!Number.isFinite(ts) || ts <= 0) {
      return "--";
    }
    return new Date(ts).toLocaleTimeString("zh-CN", {
      timeZone: "Asia/Shanghai",
      hour12: false,
      hour: "2-digit",
      minute: "2-digit"
    });
  }

  c.setOption({
    backgroundColor: "transparent",
    grid: { left: 45, right: tagMetrics.klineGridRight, top: 28, bottom: 30 },
    tooltip: {
      trigger: "item",
      axisPointer: { type: "cross" },
      formatter(params) {
        if (params && params.seriesType === "scatter" && params.data) {
          return `${escapeHtml(String(params.name || "信号"))}<br/>${escapeHtml(String(params.data.detail || ""))}`;
        }
        if (params && params.seriesType === "candlestick" && Array.isArray(params.data)) {
          const v = params.data;
          return `O ${fmtNumber(Number(v[0]))}<br/>C ${fmtNumber(Number(v[1]))}<br/>L ${fmtNumber(Number(v[2]))}<br/>H ${fmtNumber(Number(v[3]))}`;
        }
        return "";
      }
    },
    xAxis: {
      type: "category",
      data: labels,
      boundaryGap: true,
      axisLine: { lineStyle: { color: "#8ea2bb" } },
      axisPointer: {
        label: {
          formatter(params) {
            return formatBeijingTime(params && params.value);
          }
        }
      },
      axisLabel: {
        color: "#9caec6",
        formatter(value) {
          return formatBeijingTime(value);
        }
      }
    },
    yAxis: {
      scale: true,
      axisLine: { lineStyle: { color: "#8ea2bb" } },
      splitLine: { lineStyle: { color: "rgba(255,255,255,0.08)" } },
      axisLabel: { color: "#9caec6" }
    },
    series: [
      {
        name: "K",
        type: "candlestick",
        data: values,
        itemStyle: {
          color: "#3ad6a3",
          color0: "#ff6a78",
          borderColor: "#3ad6a3",
          borderColor0: "#ff6a78"
        },
        markLine: {
          symbol: ["none", "none"],
          silent: true,
          animation: false,
          precision: 2,
          label: { show: false },
          data: lineData
        }
      },
      {
        name: "signals",
        type: "scatter",
        data: signalData,
        symbolSize: 14,
        z: 6,
        label: {
          show: true,
          formatter(params) {
            return String(params.name || "").slice(0, 2);
          },
          color: "#08101a",
          fontWeight: 700,
          fontSize: 10
        }
      }
    ]
  });

  renderKlinePriceTags(lineData);
}

function renderPositionHistory(trades) {
  if (!Array.isArray(trades) || trades.length === 0) {
    els.positionHistoryBody.innerHTML = `<tr><td colspan="5">暂无历史仓位</td></tr>`;
    return;
  }
  els.positionHistoryBody.innerHTML = trades
    .map((trade) => {
      const profit = Number(trade.profit || 0);
      const pnlClass = profit >= 0 ? "positive" : "negative";
      const opened = trade.opened_at ? new Date(trade.opened_at).toLocaleString("zh-CN", { hour12: false }) : "--";
      return `<tr>
        <td>${escapeHtml(trade.symbol || "--")}</td>
        <td>${escapeHtml(opened)}</td>
        <td>${escapeHtml(trade.side || "--")}</td>
        <td>${fmtNumber(Number(trade.amount || 0))}</td>
        <td class="${pnlClass}">${fmtUsd(profit)}</td>
      </tr>`;
    })
    .join("");
}

function renderDecisionHistoryRows(items, selectedSnapshotID) {
  if (!Array.isArray(items) || items.length === 0) {
    els.decisionHistoryBody.innerHTML = `<tr><td colspan="6">暂无历史决策</td></tr>`;
    return;
  }
  els.decisionHistoryBody.innerHTML = items
    .map((item) => {
      const active = Number(item.snapshot_id) === Number(selectedSnapshotID) ? "active" : "";
      return `<tr class="${active}" data-snapshot-id="${Number(item.snapshot_id || 0)}">
        <td>${escapeHtml(item.at || "--")}</td>
        <td>${escapeHtml(item.action || "--")}</td>
        <td>${escapeHtml(item.reason || "--")}</td>
        <td>${escapeHtml(fmtConsensusValue(item.consensus_score))}</td>
        <td>${escapeHtml(fmtConsensusValue(item.consensus_confidence))}</td>
        <td>${Number(item.snapshot_id || 0)}</td>
      </tr>`;
    })
    .join("");

  els.decisionHistoryBody.querySelectorAll("tr[data-snapshot-id]").forEach((row) => {
    row.addEventListener("click", async () => {
      const snapshotID = row.getAttribute("data-snapshot-id");
      await refreshDecisionHistory(snapshotID);
    });
  });
}

function renderDecisionPlan(plan) {
  if (!plan) {
    return "";
  }
  const takeProfits = Array.isArray(plan.take_profits) ? plan.take_profits : [];
  const levels = Array.isArray(plan.take_profit_levels) ? plan.take_profit_levels : [];
  const stopDistance = Number.isFinite(Number(plan.entry_price)) && Number.isFinite(Number(plan.stop_loss))
    ? Math.abs(Number(plan.entry_price) - Number(plan.stop_loss))
    : NaN;
  const ratioItems = (levels.length > 0 ? levels : takeProfits.map((price, index) => ({ level_id: `tp${index + 1}`, price })))
    .map((level, index) => {
      const rr = calcRR(plan.entry_price, level.price, plan.stop_loss);
      return `<div class="plan-level ${level.hit ? "hit" : ""}">
        <div>
          <span>${escapeHtml(String(level.level_id || `tp${index + 1}`).toUpperCase())}</span>
          <strong>${fmtNumber(Number(level.price))}</strong>
        </div>
        <div class="plan-level-meta">
          <em>${Number.isFinite(Number(level.qty_pct)) && Number(level.qty_pct) > 0 ? `减仓 ${fmtPercent(Number(level.qty_pct), 0)}` : "目标位"}</em>
          <em>${rr ? `RR ${rr.toFixed(2)}` : "RR --"}</em>
        </div>
      </div>`;
    }).join("");

  return `<section class="decision-section">
    <div class="decision-section-head">
      <h3>当前执行几何（非快照）</h3>
      <span>${escapeHtml(plan.status || "OPEN_ACTIVE")}</span>
    </div>
    <div class="plan-hero-grid">
      <div class="plan-hero-card entry">
        <span>Entry</span>
        <strong>${fmtNumber(Number(plan.entry_price))}</strong>
        <em>${escapeHtml(actionLabel(plan.direction || "--"))}</em>
      </div>
      <div class="plan-hero-card stop">
        <span>Stop Loss</span>
        <strong>${fmtNumber(Number(plan.stop_loss))}</strong>
        <em>${Number.isFinite(stopDistance) ? `Risk Distance ${fmtSignedDelta(stopDistance)}` : "Risk Distance --"}</em>
      </div>
      <div class="plan-hero-card leverage">
        <span>Size / Leverage</span>
        <strong>${fmtNumber(Number(plan.position_size))}</strong>
        <em>${Number.isFinite(Number(plan.leverage)) ? `${fmtNumber(Number(plan.leverage))}x` : "--"}</em>
      </div>
      <div class="plan-hero-card risk">
        <span>Risk</span>
        <strong>${fmtPercent(Number(plan.risk_pct), 2)}</strong>
        <em>${Number.isFinite(Number(plan.initial_qty)) && Number(plan.initial_qty) > 0 ? `Initial Qty ${fmtNumber(Number(plan.initial_qty))}` : "实时持仓映射"}</em>
      </div>
    </div>
    <div class="plan-ladder">
      <div class="plan-ladder-head">
        <span>Profit Ladder</span>
        <span>${takeProfits.length > 0 ? `${takeProfits.length} 个目标位` : "暂无目标位"}</span>
      </div>
      <div class="plan-level-list">${ratioItems || `<div class="decision-empty-inline">暂无止盈层级</div>`}</div>
    </div>
  </section>`;
}

function renderDecisionPlanContext(planContext) {
  if (!planContext) {
    return "";
  }
  const items = [
    ["单笔风险", fmtPercent(planContext.risk_per_trade_pct, 2)],
    ["最大占用", fmtPercent(planContext.max_invest_pct, 0)],
    ["最大杠杆", Number.isFinite(Number(planContext.max_leverage)) ? `${fmtNumber(Number(planContext.max_leverage))}x` : "--"],
    ["入场偏移", Number.isFinite(Number(planContext.entry_offset_atr)) ? `${fmtNumber(Number(planContext.entry_offset_atr))} ATR` : "--"],
    ["Entry Mode", String(planContext.entry_mode || "--")],
    ["Initial Exit", String(planContext.initial_exit || "--")]
  ];
  return `<section class="decision-section">
    <div class="decision-section-head">
      <h3>开仓计算依据</h3>
      <span>Risk Management</span>
    </div>
    <div class="decision-fact-grid">
      ${items.map(([label, value]) => `<div class="decision-fact-card"><span>${escapeHtml(label)}</span><strong>${escapeHtml(String(value))}</strong></div>`).join("")}
    </div>
  </section>`;
}

function renderDecisionSieve(sieve) {
  if (!sieve) {
    return "";
  }
  const rows = Array.isArray(sieve.rows) ? sieve.rows : [];
  if (!sieve.action && !sieve.reason_code && rows.length === 0) {
    return "";
  }
  const meterWidth = Math.max(8, Math.min(100, Number.isFinite(Number(sieve.size_factor)) ? Number(sieve.size_factor) * 100 : 0));
  const matched = rows[0] || null;
  return `<section class="decision-section sieve-section ${decisionActionClass(sieve.action)}">
    <div class="decision-section-head">
      <h3>risk_management.sieve</h3>
      <span>${escapeHtml(actionLabel(sieve.action || "--"))}</span>
    </div>
    <div class="sieve-rule-focus ${matched ? "matched" : ""}">
      <div class="sieve-rule-head">
        <strong>${escapeHtml(translateSieveReason(sieve.reason_code))}</strong>
        <span>${escapeHtml(actionLabel(sieve.action || "--"))}</span>
      </div>
      <div class="sieve-rule-meta">
        <span class="flow-chip pass">size:${escapeHtml(fmtPercent(Number(sieve.size_factor), 0))}</span>
        <span class="flow-chip">min:${escapeHtml(fmtPercent(Number(sieve.min_size_factor), 0))}</span>
        <span class="flow-chip">default:${escapeHtml(fmtPercent(Number(sieve.default_size_factor), 0))}</span>
        ${matched ? `<span class="flow-chip">mechanics:${escapeHtml(matched.mechanics_tag || "--")}</span>
        <span class="flow-chip">liq:${escapeHtml(matched.liq_confidence || "--")}</span>
        <span class="flow-chip">crowd:${matched.crowding_align === true ? "same" : matched.crowding_align === false ? "opp" : "any"}</span>` : ""}
      </div>
      <div class="sieve-reason-code">${escapeHtml(sieve.reason_code || "NO_SIEVE_REASON")}</div>
      <div class="sieve-meter"><span class="sieve-meter-fill" style="width:${meterWidth.toFixed(1)}%"></span></div>
    </div>
  </section>`;
}

function renderTightenDetail(tighten) {
  if (!tighten) {
    return "";
  }
  const blockedBy = Array.isArray(tighten.blocked_by) ? tighten.blocked_by : [];
  const threshold = Number(tighten.score_threshold);
  const score = Number(tighten.score);
  const progress = Number.isFinite(score) && Number.isFinite(threshold) && threshold > 0
    ? Math.max(0, Math.min(100, (score / threshold) * 100))
    : 0;
  const stateLabel = tighten.executed ? "已执行" : tighten.eligible ? "待执行" : tighten.evaluated ? "未触发" : "未评估";
  const stateClass = tighten.executed ? "pass" : blockedBy.length > 0 ? "fail" : "neutral";
  return `<section class="decision-section tighten-section ${stateClass}">
    <div class="decision-section-head">
      <h3>持仓收紧信息</h3>
      <span>${escapeHtml(stateLabel)}</span>
    </div>
    <div class="decision-fact-grid tighten-fact-grid">
      <div class="decision-fact-card">
        <span>执行状态</span>
        <strong>${escapeHtml(tighten.executed ? "TIGHTEN EXECUTED" : "TIGHTEN CHECKED")}</strong>
      </div>
      <div class="decision-fact-card">
        <span>TP 同步收紧</span>
        <strong>${escapeHtml(tighten.tp_tightened ? "YES" : "NO")}</strong>
      </div>
      <div class="decision-fact-card">
        <span>主阻断原因</span>
        <strong>${escapeHtml(blockedBy[0] || tighten.display_reason || "--")}</strong>
      </div>
    </div>
    <div class="tighten-score-card">
      <div class="decision-kpi-head">
        <span>收紧评分</span>
        <span class="decision-chip ${stateClass}">${escapeHtml(stateLabel)}</span>
      </div>
      <div class="decision-kpi-value">${escapeHtml(fmtConsensusValue(tighten.score))}</div>
      <div class="decision-kpi-sub">阈值 ${escapeHtml(fmtConsensusValue(tighten.score_threshold))} / parse ${tighten.score_parse_ok ? "ok" : "fail"}</div>
      <div class="decision-meter"><span class="decision-meter-fill ${stateClass}" style="width:${progress.toFixed(1)}%"></span></div>
      <div class="decision-meter-meta">达成率 ${Number.isFinite(progress) ? `${Math.round(progress)}%` : "--"}</div>
    </div>
    ${blockedBy.length > 0 ? `<div class="decision-check-row">${blockedBy.map((item) => `<span class="decision-chip fail">${escapeHtml(String(item))}</span>`).join("")}</div>` : ""}
  </section>`;
}

function renderDecisionDetail(detail, fallbackMessage) {
  if (!detail) {
    els.decisionDetail.innerHTML = `<div class="decision-empty">${escapeHtml(fallbackMessage || "暂无详情")}</div>`;
    return;
  }
  state.reportCollapsed = true;
  const snapshotID = Number(detail.snapshot_id || 0);
  const action = String(detail.action || "--").toUpperCase();
  const tradeable = Boolean(detail.tradeable);
  const overallPassed = detail.consensus_passed;
  const scorePassed = detail.consensus_score_passed;
  const confidencePassed = detail.consensus_confidence_passed;
  const scoreProgress = decisionMetricProgress(detail.consensus_score, detail.consensus_score_threshold, true);
  const confidenceProgress = decisionMetricProgress(detail.consensus_confidence, detail.consensus_confidence_threshold, false);
  const reportMarkdown = cleanDecisionMarkdown(detail.report_markdown || "");
  const isTighten = action === "TIGHTEN" && detail.tighten;
  els.decisionDetail.innerHTML = `
    <div class="decision-card">
      <div class="decision-top">
        <div>
          <div class="decision-snapshot">Snapshot <span>#${snapshotID}</span></div>
          <div class="decision-chip-row">
            <span class="decision-chip ${decisionActionClass(action)}">${escapeHtml(actionLabel(action))}</span>
            <span class="decision-chip ${tradeable ? "pass" : "fail"}">Tradeable ${tradeable ? "YES" : "NO"}</span>
            <span class="decision-chip ${decisionBoolClass(overallPassed)}">共识 ${escapeHtml(fmtConsensusPassed(overallPassed))}</span>
          </div>
        </div>
        <a class="decision-link-btn" href="${escapeHtml(detail.decision_view_url || "/decision-view/")}" target="_blank" rel="noopener noreferrer">Decision View</a>
      </div>
      <div class="decision-reason-line">
        <span>触发原因</span>
        <strong>${escapeHtml(detail.reason || "--")}</strong>
      </div>
      ${isTighten ? renderTightenDetail(detail.tighten) : `<div class="decision-kpi-grid">
        <div class="decision-kpi-card">
          <div class="decision-kpi-head">
            <span>共识总分</span>
            <span class="decision-chip ${decisionBoolClass(scorePassed)}">${escapeHtml(fmtConsensusPassed(scorePassed))}</span>
          </div>
          <div class="decision-kpi-value">${escapeHtml(fmtConsensusValue(detail.consensus_score))}</div>
          <div class="decision-kpi-sub">阈值 ${escapeHtml(fmtConsensusValue(detail.consensus_score_threshold))}</div>
          <div class="decision-meter"><span class="decision-meter-fill ${decisionBoolClass(scorePassed)}" style="width:${scoreProgress ? scoreProgress.width.toFixed(1) : "0.0"}%"></span></div>
          <div class="decision-meter-meta">达成率 ${scoreProgress ? `${Math.round(scoreProgress.ratio)}%` : "--"}</div>
        </div>
        <div class="decision-kpi-card">
          <div class="decision-kpi-head">
            <span>置信度</span>
            <span class="decision-chip ${decisionBoolClass(confidencePassed)}">${escapeHtml(fmtConsensusPassed(confidencePassed))}</span>
          </div>
          <div class="decision-kpi-value">${escapeHtml(fmtConsensusValue(detail.consensus_confidence))}</div>
          <div class="decision-kpi-sub">阈值 ${escapeHtml(fmtConsensusValue(detail.consensus_confidence_threshold))}</div>
          <div class="decision-meter"><span class="decision-meter-fill ${decisionBoolClass(confidencePassed)}" style="width:${confidenceProgress ? confidenceProgress.width.toFixed(1) : "0.0"}%"></span></div>
          <div class="decision-meter-meta">达成率 ${confidenceProgress ? `${Math.round(confidenceProgress.ratio)}%` : "--"}</div>
        </div>
      </div>
      <div class="decision-check-row">
        <span class="decision-chip ${decisionBoolClass(scorePassed)}">总分 ${escapeHtml(fmtConsensusPassed(scorePassed))}</span>
        <span class="decision-chip ${decisionBoolClass(confidencePassed)}">置信度 ${escapeHtml(fmtConsensusPassed(confidencePassed))}</span>
        <span class="decision-chip ${decisionBoolClass(overallPassed)}">总判定 ${escapeHtml(fmtConsensusPassed(overallPassed))}</span>
      </div>`}
      ${!isTighten ? renderDecisionPlanContext(detail.plan_context) : ""}
      ${renderDecisionSieve(detail.sieve)}
      ${renderDecisionPlan(detail.plan)}
      <section class="decision-section decision-report-shell collapsed" data-report-shell data-collapsed="true">
        <div class="decision-section-head decision-report-head">
          <h3>决策报告</h3>
          <button type="button" class="decision-report-toggle" data-report-toggle aria-expanded="false">${reportToggleLabel(true)}</button>
        </div>
        <div class="decision-report-collapsible">
          <pre class="decision-report-body">${escapeHtml(reportMarkdown)}</pre>
          <div class="decision-report-footer">
            <button type="button" class="decision-report-toggle ghost" data-report-toggle aria-expanded="false">${reportToggleLabel(true)}</button>
          </div>
        </div>
      </section>
    </div>
  `;
  bindDecisionReportToggle();
}

function cleanDecisionMarkdown(input) {
  const text = String(input || "");
  if (!text.trim()) {
    return "";
  }
  const lines = text.split("\n");
  const filtered = lines.filter((line) => {
    const trimmed = String(line || "").trim();
    if (!trimmed) {
      return true;
    }
    if (/^\s*(状态|动作|冲突|风险)\s*[：:]\s*[-—]\s*$/.test(trimmed)) {
      return false;
    }
    if (/^\s*[-•]\s*(状态|动作|冲突|风险)\s*[：:]\s*[-—]\s*$/.test(trimmed)) {
      return false;
    }
    return true;
  });
  return filtered.join("\n");
}

async function refreshOverview() {
  const [data, accountSummary] = await Promise.all([
    fetchJSON(`${apiBase}/overview`, { symbol: state.symbol || undefined }),
    fetchJSON(`${apiBase}/account_summary`).catch(() => null)
  ]);
  const cards = Array.isArray(data.symbols) ? data.symbols : [];
  state.overviewCards = cards;
  const symbolSet = new Set(cards.map((item) => String(item.symbol || "").trim().toUpperCase()).filter(Boolean));
  if (accountSummary && accountSummary.balance && Array.isArray(accountSummary.balance.monitored_symbols)) {
    accountSummary.balance.monitored_symbols.forEach((symbol) => {
      const normalized = String(symbol || "").trim().toUpperCase();
      if (normalized) {
        symbolSet.add(normalized);
      }
    });
  }
  try {
    const monitor = await fetchJSON(`${runtimeBase}/monitor/status`);
    const monitorSymbols = Array.isArray(monitor.symbols)
      ? monitor.symbols.map((item) => String(item.symbol || "").trim().toUpperCase()).filter(Boolean)
      : [];
    monitorSymbols.forEach((symbol) => {
      symbolSet.add(symbol);
    });
  } catch (_err) {
  }
  try {
    const schedule = await fetchJSON(`${runtimeBase}/schedule/status`);
    const scheduleSymbols = Array.isArray(schedule.next_runs)
      ? schedule.next_runs.map((item) => String(item.symbol || "").trim().toUpperCase()).filter(Boolean)
      : [];
    scheduleSymbols.forEach((symbol) => {
      symbolSet.add(symbol);
    });
  } catch (_err) {
  }
  state.symbols = Array.from(symbolSet);
  state.symbols.sort();
  if (!state.symbol && state.symbols.length > 0) {
    state.symbol = state.symbols[0];
  }
  if (state.symbols.length > 0 && !state.symbols.includes(state.symbol)) {
    state.symbol = state.symbols[0];
  }
  renderSymbolSelect();
  renderLivePositions(cards);
  renderAccountSummary(accountSummary || null);
}

async function refreshFlow(snapshotID) {
  if (!state.symbol) {
    renderFlow(null);
    return null;
  }
  const data = await fetchJSON(`${apiBase}/decision_flow`, { symbol: state.symbol, snapshot_id: snapshotID || undefined });
  const intervals = Array.isArray(data && data.flow ? data.flow.intervals : [])
    ? data.flow.intervals.map((item) => String(item || "").trim().toLowerCase()).filter(Boolean)
    : [];
  state.klineIntervals = intervals;
  state.lastFlow = data.flow || null;
  if (state.symbol) {
    const savedInterval = String(state.intervalsBySymbol[state.symbol] || "").toLowerCase();
    if (savedInterval && intervals.includes(savedInterval)) {
      state.klineInterval = savedInterval;
    } else {
      const preferredInterval = pickMiddleUpperInterval(intervals);
      if (preferredInterval) {
        state.klineInterval = preferredInterval;
        state.intervalsBySymbol[state.symbol] = preferredInterval;
      }
    }
  }
  renderIntervalSelect();
  renderFlow(data.flow);
  const anchorSnapshot = data && data.flow && data.flow.anchor ? Number(data.flow.anchor.snapshot_id || 0) : 0;
  const link = `/decision-view/?symbol=${encodeURIComponent(state.symbol || "ALL")}${anchorSnapshot > 0 ? `&snapshot_id=${anchorSnapshot}` : ""}`;
  els.decisionViewLink.href = link;
  return data.flow;
}

async function refreshKline(flow) {
  if (!state.symbol) {
    renderKline({ candles: [] }, flow || null);
    return;
  }
  const data = await fetchJSON(`${apiBase}/kline`, { symbol: state.symbol, interval: state.klineInterval, limit: state.klineLimit });
  renderKline(data, flow || null);
}

async function refreshPositionHistory() {
  if (!state.symbol) {
    state.lastPositionHistory = [];
    renderPositionHistory([]);
    return;
  }
  const data = await fetchJSON(`${runtimeBase}/position/history`, { symbol: state.symbol });
  let trades = Array.isArray(data.trades) ? data.trades : [];
  if (trades.length === 0) {
    const fallbackData = await fetchJSON(`${runtimeBase}/position/history`);
    trades = Array.isArray(fallbackData.trades) ? fallbackData.trades : [];
  }
  state.lastPositionHistory = trades;
  renderPositionHistory(trades);
}

async function refreshDecisionHistory(snapshotID) {
  if (!state.symbol) {
    state.lastDecisionItems = [];
    state.selectedDecisionSnapshotID = 0;
    state.selectedDecisionSnapshotSymbol = "";
    renderDecisionHistoryRows([], 0);
    renderDecisionDetail(null, "暂无历史决策");
    const flow = await refreshFlow();
    await refreshKline(flow);
    return;
  }
  if (state.selectedDecisionSnapshotSymbol !== state.symbol) {
    state.selectedDecisionSnapshotID = 0;
    state.selectedDecisionSnapshotSymbol = state.symbol;
  }
  const requestSeq = ++state.historyRequestSeq;
  const data = await fetchJSON(`${apiBase}/decision_history`, {
    symbol: state.symbol,
    limit: 20
  });
  if (requestSeq !== state.historyRequestSeq) {
    return;
  }
  const items = Array.isArray(data.items) ? data.items : [];
  state.lastDecisionItems = items;
  state.lastHistoryItems = items;
  const preferredSnapshotID = Number(snapshotID || state.selectedDecisionSnapshotID || 0);
  const matchedSnapshot = items.find((item) => Number(item.snapshot_id || 0) === preferredSnapshotID);
  const selectedSnapshotID = matchedSnapshot
    ? preferredSnapshotID
    : Number(items[0] && items[0].snapshot_id ? items[0].snapshot_id : 0);
  state.selectedDecisionSnapshotID = selectedSnapshotID;
  state.selectedDecisionSnapshotSymbol = state.symbol;
  renderDecisionHistoryRows(items, selectedSnapshotID);
  if (selectedSnapshotID > 0) {
    const detailData = await fetchJSON(`${apiBase}/decision_history`, {
      symbol: state.symbol,
      limit: 20,
      snapshot_id: selectedSnapshotID
    });
    if (requestSeq !== state.historyRequestSeq) {
      return;
    }
    renderDecisionDetail(detailData.detail || null, detailData.message || "");
    const flow = await refreshFlow(selectedSnapshotID);
    if (requestSeq !== state.historyRequestSeq) {
      return;
    }
    await refreshKline(flow);
    return;
  }
  renderDecisionDetail(null, data.message || "暂无历史决策");
  const flow = await refreshFlow();
  if (requestSeq !== state.historyRequestSeq) {
    return;
  }
  await refreshKline(flow);
}

async function refreshStatus() {
  try {
    await fetchJSON(`${runtimeBase}/schedule/status`);
    els.status.classList.remove("positive", "negative");
    els.status.textContent = "Healthy";
    els.status.classList.add("positive");
  } catch (_err) {
    els.status.classList.remove("positive", "negative");
    els.status.textContent = "Degraded";
    els.status.classList.add("negative");
  }
}

async function refreshSymbolScope() {
  await refreshPositionHistory();
  await refreshDecisionHistory(state.selectedDecisionSnapshotID || undefined);
}

async function runRefreshCycle(mode) {
  const cycleMode = mode === "scope" || mode === "auto" ? mode : "full";
  if (state.refreshing) {
    if (cycleMode === "full" || state.pendingRefreshMode !== "full") {
      state.pendingRefreshMode = cycleMode;
    }
    return;
  }
  state.refreshing = true;
  try {
    if (cycleMode === "full" || cycleMode === "auto") {
      await refreshOverview();
    }
    if (cycleMode === "auto") {
      await refreshPositionHistory();
    } else {
      await refreshSymbolScope();
    }
    await refreshStatus();
  } catch (err) {
    renderGlobalError(err);
  } finally {
    state.refreshing = false;
  }

  if (state.pendingRefreshMode) {
    const nextMode = state.pendingRefreshMode;
    state.pendingRefreshMode = "";
    await runRefreshCycle(nextMode);
  }
}

async function bootstrap() {
  startHeroTitleTyping();
  updateClock();
  setInterval(updateClock, 1000);

  els.symbolSelect.addEventListener("change", async () => {
    state.symbol = els.symbolSelect.value;
    state.selectedDecisionSnapshotID = 0;
    state.selectedDecisionSnapshotSymbol = state.symbol;
    await runRefreshCycle("scope");
  });

  els.intervalSelect.addEventListener("change", async () => {
    const nextInterval = String(els.intervalSelect.value || "").toLowerCase();
    if (!nextInterval) {
      return;
    }
    state.klineInterval = nextInterval;
    if (state.symbol) {
      state.intervalsBySymbol[state.symbol] = nextInterval;
    }
    await refreshKline(state.lastFlow);
  });

  await runRefreshCycle("full");

  setInterval(async () => {
    await runRefreshCycle("auto");
  }, refreshMs);

  let resizeQueued = false;
  window.addEventListener("resize", () => {
    if (resizeQueued) {
      return;
    }
    resizeQueued = true;
    window.requestAnimationFrame(() => {
      resizeQueued = false;
      if (chart) {
        chart.resize();
      }
      if (state.lastFlow) {
        renderFlow(state.lastFlow);
      }
    });
  });
}

bootstrap();

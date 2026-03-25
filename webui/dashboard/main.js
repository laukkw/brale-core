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
  selectedDecisionAt: "",
  historyRequestSeq: 0,
  reportCollapsed: true,
  refreshing: false,
  pendingRefreshMode: "",
  renderCache: {
    symbolOptions: "",
    intervalOptions: "",
    livePositions: "",
    positionHistory: "",
    decisionHistory: "",
    flowGraph: "",
    flowMeta: "",
    flowDataKey: "",
    klineDataKey: ""
  }
};

const els = {
  heroTitle: document.getElementById("hero-title"),
  status: document.getElementById("runtime-status"),
  clock: document.getElementById("clock-chip"),
  symbolSelect: document.getElementById("symbol-select"),
  intervalSelect: document.getElementById("interval-select"),
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
let activeHeldPositionCard = null;
let activeHeldHistoryRow = null;

const FLOW_LAYOUT_MIN_INTERVAL_MS = 120;
const KLINE_TAG_MIN_INTERVAL_MS = 120;

const flowLayoutTask = {
  raf: 0,
  timer: 0,
  settleTimer: 0,
  lastRunAt: 0,
  runner: null
};

const klineTagTask = {
  raf: 0,
  timer: 0,
  settleTimer: 0,
  lastRunAt: 0,
  runner: null
};

function prefersReducedMotion() {
  return Boolean(window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches);
}

function addTransientClass(element, className, durationMs) {
  if (!element || !className) {
    return;
  }
  element.classList.remove(className);
  if (window.requestAnimationFrame) {
    window.requestAnimationFrame(() => {
      element.classList.add(className);
    });
  } else {
    element.classList.add(className);
  }
  const delay = Number.isFinite(Number(durationMs)) ? Number(durationMs) : 260;
  window.setTimeout(() => {
    element.classList.remove(className);
  }, delay);
}

function animateDecisionDetailEntry() {
  const target = els.decisionDetail;
  if (!target || prefersReducedMotion()) {
    return;
  }
  target.classList.remove("motion-enter", "motion-enter-active");
  target.classList.add("motion-enter");
  window.requestAnimationFrame(() => {
    target.classList.add("motion-enter-active");
  });
  window.setTimeout(() => {
    target.classList.remove("motion-enter", "motion-enter-active");
  }, 320);
}

function startHeroTitleTyping() {
  const el = els.heroTitle;
  if (!el) {
    return;
  }
  const fullText = String(el.getAttribute("data-text") || el.textContent || "");
  if (!fullText) {
    return;
  }
  if (prefersReducedMotion()) {
    el.textContent = fullText;
    el.classList.remove("typing");
    el.classList.add("typed");
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
    return "已阻断";
  }
  if (status === "ok") {
    return "通过";
  }
  return "评估中";
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
  return labels[code] || (code ? `风险筛选命中 ${code}` : "当前未命中额外风险筛选原因");
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

function sanitizeLinkHref(input, fallback) {
  const fallbackHref = String(fallback || "/decision-view/");
  const raw = String(input || "").trim();
  if (!raw) {
    return fallbackHref;
  }
  try {
    const parsed = new URL(raw, window.location.origin);
    if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
      return fallbackHref;
    }
    return parsed.href;
  } catch (_err) {
    return fallbackHref;
  }
}

function setInnerHTMLIfChanged(element, cacheKey, html) {
  if (!element) {
    return false;
  }
  const next = String(html || "");
  if (state.renderCache[cacheKey] === next) {
    return false;
  }
  element.innerHTML = next;
  state.renderCache[cacheKey] = next;
  return true;
}

function cssVar(name, fallback) {
  const value = window.getComputedStyle(document.documentElement).getPropertyValue(name).trim();
  return value || fallback;
}

function readThemePalette() {
  return {
    accent: cssVar("--accent", "#3ad6a3"),
    accent2: cssVar("--accent-2", "#f6bd43"),
    danger: cssVar("--danger", "#ff6a78"),
    info: cssVar("--info", "#5bc0ff"),
    axis: cssVar("--axis", "#8ea2bb"),
    lineMuted: cssVar("--line-muted", "rgba(255,255,255,0.08)"),
    textSubtle: cssVar("--txt-2", "#9caec6"),
    signalLabel: cssVar("--signal-label", "#08101a"),
    flowLink: cssVar("--flow-link", "#6db9ff")
  };
}

const HASH_OFFSET_BASIS = 2166136261;

function hashAppend(hash, value) {
  let next = hash >>> 0;
  const text = String(value ?? "");
  for (let i = 0; i < text.length; i += 1) {
    next ^= text.charCodeAt(i);
    next = Math.imul(next, 16777619);
  }
  next ^= 124;
  return Math.imul(next, 16777619) >>> 0;
}

function hashToKey(hash) {
  return (hash >>> 0).toString(36);
}

function buildFlowRenderKey(flow) {
  const nodes = flow && Array.isArray(flow.nodes) ? flow.nodes : [];
  const trace = flow && flow.trace ? flow.trace : {};
  const tighten = flow && flow.tighten ? flow.tighten : {};
  const anchor = flow && flow.anchor ? flow.anchor : {};
  let hash = HASH_OFFSET_BASIS;

  hash = hashAppend(hash, nodes.length);
  nodes.forEach((node) => {
    hash = hashAppend(hash, node && node.stage);
    hash = hashAppend(hash, node && node.status);
    hash = hashAppend(hash, node && node.outcome);
    hash = hashAppend(hash, node && node.reason);
    const values = Array.isArray(node && node.values) ? node.values : [];
    hash = hashAppend(hash, values.length);
    values.forEach((field) => {
      hash = hashAppend(hash, field && field.key);
      hash = hashAppend(hash, field && field.value);
      hash = hashAppend(hash, field && field.state);
    });
  });

  function appendStageList(label, items) {
    const list = Array.isArray(items) ? items : [];
    hash = hashAppend(hash, label);
    hash = hashAppend(hash, list.length);
    list.forEach((item) => {
      hash = hashAppend(hash, item && item.stage);
      hash = hashAppend(hash, item && item.status);
      hash = hashAppend(hash, item && item.reason);
      hash = hashAppend(hash, item && item.summary);
      hash = hashAppend(hash, item && item.mode);
      hash = hashAppend(hash, item && item.action);
      const values = Array.isArray(item && item.values) ? item.values : [];
      hash = hashAppend(hash, values.length);
      values.forEach((field) => {
        hash = hashAppend(hash, field && field.key);
        hash = hashAppend(hash, field && field.value);
        hash = hashAppend(hash, field && field.state);
      });
    });
  }

  appendStageList("agents", trace.agents);
  appendStageList("providers", trace.providers);

  const gate = trace && trace.gate ? trace.gate : {};
  hash = hashAppend(hash, gate && gate.action);
  hash = hashAppend(hash, gate && gate.status);
  hash = hashAppend(hash, gate && gate.reason);
  const gateRules = Array.isArray(gate && gate.rules) ? gate.rules : [];
  hash = hashAppend(hash, gateRules.length);
  gateRules.forEach((rule) => {
    hash = hashAppend(hash, rule && rule.key);
    hash = hashAppend(hash, rule && rule.value);
    hash = hashAppend(hash, rule && rule.state);
  });

  const inPosition = trace && trace.in_position ? trace.in_position : {};
  hash = hashAppend(hash, inPosition && inPosition.active);
  hash = hashAppend(hash, inPosition && inPosition.side);
  hash = hashAppend(hash, inPosition && inPosition.status);
  hash = hashAppend(hash, inPosition && inPosition.reason);

  hash = hashAppend(hash, tighten && tighten.executed);
  hash = hashAppend(hash, tighten && tighten.score);
  hash = hashAppend(hash, tighten && tighten.score_threshold);
  hash = hashAppend(hash, tighten && tighten.display_reason);

  hash = hashAppend(hash, anchor && anchor.type);
  hash = hashAppend(hash, anchor && anchor.snapshot_id);

  return hashToKey(hash);
}

function runThrottledTask(task) {
  if (!task.runner) {
    return;
  }
  const runner = task.runner;
  task.lastRunAt = Date.now();
  runner();
}

function scheduleThrottledTask(task, minIntervalMs) {
  if (!task.runner) {
    return;
  }
  if (task.raf || task.timer) {
    return;
  }
  const now = Date.now();
  const elapsed = now - task.lastRunAt;
  const wait = Math.max(0, minIntervalMs - elapsed);
  const queueRaf = () => {
    task.raf = window.requestAnimationFrame(() => {
      task.raf = 0;
      runThrottledTask(task);
    });
  };
  if (wait === 0) {
    queueRaf();
    return;
  }
  task.timer = window.setTimeout(() => {
    task.timer = 0;
    queueRaf();
  }, wait);
}

function scheduleFlowLayout(needSettle) {
  scheduleThrottledTask(flowLayoutTask, FLOW_LAYOUT_MIN_INTERVAL_MS);
  if (!needSettle) {
    return;
  }
  if (flowLayoutTask.settleTimer) {
    window.clearTimeout(flowLayoutTask.settleTimer);
  }
  flowLayoutTask.settleTimer = window.setTimeout(() => {
    flowLayoutTask.settleTimer = 0;
    scheduleThrottledTask(flowLayoutTask, FLOW_LAYOUT_MIN_INTERVAL_MS);
  }, 240);
}

function scheduleKlineTagRelayout(needSettle) {
  scheduleThrottledTask(klineTagTask, KLINE_TAG_MIN_INTERVAL_MS);
  if (!needSettle) {
    return;
  }
  if (klineTagTask.settleTimer) {
    window.clearTimeout(klineTagTask.settleTimer);
  }
  klineTagTask.settleTimer = window.setTimeout(() => {
    klineTagTask.settleTimer = 0;
    scheduleThrottledTask(klineTagTask, KLINE_TAG_MIN_INTERVAL_MS);
  }, 200);
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
  flowLayoutTask.runner = null;
  state.renderCache.flowDataKey = "";
  setInnerHTMLIfChanged(els.flowGraph, "flowGraph", "");
  setInnerHTMLIfChanged(els.flowMeta, "flowMeta", `<div>加载失败：${escapeHtml(msg)}。请稍后重试。</div>`);
  els.decisionDetail.textContent = `加载失败：${msg}。请刷新页面后重试。`;
}

function renderSymbolSelect() {
  const options = state.symbols
    .map((symbol) => `<option value="${escapeHtml(symbol)}" ${symbol === state.symbol ? "selected" : ""}>${escapeHtml(symbol)}</option>`)
    .join("");
  setInnerHTMLIfChanged(els.symbolSelect, "symbolOptions", options);
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
    timeZone: "Asia/Shanghai",
    hour12: false,
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit"
  });
}

const beijingDateTimeFormatter = new Intl.DateTimeFormat("en-CA", {
  timeZone: "Asia/Shanghai",
  hour12: false,
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit"
});

function parseTimestamp(value) {
  const text = String(value || "").trim();
  if (!text) {
    return Number.NaN;
  }
  const ts = Date.parse(text);
  return Number.isFinite(ts) ? ts : Number.NaN;
}

function formatBeijingDateTime(value) {
  const text = String(value || "").trim();
  if (!text) {
    return "--";
  }
  const ts = parseTimestamp(text);
  if (!Number.isFinite(ts)) {
    return text;
  }
  const parts = beijingDateTimeFormatter.formatToParts(new Date(ts));
  const partMap = {};
  parts.forEach((part) => {
    if (part.type !== "literal") {
      partMap[part.type] = part.value;
    }
  });
  return `${partMap.year || "0000"}-${partMap.month || "00"}-${partMap.day || "00"} ${partMap.hour || "00"}:${partMap.minute || "00"}:${partMap.second || "00"}`;
}

function sortByDateFieldDesc(items, fieldName) {
  const rows = Array.isArray(items) ? items : [];
  return rows
    .map((item, index) => ({
      item,
      index,
      ts: parseTimestamp(item && item[fieldName])
    }))
    .sort((left, right) => {
      const leftValid = Number.isFinite(left.ts);
      const rightValid = Number.isFinite(right.ts);
      if (leftValid && rightValid && left.ts !== right.ts) {
        return right.ts - left.ts;
      }
      if (leftValid && !rightValid) {
        return -1;
      }
      if (!leftValid && rightValid) {
        return 1;
      }
      return left.index - right.index;
    })
    .map((entry) => entry.item);
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
      <strong>止盈止损历史</strong>
      <span>共 ${versions.length} 个版本（从旧到新）</span>
    </div>
    <div class="position-hold-timeline">
      ${versions.map((item, index) => {
        const isCurrent = index === versions.length - 1;
        const card = `<article class="risk-version-card ${isCurrent ? "current" : ""}">
          <div class="risk-version-top">
            <span class="risk-version-label">${escapeHtml(isCurrent ? "当前版本" : `历史版本 ${index + 1}`)}</span>
            <em>${escapeHtml(fmtShortTime(item.created_at))}</em>
          </div>
          <div class="risk-version-values">
            <div class="risk-version-metric stop">
              <span>止损价</span>
              <strong>${fmtNumber(Number(item.stop_loss))}</strong>
            </div>
            <div class="risk-version-metric tp">
              <span>止盈价</span>
              <div class="risk-version-tp-list">${renderRiskTakeProfitPills(item.take_profits)}</div>
            </div>
          </div>
          <div class="risk-version-foot">${escapeHtml(String(item.label || item.source || "系统更新"))}</div>
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
  const target = card || activeHeldPositionCard;
  if (target) {
    target.classList.remove("pressing", "holding");
    if (target.getAttribute("data-holdable") === "true") {
      target.setAttribute("aria-expanded", "false");
    }
  }
  if (!card || card === activeHeldPositionCard) {
    activeHeldPositionCard = null;
  }
}

function clearHistoryPositionHold(row) {
  const target = row || activeHeldHistoryRow;
  if (!target) {
    if (!row) {
      activeHeldHistoryRow = null;
    }
    return;
  }
  target.classList.remove("pressing", "holding");
  if (target.getAttribute("data-history-holdable") === "true") {
    target.setAttribute("aria-expanded", "false");
  }
  const rowID = String(target.getAttribute("data-history-row-id") || "");
  if (rowID) {
    const preview = els.positionHistoryBody.querySelector(`[data-history-preview-for='${CSS.escape(rowID)}']`);
    if (preview) {
      preview.classList.remove("holding");
      preview.setAttribute("aria-hidden", "true");
    }
  }
  if (!row || row === activeHeldHistoryRow) {
    activeHeldHistoryRow = null;
  }
}

function bindLivePositionHold() {
  const cards = els.livePositionList.querySelectorAll("[data-position-card][data-holdable='true']");
  cards.forEach((card) => {
    const startHold = (event) => {
      if (event && Number.isFinite(Number(event.button)) && Number(event.button) !== 0) {
        return;
      }
      clearPositionHold(activeHeldPositionCard);
      card.classList.remove("pressing");
      card.classList.add("holding");
      card.setAttribute("aria-expanded", "true");
      activeHeldPositionCard = card;
      if (event && Number.isFinite(Number(event.pointerId)) && typeof card.setPointerCapture === "function") {
        try {
          card.setPointerCapture(event.pointerId);
        } catch (_err) {
          void _err;
        }
      }
    };
    const endHold = (event) => {
      if (event && Number.isFinite(Number(event.pointerId)) && typeof card.hasPointerCapture === "function" && card.hasPointerCapture(event.pointerId) && typeof card.releasePointerCapture === "function") {
        try {
          card.releasePointerCapture(event.pointerId);
        } catch (_err) {
          void _err;
        }
      }
      clearPositionHold(card);
    };
    card.addEventListener("pointerdown", startHold);
    card.addEventListener("pointerup", endHold);
    card.addEventListener("pointerleave", endHold);
    card.addEventListener("pointercancel", endHold);
    card.addEventListener("keydown", (event) => {
      if (event.key === "Enter" || event.key === " ") {
        event.preventDefault();
        startHold();
      }
      if (event.key === "Escape") {
        event.preventDefault();
        clearPositionHold(card);
      }
    });
    card.addEventListener("keyup", (event) => {
      if (event.key === "Enter" || event.key === " ") {
        event.preventDefault();
        endHold();
      }
    });
    card.addEventListener("blur", () => {
      clearPositionHold(card);
    });
  });
}

function bindHistoryPositionHold() {
  const rows = els.positionHistoryBody.querySelectorAll("[data-history-row-id][data-history-holdable='true']");
  rows.forEach((row) => {
    const rowID = String(row.getAttribute("data-history-row-id") || "");
    const preview = rowID
      ? els.positionHistoryBody.querySelector(`[data-history-preview-for='${CSS.escape(rowID)}']`)
      : null;
    const togglePreview = () => {
      const expanded = row.classList.contains("holding");
      if (expanded) {
        clearHistoryPositionHold(row);
        return;
      }
      clearHistoryPositionHold(activeHeldHistoryRow);
      row.classList.remove("pressing");
      row.classList.add("holding");
      row.setAttribute("aria-expanded", "true");
      if (preview) {
        preview.classList.add("holding");
        preview.setAttribute("aria-hidden", "false");
      }
      activeHeldHistoryRow = row;
    };

    row.addEventListener("click", togglePreview);
    row.addEventListener("keydown", (event) => {
      if (event.key === "Enter" || event.key === " ") {
        event.preventDefault();
        togglePreview();
      }
      if (event.key === "Escape") {
        event.preventDefault();
        clearHistoryPositionHold(row);
      }
    });
    row.addEventListener("blur", () => {
      clearHistoryPositionHold(row);
    });
  });
}

function renderLivePositions(cards) {
  if (!Array.isArray(cards) || cards.length === 0) {
    setInnerHTMLIfChanged(els.livePositionList, "livePositions", `<div class="position-empty">当前没有持仓。开仓后会在这里显示。</div>`);
    clearPositionHold();
    return;
  }

  const markup = cards
    .map((card) => {
      const position = card.position || {};
      const timeline = Array.isArray(position.risk_plan_timeline) ? position.risk_plan_timeline : [];
      const timelineMarkup = renderRiskPlanTimeline(timeline);
      const holdable = timelineMarkup !== "";
      const holdAttrs = holdable
        ? `tabindex="0" role="button" aria-expanded="false" aria-label="查看 ${escapeHtml(card.symbol)} 的止盈止损历史版本"`
        : "";
      const pnl = card.pnl || {};
      const realizedClass = Number(pnl.realized) >= 0 ? "positive" : "negative";
      const unrealizedClass = Number(pnl.unrealized) >= 0 ? "positive" : "negative";
      const leverage = Number(position.leverage);
      const leverageLabel = Number.isFinite(leverage) && leverage > 0 ? `（${fmtNumber(leverage)}x）` : "";
      return `<article class="position-card ${holdable ? "holdable" : ""}" data-position-card data-holdable="${holdable ? "true" : "false"}" ${holdAttrs}>
        <div class="position-head">
          <div>
            <div class="position-symbol">${escapeHtml(card.symbol)}</div>
            ${holdable ? `<div class="position-hold-hint">按住查看止盈止损历史版本</div>` : ""}
          </div>
          <span class="side-chip">${escapeHtml(position.side || "--")}</span>
        </div>
        <div class="metrics-grid">
          <div class="metric"><span class="k">仓位规模</span><span class="v">${fmtNumber(position.amount)}</span></div>
          <div class="metric"><span class="k">入场价</span><span class="v">${fmtNumber(position.entry_price)}</span></div>
          <div class="metric"><span class="k">当前价</span><span class="v">${fmtNumber(position.current_price)}</span></div>
          <div class="metric"><span class="k">止盈 TP</span><span class="v positive">${(position.take_profits || []).map((v) => fmtNumber(Number(v))).join(" / ") || "--"}</span></div>
          <div class="metric"><span class="k">止损 SL</span><span class="v negative">${fmtNumber(position.stop_loss)}</span></div>
          <div class="metric"><span class="k">未实现盈亏</span><span class="v ${unrealizedClass}">${fmtUsd(Number(pnl.unrealized))}</span></div>
          <div class="metric"><span class="k">合计盈亏</span><span class="v ${Number(pnl.total) >= 0 ? "positive" : "negative"}">${fmtUsd(Number(pnl.total || 0))}</span></div>
          <div class="metric"><span class="k">已实现盈亏${leverageLabel}</span><span class="v ${realizedClass}">${fmtUsd(Number(pnl.realized))}</span></div>
        </div>
        ${timelineMarkup}
      </article>`;
    })
    .join("");
  const updated = setInnerHTMLIfChanged(els.livePositionList, "livePositions", markup);
  if (updated) {
    bindLivePositionHold();
  }
}

function renderIntervalSelect() {
  const intervals = Array.isArray(state.klineIntervals) ? state.klineIntervals : [];
  const options = intervals
    .map((intervalValue) => {
      const selected = intervalValue === state.klineInterval ? "selected" : "";
      return `<option value="${escapeHtml(intervalValue)}" ${selected}>${escapeHtml(intervalValue.toUpperCase())}</option>`;
    })
    .join("");
  setInnerHTMLIfChanged(els.intervalSelect, "intervalOptions", options);
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

  function applyPnlState(el, value) {
    if (!el) {
      return;
    }
    const card = el.closest(".pnl-card");
    if (!card) {
      return;
    }
    card.classList.remove("positive", "negative");
    const numeric = Number(value || 0);
    card.classList.add(numeric >= 0 ? "positive" : "negative");
  }

  applyPnlState(els.acctProfitClosed, profit.closed_profit);
  applyPnlState(els.acctProfitAll, profit.all_profit);
}

function renderFlow(flow) {
  const nodes = flow && Array.isArray(flow.nodes) ? flow.nodes : [];
  const trace = flow && flow.trace ? flow.trace : {};
  if (nodes.length === 0) {
    flowLayoutTask.runner = null;
    state.renderCache.flowDataKey = "";
    setInnerHTMLIfChanged(els.flowGraph, "flowGraph", "");
    setInnerHTMLIfChanged(els.flowMeta, "flowMeta", "");
    return;
  }

  const flowDataKey = buildFlowRenderKey(flow);
  if (state.renderCache.flowDataKey === flowDataKey) {
    setInnerHTMLIfChanged(els.flowMeta, "flowMeta", "");
    return;
  }
  state.renderCache.flowDataKey = flowDataKey;

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
    return `通过 ${pass} · 阻断 ${block} · 枚举 ${enumCount}`;
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
  const providerTitlePrefix = providerMode === "in_position" ? "持仓管理汇总" : "规则汇总";

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
    node("agent-indicator", "flow-pos-agent-indicator", stageStatus(agentIndicator, "ok"), "Agent / 指标", stageSummary(agentIndicator, summarizeStageValues(agentIndicator && agentIndicator.values)), agentIndicator && agentIndicator.values, stageReason(agentIndicator, ""), "agent"),
    node("agent-structure", "flow-pos-agent-structure", stageStatus(agentStructure, "ok"), "Agent / 结构", stageSummary(agentStructure, summarizeStageValues(agentStructure && agentStructure.values)), agentStructure && agentStructure.values, stageReason(agentStructure, ""), "agent"),
    node("agent-mechanics", "flow-pos-agent-mechanics", stageStatus(agentMechanics, "ok"), "Agent / 力学", stageSummary(agentMechanics, summarizeStageValues(agentMechanics && agentMechanics.values)), agentMechanics && agentMechanics.values, stageReason(agentMechanics, ""), "agent"),
    node("provider-indicator", "flow-pos-provider-indicator", stageStatus(providerIndicator, "ok"), `${providerTitlePrefix} / 指标`, stageSummary(providerIndicator, summarizeStageValues(providerIndicator && providerIndicator.values)), providerIndicator && providerIndicator.values, stageReason(providerIndicator, ""), "provider"),
    node("provider-structure", "flow-pos-provider-structure", stageStatus(providerStructure, "ok"), `${providerTitlePrefix} / 结构`, stageSummary(providerStructure, summarizeStageValues(providerStructure && providerStructure.values)), providerStructure && providerStructure.values, stageReason(providerStructure, ""), "provider"),
    node("provider-mechanics", "flow-pos-provider-mechanics", stageStatus(providerMechanics, "ok"), `${providerTitlePrefix} / 力学`, stageSummary(providerMechanics, summarizeStageValues(providerMechanics && providerMechanics.values)), providerMechanics && providerMechanics.values, stageReason(providerMechanics, ""), "provider")
  ];

  if (inPosition && inPosition.active) {
    graphNodes.push(node("inposition", "flow-pos-inposition", stageStatus(inPosition, "ok"), "持仓状态", String(inPosition.side || "open"), [{ key: "active", value: "true", state: "pass" }, { key: "side", value: String(inPosition.side || "-") }], stageReason(inPosition, "已有持仓，链路进入监控/管理路径"), "monitor"));
  }

  const gateStatus = stageStatus(gate, "ok");
  const gateReason = stageReason(gate, "");
  graphNodes.push(node("gate", "flow-pos-gate", gateStatus, "决策网关", `${actionLabel(gate && gate.action ? gate.action : "--")}`, gate && Array.isArray(gate.rules) ? gate.rules : [], gateReason, "gate"));

  const resultStatus = String(resultNode && resultNode.status || "").trim().toLowerCase() || "ok";
  const resultReason = String(resultNode && resultNode.reason || "").trim();
  const resultDesc = resultNode ? String(resultNode.outcome || "-") : "-";
  const resultValues = resultNode && Array.isArray(resultNode.values) ? resultNode.values : [];
  graphNodes.push(node("result", "flow-pos-result", resultStatus, "执行结果", resultDesc, resultValues, resultReason, "result"));

  function renderFieldList(fields) {
    if (!Array.isArray(fields) || fields.length === 0) {
      return `<span class="flow-chip">--</span>`;
    }
    return fields.map((field) => {
      const stateValue = String(field && field.state || "");
      const stateClass = stateValue === "block" ? "block" : (stateValue === "pass" ? "pass" : "");
      const keyText = formatFlowFieldKey(field && field.key);
      const valueText = formatFlowFieldValue(field && field.key, field && field.value);
      return `<span class="flow-chip ${stateClass}">${escapeHtml(keyText)}:${escapeHtml(valueText)}</span>`;
    }).join("");
  }

  function formatFlowFieldKey(key) {
    const normalized = String(key || "").trim().toLowerCase();
    if (normalized === "plan_source") {
      return "plan来源";
    }
    return String(key || "");
  }

  function formatFlowFieldValue(key, value) {
    const normalizedKey = String(key || "").trim().toLowerCase();
    const text = String(value || "");
    if (normalizedKey === "plan_source") {
      const source = text.trim().toLowerCase();
      if (source === "llm") {
        return "llm自动生成";
      }
      if (source === "go") {
        return "go计算得到";
      }
    }
    return text;
  }

  function stageTypeLabel(stageType) {
    const key = String(stageType || "").trim().toLowerCase();
    if (key === "agent") {
      return "agent";
    }
    if (key === "provider") {
      return "汇总";
    }
    if (key === "monitor") {
      return "监控";
    }
    if (key === "gate") {
      return "网关";
    }
    if (key === "result") {
      return "结果";
    }
    return "阶段";
  }

  const nodesHTML = graphNodes.map((item, index) => {
    const reasonAttr = item.reason ? ` title="${escapeHtml(item.reason)}" data-reason="${escapeHtml(item.reason)}"` : "";
    return `<article id="flow-node-${escapeHtml(item.id)}" class="flow-node ${escapeHtml(item.status)} ${escapeHtml(item.posClass)}" style="--flow-index:${index}"${reasonAttr}>
      <div class="flow-node-topline">
        <span class="flow-node-kind">${escapeHtml(stageTypeLabel(item.stageType))}</span>
        <span class="flow-node-state ${escapeHtml(item.status)}">${escapeHtml(flowStatusLabel(item.status))}</span>
      </div>
      <div class="flow-node-title">${escapeHtml(item.title)}</div>
      <div class="flow-node-desc">${escapeHtml(item.desc)}</div>
      ${Array.isArray(item.detail) && item.detail.length > 0 ? `<div class="flow-node-values">${renderFieldList(item.detail)}</div>` : ""}
    </article>`;
  }).join("");

  const flowMarkup = `<div class="flow-backdrop"></div><svg class="flow-links" id="flow-links" aria-hidden="true"></svg><div class="flow-dag">${nodesHTML}</div>`;
  setInnerHTMLIfChanged(els.flowGraph, "flowGraph", flowMarkup);

  function drawFlowLinks() {
    const svg = document.getElementById("flow-links");
    if (!svg) {
      return;
    }
    const palette = readThemePalette();
    const flowLinkColor = escapeHtml(String(palette.flowLink || "#6db9ff"));
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

    const defs = `<defs><marker id="flow-arrow-head" markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto"><path d="M0,0 L8,4 L0,8 z" fill="${flowLinkColor}"></path></marker></defs>`;
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

  flowLayoutTask.runner = () => {
    drawFlowLinks();
    const svg = document.getElementById("flow-links");
    if (!svg) {
      return;
    }
    window.requestAnimationFrame(() => {
      svg.classList.add("ready");
    });
  };

  scheduleFlowLayout(true);

  setInnerHTMLIfChanged(els.flowMeta, "flowMeta", "");
}

function reportToggleLabel(collapsed) {
  return collapsed ? "展开完整报告" : "收起完整报告";
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
  if (!window.echarts || !dom) {
    throw new Error("图表模块加载失败，请刷新页面后重试");
  }
  chart = window.echarts.init(dom);
  return chart;
}

function renderKline(payload, flow) {
  const candles = Array.isArray(payload && payload.candles) ? payload.candles : [];
  const intervalLabel = String(payload && payload.interval ? payload.interval : state.klineInterval).toUpperCase();
  const c = ensureChart();
  const palette = readThemePalette();

  if (candles.length === 0) {
    const emptyDataKey = `empty|${state.symbol || "ALL"}|${intervalLabel}`;
    klineTagTask.runner = null;
    if (state.renderCache.klineDataKey !== emptyDataKey) {
      state.renderCache.klineDataKey = emptyDataKey;
      els.klinePriceTags.innerHTML = "";
      c.setOption({ title: { text: "暂无K线数据，请切换周期或稍后重试", left: "center", top: "middle", textStyle: { color: palette.textSubtle, fontSize: 14 } }, xAxis: { show: false }, yAxis: { show: false }, series: [] });
    }
    return;
  }

  const labels = [];
  const values = [];
  let candleHash = HASH_OFFSET_BASIS;
  candles.forEach((item) => {
    const openTime = Number(item && item.open_time || 0);
    const open = Number(item && item.open || 0);
    const close = Number(item && item.close || 0);
    const low = Number(item && item.low || 0);
    const high = Number(item && item.high || 0);
    labels.push(openTime);
    values.push([open, close, low, high]);
    candleHash = hashAppend(candleHash, openTime);
    candleHash = hashAppend(candleHash, open);
    candleHash = hashAppend(candleHash, close);
    candleHash = hashAppend(candleHash, low);
    candleHash = hashAppend(candleHash, high);
  });
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

  const markerName = flow && flow.anchor ? `${flow.anchor.type || "anchor"}#${flow.anchor.snapshot_id || 0}` : "锚点";
  const markerData = [];
  let markerHash = HASH_OFFSET_BASIS;

  function pushMarker(name, coord, value, color) {
    const item = {
      name,
      coord,
      value,
      itemStyle: { color }
    };
    markerData.push(item);
    markerHash = hashAppend(markerHash, name);
    markerHash = hashAppend(markerHash, coord && coord[0]);
    markerHash = hashAppend(markerHash, coord && coord[1]);
    markerHash = hashAppend(markerHash, value);
    markerHash = hashAppend(markerHash, color);
  }

  pushMarker(markerName, [labels[labels.length - 1], values[values.length - 1][1]], `锚点: ${markerName}`, palette.accent2);

  state.lastPositionHistory.forEach((trade) => {
    const openedAt = trade && trade.opened_at ? Date.parse(trade.opened_at) : 0;
    const closedAt = trade && trade.closed_at ? Date.parse(trade.closed_at) : 0;
    const openPoint = nearestPointByTime(openedAt);
    if (openPoint) {
      pushMarker("开仓", [openPoint.x, openPoint.y], `开仓 ${trade.symbol || ""} ${trade.side || ""} 数量:${fmtNumber(Number(trade.amount || 0))}`, palette.accent);
    }
    const closePoint = nearestPointByTime(closedAt);
    if (closePoint) {
      pushMarker("平仓", [closePoint.x, closePoint.y], `平仓 ${trade.symbol || ""} 收益:${fmtUsd(Number(trade.profit || 0))}`, palette.danger);
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
    pushMarker("收紧", [point.x, point.y], `收紧止损: ${item.reason || "-"}`, palette.accent2);
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
  const currentLine = priceLine("当前", currentPrice, palette.info, "solid");
  if (currentLine) {
    lineData.push(currentLine);
  }
  const stopLine = priceLine("止损", stopLoss, palette.danger, "dashed");
  if (stopLine) {
    lineData.push(stopLine);
  }
  takeProfits.forEach((value, index) => {
    const tpLine = priceLine(`止盈${index + 1}`, value, palette.accent, "dashed");
    if (tpLine) {
      lineData.push(tpLine);
    }
  });

  const axisPrices = [];
  values.forEach((entry) => {
    if (!Array.isArray(entry)) {
      return;
    }
    const low = Number(entry[2]);
    const high = Number(entry[3]);
    if (Number.isFinite(low)) {
      axisPrices.push(low);
    }
    if (Number.isFinite(high)) {
      axisPrices.push(high);
    }
  });
  lineData.forEach((line) => {
    const y = Number(line && line.yAxis);
    if (Number.isFinite(y)) {
      axisPrices.push(y);
    }
  });

  let yAxisMin;
  let yAxisMax;
  if (axisPrices.length > 0) {
    const rawMin = Math.min(...axisPrices);
    const rawMax = Math.max(...axisPrices);
    const span = rawMax - rawMin;
    const safeSpan = span > 0 ? span : Math.max(Math.abs(rawMax) * 0.01, 1);
    const pad = safeSpan * 0.08;
    yAxisMin = rawMin - pad;
    yAxisMax = rawMax + pad;
  }

  let lineHash = HASH_OFFSET_BASIS;
  lineData.forEach((line) => {
    lineHash = hashAppend(lineHash, line && line.name);
    lineHash = hashAppend(lineHash, line && line.yAxis);
    lineHash = hashAppend(lineHash, line && line.lineStyle && line.lineStyle.color);
    lineHash = hashAppend(lineHash, line && line.lineStyle && line.lineStyle.type);
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

  let klineHash = HASH_OFFSET_BASIS;
  klineHash = hashAppend(klineHash, state.symbol || "ALL");
  klineHash = hashAppend(klineHash, intervalLabel);
  klineHash = hashAppend(klineHash, candleHash);
  klineHash = hashAppend(klineHash, markerHash);
  klineHash = hashAppend(klineHash, lineHash);
  klineHash = hashAppend(klineHash, compactViewport ? "compact" : "wide");
  klineHash = hashAppend(klineHash, tagMetrics.klineGridRight);
  const klineDataKey = hashToKey(klineHash);
  if (state.renderCache.klineDataKey === klineDataKey) {
    klineTagTask.runner = () => {
      renderKlinePriceTags(lineData);
    };
    return;
  }
  state.renderCache.klineDataKey = klineDataKey;

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
      const color = line && line.lineStyle ? line.lineStyle.color : palette.info;
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
          return `${escapeHtml(String(params.name || "交易信号"))}<br/>${escapeHtml(String(params.data.detail || ""))}`;
        }
        if (params && params.seriesType === "candlestick" && Array.isArray(params.data)) {
          const v = params.data;
          return `开 ${fmtNumber(Number(v[0]))}<br/>收 ${fmtNumber(Number(v[1]))}<br/>低 ${fmtNumber(Number(v[2]))}<br/>高 ${fmtNumber(Number(v[3]))}`;
        }
        return "";
      }
    },
    xAxis: {
      type: "category",
      data: labels,
      boundaryGap: true,
      axisLine: { lineStyle: { color: palette.axis } },
      axisPointer: {
        label: {
          formatter(params) {
            return formatBeijingTime(params && params.value);
          }
        }
      },
      axisLabel: {
        color: palette.textSubtle,
        formatter(value) {
          return formatBeijingTime(value);
        }
      }
    },
    yAxis: {
      scale: true,
      min: yAxisMin,
      max: yAxisMax,
      axisLine: { lineStyle: { color: palette.axis } },
      splitLine: { lineStyle: { color: palette.lineMuted } },
      axisLabel: { color: palette.textSubtle }
    },
    series: [
      {
        name: "K",
        type: "candlestick",
        data: values,
        itemStyle: {
          color: palette.accent,
          color0: palette.danger,
          borderColor: palette.accent,
          borderColor0: palette.danger
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
          color: palette.signalLabel,
          fontWeight: 700,
          fontSize: 10
        }
      }
    ]
  });

  klineTagTask.runner = () => {
    renderKlinePriceTags(lineData);
  };
  scheduleKlineTagRelayout(true);
}

function renderPositionHistory(trades) {
  if (!Array.isArray(trades) || trades.length === 0) {
    clearHistoryPositionHold();
    setInnerHTMLIfChanged(els.positionHistoryBody, "positionHistory", `<tr><td colspan="5">暂无历史仓位，平仓后会在这里显示。</td></tr>`);
    return;
  }
  clearHistoryPositionHold();
  const orderedTrades = sortByDateFieldDesc(trades, "opened_at");
  const markup = orderedTrades
    .map((trade, index) => {
      const profit = Number(trade.profit || 0);
      const pnlClass = profit >= 0 ? "positive" : "negative";
      const opened = formatBeijingDateTime(trade.opened_at);
      const timeline = Array.isArray(trade.risk_plan_timeline) ? trade.risk_plan_timeline : [];
      const timelineMarkup = renderRiskPlanTimeline(timeline);
      const holdable = timelineMarkup !== "";
      const rowID = `history-position-${index}`;
      const rowAttrs = holdable
        ? `data-history-holdable="true" data-history-row-id="${rowID}" tabindex="0" role="button" aria-expanded="false" aria-label="展开查看 ${escapeHtml(trade.symbol || "该仓位")} 的止盈止损历史版本"`
        : `data-history-holdable="false" data-history-row-id="${rowID}"`;
      const previewRow = holdable
        ? `<tr class="position-history-preview-row" data-history-preview-for="${rowID}" aria-hidden="true">
        <td colspan="5">
          ${timelineMarkup}
        </td>
      </tr>`
        : "";
      return `<tr class="position-history-row ${holdable ? "holdable" : ""}" ${rowAttrs}>
        <td>${escapeHtml(trade.symbol || "--")}</td>
        <td>${escapeHtml(opened)}</td>
        <td>${escapeHtml(trade.side || "--")}</td>
        <td>${fmtNumber(Number(trade.amount || 0))}</td>
        <td class="${pnlClass}">${fmtUsd(profit)}</td>
      </tr>${previewRow}`;
    })
    .join("");
  const updated = setInnerHTMLIfChanged(els.positionHistoryBody, "positionHistory", markup);
  if (updated) {
    bindHistoryPositionHold();
  }
}

function renderDecisionHistoryRows(items, selectedSnapshotID) {
  if (!Array.isArray(items) || items.length === 0) {
    setInnerHTMLIfChanged(els.decisionHistoryBody, "decisionHistory", `<tr><td colspan="6">暂无历史决策，系统产生新决策后会在这里显示。</td></tr>`);
    return;
  }
  const markup = items
    .map((item) => {
      const active = Number(item.snapshot_id) === Number(selectedSnapshotID) ? "active" : "";
      return `<tr class="${active}" data-snapshot-id="${Number(item.snapshot_id || 0)}" tabindex="0" role="button" aria-label="查看快照 ${Number(item.snapshot_id || 0)} 的决策详情">
        <td>${escapeHtml(formatBeijingDateTime(item.at))}</td>
        <td>${escapeHtml(actionLabel(String(item.action || "--").toUpperCase()))}</td>
        <td>${escapeHtml(item.reason || "--")}</td>
        <td>${escapeHtml(fmtConsensusValue(item.consensus_score))}</td>
        <td>${escapeHtml(fmtConsensusValue(item.consensus_confidence))}</td>
        <td>${Number(item.snapshot_id || 0)}</td>
      </tr>`;
    })
    .join("");
  const updated = setInnerHTMLIfChanged(els.decisionHistoryBody, "decisionHistory", markup);

  if (!updated) {
    return;
  }

  els.decisionHistoryBody.querySelectorAll("tr[data-snapshot-id]").forEach((row) => {
    const openDetail = async () => {
      addTransientClass(row, "row-activated", 220);
      const snapshotID = row.getAttribute("data-snapshot-id");
      await refreshDecisionHistory(snapshotID);
    };
    row.addEventListener("click", openDetail);
    row.addEventListener("keydown", async (event) => {
      if (event.key !== "Enter" && event.key !== " ") {
        return;
      }
      event.preventDefault();
      await openDetail();
    });
  });
}

function renderDecisionPlan(plan) {
  if (!plan) {
    return "";
  }
  function formatPlanStatus(status) {
    const key = String(status || "").trim().toUpperCase();
    const labels = {
      OPEN_ACTIVE: "执行中",
      OPENED: "已开仓",
      PENDING: "待执行",
      CLOSED: "已结束"
    };
    return labels[key] || (key || "执行中");
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
      <h3>当前执行计划（非历史快照）</h3>
      <span>${escapeHtml(formatPlanStatus(plan.status))}</span>
    </div>
    <div class="plan-hero-grid">
      <div class="plan-hero-card entry">
        <span>入场价</span>
        <strong>${fmtNumber(Number(plan.entry_price))}</strong>
        <em>${escapeHtml(actionLabel(plan.direction || "--"))}</em>
      </div>
      <div class="plan-hero-card stop">
        <span>止损价</span>
        <strong>${fmtNumber(Number(plan.stop_loss))}</strong>
        <em>${Number.isFinite(stopDistance) ? `风险距离 ${fmtSignedDelta(stopDistance)}` : "风险距离 --"}</em>
      </div>
      <div class="plan-hero-card leverage">
        <span>仓位 / 杠杆</span>
        <strong>${fmtNumber(Number(plan.position_size))}</strong>
        <em>${Number.isFinite(Number(plan.leverage)) ? `${fmtNumber(Number(plan.leverage))}x` : "--"}</em>
      </div>
      <div class="plan-hero-card risk">
        <span>单笔风险</span>
        <strong>${fmtPercent(Number(plan.risk_pct), 2)}</strong>
        <em>${Number.isFinite(Number(plan.initial_qty)) && Number(plan.initial_qty) > 0 ? `初始数量 ${fmtNumber(Number(plan.initial_qty))}` : "根据实时持仓计算"}</em>
      </div>
    </div>
    <div class="plan-ladder">
      <div class="plan-ladder-head">
        <span>止盈阶梯</span>
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
  const planSource = String(planContext.plan_source || "").trim().toLowerCase();
  const initialExitDisplay = planSource === "llm"
    ? "llm作为初始退出规则"
    : String(planContext.initial_exit || "--");
  const items = [
    ["单笔风险", fmtPercent(planContext.risk_per_trade_pct, 2)],
    ["最大占用", fmtPercent(planContext.max_invest_pct, 0)],
    ["最大杠杆", Number.isFinite(Number(planContext.max_leverage)) ? `${fmtNumber(Number(planContext.max_leverage))}x` : "--"],
    ["入场偏移", Number.isFinite(Number(planContext.entry_offset_atr)) ? `${fmtNumber(Number(planContext.entry_offset_atr))} ATR` : "--"],
    ["入场模式", String(planContext.entry_mode || "--")],
    ["初始退出规则", initialExitDisplay]
  ];
  return `<section class="decision-section">
    <div class="decision-section-head">
      <h3>开仓计算依据</h3>
      <span>风险参数</span>
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
      <h3>风险筛选器</h3>
      <span>${escapeHtml(actionLabel(sieve.action || "--"))}</span>
    </div>
    <div class="sieve-rule-focus ${matched ? "matched" : ""}">
      <div class="sieve-rule-head">
        <strong>${escapeHtml(translateSieveReason(sieve.reason_code))}</strong>
        <span>${escapeHtml(actionLabel(sieve.action || "--"))}</span>
      </div>
      <div class="sieve-rule-meta">
        <span class="flow-chip pass">仓位系数:${escapeHtml(fmtPercent(Number(sieve.size_factor), 0))}</span>
        <span class="flow-chip">最小仓位:${escapeHtml(fmtPercent(Number(sieve.min_size_factor), 0))}</span>
        <span class="flow-chip">默认仓位:${escapeHtml(fmtPercent(Number(sieve.default_size_factor), 0))}</span>
        ${matched ? `<span class="flow-chip">力学标签:${escapeHtml(matched.mechanics_tag || "--")}</span>
        <span class="flow-chip">清算置信:${escapeHtml(matched.liq_confidence || "--")}</span>
        <span class="flow-chip">拥挤方向:${matched.crowding_align === true ? "同向" : matched.crowding_align === false ? "反向" : "不限"}</span>` : ""}
      </div>
      <div class="sieve-reason-code">${escapeHtml(sieve.reason_code || "无命中原因")}</div>
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
      <h3>持仓收紧评估</h3>
      <span>${escapeHtml(stateLabel)}</span>
    </div>
    <div class="decision-fact-grid tighten-fact-grid">
      <div class="decision-fact-card">
        <span>执行状态</span>
        <strong>${escapeHtml(tighten.executed ? "已执行收紧" : "仅完成评估")}</strong>
      </div>
      <div class="decision-fact-card">
        <span>止盈同步收紧</span>
        <strong>${escapeHtml(tighten.tp_tightened ? "是" : "否")}</strong>
      </div>
      <div class="decision-fact-card">
        <span>主要阻断原因</span>
        <strong>${escapeHtml(blockedBy[0] || tighten.display_reason || "--")}</strong>
      </div>
    </div>
    <div class="tighten-score-card">
      <div class="decision-kpi-head">
        <span>收紧评分</span>
        <span class="decision-chip ${stateClass}">${escapeHtml(stateLabel)}</span>
      </div>
      <div class="decision-kpi-value">${escapeHtml(fmtConsensusValue(tighten.score))}</div>
      <div class="decision-kpi-sub">阈值 ${escapeHtml(fmtConsensusValue(tighten.score_threshold))} / 解析${tighten.score_parse_ok ? "成功" : "失败"}</div>
      <div class="decision-meter"><span class="decision-meter-fill ${stateClass}" style="width:${progress.toFixed(1)}%"></span></div>
      <div class="decision-meter-meta">达成率 ${Number.isFinite(progress) ? `${Math.round(progress)}%` : "--"}</div>
    </div>
    ${blockedBy.length > 0 ? `<div class="decision-check-row">${blockedBy.map((item) => `<span class="decision-chip fail">${escapeHtml(String(item))}</span>`).join("")}</div>` : ""}
  </section>`;
}

function renderDecisionDetail(detail, fallbackMessage, selectedDecisionAt) {
  if (!detail) {
    els.decisionDetail.innerHTML = `<div class="decision-empty">${escapeHtml(fallbackMessage || "暂无详情，请先选择一条决策记录")}</div>`;
    animateDecisionDetailEntry();
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
  const decisionAtRaw = selectedDecisionAt || detail.at || (detail.plan && detail.plan.opened_at) || "";
  const decisionAtText = formatBeijingDateTime(decisionAtRaw);
  const decisionTimeLine = decisionAtText === "--" ? "" : `<div class="decision-snapshot-time">北京时间 ${escapeHtml(decisionAtText)}</div>`;
  const reportBodyID = `decision-report-body-${snapshotID || "latest"}`;
  const decisionViewHref = sanitizeLinkHref(detail.decision_view_url, "/decision-view/");
  els.decisionDetail.innerHTML = `
    <div class="decision-card">
      <div class="decision-top">
        <div>
          <div class="decision-snapshot">快照 <span>#${snapshotID}</span></div>
          ${decisionTimeLine}
          <div class="decision-chip-row">
            <span class="decision-chip ${decisionActionClass(action)}">${escapeHtml(actionLabel(action))}</span>
            <span class="decision-chip ${tradeable ? "pass" : "fail"}">可交易 ${tradeable ? "是" : "否"}</span>
            <span class="decision-chip ${decisionBoolClass(overallPassed)}">共识 ${escapeHtml(fmtConsensusPassed(overallPassed))}</span>
          </div>
        </div>
        <a class="decision-link-btn" href="${escapeHtml(decisionViewHref)}" target="_blank" rel="noopener noreferrer">打开决策视图</a>
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
          <h3>模型决策报告</h3>
          <button type="button" class="decision-report-toggle" data-report-toggle aria-expanded="false" aria-controls="${reportBodyID}">${reportToggleLabel(true)}</button>
        </div>
        <div class="decision-report-collapsible" id="${reportBodyID}">
          <pre class="decision-report-body">${escapeHtml(reportMarkdown)}</pre>
          <div class="decision-report-footer">
            <button type="button" class="decision-report-toggle ghost" data-report-toggle aria-expanded="false" aria-controls="${reportBodyID}">${reportToggleLabel(true)}</button>
          </div>
        </div>
      </section>
    </div>
  `;
  bindDecisionReportToggle();
  animateDecisionDetailEntry();
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
  const orderedTrades = sortByDateFieldDesc(trades, "opened_at");
  state.lastPositionHistory = orderedTrades;
  renderPositionHistory(orderedTrades);
}

async function refreshDecisionHistory(snapshotID) {
  if (!state.symbol) {
    state.lastDecisionItems = [];
    state.selectedDecisionSnapshotID = 0;
    state.selectedDecisionSnapshotSymbol = "";
    state.selectedDecisionAt = "";
    renderDecisionHistoryRows([], 0);
    renderDecisionDetail(null, "暂无历史决策，请等待下一次策略评估。", "");
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
  const items = sortByDateFieldDesc(Array.isArray(data.items) ? data.items : [], "at");
  state.lastDecisionItems = items;
  state.lastHistoryItems = items;
  const preferredSnapshotID = Number(snapshotID || state.selectedDecisionSnapshotID || 0);
  const matchedSnapshot = items.find((item) => Number(item.snapshot_id || 0) === preferredSnapshotID);
  const selectedSnapshotID = matchedSnapshot
    ? preferredSnapshotID
    : Number(items[0] && items[0].snapshot_id ? items[0].snapshot_id : 0);
  const selectedSnapshotItem = items.find((item) => Number(item.snapshot_id || 0) === selectedSnapshotID) || null;
  state.selectedDecisionAt = selectedSnapshotItem && selectedSnapshotItem.at ? String(selectedSnapshotItem.at) : "";
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
    renderDecisionDetail(detailData.detail || null, detailData.message || "", state.selectedDecisionAt);
    const flow = await refreshFlow(selectedSnapshotID);
    if (requestSeq !== state.historyRequestSeq) {
      return;
    }
    await refreshKline(flow);
    return;
  }
  renderDecisionDetail(null, data.message || "暂无历史决策，请等待下一次策略评估。", "");
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
    els.status.textContent = "运行正常";
    els.status.classList.add("positive");
    addTransientClass(els.status, "status-pulse", 320);
  } catch (_err) {
    els.status.classList.remove("positive", "negative");
    els.status.textContent = "连接异常";
    els.status.classList.add("negative");
    addTransientClass(els.status, "status-pulse", 320);
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
  document.body.classList.add("dashboard-refreshing");
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
    document.body.classList.remove("dashboard-refreshing");
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
    addTransientClass(els.symbolSelect, "control-bump", 240);
    state.symbol = els.symbolSelect.value;
    state.selectedDecisionSnapshotID = 0;
    state.selectedDecisionSnapshotSymbol = state.symbol;
    await runRefreshCycle("scope");
  });

  els.intervalSelect.addEventListener("change", async () => {
    addTransientClass(els.intervalSelect, "control-bump", 240);
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
        scheduleKlineTagRelayout(false);
      }
      if (state.lastFlow) {
        scheduleFlowLayout(false);
      }
    });
  });
}

bootstrap();

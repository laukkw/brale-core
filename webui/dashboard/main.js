const dashboardBase = String(window.__DASHBOARD_BASE__ || "/dashboard").replace(/\/$/, "");
const apiBase = "/api/runtime/dashboard";
const runtimeBase = "/api/runtime";
const refreshMs = 15000;
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
  lastPositionHistory: [],
  historyRequestSeq: 0,
  refreshing: false,
  pendingRefreshMode: ""
};

const els = {
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

function renderLivePositions(cards) {
  if (!Array.isArray(cards) || cards.length === 0) {
    els.livePositionList.innerHTML = `<div class="position-empty">当前无实时持仓</div>`;
    return;
  }

  els.livePositionList.innerHTML = cards
    .map((card) => {
      const position = card.position || {};
      const pnl = card.pnl || {};
      const realizedClass = Number(pnl.realized) >= 0 ? "positive" : "negative";
      const unrealizedClass = Number(pnl.unrealized) >= 0 ? "positive" : "negative";
      return `<article class="position-card">
        <div class="position-head">
          <div class="position-symbol">${escapeHtml(card.symbol)}</div>
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
      </article>`;
    })
    .join("");
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
    els.flowMeta.innerHTML = "暂无决策流数据";
    return;
  }

  const gate = trace && trace.gate ? trace.gate : null;
  const inPosition = trace && trace.in_position ? trace.in_position : null;
  const tighten = flow && flow.tighten ? flow.tighten : null;
  const action = String(gate && gate.action ? gate.action : "").toUpperCase();
  const resultNode = nodes.find((item) => String(item && item.stage || "").toLowerCase() === "result") || null;

  function node(id, posClass, status, title, desc, detail, reason) {
    return { id, posClass, status, title, desc, detail, reason };
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

  function providerStatus(stage) {
    const values = stage && Array.isArray(stage.values) ? stage.values : [];
    return values.some((item) => String(item && item.state || "") === "block") ? "blocked" : "ok";
  }

  const graphNodes = [
    node("agent-indicator", "flow-pos-agent-indicator", "ok", "Agent/indicator", summarizeStageValues(agentIndicator && agentIndicator.values), agentIndicator && agentIndicator.values, ""),
    node("agent-structure", "flow-pos-agent-structure", "ok", "Agent/structure", summarizeStageValues(agentStructure && agentStructure.values), agentStructure && agentStructure.values, ""),
    node("agent-mechanics", "flow-pos-agent-mechanics", "ok", "Agent/mechanics", summarizeStageValues(agentMechanics && agentMechanics.values), agentMechanics && agentMechanics.values, ""),
    node("provider-indicator", "flow-pos-provider-indicator", providerStatus(providerIndicator), `${providerTitlePrefix}/indicator`, summarizeStageValues(providerIndicator && providerIndicator.values), providerIndicator && providerIndicator.values, providerStatus(providerIndicator) === "blocked" ? "provider blocked by indicator rules" : ""),
    node("provider-structure", "flow-pos-provider-structure", providerStatus(providerStructure), `${providerTitlePrefix}/structure`, summarizeStageValues(providerStructure && providerStructure.values), providerStructure && providerStructure.values, providerStatus(providerStructure) === "blocked" ? "provider blocked by structure rules" : ""),
    node("provider-mechanics", "flow-pos-provider-mechanics", providerStatus(providerMechanics), `${providerTitlePrefix}/mechanics`, summarizeStageValues(providerMechanics && providerMechanics.values), providerMechanics && providerMechanics.values, providerStatus(providerMechanics) === "blocked" ? "provider blocked by mechanics rules" : "")
  ];

  if (inPosition && inPosition.active) {
    graphNodes.push(node("inposition", "flow-pos-inposition", "ok", "InPosition", String(inPosition.side || "open"), [{ key: "active", value: "true", state: "pass" }, { key: "side", value: String(inPosition.side || "-") }], ""));
  }

  const gateStatus = gate && gate.tradeable ? "ok" : "blocked";
  const gateReason = gate && gate.reason ? String(gate.reason) : (gateStatus === "blocked" ? "gate blocked" : "");
  graphNodes.push(node("gate", "flow-pos-gate", gateStatus, "Gate", `${gate && gate.action ? gate.action : "--"}`, gate && Array.isArray(gate.rules) ? gate.rules : [], gateReason));

  const resultStatus = resultNode && String(resultNode.outcome || "").toLowerCase().includes("blocked") ? "blocked" : "ok";
  const resultReason = resultStatus === "blocked" ? String(resultNode && resultNode.outcome || "blocked") : "";
  const resultDesc = action === "TIGHTEN" && tighten ? `TIGHTEN ${tighten.triggered ? "triggered" : "blocked"}` : (resultNode ? String(resultNode.outcome || "-") : "-");
  graphNodes.push(node("result", "flow-pos-result", resultStatus, "Result", resultDesc, [], resultReason));

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

  const nodesHTML = graphNodes.map((item) => {
    const reasonAttr = item.reason ? ` title="${escapeHtml(item.reason)}" data-reason="${escapeHtml(item.reason)}"` : "";
    return `<article id="flow-node-${escapeHtml(item.id)}" class="flow-node ${escapeHtml(item.status)} ${escapeHtml(item.posClass)}"${reasonAttr}>
      <div class="flow-node-title">${escapeHtml(item.title)}</div>
      <div class="flow-node-desc">${escapeHtml(item.desc)}</div>
      <div class="flow-node-values">${renderFieldList(item.detail)}</div>
    </article>`;
  }).join("");

  els.flowGraph.innerHTML = `<svg class="flow-links" id="flow-links" aria-hidden="true"></svg><div class="flow-dag">${nodesHTML}</div>`;

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

    const defs = `<defs><marker id="flow-arrow-head" markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto"><path d="M0,0 L8,4 L0,8 z" fill="#3a8fd6"></path></marker></defs>`;
    const links = [];
    const rows = ["indicator", "structure", "mechanics"];
    rows.forEach((row) => {
      const a = pointOf(`agent-${row}`, "right");
      const p = pointOf(`provider-${row}`, "left");
      if (a && p) {
        links.push(`<line x1="${a.x}" y1="${a.y}" x2="${p.x}" y2="${p.y}" stroke="#3a8fd6" stroke-width="2" marker-end="url(#flow-arrow-head)" />`);
      }
      const pr = pointOf(`provider-${row}`, "right");
      const gLeft = pointOf("gate", "left");
      if (pr && gLeft) {
        links.push(`<line x1="${pr.x}" y1="${pr.y}" x2="${gLeft.x}" y2="${gLeft.y}" stroke="#3a8fd6" stroke-width="2" marker-end="url(#flow-arrow-head)" />`);
      }
    });
    const ip = pointOf("inposition", "right");
    const gTop = pointOf("gate", "left");
    if (ip && gTop) {
      links.push(`<line x1="${ip.x}" y1="${ip.y}" x2="${gTop.x}" y2="${gTop.y}" stroke="#3a8fd6" stroke-width="2" marker-end="url(#flow-arrow-head)" />`);
    }
    const g = pointOf("gate", "right");
    const r = pointOf("result", "left");
    if (g && r) {
      links.push(`<line x1="${g.x}" y1="${g.y}" x2="${r.x}" y2="${r.y}" stroke="#3a8fd6" stroke-width="2" marker-end="url(#flow-arrow-head)" />`);
    }

    svg.setAttribute("viewBox", `0 0 ${Math.max(1, Math.round(graphRect.width))} ${Math.max(1, Math.round(graphRect.height))}`);
    svg.innerHTML = `${defs}${links.join("")}`;
  }
  drawFlowLinks();

  const anchor = flow && flow.anchor ? flow.anchor : {};
  const intervalText = Array.isArray(flow && flow.intervals) && flow.intervals.length > 0 ? flow.intervals.join(" / ") : "--";
  els.flowMeta.innerHTML = [
    `<div class="flow-headline"><strong>${escapeHtml(state.symbol || "--")}</strong> latest decision flow</div>`,
    `<div class="flow-brief"><strong>Anchor:</strong> ${escapeHtml(String(anchor.type || "--"))} #${Number(anchor.snapshot_id || 0)} | <strong>Intervals:</strong> ${escapeHtml(intervalText)} | <strong>Gate:</strong> ${escapeHtml(gate && gate.action ? gate.action : "--")}</div>`
  ].join("");
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
    const minGap = 28;
    for (let i = 1; i < tagItems.length; i += 1) {
      if (tagItems[i].pixel-tagItems[i - 1].pixel < minGap) {
        tagItems[i].pixel = tagItems[i - 1].pixel + minGap;
      }
    }
    if (tagItems.length > 0 && tagItems[tagItems.length - 1].pixel > maxY) {
      const overflow = tagItems[tagItems.length - 1].pixel - maxY;
      for (let i = 0; i < tagItems.length; i += 1) {
        tagItems[i].pixel = Math.max(minY, tagItems[i].pixel - overflow);
      }
    }

    const tags = tagItems.map((item) => `<div class="price-tag" style="top:${item.pixel}px;background:${escapeHtml(item.color)}">${escapeHtml(item.name)} ${item.y.toFixed(2)}</div>`);
    els.klinePriceTags.innerHTML = tags.join("");
  }

  c.setOption({
    backgroundColor: "transparent",
    grid: { left: 45, right: 168, top: 28, bottom: 30 },
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
      axisLabel: {
        color: "#9caec6",
        formatter(value) {
          return new Date(Number(value)).toLocaleTimeString("zh-CN", { hour12: false, hour: "2-digit", minute: "2-digit" });
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
    els.decisionHistoryBody.innerHTML = `<tr><td colspan="4">暂无历史决策</td></tr>`;
    return;
  }
  els.decisionHistoryBody.innerHTML = items
    .map((item) => {
      const active = Number(item.snapshot_id) === Number(selectedSnapshotID) ? "active" : "";
      return `<tr class="${active}" data-snapshot-id="${Number(item.snapshot_id || 0)}">
        <td>${escapeHtml(item.at || "--")}</td>
        <td>${escapeHtml(item.action || "--")}</td>
        <td>${escapeHtml(item.reason || "--")}</td>
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

function renderDecisionDetail(detail, fallbackMessage) {
  if (!detail) {
    els.decisionDetail.innerHTML = `<div>${escapeHtml(fallbackMessage || "暂无详情")}</div>`;
    return;
  }
  const reportMarkdown = cleanDecisionMarkdown(detail.report_markdown || "");
  els.decisionDetail.innerHTML = [
    `<div><strong>Snapshot:</strong> ${Number(detail.snapshot_id || 0)}</div>`,
    `<div><strong>Action:</strong> ${escapeHtml(detail.action || "--")}</div>`,
    `<div><strong>Reason:</strong> ${escapeHtml(detail.reason || "--")}</div>`,
    `<div><strong>Tradeable:</strong> ${detail.tradeable ? "YES" : "NO"}</div>`,
    `<div><strong>Decision Link:</strong> <a href="${escapeHtml(detail.decision_view_url || "/decision-view/")}" target="_blank" rel="noopener noreferrer">Open</a></div>`,
    `<pre>${escapeHtml(reportMarkdown)}</pre>`
  ].join("");
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

async function refreshFlow() {
  if (!state.symbol) {
    renderFlow(null);
    return null;
  }
  const data = await fetchJSON(`${apiBase}/decision_flow`, { symbol: state.symbol });
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
    renderDecisionHistoryRows([], 0);
    renderDecisionDetail(null, "暂无历史决策");
    return;
  }
  const requestSeq = ++state.historyRequestSeq;
  const data = await fetchJSON(`${apiBase}/decision_history`, {
    symbol: state.symbol,
    limit: 20,
    snapshot_id: snapshotID || undefined
  });
  if (requestSeq !== state.historyRequestSeq) {
    return;
  }
  const items = Array.isArray(data.items) ? data.items : [];
  state.lastDecisionItems = items;
  state.lastHistoryItems = items;
  const selectedSnapshotID = snapshotID ? Number(snapshotID) : Number(items[0] && items[0].snapshot_id ? items[0].snapshot_id : 0);
  renderDecisionHistoryRows(items, selectedSnapshotID);
  if (data.detail) {
    renderDecisionDetail(data.detail, data.message || "");
    return;
  }
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
    return;
  }
  renderDecisionDetail(null, data.message || "暂无历史决策");
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
  const flow = await refreshFlow();
  await Promise.all([refreshKline(flow), refreshDecisionHistory(), refreshPositionHistory()]);
}

async function runRefreshCycle(mode) {
  const cycleMode = mode === "scope" ? "scope" : "full";
  if (state.refreshing) {
    if (cycleMode === "full" || state.pendingRefreshMode !== "full") {
      state.pendingRefreshMode = cycleMode;
    }
    return;
  }
  state.refreshing = true;
  try {
    if (cycleMode === "full") {
      await refreshOverview();
    }
    await refreshSymbolScope();
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
  updateClock();
  setInterval(updateClock, 1000);

  els.symbolSelect.addEventListener("change", async () => {
    state.symbol = els.symbolSelect.value;
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
    await runRefreshCycle("scope");
  });

  await runRefreshCycle("full");

  setInterval(async () => {
    await runRefreshCycle("full");
  }, refreshMs);

  window.addEventListener("resize", () => {
    if (chart) {
      chart.resize();
    }
    if (state.lastFlow) {
      renderFlow(state.lastFlow);
    }
  });
}

bootstrap();

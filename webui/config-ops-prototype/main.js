const serviceBraleEl = document.getElementById("service-brale");
const serviceFreqtradeEl = document.getElementById("service-freqtrade");
const runningSymbolsEl = document.getElementById("running-symbols");
const opsLogEl = document.getElementById("ops-log");

const opSoftStopBtn = document.getElementById("op-soft-stop");
const opMakeStopBtn = document.getElementById("op-make-stop");
const opMakeStartBtn = document.getElementById("op-make-start");
const jumpSystemBtn = document.getElementById("jump-system-btn");

const symbolTabsEl = document.getElementById("symbol-tabs");
const symbolFieldsEl = document.getElementById("symbol-fields");
const strategyFieldsEl = document.getElementById("strategy-fields");
const systemFieldsEl = document.getElementById("system-fields");

const submitSymbolBtn = document.getElementById("submit-symbol-btn");
const submitStrategyBtn = document.getElementById("submit-strategy-btn");
const submitSystemBtn = document.getElementById("submit-system-btn");

const cmpSymbolBeforeEl = document.getElementById("cmp-symbol-before");
const cmpSymbolAfterEl = document.getElementById("cmp-symbol-after");
const cmpStrategyBeforeEl = document.getElementById("cmp-strategy-before");
const cmpStrategyAfterEl = document.getElementById("cmp-strategy-after");
const cmpSystemBeforeEl = document.getElementById("cmp-system-before");
const cmpSystemAfterEl = document.getElementById("cmp-system-after");

const helpPanelEl = document.getElementById("help-panel");
const helpTitleEl = document.getElementById("help-title");
const helpContentEl = document.getElementById("help-content");
const helpCloseEl = document.getElementById("help-close");
const helpScrimEl = document.getElementById("help-scrim");

const confirmModalEl = document.getElementById("confirm-modal");
const confirmTitleEl = document.getElementById("confirm-title");
const confirmDescEl = document.getElementById("confirm-desc");
const confirmDiffsEl = document.getElementById("confirm-diffs");
const confirmCancelTopEl = document.getElementById("confirm-cancel-top");
const confirmCancelBtnEl = document.getElementById("confirm-cancel-btn");
const confirmApplyBtnEl = document.getElementById("confirm-apply-btn");

const toastEl = document.getElementById("toast");

const HELP_HOLD_DELAY = 220;

const HELP_TEXT = {
  log_format: "日志格式，可选 text/json。",
  log_level: "日志级别，常见值 debug/info/warn/error。",
  log_path: "日志文件写入路径。",
  db_path: "SQLite 数据库路径。",
  persist_mode: "持久化模式，minimal 更轻量，full 更完整。",
  execution_system: "执行系统标识，当前通常是 freqtrade。",
  exec_endpoint: "执行系统 API 地址。",
  exec_api_key: "执行系统 API Key。",
  exec_api_secret: "执行系统 API Secret。",
  exec_auth: "执行鉴权方式，如 basic/header。",
  enable_scheduled_decision: "是否启用定时决策循环。",
  llm_min_interval: "LLM 最小请求间隔，防止请求过密。",
  intervals: "该币种使用的 K 线周期数组。",
  kline_limit: "单周期拉取的 K 线数量。",
  risk_per_trade_pct: "单笔风险占比。",
  max_invest_pct: "单笔最大资金占比。",
  max_leverage: "最大杠杆。",
  entry_mode: "入场模式：orderbook / atr_offset / market。",
  policy: "初始止损止盈策略。",
};

const STRATEGY_SIEVE_ROWS = [
  { mechanics_tag: "fuel_ready", liq_confidence: "high", crowding_align: false, gate_action: "ALLOW", size_factor: 1.0, reason_code: "FUEL_HIGH" },
  { mechanics_tag: "fuel_ready", liq_confidence: "low", crowding_align: false, gate_action: "ALLOW", size_factor: 0.7, reason_code: "FUEL_LOW" },
  { mechanics_tag: "fuel_ready", liq_confidence: "high", crowding_align: true, gate_action: "ALLOW", size_factor: 0.7, reason_code: "FUEL_HIGH_ALIGN" },
  { mechanics_tag: "fuel_ready", liq_confidence: "low", crowding_align: true, gate_action: "ALLOW", size_factor: 0.7, reason_code: "FUEL_LOW_ALIGN" },
  { mechanics_tag: "neutral", liq_confidence: "high", crowding_align: false, gate_action: "ALLOW", size_factor: 0.7, reason_code: "NEUTRAL_HIGH" },
  { mechanics_tag: "neutral", liq_confidence: "low", crowding_align: false, gate_action: "ALLOW", size_factor: 0.5, reason_code: "NEUTRAL_LOW" },
  { mechanics_tag: "neutral", liq_confidence: "high", crowding_align: true, gate_action: "ALLOW", size_factor: 0.7, reason_code: "NEUTRAL_HIGH_ALIGN" },
  { mechanics_tag: "neutral", liq_confidence: "low", crowding_align: true, gate_action: "ALLOW", size_factor: 0.5, reason_code: "NEUTRAL_LOW_ALIGN" },
  { mechanics_tag: "crowded_long", liq_confidence: "high", crowding_align: true, gate_action: "ALLOW", size_factor: 0.4, reason_code: "CROWD_ALIGN_HIGH" },
  { mechanics_tag: "crowded_long", liq_confidence: "low", crowding_align: true, gate_action: "WAIT", size_factor: 0.0, reason_code: "CROWD_ALIGN_LOW_BLOCK" },
  { mechanics_tag: "crowded_short", liq_confidence: "high", crowding_align: true, gate_action: "ALLOW", size_factor: 0.4, reason_code: "CROWD_ALIGN_HIGH" },
  { mechanics_tag: "crowded_short", liq_confidence: "low", crowding_align: true, gate_action: "WAIT", size_factor: 0.0, reason_code: "CROWD_ALIGN_LOW_BLOCK" },
  { mechanics_tag: "crowded_long", liq_confidence: "high", crowding_align: false, gate_action: "ALLOW", size_factor: 0.3, reason_code: "CROWD_COUNTER_HIGH" },
  { mechanics_tag: "crowded_long", liq_confidence: "low", crowding_align: false, gate_action: "ALLOW", size_factor: 0.5, reason_code: "CROWD_COUNTER_LOW" },
  { mechanics_tag: "crowded_short", liq_confidence: "high", crowding_align: false, gate_action: "ALLOW", size_factor: 0.3, reason_code: "CROWD_COUNTER_HIGH" },
  { mechanics_tag: "crowded_short", liq_confidence: "low", crowding_align: false, gate_action: "ALLOW", size_factor: 0.5, reason_code: "CROWD_COUNTER_LOW" },
  { mechanics_tag: "liquidation_cascade", liq_confidence: "high", crowding_align: false, gate_action: "VETO", size_factor: 0.0, reason_code: "LIQ_CASCADE" },
  { mechanics_tag: "liquidation_cascade", liq_confidence: "low", crowding_align: false, gate_action: "VETO", size_factor: 0.0, reason_code: "LIQ_CASCADE" },
  { mechanics_tag: "liquidation_cascade", liq_confidence: "high", crowding_align: true, gate_action: "VETO", size_factor: 0.0, reason_code: "LIQ_CASCADE" },
  { mechanics_tag: "liquidation_cascade", liq_confidence: "low", crowding_align: true, gate_action: "VETO", size_factor: 0.0, reason_code: "LIQ_CASCADE" },
];

function deepClone(value) {
  return JSON.parse(JSON.stringify(value));
}

function baseSystemConfig() {
  return {
    log_format: "text",
    log_level: "info",
    log_path: "./data/brale-core.log",
    db_path: "./data/brale-core.db",
    persist_mode: "minimal",
    execution_system: "freqtrade",
    exec_endpoint: "http://127.0.0.1:8080/api/v1",
    exec_api_key: "${EXEC_USERNAME}",
    exec_api_secret: "${EXEC_SECRET}",
    exec_auth: "basic",
    enable_scheduled_decision: true,
    llm_min_interval: "5s",
    llm_models: {
      "${LLM_MODEL_INDICATOR}": { endpoint: "${LLM_INDICATOR_ENDPOINT}", api_key: "${LLM_INDICATOR_API_KEY}", timeout_sec: 300, concurrency: 1 },
      "${LLM_MODEL_STRUCTURE}": { endpoint: "${LLM_STRUCTURE_ENDPOINT}", api_key: "${LLM_STRUCTURE_API_KEY}", timeout_sec: 300, concurrency: 1 },
      "${LLM_MODEL_MECHANICS}": { endpoint: "${LLM_MECHANICS_ENDPOINT}", api_key: "${LLM_MECHANICS_API_KEY}", timeout_sec: 300, concurrency: 1 },
    },
    webhook: {
      enabled: true,
      addr: ":9991",
      queue_size: 1024,
      worker_count: 4,
      fallback_order_poll_sec: 120,
      fallback_reconcile_sec: 300,
    },
    notification: {
      enabled: true,
      startup_notify_enabled: "${NOTIFICATION_STARTUP_NOTIFY_ENABLED}",
      telegram: {
        enabled: "${NOTIFICATION_TELEGRAM_ENABLED}",
        token: "${NOTIFICATION_TELEGRAM_TOKEN}",
        chat_id: "${NOTIFICATION_TELEGRAM_CHAT_ID}",
      },
      feishu: {
        enabled: "${NOTIFICATION_FEISHU_ENABLED}",
        app_id: "${NOTIFICATION_FEISHU_APP_ID}",
        app_secret: "${NOTIFICATION_FEISHU_APP_SECRET}",
        bot_enabled: "${NOTIFICATION_FEISHU_BOT_ENABLED}",
        bot_mode: "${NOTIFICATION_FEISHU_BOT_MODE}",
        verification_token: "${NOTIFICATION_FEISHU_VERIFICATION_TOKEN}",
        encrypt_key: "${NOTIFICATION_FEISHU_ENCRYPT_KEY}",
        default_receive_id_type: "${NOTIFICATION_FEISHU_DEFAULT_RECEIVE_ID_TYPE}",
        default_receive_id: "${NOTIFICATION_FEISHU_DEFAULT_RECEIVE_ID}",
      },
    },
  };
}

function baseSymbolConfig(symbol) {
  return {
    symbol,
    intervals: ["1h", "4h", "1d"],
    kline_limit: 300,
    agent: { indicator: true, structure: true, mechanics: true },
    require: { oi: true, funding: true, long_short: true, fear_greed: true, liquidations: false },
    indicators: {
      ema_fast: 21,
      ema_mid: 50,
      ema_slow: 200,
      rsi_period: 14,
      atr_period: 14,
      macd_fast: 12,
      macd_slow: 26,
      macd_signal: 9,
      last_n: 5,
      pretty: false,
    },
    consensus: { score_threshold: 0.35, confidence_threshold: 0.52 },
    cooldown: { enabled: false, entry_cooldown_sec: 120 },
    llm: {
      agent: {
        indicator: { model: "${LLM_MODEL_INDICATOR}", temperature: 0.2 },
        structure: { model: "${LLM_MODEL_STRUCTURE}", temperature: 0.2 },
        mechanics: { model: "${LLM_MODEL_MECHANICS}", temperature: 0.2 },
      },
      provider: {
        indicator: { model: "${LLM_MODEL_INDICATOR}", temperature: 0.2 },
        structure: { model: "${LLM_MODEL_STRUCTURE}", temperature: 0.2 },
        mechanics: { model: "${LLM_MODEL_MECHANICS}", temperature: 0.2 },
      },
    },
  };
}

function baseStrategyConfig(symbol, id) {
  return {
    symbol,
    id,
    rule_chain: "configs/rules/default.json",
    risk_management: {
      risk_per_trade_pct: 0.01,
      max_invest_pct: 0.5,
      max_leverage: 10,
      grade_3_factor: 1,
      grade_2_factor: 0.7,
      grade_1_factor: 0.4,
      entry_mode: "orderbook",
      orderbook_depth: 5,
      entry_offset_atr: 0.1,
      breakeven_fee_pct: 0,
      initial_exit: {
        policy: "atr_structure_v1",
        structure_interval: "1h",
        params: {
          stop_atr_multiplier: 2,
          stop_min_distance_pct: 0.008,
          take_profit_rr: [1, 2],
          take_profit_ratios: [0.6, 0.4],
        },
      },
      slippage_buffer_pct: 0.0005,
      tighten_atr: {
        structure_threatened: 1.2,
        tp1_atr: 0.8,
        tp2_atr: 1.5,
        min_tp_distance_pct: 0.006,
        min_tp_gap_pct: 0.005,
        min_update_interval_sec: 600,
      },
      sieve: {
        min_size_factor: 0.35,
        default_gate_action: "ALLOW",
        default_size_factor: 1,
        rows: deepClone(STRATEGY_SIEVE_ROWS),
      },
    },
  };
}

const state = {
  selectedSymbol: "ETHUSDT",
  services: {
    current: { brale: "running", freqtrade: "running" },
    planned: { brale: "running", freqtrade: "running" },
  },
  system: {
    before: baseSystemConfig(),
    after: baseSystemConfig(),
  },
  symbols: {
    before: {
      ETHUSDT: {
        symbol: baseSymbolConfig("ETHUSDT"),
        strategy: baseStrategyConfig("ETHUSDT", "eth-breakout-1"),
      },
      SOLUSDT: {
        symbol: baseSymbolConfig("SOLUSDT"),
        strategy: baseStrategyConfig("SOLUSDT", "sol-breakout-1"),
      },
    },
    after: {},
  },
  logs: ["[ready] 新版运维布局已加载。"],
  openHelpButton: null,
};

state.symbols.after = deepClone(state.symbols.before);

let confirmAction = null;
let holdTimer = null;
let toastTimer = null;

function nowText() {
  return new Date().toLocaleString("zh-CN", { hour12: false });
}

function pushLog(message) {
  state.logs.push(`[${nowText()}] ${message}`);
  if (state.logs.length > 180) {
    state.logs = state.logs.slice(state.logs.length - 180);
  }
  opsLogEl.textContent = state.logs.join("\n");
}

function showToast(text) {
  if (toastTimer) {
    clearTimeout(toastTimer);
    toastTimer = null;
  }
  toastEl.textContent = text;
  toastEl.classList.add("show");
  toastTimer = setTimeout(() => {
    toastEl.classList.remove("show");
  }, 1700);
}

function symbolNames(source) {
  return Object.keys(source).sort();
}

function ensureSelectedSymbol() {
  const symbols = symbolNames(state.symbols.after);
  if (!symbols.length) {
    state.selectedSymbol = "";
    return;
  }
  if (!symbols.includes(state.selectedSymbol)) {
    state.selectedSymbol = symbols[0];
  }
}

function parsePath(path) {
  const parts = [];
  const regex = /([^[.\]]+)|(\[(\d+)\])/g;
  let matched = regex.exec(path);
  while (matched !== null) {
    if (matched[1]) {
      parts.push(matched[1]);
    } else if (matched[3]) {
      parts.push(Number.parseInt(matched[3], 10));
    }
    matched = regex.exec(path);
  }
  return parts;
}

function getByPath(obj, path) {
  const parts = parsePath(path);
  let cur = obj;
  for (let i = 0; i < parts.length; i += 1) {
    if (cur == null) {
      return undefined;
    }
    cur = cur[parts[i]];
  }
  return cur;
}

function setByPath(obj, path, value) {
  const parts = parsePath(path);
  if (!parts.length) {
    return;
  }
  let cur = obj;
  for (let i = 0; i < parts.length - 1; i += 1) {
    cur = cur[parts[i]];
    if (cur == null) {
      return;
    }
  }
  cur[parts[parts.length - 1]] = value;
}

function flattenLeaves(obj, prefix = "") {
  if (obj === null || obj === undefined) {
    return [];
  }
  if (typeof obj !== "object") {
    return [{ path: prefix, value: obj }];
  }
  if (Array.isArray(obj)) {
    const out = [];
    obj.forEach((item, idx) => {
      out.push(...flattenLeaves(item, `${prefix}[${idx}]`));
    });
    return out;
  }
  const out = [];
  Object.keys(obj).forEach((key) => {
    const nextPrefix = prefix ? `${prefix}.${key}` : key;
    out.push(...flattenLeaves(obj[key], nextPrefix));
  });
  return out;
}

function parseInput(raw, sample) {
  const text = String(raw);
  if (typeof sample === "boolean") {
    return text === "true";
  }
  if (typeof sample === "number") {
    const n = Number(text);
    return Number.isFinite(n) ? n : sample;
  }
  return text;
}

function helpText(path, value) {
  const exact = HELP_TEXT[path] || HELP_TEXT[path.split(".").slice(-1)[0]];
  if (exact) {
    return exact;
  }
  if (path.includes("notification")) {
    return "通知类字段会影响机器人启动告警和消息推送。";
  }
  if (path.includes("llm")) {
    return "LLM 相关字段用于模型路由、接口地址和推理参数。";
  }
  return `字段 ${path} 当前值为 ${String(value)}。此字段会定点覆盖回目标文件。`;
}

function anchorHelpPanel(button) {
  const rect = button.getBoundingClientRect();
  const panelRect = helpPanelEl.getBoundingClientRect();
  const panelWidth = Math.min(Math.max(panelRect.width || 360, 300), window.innerWidth - 12);
  const panelHeight = Math.min(Math.max(panelRect.height || 120, 120), window.innerHeight - 12);

  let left = rect.left + rect.width / 2 - panelWidth / 2;
  left = Math.min(Math.max(6, left), window.innerWidth - panelWidth - 6);

  let top = rect.top - panelHeight - 10;
  if (top < 6) {
    top = rect.bottom + 10;
  }
  top = Math.min(Math.max(6, top), window.innerHeight - panelHeight - 6);

  helpPanelEl.style.left = `${left}px`;
  helpPanelEl.style.top = `${top}px`;
  helpPanelEl.style.width = `${panelWidth}px`;
}

function showHelp(button, key, value) {
  state.openHelpButton = button;
  helpTitleEl.textContent = key;
  helpContentEl.textContent = helpText(key, value);
  helpScrimEl.classList.add("active");
  helpPanelEl.classList.add("active");
  anchorHelpPanel(button);
}

function hideHelp() {
  state.openHelpButton = null;
  helpScrimEl.classList.remove("active");
  helpPanelEl.classList.remove("active");
}

function bindHelpHold(helpBtn, path, getValue) {
  let shown = false;

  const clearTimer = () => {
    if (holdTimer) {
      clearTimeout(holdTimer);
      holdTimer = null;
    }
  };

  const onPressStart = (event) => {
    event.stopPropagation();
    if (Number.isFinite(Number(event.button)) && Number(event.button) !== 0) {
      return;
    }
    clearTimer();
    holdTimer = setTimeout(() => {
      shown = true;
      showHelp(helpBtn, path, getValue());
    }, HELP_HOLD_DELAY);
  };

  const onPressEnd = (event) => {
    event.stopPropagation();
    clearTimer();
    if (shown) {
      hideHelp();
      shown = false;
    }
  };

  helpBtn.addEventListener("pointerdown", onPressStart);
  helpBtn.addEventListener("pointerup", onPressEnd);
  helpBtn.addEventListener("pointerleave", onPressEnd);
  helpBtn.addEventListener("pointercancel", onPressEnd);
  helpBtn.addEventListener("contextmenu", (event) => event.preventDefault());

  helpBtn.addEventListener("keydown", (event) => {
    if (event.key !== "Enter" && event.key !== " ") {
      return;
    }
    onPressStart(event);
  });
  helpBtn.addEventListener("keyup", (event) => {
    if (event.key !== "Enter" && event.key !== " ") {
      return;
    }
    onPressEnd(event);
  });
}

function createFieldRow(path, value, onValueChange) {
  const row = document.createElement("div");
  row.className = "field-row";

  const label = document.createElement("div");
  label.className = "field-label";

  const titleWrap = document.createElement("div");
  titleWrap.className = "field-title";

  const title = document.createElement("span");
  title.textContent = path;
  titleWrap.appendChild(title);

  const helpBtn = document.createElement("button");
  helpBtn.type = "button";
  helpBtn.className = "help-btn";
  helpBtn.textContent = "?";
  titleWrap.appendChild(helpBtn);

  label.appendChild(titleWrap);
  row.appendChild(label);

  let input;
  if (typeof value === "boolean") {
    input = document.createElement("select");
    input.innerHTML = "<option value=\"true\">true</option><option value=\"false\">false</option>";
    input.value = value ? "true" : "false";
  } else if (typeof value === "number") {
    input = document.createElement("input");
    input.type = "number";
    input.step = "any";
    input.value = String(value);
  } else {
    input = document.createElement("input");
    input.type = "text";
    input.value = String(value);
  }
  input.className = "field-input";

  const applyValue = () => {
    const parsed = parseInput(input.value, value);
    onValueChange(parsed);
  };

  input.addEventListener("change", applyValue);
  input.addEventListener("blur", applyValue);

  bindHelpHold(helpBtn, path, () => value);

  row.appendChild(input);
  return row;
}

function renderFieldList(container, obj, onPathValue) {
  container.innerHTML = "";
  const leaves = flattenLeaves(obj);
  leaves.forEach((entry) => {
    const row = createFieldRow(entry.path, entry.value, (newValue) => {
      onPathValue(entry.path, newValue);
    });
    container.appendChild(row);
  });
}

function formatTomlValue(value) {
  if (typeof value === "string") {
    return `"${value.replaceAll('"', '\\"')}"`;
  }
  if (typeof value === "boolean") {
    return value ? "true" : "false";
  }
  if (typeof value === "number") {
    return String(value);
  }
  if (Array.isArray(value)) {
    if (value.every((item) => typeof item !== "object" || item === null)) {
      return `[${value.map((item) => formatTomlValue(item)).join(", ")}]`;
    }
  }
  return "";
}

function renderToml(obj) {
  const lines = [];
  const renderNode = (node, sectionPath) => {
    const scalars = [];
    const sections = [];
    const arrays = [];

    Object.keys(node).forEach((key) => {
      const value = node[key];
      if (value && typeof value === "object" && !Array.isArray(value)) {
        sections.push([key, value]);
        return;
      }
      if (Array.isArray(value) && value.some((item) => item && typeof item === "object")) {
        arrays.push([key, value]);
        return;
      }
      scalars.push([key, value]);
    });

    scalars.forEach(([key, value]) => {
      lines.push(`${key} = ${formatTomlValue(value)}`);
    });

    sections.forEach(([key, value]) => {
      lines.push("");
      const fullPath = sectionPath ? `${sectionPath}.${key}` : key;
      lines.push(`[${fullPath}]`);
      renderNode(value, fullPath);
    });

    arrays.forEach(([key, value]) => {
      const fullPath = sectionPath ? `${sectionPath}.${key}` : key;
      value.forEach((item) => {
        lines.push("");
        lines.push(`[[${fullPath}]]`);
        renderNode(item, fullPath);
      });
    });
  };

  renderNode(obj, "");
  return lines.join("\n").replace(/\n{3,}/g, "\n\n");
}

function fileSnapshot(path, beforeText, afterText) {
  return {
    path,
    before: beforeText,
    after: afterText,
  };
}

function symbolFileSnapshot(symbol) {
  const beforeBundle = state.symbols.before[symbol];
  const afterBundle = state.symbols.after[symbol];
  return fileSnapshot(
    `configs/symbols/${symbol}.toml`,
    beforeBundle ? renderToml(beforeBundle.symbol) : "",
    afterBundle ? renderToml(afterBundle.symbol) : "",
  );
}

function strategyFileSnapshot(symbol) {
  const beforeBundle = state.symbols.before[symbol];
  const afterBundle = state.symbols.after[symbol];
  return fileSnapshot(
    `configs/strategies/${symbol}.toml`,
    beforeBundle ? renderToml(beforeBundle.strategy) : "",
    afterBundle ? renderToml(afterBundle.strategy) : "",
  );
}

function systemFileSnapshot() {
  return fileSnapshot("configs/system.toml", renderToml(state.system.before), renderToml(state.system.after));
}

function diffType(file) {
  const before = (file.before || "").trim();
  const after = (file.after || "").trim();
  if (!before && after) {
    return "added";
  }
  if (before && !after) {
    return "removed";
  }
  return "modified";
}

function createDiffCard(file) {
  const card = document.createElement("article");
  card.className = "file-diff-card";

  const head = document.createElement("div");
  head.className = "file-diff-head";
  const title = document.createElement("strong");
  title.textContent = file.path;
  head.appendChild(title);
  const badge = document.createElement("span");
  const kind = diffType(file);
  badge.className = `file-badge ${kind}`;
  badge.textContent = kind === "added" ? "新增" : kind === "removed" ? "删除" : "修改";
  head.appendChild(badge);
  card.appendChild(head);

  const body = document.createElement("div");
  body.className = "file-diff-body";

  const beforePane = document.createElement("div");
  beforePane.className = "file-pane";
  beforePane.innerHTML = "<h4>旧</h4>";
  const beforePre = document.createElement("pre");
  beforePre.textContent = file.before || "(new file)";
  beforePane.appendChild(beforePre);

  const afterPane = document.createElement("div");
  afterPane.className = "file-pane";
  afterPane.innerHTML = "<h4>新</h4>";
  const afterPre = document.createElement("pre");
  afterPre.textContent = file.after || "(removed file)";
  afterPane.appendChild(afterPre);

  body.appendChild(beforePane);
  body.appendChild(afterPane);
  card.appendChild(body);
  return card;
}

function openConfirmModal(config) {
  const files = config.files.filter((file) => (file.before || "").trim() !== (file.after || "").trim());
  if (!files.length) {
    showToast("该页面没有可提交的变更");
    return;
  }

  confirmTitleEl.textContent = config.title;
  confirmDescEl.textContent = config.description;
  confirmDiffsEl.innerHTML = "";
  files.forEach((file) => {
    confirmDiffsEl.appendChild(createDiffCard(file));
  });
  confirmAction = config.onConfirm;
  confirmModalEl.classList.add("active");
  confirmModalEl.setAttribute("aria-hidden", "false");
}

function closeConfirmModal() {
  confirmAction = null;
  confirmModalEl.classList.remove("active");
  confirmModalEl.setAttribute("aria-hidden", "true");
}

function renderServices() {
  const applyService = (root, status) => {
    const badge = root.querySelector(".service-state");
    badge.textContent = status;
    badge.classList.toggle("running", status === "running");
    badge.classList.toggle("stopped", status !== "running");
  };

  applyService(serviceBraleEl, state.services.current.brale);
  applyService(serviceFreqtradeEl, state.services.current.freqtrade);

  runningSymbolsEl.innerHTML = "";
  symbolNames(state.symbols.after).forEach((symbol, idx) => {
    const li = document.createElement("li");
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "symbol-jump";
    btn.textContent = `${symbol}：下次执行时间 ${new Date(Date.now() + (idx + 1) * 60000).toLocaleTimeString("zh-CN", { hour12: false })}（点击跳转）`;
    btn.addEventListener("click", () => {
      state.selectedSymbol = symbol;
      renderEditors();
      renderComparePanel();
      document.getElementById("symbol-panel").scrollIntoView({ behavior: "smooth", block: "start" });
    });
    li.appendChild(btn);
    runningSymbolsEl.appendChild(li);
  });
}

function renderSymbolTabs() {
  ensureSelectedSymbol();
  symbolTabsEl.innerHTML = "";
  symbolNames(state.symbols.after).forEach((symbol) => {
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "symbol-tab";
    if (symbol === state.selectedSymbol) {
      btn.classList.add("active");
    }
    btn.textContent = symbol;
    btn.addEventListener("click", () => {
      state.selectedSymbol = symbol;
      renderEditors();
      renderComparePanel();
    });
    symbolTabsEl.appendChild(btn);
  });
}

function renderEditors() {
  renderSymbolTabs();

  const symbolBundle = state.symbols.after[state.selectedSymbol];
  if (symbolBundle) {
    renderFieldList(symbolFieldsEl, symbolBundle.symbol, (path, value) => {
      setByPath(symbolBundle.symbol, path, value);
      renderComparePanel();
    });

    renderFieldList(strategyFieldsEl, symbolBundle.strategy, (path, value) => {
      setByPath(symbolBundle.strategy, path, value);
      renderComparePanel();
    });
  } else {
    symbolFieldsEl.textContent = "暂无币种。";
    strategyFieldsEl.textContent = "暂无币种。";
  }

  renderFieldList(systemFieldsEl, state.system.after, (path, value) => {
    setByPath(state.system.after, path, value);
    renderComparePanel();
  });
}

function renderComparePanel() {
  ensureSelectedSymbol();
  const symbol = state.selectedSymbol;
  const beforeBundle = symbol ? state.symbols.before[symbol] : null;
  const afterBundle = symbol ? state.symbols.after[symbol] : null;

  cmpSymbolBeforeEl.textContent = beforeBundle ? renderToml(beforeBundle.symbol) : "(不存在)";
  cmpSymbolAfterEl.textContent = afterBundle ? renderToml(afterBundle.symbol) : "(不存在)";

  cmpStrategyBeforeEl.textContent = beforeBundle ? renderToml(beforeBundle.strategy) : "(不存在)";
  cmpStrategyAfterEl.textContent = afterBundle ? renderToml(afterBundle.strategy) : "(不存在)";

  cmpSystemBeforeEl.textContent = renderToml(state.system.before);
  cmpSystemAfterEl.textContent = renderToml(state.system.after);
}

function restartProgram(reason) {
  state.services.planned.brale = "running";
  state.services.planned.freqtrade = "running";
  state.services.current.brale = "running";
  state.services.current.freqtrade = "running";
  renderServices();
  pushLog(`${reason}：已执行 make stop && make start`);
  showToast("已执行重启流程");
}

function bindEvents() {
  jumpSystemBtn.addEventListener("click", () => {
    document.getElementById("system-panel").scrollIntoView({ behavior: "smooth", block: "start" });
  });

  submitSymbolBtn.addEventListener("click", () => {
    const symbol = state.selectedSymbol;
    if (!symbol) {
      showToast("当前没有可提交的币种");
      return;
    }
    openConfirmModal({
      title: `确认提交 symbols（${symbol}）`,
      description: "确认后覆盖 symbols 配置文件并立即重启。",
      files: [symbolFileSnapshot(symbol)],
      onConfirm: () => {
        state.symbols.before[symbol].symbol = deepClone(state.symbols.after[symbol].symbol);
        renderComparePanel();
        restartProgram(`symbols/${symbol}.toml 已覆盖`);
      },
    });
  });

  submitStrategyBtn.addEventListener("click", () => {
    const symbol = state.selectedSymbol;
    if (!symbol) {
      showToast("当前没有可提交的币种");
      return;
    }
    openConfirmModal({
      title: `确认提交 strategies（${symbol}）`,
      description: "确认后覆盖 strategies 配置文件并立即重启。",
      files: [strategyFileSnapshot(symbol)],
      onConfirm: () => {
        state.symbols.before[symbol].strategy = deepClone(state.symbols.after[symbol].strategy);
        renderComparePanel();
        restartProgram(`strategies/${symbol}.toml 已覆盖`);
      },
    });
  });

  submitSystemBtn.addEventListener("click", () => {
    openConfirmModal({
      title: "确认提交全局配置",
      description: "确认后覆盖 system.toml 并立即重启。",
      files: [systemFileSnapshot()],
      onConfirm: () => {
        state.system.before = deepClone(state.system.after);
        renderComparePanel();
        restartProgram("system.toml 已覆盖");
      },
    });
  });

  opSoftStopBtn.addEventListener("click", () => {
    state.services.planned.brale = "stopped";
    state.services.planned.freqtrade = "stopped";
    pushLog("已请求 schedule disable（软停计划已设置）");
    renderServices();
  });

  opMakeStopBtn.addEventListener("click", () => {
    state.services.current.brale = "stopped";
    state.services.current.freqtrade = "stopped";
    state.services.planned.brale = "stopped";
    state.services.planned.freqtrade = "stopped";
    pushLog("执行 make stop（brale/freqtrade 已停止）");
    renderServices();
    showToast("服务已停止");
  });

  opMakeStartBtn.addEventListener("click", () => {
    restartProgram("执行 make start");
  });

  helpCloseEl.addEventListener("click", () => {
    hideHelp();
  });
  helpScrimEl.addEventListener("pointerdown", () => {
    hideHelp();
  });

  document.addEventListener("pointerdown", (event) => {
    const target = event.target;
    if (!(target instanceof Element)) {
      return;
    }
    if (helpPanelEl.contains(target)) {
      return;
    }
    if (target.classList.contains("help-btn")) {
      return;
    }
    hideHelp();
  });

  window.addEventListener("resize", () => {
    if (state.openHelpButton) {
      anchorHelpPanel(state.openHelpButton);
    }
  });

  confirmCancelTopEl.addEventListener("click", () => {
    closeConfirmModal();
  });
  confirmCancelBtnEl.addEventListener("click", () => {
    closeConfirmModal();
  });
  confirmApplyBtnEl.addEventListener("click", () => {
    if (typeof confirmAction === "function") {
      confirmAction();
    }
    closeConfirmModal();
  });
  confirmModalEl.addEventListener("click", (event) => {
    if (event.target === confirmModalEl) {
      closeConfirmModal();
    }
  });
}

function initialize() {
  bindEvents();
  renderServices();
  renderEditors();
  renderComparePanel();
  opsLogEl.textContent = state.logs.join("\n");
}

initialize();

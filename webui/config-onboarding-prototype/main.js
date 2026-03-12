const steps = Array.from(document.querySelectorAll(".wizard-step"));
const stepItems = Array.from(document.querySelectorAll(".step"));
const tabs = Array.from(document.querySelectorAll(".tab"));
const form = document.getElementById("config-form");

const progressFill = document.getElementById("progress-fill");
const progressText = document.getElementById("progress-text");
const prevBtn = document.getElementById("prev-btn");
const nextBtn = document.getElementById("next-btn");
const previewEl = document.getElementById("preview");

const symbolTabsEl = document.getElementById("symbol-tabs");
const saveSymbolBtn = document.getElementById("save-symbol-btn");
const saveStatusEl = document.getElementById("save-status");

const telegramEnabledEl = document.getElementById("telegram_enabled");
const feishuEnabledEl = document.getElementById("feishu_enabled");
const telegramFieldsEl = document.getElementById("telegram-fields");
const feishuFieldsEl = document.getElementById("feishu-fields");

const strategyInputs = {
  risk_per_trade_pct: document.getElementById("risk_per_trade_pct"),
  max_invest_pct: document.getElementById("max_invest_pct"),
  max_leverage: document.getElementById("max_leverage"),
  entry_mode: document.getElementById("entry_mode"),
  exit_policy: document.getElementById("exit_policy"),
  tighten_min_update_interval_sec: document.getElementById("tighten_min_update_interval_sec"),
  ema_fast: document.getElementById("ema_fast"),
  ema_mid: document.getElementById("ema_mid"),
  ema_slow: document.getElementById("ema_slow"),
  rsi_period: document.getElementById("rsi_period"),
  last_n: document.getElementById("last_n"),
  macd_fast: document.getElementById("macd_fast"),
  macd_slow: document.getElementById("macd_slow"),
  macd_signal: document.getElementById("macd_signal")
};

const intervalInputs = Array.from(document.querySelectorAll('input[name="strategy_intervals"]'));

let currentStep = 0;
let currentPreviewTab = "env";
let activeSymbol = "";

const symbolSaved = {};
const symbolDrafts = {};

function getSelectedSymbols() {
  return Array.from(form.querySelectorAll('[name="symbols"]:checked')).map((el) => el.value);
}

function value(name, fallback = "") {
  const el = form.querySelector(`[name="${name}"]`);
  if (!el) return fallback;
  const text = String(el.value || "").trim();
  return text === "" ? fallback : text;
}

function checked(name) {
  const el = form.querySelector(`[name="${name}"]`);
  return Boolean(el && el.checked);
}

function toNumber(raw, fallback) {
  if (raw === "") return fallback;
  const n = Number(raw);
  return Number.isFinite(n) ? n : fallback;
}

function toInteger(raw, fallback) {
  if (raw === "") return fallback;
  const n = Number.parseInt(raw, 10);
  return Number.isFinite(n) ? n : fallback;
}

function clone(obj) {
  return JSON.parse(JSON.stringify(obj));
}

function isEqual(a, b) {
  return JSON.stringify(a) === JSON.stringify(b);
}

function defaultSymbolConfig() {
  return {
    risk_per_trade_pct: 0.5,
    max_invest_pct: 1.0,
    max_leverage: 3,
    intervals: ["1h", "4h", "1d"],
    entry_mode: "orderbook",
    exit_policy: "atr_structure_v1",
    tighten_min_update_interval_sec: 300,
    ema_fast: 21,
    ema_mid: 50,
    ema_slow: 200,
    rsi_period: 14,
    last_n: 5,
    macd_fast: 12,
    macd_slow: 26,
    macd_signal: 9
  };
}

function collectStrategyForm() {
  const intervals = intervalInputs.filter((el) => el.checked).map((el) => el.value);
  return {
    risk_per_trade_pct: toNumber(String(strategyInputs.risk_per_trade_pct.value || "").trim(), 0.5),
    max_invest_pct: toNumber(String(strategyInputs.max_invest_pct.value || "").trim(), 1.0),
    max_leverage: toInteger(String(strategyInputs.max_leverage.value || "").trim(), 3),
    intervals: intervals.length ? intervals : ["1h", "4h", "1d"],
    entry_mode: String(strategyInputs.entry_mode.value || "").trim() || "orderbook",
    exit_policy: String(strategyInputs.exit_policy.value || "").trim() || "atr_structure_v1",
    tighten_min_update_interval_sec: toInteger(String(strategyInputs.tighten_min_update_interval_sec.value || "").trim(), 300),
    ema_fast: toInteger(String(strategyInputs.ema_fast.value || "").trim(), 21),
    ema_mid: toInteger(String(strategyInputs.ema_mid.value || "").trim(), 50),
    ema_slow: toInteger(String(strategyInputs.ema_slow.value || "").trim(), 200),
    rsi_period: toInteger(String(strategyInputs.rsi_period.value || "").trim(), 14),
    last_n: toInteger(String(strategyInputs.last_n.value || "").trim(), 5),
    macd_fast: toInteger(String(strategyInputs.macd_fast.value || "").trim(), 12),
    macd_slow: toInteger(String(strategyInputs.macd_slow.value || "").trim(), 26),
    macd_signal: toInteger(String(strategyInputs.macd_signal.value || "").trim(), 9)
  };
}

function fillStrategyForm(cfg) {
  strategyInputs.risk_per_trade_pct.value = cfg?.risk_per_trade_pct ?? "";
  strategyInputs.max_invest_pct.value = cfg?.max_invest_pct ?? "";
  strategyInputs.max_leverage.value = cfg?.max_leverage ?? "";
  strategyInputs.entry_mode.value = cfg?.entry_mode ?? "";
  strategyInputs.exit_policy.value = cfg?.exit_policy ?? "";
  strategyInputs.tighten_min_update_interval_sec.value = cfg?.tighten_min_update_interval_sec ?? "";
  strategyInputs.ema_fast.value = cfg?.ema_fast ?? "";
  strategyInputs.ema_mid.value = cfg?.ema_mid ?? "";
  strategyInputs.ema_slow.value = cfg?.ema_slow ?? "";
  strategyInputs.rsi_period.value = cfg?.rsi_period ?? "";
  strategyInputs.last_n.value = cfg?.last_n ?? "";
  strategyInputs.macd_fast.value = cfg?.macd_fast ?? "";
  strategyInputs.macd_slow.value = cfg?.macd_slow ?? "";
  strategyInputs.macd_signal.value = cfg?.macd_signal ?? "";

  const intervals = Array.isArray(cfg?.intervals) ? cfg.intervals : [];
  intervalInputs.forEach((el) => {
    el.checked = intervals.includes(el.value);
  });
}

function saveCurrentSymbolDraft() {
  if (!activeSymbol) return;
  symbolDrafts[activeSymbol] = collectStrategyForm();
}

function getSymbolEffectiveConfig(symbol) {
  if (symbol === activeSymbol) {
    return collectStrategyForm();
  }
  if (symbolSaved[symbol]) return clone(symbolSaved[symbol]);
  if (symbolDrafts[symbol]) return clone(symbolDrafts[symbol]);
  return defaultSymbolConfig();
}

function updateSaveStatus() {
  if (!activeSymbol) {
    saveStatusEl.textContent = "请先在步骤 1 勾选币种";
    return;
  }
  const draft = collectStrategyForm();
  if (symbolSaved[activeSymbol] && isEqual(symbolSaved[activeSymbol], draft)) {
    saveStatusEl.textContent = `${activeSymbol} 已保存`;
    return;
  }
  if (symbolDrafts[activeSymbol]) {
    saveStatusEl.textContent = `${activeSymbol} 编辑中（未保存）`;
    return;
  }
  saveStatusEl.textContent = "未保存";
}

function renderSymbolTabs() {
  const symbols = getSelectedSymbols();
  symbolTabsEl.innerHTML = "";

  if (!symbols.length) {
    const tip = document.createElement("span");
    tip.textContent = "请先在步骤 1 勾选至少一个币种";
    symbolTabsEl.appendChild(tip);
    saveSymbolBtn.disabled = true;
    activeSymbol = "";
    fillStrategyForm(null);
    updateSaveStatus();
    return;
  }

  saveSymbolBtn.disabled = false;
  if (!symbols.includes(activeSymbol)) {
    activeSymbol = symbols[0];
  }

  symbols.forEach((symbol) => {
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "symbol-tab";
    if (symbol === activeSymbol) btn.classList.add("active");
    if (symbolSaved[symbol]) btn.classList.add("saved");
    btn.textContent = symbol;
    btn.addEventListener("click", () => {
      saveCurrentSymbolDraft();
      activeSymbol = symbol;
      const cfg = symbolDrafts[symbol] || symbolSaved[symbol] || null;
      fillStrategyForm(cfg);
      renderSymbolTabs();
      updateSaveStatus();
      renderPreview();
    });
    symbolTabsEl.appendChild(btn);
  });

  const cfg = symbolDrafts[activeSymbol] || symbolSaved[activeSymbol] || null;
  fillStrategyForm(cfg);
  updateSaveStatus();
}

function payload() {
  const symbols = getSelectedSymbols();
  const symbolDetail = {};
  symbols.forEach((symbol) => {
    symbolDetail[symbol] = getSymbolEffectiveConfig(symbol);
  });

  return {
    dry_run: true,
    max_open_trades: 10,
    symbols,
    symbol_detail: symbolDetail,

    exec_username: value("exec_username", "${EXEC_USERNAME}"),
    exec_secret: value("exec_secret", "${EXEC_SECRET}"),
    proxy_enabled: checked("proxy_enabled"),
    proxy_host: value("proxy_host", "host.docker.internal"),
    proxy_port: toInteger(value("proxy_port", ""), 7890),

    llm_model_indicator: value("llm_model_indicator", "${LLM_MODEL_INDICATOR}"),
    llm_indicator_endpoint: value("llm_indicator_endpoint", "${LLM_INDICATOR_ENDPOINT}"),
    llm_indicator_key: value("llm_indicator_key", "${LLM_INDICATOR_API_KEY}"),
    llm_model_structure: value("llm_model_structure", "${LLM_MODEL_STRUCTURE}"),
    llm_structure_endpoint: value("llm_structure_endpoint", "${LLM_STRUCTURE_ENDPOINT}"),
    llm_structure_key: value("llm_structure_key", "${LLM_STRUCTURE_API_KEY}"),
    llm_model_mechanics: value("llm_model_mechanics", "${LLM_MODEL_MECHANICS}"),
    llm_mechanics_endpoint: value("llm_mechanics_endpoint", "${LLM_MECHANICS_ENDPOINT}"),
    llm_mechanics_key: value("llm_mechanics_key", "${LLM_MECHANICS_API_KEY}"),

    telegram_enabled: checked("telegram_enabled"),
    telegram_token: value("telegram_token", ""),
    telegram_chat_id: value("telegram_chat_id", ""),
    feishu_enabled: checked("feishu_enabled"),
    feishu_app_id: value("feishu_app_id", ""),
    feishu_app_secret: value("feishu_app_secret", ""),
    feishu_bot_enabled: checked("feishu_bot_enabled"),
    feishu_bot_mode: value("feishu_bot_mode", "long_connection"),
    feishu_verification_token: value("feishu_verification_token", ""),
    feishu_encrypt_key: value("feishu_encrypt_key", ""),
    feishu_default_receive_id_type: value("feishu_default_receive_id_type", "chat_id"),
    feishu_default_receive_id: value("feishu_default_receive_id", "")
  };
}

function renderEnv(data) {
  return [
    `EXEC_USERNAME=${data.exec_username}`,
    `EXEC_SECRET=${data.exec_secret}`,
    `PROXY_ENABLED=${data.proxy_enabled}`,
    `PROXY_HOST=${data.proxy_host}`,
    `PROXY_PORT=${data.proxy_port}`,
    "",
    `LLM_MODEL_INDICATOR=${data.llm_model_indicator}`,
    `LLM_INDICATOR_ENDPOINT=${data.llm_indicator_endpoint}`,
    `LLM_INDICATOR_API_KEY=${data.llm_indicator_key}`,
    "",
    `LLM_MODEL_STRUCTURE=${data.llm_model_structure}`,
    `LLM_STRUCTURE_ENDPOINT=${data.llm_structure_endpoint}`,
    `LLM_STRUCTURE_API_KEY=${data.llm_structure_key}`,
    "",
    `LLM_MODEL_MECHANICS=${data.llm_model_mechanics}`,
    `LLM_MECHANICS_ENDPOINT=${data.llm_mechanics_endpoint}`,
    `LLM_MECHANICS_API_KEY=${data.llm_mechanics_key}`,
    "",
    `NOTIFICATION_TELEGRAM_ENABLED=${data.telegram_enabled}`,
    `NOTIFICATION_TELEGRAM_TOKEN=${data.telegram_token}`,
    `NOTIFICATION_TELEGRAM_CHAT_ID=${data.telegram_chat_id}`,
    "",
    `NOTIFICATION_FEISHU_ENABLED=${data.feishu_enabled}`,
    `NOTIFICATION_FEISHU_APP_ID=${data.feishu_app_id}`,
    `NOTIFICATION_FEISHU_APP_SECRET=${data.feishu_app_secret}`,
    `NOTIFICATION_FEISHU_BOT_ENABLED=${data.feishu_bot_enabled}`,
    `NOTIFICATION_FEISHU_BOT_MODE=${data.feishu_bot_mode}`,
    `NOTIFICATION_FEISHU_VERIFICATION_TOKEN=${data.feishu_verification_token}`,
    `NOTIFICATION_FEISHU_ENCRYPT_KEY=${data.feishu_encrypt_key}`,
    `NOTIFICATION_FEISHU_DEFAULT_RECEIVE_ID_TYPE=${data.feishu_default_receive_id_type}`,
    `NOTIFICATION_FEISHU_DEFAULT_RECEIVE_ID=${data.feishu_default_receive_id}`
  ].join("\n");
}

function renderFreqtrade(data) {
  const cfg = {
    dry_run: true,
    max_open_trades: 10,
    api_server: {
      enabled: true,
      username: data.exec_username,
      password: data.exec_secret
    },
    exchange: {
      name: "binance",
      key: "",
      secret: ""
    }
  };

  if (data.proxy_enabled) {
    const proxyUrl = `http://${data.proxy_host}:${data.proxy_port}`;
    cfg.exchange.ccxt_config = { proxies: { http: proxyUrl, https: proxyUrl } };
    cfg.exchange.ccxt_async_config = { aiohttp_proxy: proxyUrl };
  }
  return JSON.stringify(cfg, null, 2);
}

function renderSymbolConfig(data) {
  const symbols = data.symbols.length ? data.symbols : ["ETHUSDT"];
  return symbols
    .map((symbol) => {
      const cfg = data.symbol_detail[symbol] || defaultSymbolConfig();
      return [
        `# ${symbol} symbols/${symbol}.toml`,
        `symbol = "${symbol}"`,
        `intervals = [${cfg.intervals.map((v) => `"${v}"`).join(", ")}]`,
        "",
        `[indicators]`,
        `ema_fast = ${cfg.ema_fast}`,
        `ema_mid = ${cfg.ema_mid}`,
        `ema_slow = ${cfg.ema_slow}`,
        `rsi_period = ${cfg.rsi_period}`,
        `last_n = ${cfg.last_n}`,
        `macd_fast = ${cfg.macd_fast}`,
        `macd_slow = ${cfg.macd_slow}`,
        `macd_signal = ${cfg.macd_signal}`,
        "",
        `# ${symbol} strategies/${symbol}.toml`,
        `[risk_management]`,
        `risk_per_trade_pct = ${cfg.risk_per_trade_pct}`,
        `max_invest_pct = ${cfg.max_invest_pct}`,
        `max_leverage = ${cfg.max_leverage}`,
        `entry_mode = "${cfg.entry_mode}"`,
        "",
        `[risk_management.initial_exit]`,
        `policy = "${cfg.exit_policy}"`,
        "",
        `[risk_management.tighten_atr]`,
        `min_update_interval_sec = ${cfg.tighten_min_update_interval_sec}`
      ].join("\n");
    })
    .join("\n\n");
}

function renderScript(data) {
  return [
    "#!/usr/bin/env bash",
    "set -euo pipefail",
    "",
    "cp env_example .env",
    "python3 scripts/prepare_stack.py --env-file .env",
    "make start",
    "",
    "# mode: dry-run (fixed)",
    "# max_open_trades: 10 (fixed)",
    `# symbols: ${(data.symbols.length ? data.symbols : ["ETHUSDT"]).join(", ")}`
  ].join("\n");
}

function syncNotifyFields() {
  const telegramOn = telegramEnabledEl.checked;
  const feishuOn = feishuEnabledEl.checked;
  telegramFieldsEl.classList.toggle("hidden", !telegramOn);
  feishuFieldsEl.classList.toggle("hidden", !feishuOn);
}

function renderPreview() {
  const data = payload();
  syncNotifyFields();
  if (currentPreviewTab === "env") {
    previewEl.textContent = renderEnv(data);
    return;
  }
  if (currentPreviewTab === "freq") {
    previewEl.textContent = renderFreqtrade(data);
    return;
  }
  if (currentPreviewTab === "symbol") {
    previewEl.textContent = renderSymbolConfig(data);
    return;
  }
  previewEl.textContent = renderScript(data);
}

function syncStepUI() {
  steps.forEach((step, idx) => {
    step.classList.toggle("active", idx === currentStep);
  });
  stepItems.forEach((step, idx) => {
    step.classList.toggle("active", idx === currentStep);
  });

  const pct = ((currentStep + 1) / steps.length) * 100;
  progressFill.style.width = `${pct}%`;
  progressText.textContent = `步骤 ${currentStep + 1} / ${steps.length}`;

  prevBtn.disabled = currentStep === 0;
  nextBtn.textContent = currentStep === steps.length - 1 ? "回到第一步" : "下一步";

  renderPreview();
}

prevBtn.addEventListener("click", () => {
  currentStep = Math.max(0, currentStep - 1);
  syncStepUI();
});

nextBtn.addEventListener("click", () => {
  if (currentStep === steps.length - 1) {
    currentStep = 0;
  } else {
    currentStep += 1;
  }
  syncStepUI();
});

stepItems.forEach((stepEl, idx) => {
  stepEl.addEventListener("click", () => {
    currentStep = idx;
    syncStepUI();
  });
});

tabs.forEach((tab) => {
  tab.addEventListener("click", () => {
    currentPreviewTab = tab.dataset.tab || "env";
    tabs.forEach((item) => {
      item.classList.toggle("active", item === tab);
    });
    renderPreview();
  });
});

form.querySelectorAll('[name="symbols"]').forEach((el) => {
  el.addEventListener("change", () => {
    saveCurrentSymbolDraft();
    renderSymbolTabs();
    renderPreview();
  });
});

Object.values(strategyInputs).forEach((input) => {
  input.addEventListener("input", () => {
    saveCurrentSymbolDraft();
    updateSaveStatus();
    renderPreview();
  });
  input.addEventListener("change", renderPreview);
});

intervalInputs.forEach((input) => {
  input.addEventListener("change", () => {
    saveCurrentSymbolDraft();
    updateSaveStatus();
    renderPreview();
  });
});

saveSymbolBtn.addEventListener("click", () => {
  if (!activeSymbol) return;
  const cfg = collectStrategyForm();
  symbolSaved[activeSymbol] = clone(cfg);
  symbolDrafts[activeSymbol] = clone(cfg);
  renderSymbolTabs();
  updateSaveStatus();
  renderPreview();
});

telegramEnabledEl.addEventListener("change", renderPreview);
feishuEnabledEl.addEventListener("change", renderPreview);
form.addEventListener("input", renderPreview);
form.addEventListener("change", renderPreview);

renderSymbolTabs();
syncNotifyFields();
syncStepUI();

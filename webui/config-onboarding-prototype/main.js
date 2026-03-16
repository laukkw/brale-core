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
const generateBtn = document.getElementById("generate-btn");
const generateStatusEl = document.getElementById("generate-status");
const startupCheckBtn = document.getElementById("startup-check-btn");
const startupStartBtn = document.getElementById("startup-start-btn");
const startupRunStatusEl = document.getElementById("startup-run-status");
const startupDockerStatusEl = document.getElementById("startup-docker-status");
const startupDockerVersionEl = document.getElementById("startup-docker-version");
const startupComposeStatusEl = document.getElementById("startup-compose-status");
const startupComposeVersionEl = document.getElementById("startup-compose-version");
const startupConfigStatusEl = document.getElementById("startup-config-status");
const startupConfigDetailEl = document.getElementById("startup-config-detail");
const startupProgressFillEl = document.getElementById("startup-progress-fill");
const startupProgressTextEl = document.getElementById("startup-progress-text");
const startupSpeedTextEl = document.getElementById("startup-speed-text");
const startupLogEl = document.getElementById("startup-log");
const startupMonitorRefreshBtn = document.getElementById("startup-monitor-refresh-btn");
const startupMonitorUpdatedEl = document.getElementById("startup-monitor-updated");
const startupBraleStatusEl = document.getElementById("startup-brale-status");
const startupBraleLinkEl = document.getElementById("startup-brale-link");
const startupBraleOpenBtn = document.getElementById("startup-brale-open-btn");
const startupBraleStopBtn = document.getElementById("startup-brale-stop-btn");
const startupFreqtradeStatusEl = document.getElementById("startup-freqtrade-status");
const startupFreqtradeLinkEl = document.getElementById("startup-freqtrade-link");
const startupFreqtradeOpenBtn = document.getElementById("startup-freqtrade-open-btn");
const startupFreqtradeStopBtn = document.getElementById("startup-freqtrade-stop-btn");
const startupFreqtradeRefreshBtn = document.getElementById("startup-freqtrade-refresh-btn");
const startupMonitorLogEl = document.getElementById("startup-monitor-log");

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
let startupEventSource = null;
let startupMonitorPollTimer = null;
let previewRenderScheduled = false;
let previewRenderSeq = 0;
let previewScriptRequestSeq = 0;

const STARTUP_LOG_MAX_LINES = 500;
const previewScriptCache = { key: "", content: "" };
let startupLogLines = [];
let startupMonitorLogLines = [];

const startupStepIndex = stepItems.findIndex((step) => step.dataset.step === "5");
const monitorStepIndex = stepItems.findIndex((step) => step.dataset.step === "6");

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
      scheduleRenderPreview();
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
    proxy_scheme: "http",
    proxy_no_proxy: "localhost,127.0.0.1,brale,freqtrade",

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
    "# generated by onboarding",
    ".PHONY: onboarding-up",
    "",
    "onboarding-up:",
    "\t$(MAKE) onboarding-start",
    "",
    "# mode: dry-run (fixed)",
    "# max_open_trades: 10 (fixed)",
    `# symbols: ${(data.symbols.length ? data.symbols : ["ETHUSDT"]).join(", ")}`
  ].join("\n");
}

function trimLogBuffer(lines) {
  if (lines.length <= STARTUP_LOG_MAX_LINES) {
    return lines;
  }
  return lines.slice(lines.length - STARTUP_LOG_MAX_LINES);
}

function renderStartupLogs() {
  if (startupLogEl) {
    startupLogEl.textContent = startupLogLines.join("\n");
    startupLogEl.scrollTop = startupLogEl.scrollHeight;
  }
  if (startupMonitorLogEl) {
    startupMonitorLogEl.textContent = startupMonitorLogLines.join("\n");
    startupMonitorLogEl.scrollTop = startupMonitorLogEl.scrollHeight;
  }
}

function resetStartupLogs(mainSeed, monitorSeed) {
  startupLogLines = mainSeed ? [mainSeed] : [];
  startupMonitorLogLines = monitorSeed ? [monitorSeed] : [];
  renderStartupLogs();
}

function setStartupStatus(el, ok, text) {
  if (!el) return;
  el.textContent = text;
  el.classList.remove("ok", "fail");
  el.classList.add(ok ? "ok" : "fail");
}

function appendStartupLog(text) {
  startupLogLines = trimLogBuffer([...startupLogLines, text]);
  startupMonitorLogLines = trimLogBuffer([...startupMonitorLogLines, text]);
  renderStartupLogs();
}

function resetStartupProgress() {
  startupProgressFillEl.style.width = "0%";
  startupProgressTextEl.textContent = "拉取进度：0%";
  startupSpeedTextEl.textContent = "速度：-";
}

function applyStartupProgress(progress) {
  const pct = Number(progress?.percent || 0);
  const clamped = Math.max(0, Math.min(100, pct));
  startupProgressFillEl.style.width = `${clamped.toFixed(1)}%`;
  const current = progress?.current || "-";
  const total = progress?.total || "-";
  startupProgressTextEl.textContent = `拉取进度：${clamped.toFixed(1)}% (${current} / ${total})`;
  startupSpeedTextEl.textContent = `速度：${progress?.speed || "-"}`;
}

function setStartupLink(el, enabled, url, enabledText, disabledText) {
  if (!el) return;
  el.href = url;
  el.textContent = enabled ? enabledText : disabledText;
  el.classList.toggle("disabled", !enabled);
  el.setAttribute("aria-disabled", enabled ? "false" : "true");
}

function applyStartupMonitor(result) {
  const braleRunning = Boolean(result?.brale_running);
  const braleURL = String(result?.brale_url || "http://127.0.0.1:9991/dashboard/");
  const freqtradeRunning = Boolean(result?.freqtrade_running);
  const freqtradeURL = String(result?.freqtrade_url || "http://127.0.0.1:8080");

  setStartupStatus(startupBraleStatusEl, braleRunning, braleRunning ? "运行中" : "未运行");
  setStartupStatus(startupFreqtradeStatusEl, freqtradeRunning, freqtradeRunning ? "运行中" : "未运行");

  setStartupLink(
    startupBraleLinkEl,
    braleRunning,
    braleURL,
    "打开 brale-core Dashboard（9991）",
    "brale-core 未就绪，启动成功后可跳转"
  );
  setStartupLink(
    startupFreqtradeLinkEl,
    freqtradeRunning,
    freqtradeURL,
    "打开 freqtrade 后台（8080）",
    "freqtrade 未就绪，启动成功后可跳转"
  );

  if (startupMonitorUpdatedEl) {
    startupMonitorUpdatedEl.textContent = `最近刷新：${new Date().toLocaleTimeString("zh-CN", { hour12: false })}`;
  }

  if (startupBraleOpenBtn) startupBraleOpenBtn.disabled = !braleRunning;
  if (startupBraleStopBtn) startupBraleStopBtn.disabled = !braleRunning;
  if (startupFreqtradeOpenBtn) startupFreqtradeOpenBtn.disabled = !freqtradeRunning;
  if (startupFreqtradeStopBtn) startupFreqtradeStopBtn.disabled = !freqtradeRunning;
}

async function runStartupServiceAction(service, action, triggerBtn) {
  if (triggerBtn) triggerBtn.disabled = true;
  const actionText = action === "start"
    ? "启动"
    : action === "stop"
      ? "停止"
      : action === "pull-rebuild"
        ? "拉取 brale-core 代码并 Rebuild"
        : action;
  const startedAt = Date.now();
  let heartbeatTimer = null;
  startupRunStatusEl.textContent = `执行中：${service} ${actionText}...`;
  appendStartupLog(`[INFO] ${service} ${actionText} 已开始，可能需要几分钟...`);
  heartbeatTimer = setInterval(() => {
    const elapsed = Math.max(1, Math.floor((Date.now() - startedAt) / 1000));
    appendStartupLog(`[INFO] ${service} ${actionText} 执行中，已耗时 ${elapsed}s`);
  }, 15000);
  try {
    const resp = await fetch("api/startup/service-action", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ service, action })
    });
    const text = await resp.text();
    let result = null;
    try {
      result = JSON.parse(text);
    } catch {
      result = null;
    }
    if (!resp.ok) {
      const detail = result
        ? [result.error, result.output].filter((item) => Boolean(item && String(item).trim())).join("\n")
        : "";
      throw new Error(detail || text || `HTTP ${resp.status}`);
    }
    const outputText = result?.output ? String(result.output).trim() : "";
    if (outputText) {
      appendStartupLog(`[${service}] ${outputText}`);
    } else {
      appendStartupLog(`[INFO] ${service} ${actionText} 已完成（命令输出为空）`);
    }
    if (result?.monitor) {
      applyStartupMonitor(result.monitor);
    } else {
      await refreshStartupMonitor(true);
    }
    startupRunStatusEl.textContent = `${service} ${actionText}完成`;
  } catch (err) {
    startupRunStatusEl.textContent = `操作失败: ${err?.message || err}`;
    appendStartupLog(`[ERR] ${service} ${action} 失败: ${err?.message || err}`);
    await refreshStartupMonitor(true);
  } finally {
    if (heartbeatTimer) {
      clearInterval(heartbeatTimer);
    }
    if (triggerBtn) triggerBtn.disabled = false;
  }
}

async function refreshStartupMonitor(silent = false) {
  if (startupMonitorRefreshBtn && !silent) {
    startupMonitorRefreshBtn.disabled = true;
  }
  try {
    const resp = await fetch("api/startup/monitor");
    const text = await resp.text();
    if (!resp.ok) {
      throw new Error(text || `HTTP ${resp.status}`);
    }
    const result = JSON.parse(text);
    applyStartupMonitor(result);
  } catch (err) {
    if (!silent) {
      startupRunStatusEl.textContent = `监控刷新失败: ${err?.message || err}`;
    }
  } finally {
    if (startupMonitorRefreshBtn) {
      startupMonitorRefreshBtn.disabled = false;
    }
  }
}

function stopStartupMonitorPoll() {
  if (startupMonitorPollTimer) {
    clearInterval(startupMonitorPollTimer);
    startupMonitorPollTimer = null;
  }
}

function startStartupMonitorPoll() {
  stopStartupMonitorPoll();
  startupMonitorPollTimer = setInterval(() => {
    void refreshStartupMonitor(true);
  }, 5000);
}

async function checkStartup() {
  startupCheckBtn.disabled = true;
  startupRunStatusEl.textContent = "检测中...";
  try {
    const resp = await fetch("api/startup/check");
    const text = await resp.text();
    if (!resp.ok) {
      throw new Error(text || `HTTP ${resp.status}`);
    }
    const result = JSON.parse(text);
    setStartupStatus(startupDockerStatusEl, Boolean(result.docker_installed), result.docker_installed ? "已安装" : "未安装");
    startupDockerVersionEl.textContent = result.docker_version || "-";
    setStartupStatus(startupComposeStatusEl, Boolean(result.compose_installed), result.compose_installed ? "已安装" : "未安装");
    startupComposeVersionEl.textContent = result.compose_version || "-";
    setStartupStatus(startupConfigStatusEl, Boolean(result.config_ok), result.config_ok ? "通过" : "失败");
    startupConfigDetailEl.textContent = (result.config_detail || "-").slice(0, 180);
    applyStartupMonitor(result);
    startupStartBtn.disabled = !Boolean(result.ready);
    startupRunStatusEl.textContent = result.ready ? "检测通过，可一键运行启动" : "检测未通过，请先修复环境或配置";
  } catch (err) {
    startupRunStatusEl.textContent = `检测失败: ${err?.message || err}`;
    startupStartBtn.disabled = true;
  } finally {
    startupCheckBtn.disabled = false;
    if (currentStep === monitorStepIndex) {
      syncStepUI();
    }
  }
}

function stopStartupStream() {
  if (startupEventSource) {
    startupEventSource.close();
    startupEventSource = null;
  }
  stopStartupMonitorPoll();
}

function startStartup() {
  stopStartupStream();
  startupStartBtn.disabled = true;
  startupRunStatusEl.textContent = "启动中...";
  resetStartupLogs("", "监控操作日志：启动任务中...");
  resetStartupProgress();
  if (currentStep === monitorStepIndex) {
    syncStepUI();
  }
  void refreshStartupMonitor(true);
  startStartupMonitorPoll();

  const es = new EventSource("api/startup/start-stream");
  startupEventSource = es;

  es.addEventListener("status", (event) => {
    try {
      const data = JSON.parse(event.data || "{}");
      if (data.message) {
        appendStartupLog(data.message);
      }
    } catch {
      appendStartupLog("收到状态更新");
    }
  });

  es.addEventListener("log", (event) => {
    try {
      const data = JSON.parse(event.data || "{}");
      if (data.line) {
        appendStartupLog(data.line);
      }
    } catch {
      appendStartupLog(event.data || "");
    }
  });

  es.addEventListener("progress", (event) => {
    try {
      const data = JSON.parse(event.data || "{}");
      applyStartupProgress(data);
    } catch (err) {
      appendStartupLog(`进度解析失败: ${err?.message || err}`);
    }
  });

  es.addEventListener("done", (event) => {
    try {
      const data = JSON.parse(event.data || "{}");
      if (data.ok) {
        startupRunStatusEl.textContent = "启动完成";
        appendStartupLog("[OK] 启动完成");
      } else {
        startupRunStatusEl.textContent = `启动失败: ${data.error || "未知错误"}`;
        appendStartupLog(`[ERR] ${data.error || "启动失败"}`);
      }
    } catch {
      startupRunStatusEl.textContent = "启动结束";
    }
    stopStartupStream();
    startupStartBtn.disabled = false;
    if (currentStep === monitorStepIndex) {
      syncStepUI();
    }
    void refreshStartupMonitor(false);
  });

  es.onerror = () => {
    appendStartupLog("[ERR] 启动流连接中断");
    startupRunStatusEl.textContent = "启动流中断，请查看日志并重试";
    stopStartupStream();
    startupStartBtn.disabled = false;
    if (currentStep === monitorStepIndex) {
      syncStepUI();
    }
    void refreshStartupMonitor(false);
  };
}

async function generateConfigs() {
  generateBtn.disabled = true;
  generateStatusEl.textContent = "生成中...";
  try {
    const resp = await fetch("api/generate", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload())
    });
    const text = await resp.text();
    if (!resp.ok) {
      throw new Error(text || `HTTP ${resp.status}`);
    }
    const result = JSON.parse(text);
    const files = Array.isArray(result.files) ? result.files.length : 0;
    generateStatusEl.textContent = `已生成 ${files} 个文件，可直接 make start`;
  } catch (err) {
    generateStatusEl.textContent = `生成失败: ${err?.message || err}`;
  } finally {
    generateBtn.disabled = false;
  }
}

function syncNotifyFields() {
  const telegramOn = telegramEnabledEl.checked;
  const feishuOn = feishuEnabledEl.checked;
  telegramFieldsEl.classList.toggle("hidden", !telegramOn);
  feishuFieldsEl.classList.toggle("hidden", !feishuOn);
}

function scheduleRenderPreview() {
  if (previewRenderScheduled) {
    return;
  }
  previewRenderScheduled = true;
  const run = () => {
    previewRenderScheduled = false;
    void renderPreview();
  };
  if (typeof window.requestAnimationFrame === "function") {
    window.requestAnimationFrame(run);
    return;
  }
  window.setTimeout(run, 0);
}

function getPreviewFileContent(files, targetPath) {
  if (!Array.isArray(files)) {
    return "";
  }
  const hit = files.find((item) => String(item?.path || "") === targetPath);
  return hit && typeof hit.content === "string" ? hit.content : "";
}

async function fetchServerScriptPreview(data, cacheKey) {
  if (previewScriptCache.key === cacheKey && previewScriptCache.content) {
    return previewScriptCache.content;
  }
  const reqSeq = ++previewScriptRequestSeq;
  const resp = await fetch("api/preview", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data)
  });
  const text = await resp.text();
  if (!resp.ok) {
    throw new Error(text || `HTTP ${resp.status}`);
  }
  const result = JSON.parse(text);
  const scriptContent = getPreviewFileContent(result?.files, "scripts/onboarding-start.mk");
  if (reqSeq !== previewScriptRequestSeq) {
    return "";
  }
  if (scriptContent) {
    previewScriptCache.key = cacheKey;
    previewScriptCache.content = scriptContent;
  }
  return scriptContent;
}

async function renderPreview() {
  const renderSeq = ++previewRenderSeq;
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

  const cacheKey = JSON.stringify(data);
  if (previewScriptCache.key === cacheKey && previewScriptCache.content) {
    previewEl.textContent = previewScriptCache.content;
    return;
  }

  previewEl.textContent = "onboarding-start.mk 预览加载中...";
  try {
    const serverScript = await fetchServerScriptPreview(data, cacheKey);
    if (renderSeq !== previewRenderSeq) {
      return;
    }
    previewEl.textContent = serverScript || renderScript(data);
  } catch {
    if (renderSeq !== previewRenderSeq) {
      return;
    }
    previewEl.textContent = renderScript(data);
  }
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

  const isLastStep = currentStep === steps.length - 1;
  prevBtn.disabled = currentStep === 0;
  if (isLastStep) {
    nextBtn.textContent = "一键运行启动";
    nextBtn.disabled = startupStartBtn.disabled || Boolean(startupEventSource);
  } else {
    nextBtn.textContent = "下一步";
    nextBtn.disabled = false;
  }
  if (currentStep === startupStepIndex && startupRunStatusEl.textContent === "等待检测") {
    checkStartup();
  }
  if (currentStep === monitorStepIndex) {
    void refreshStartupMonitor(true);
  }

  void renderPreview();
}

prevBtn.addEventListener("click", () => {
  currentStep = Math.max(0, currentStep - 1);
  syncStepUI();
});

nextBtn.addEventListener("click", () => {
  if (currentStep === steps.length - 1) {
    startStartup();
    return;
  }
  currentStep += 1;
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
    scheduleRenderPreview();
  });
});

form.querySelectorAll('[name="symbols"]').forEach((el) => {
  el.addEventListener("change", () => {
    saveCurrentSymbolDraft();
    renderSymbolTabs();
    scheduleRenderPreview();
  });
});

Object.values(strategyInputs).forEach((input) => {
  input.addEventListener("input", () => {
    saveCurrentSymbolDraft();
    updateSaveStatus();
    scheduleRenderPreview();
  });
  input.addEventListener("change", scheduleRenderPreview);
});

intervalInputs.forEach((input) => {
  input.addEventListener("change", () => {
    saveCurrentSymbolDraft();
    updateSaveStatus();
    scheduleRenderPreview();
  });
});

saveSymbolBtn.addEventListener("click", () => {
  if (!activeSymbol) return;
  const cfg = collectStrategyForm();
  symbolSaved[activeSymbol] = clone(cfg);
  symbolDrafts[activeSymbol] = clone(cfg);
  renderSymbolTabs();
  updateSaveStatus();
  scheduleRenderPreview();
});

telegramEnabledEl.addEventListener("change", scheduleRenderPreview);
feishuEnabledEl.addEventListener("change", scheduleRenderPreview);
form.addEventListener("input", scheduleRenderPreview);
form.addEventListener("change", scheduleRenderPreview);
generateBtn.addEventListener("click", generateConfigs);
startupCheckBtn.addEventListener("click", checkStartup);
startupStartBtn.addEventListener("click", startStartup);
if (startupMonitorRefreshBtn) {
  startupMonitorRefreshBtn.addEventListener("click", () => {
    void refreshStartupMonitor(false);
  });
}
if (startupBraleOpenBtn) {
  startupBraleOpenBtn.addEventListener("click", () => {
    if (startupBraleLinkEl && !startupBraleLinkEl.classList.contains("disabled")) {
      window.open(startupBraleLinkEl.href, "_blank", "noopener,noreferrer");
    }
  });
}
if (startupBraleStopBtn) {
  startupBraleStopBtn.addEventListener("click", () => {
    void runStartupServiceAction("brale", "stop", startupBraleStopBtn);
  });
}
if (startupFreqtradeOpenBtn) {
  startupFreqtradeOpenBtn.addEventListener("click", () => {
    if (startupFreqtradeLinkEl && !startupFreqtradeLinkEl.classList.contains("disabled")) {
      window.open(startupFreqtradeLinkEl.href, "_blank", "noopener,noreferrer");
    }
  });
}
if (startupFreqtradeStopBtn) {
  startupFreqtradeStopBtn.addEventListener("click", () => {
    void runStartupServiceAction("freqtrade", "stop", startupFreqtradeStopBtn);
  });
}
if (startupFreqtradeRefreshBtn) {
  startupFreqtradeRefreshBtn.addEventListener("click", () => {
    void runStartupServiceAction("brale", "pull-rebuild", startupFreqtradeRefreshBtn);
  });
}

renderSymbolTabs();
syncNotifyFields();
resetStartupLogs("等待启动...", "监控操作日志：等待操作...");
resetStartupProgress();
void refreshStartupMonitor(true);
syncStepUI();

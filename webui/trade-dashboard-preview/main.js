const dashboardData = {
  recentTenWinRate: 80.0,
  symbols: {
    "BTC/USDT": {
      interval: "4H",
      live: {
        direction: "LONG",
        size: "0.82 BTC",
        entry: 64210,
        current: 64988,
        tp: 65880,
        sl: 63520,
        realized: 1328.4,
        unrealized: 647.2
      },
      candles: [
        ["09:00", 63520, 64260, 63210, 64480],
        ["10:00", 64260, 63990, 63820, 64510],
        ["11:00", 63990, 64680, 63740, 64930],
        ["12:00", 64680, 64420, 64280, 64890],
        ["13:00", 64420, 65090, 64370, 65210],
        ["14:00", 65090, 64810, 64640, 65320],
        ["15:00", 64810, 65320, 64710, 65510],
        ["16:00", 65320, 64988, 64790, 65640]
      ],
      decisions: [
        {
          id: "BTC-D-20260309-01",
          time: "2026-03-09 11:04",
          aiDecision: "结构突破 + 新闻风险中性",
          action: "开多 0.82 BTC",
          positionChange: "0 -> 0.82 BTC",
          result: "+647.2 USDT 未实现",
          confidence: 0.81,
          why: "4H 区间上沿突破后回踩确认，成交量与OI同步放大，Gate评分超过阈值。",
          keyPoint: { name: "Open Long", coord: ["11:00", 64680] }
        },
        {
          id: "BTC-D-20260308-03",
          time: "2026-03-08 18:12",
          aiDecision: "触发 tighten 条件",
          action: "减仓 30%",
          positionChange: "1.10 -> 0.77 BTC",
          result: "+324.6 USDT 已实现",
          confidence: 0.74,
          why: "波动突增且短周期背离，风险模块提升保护级别，执行部分止盈。",
          keyPoint: { name: "Tighten", coord: ["14:00", 64810] }
        }
      ],
      closedPositions: [
        ["03-09 00:20", "SHORT", "65240 -> 64880", "+518.2"],
        ["03-08 18:12", "LONG", "64610 -> 64810", "+324.6"],
        ["03-08 09:45", "SHORT", "64550 -> 63990", "+1003.8"]
      ]
    },
    "ETH/USDT": {
      interval: "1H",
      live: {
        direction: "SHORT",
        size: "12.5 ETH",
        entry: 3388,
        current: 3342,
        tp: 3285,
        sl: 3440,
        realized: 452.7,
        unrealized: 265.3
      },
      candles: [
        ["09:00", 3430, 3410, 3396, 3444],
        ["10:00", 3410, 3394, 3388, 3420],
        ["11:00", 3394, 3378, 3365, 3405],
        ["12:00", 3378, 3386, 3362, 3390],
        ["13:00", 3386, 3362, 3350, 3392],
        ["14:00", 3362, 3354, 3342, 3368],
        ["15:00", 3354, 3348, 3338, 3360],
        ["16:00", 3348, 3342, 3334, 3350]
      ],
      decisions: [
        {
          id: "ETH-D-20260309-02",
          time: "2026-03-09 10:30",
          aiDecision: "上冲动能减弱 + 资金费率偏高",
          action: "开空 12.5 ETH",
          positionChange: "0 -> -12.5 ETH",
          result: "+265.3 USDT 未实现",
          confidence: 0.77,
          why: "阻力位反复测试失败且量能背离，风险回报比向空头倾斜。",
          keyPoint: { name: "Open Short", coord: ["10:00", 3394] }
        },
        {
          id: "ETH-D-20260308-06",
          time: "2026-03-08 21:40",
          aiDecision: "触发止盈带",
          action: "分批止盈 40%",
          positionChange: "-20 -> -12 ETH",
          result: "+180.0 USDT 已实现",
          confidence: 0.69,
          why: "价格触及 TP1，波动率回落，按计划兑现部分利润。",
          keyPoint: { name: "TP1", coord: ["13:00", 3362] }
        }
      ],
      closedPositions: [
        ["03-09 06:10", "LONG", "3310 -> 3368", "+290.4"],
        ["03-08 21:40", "SHORT", "3416 -> 3362", "+180.0"],
        ["03-08 14:32", "LONG", "3340 -> 3324", "-120.5"]
      ]
    }
  }
};

let klineChart = null;

function formatUsd(value) {
  const sign = value >= 0 ? "+" : "";
  return `${sign}${value.toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 })} USDT`;
}

function updateClock() {
  const clockNode = document.getElementById("clock-chip");
  const now = new Date();
  clockNode.textContent = `CN ${now.toLocaleTimeString("zh-CN", { hour12: false })}`;
}

function getSymbolKeys() {
  return Object.keys(dashboardData.symbols);
}

function isOpenAction(actionText) {
  return String(actionText).startsWith("开");
}

function getOpenDecisionList(symbol) {
  return dashboardData.symbols[symbol].decisions.filter((item) => isOpenAction(item.action));
}

function renderLivePositions() {
  const host = document.getElementById("live-position-list");
  host.innerHTML = getSymbolKeys()
    .map((symbol) => {
      const live = dashboardData.symbols[symbol].live;
      const unrealizedClass = live.unrealized >= 0 ? "positive" : "negative";
      const realizedClass = live.realized >= 0 ? "positive" : "negative";
      return `<div class="live-row">
        <div><span class="k">交易对 / 方向</span><span class="v">${symbol} · ${live.direction}</span></div>
        <div><span class="k">仓位规模</span><span class="v">${live.size}</span></div>
        <div><span class="k">入场 / 当前</span><span class="v">${live.entry.toLocaleString("en-US")} / ${live.current.toLocaleString("en-US")}</span></div>
        <div><span class="k">止盈 TP</span><span class="v positive">${live.tp.toLocaleString("en-US")}</span></div>
        <div><span class="k">止损 SL</span><span class="v negative">${live.sl.toLocaleString("en-US")}</span></div>
        <div><span class="k">已实现盈亏</span><span class="v ${realizedClass}">${formatUsd(live.realized)}</span></div>
        <div><span class="k">未实现盈亏</span><span class="v ${unrealizedClass}">${formatUsd(live.unrealized)}</span></div>
      </div>`;
    })
    .join("");
}

function renderPnlSummary() {
  const totals = getSymbolKeys().reduce(
    (acc, symbol) => {
      const live = dashboardData.symbols[symbol].live;
      return {
        realized: acc.realized + live.realized,
        unrealized: acc.unrealized + live.unrealized
      };
    },
    { realized: 0, unrealized: 0 }
  );

  document.getElementById("pnl-realized").textContent = formatUsd(totals.realized);
  document.getElementById("pnl-unrealized").textContent = formatUsd(totals.unrealized);
  document.getElementById("pnl-winrate").textContent = `${dashboardData.recentTenWinRate.toFixed(1)}%`;
}

function renderClosedPositions(symbol) {
  const body = document.getElementById("position-history-body");
  const rows = dashboardData.symbols[symbol].closedPositions;
  body.innerHTML = rows
    .map((row) => {
      const pnlClass = row[3].startsWith("-") ? "negative" : "positive";
      return `<tr><td>${row[0]}</td><td>${row[1]}</td><td>${row[2]}</td><td class="${pnlClass}">${row[3]} USDT</td></tr>`;
    })
    .join("");
}

function renderDecisionTrace(selectedDecision) {
  const traceList = document.getElementById("trace-list");
  const steps = [
    { tag: "AI", value: selectedDecision.aiDecision },
    { tag: "POSITION", value: selectedDecision.positionChange },
    { tag: "RESULT", value: selectedDecision.result }
  ];
  traceList.innerHTML = steps
    .map((step) => `<li><span class="trace-tag">${step.tag}</span>${step.value}</li>`)
    .join("");
}

function renderDecisionDetails(selectedDecision) {
  const detail = document.getElementById("decision-detail");
  detail.innerHTML = [
    `<strong>${selectedDecision.id}</strong> | 置信度 ${(selectedDecision.confidence * 100).toFixed(1)}%`,
    `<div>原因: ${selectedDecision.why}</div>`,
    `<div>仓位变化: ${selectedDecision.positionChange}</div>`,
    `<div>结果: ${selectedDecision.result}</div>`
  ].join("");
}

function renderDecisionTable(symbol, onSelect) {
  const body = document.getElementById("decision-history-body");
  const list = dashboardData.symbols[symbol].decisions;

  body.innerHTML = list
    .map(
      (item, idx) => `<tr data-id="${item.id}" class="${idx === 0 ? "active" : ""}">
        <td>${item.time}</td>
        <td>${item.aiDecision}</td>
        <td>${item.action}</td>
        <td>${item.result}</td>
      </tr>`
    )
    .join("");

  body.querySelectorAll("tr").forEach((tr) => {
    tr.addEventListener("click", () => {
      body.querySelectorAll("tr").forEach((row) => {
        row.classList.remove("active");
      });
      tr.classList.add("active");
      const selected = list.find((d) => d.id === tr.dataset.id);
      onSelect(selected);
    });
  });
}

function renderSymbolSelect(currentSymbol, onChange) {
  const select = document.getElementById("symbol-select");
  select.innerHTML = getSymbolKeys()
    .map((symbol) => `<option value="${symbol}" ${symbol === currentSymbol ? "selected" : ""}>${symbol}</option>`)
    .join("");

  select.onchange = () => {
    onChange(select.value);
  };
}

function renderKline(symbol, selectedDecision) {
  const chartDom = document.getElementById("kline-chart");
  if (!klineChart) {
    klineChart = echarts.init(chartDom);
  }

  const symbolData = dashboardData.symbols[symbol];
  document.getElementById("kline-range").textContent = `${symbolData.interval} Window`;

  const labels = symbolData.candles.map((c) => c[0]);
  const values = symbolData.candles.map((c) => [c[1], c[2], c[3], c[4]]);

  const point = selectedDecision ? selectedDecision.keyPoint : { name: "Key Point", coord: [labels[labels.length - 1], values[values.length - 1][1]] };

  klineChart.setOption({
    backgroundColor: "transparent",
    grid: { left: 45, right: 20, top: 28, bottom: 30 },
    tooltip: {
      trigger: "axis",
      axisPointer: { type: "cross" }
    },
    xAxis: {
      type: "category",
      data: labels,
      boundaryGap: true,
      axisLine: { lineStyle: { color: "#8ea2bb" } },
      axisLabel: { color: "#9caec6" }
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
        markPoint: {
          symbolSize: 58,
          label: { color: "#0d1218", fontWeight: 700 },
          data: [
            {
              name: point.name,
              coord: point.coord,
              value: point.name,
              itemStyle: { color: "#f6bd43" }
            }
          ]
        }
      }
    ]
  });
}

function bootstrapDashboard() {
  updateClock();
  setInterval(updateClock, 1000);

  renderLivePositions();
  renderPnlSummary();

  let currentSymbol = getSymbolKeys()[0];
  let currentOpenDecision = getOpenDecisionList(currentSymbol)[0];

  function renderSymbolScope() {
    renderClosedPositions(currentSymbol);
    renderDecisionTable(currentSymbol, (selected) => {
      renderDecisionDetails(selected);
      if (isOpenAction(selected.action)) {
        currentOpenDecision = selected;
      }
      renderDecisionTrace(currentOpenDecision || selected);
      renderKline(currentSymbol, currentOpenDecision || selected);
    });

    const fallbackDecision = dashboardData.symbols[currentSymbol].decisions[0];
    const viewDecision = currentOpenDecision || fallbackDecision;
    renderDecisionTrace(viewDecision);
    renderDecisionDetails(fallbackDecision);
    renderKline(currentSymbol, viewDecision);
  }

  renderSymbolSelect(currentSymbol, (symbol) => {
    currentSymbol = symbol;
    currentOpenDecision = getOpenDecisionList(currentSymbol)[0] || dashboardData.symbols[currentSymbol].decisions[0];
    renderSymbolScope();
  });

  renderSymbolScope();

  window.addEventListener("resize", () => {
    if (klineChart) {
      klineChart.resize();
    }
  });
}

bootstrapDashboard();

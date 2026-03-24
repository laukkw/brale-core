# Brale (Break a Leg) 🎭

- **An AI-driven multi-agent quantitative strategy engine**
- *If trading is a performance, then "Break a leg" means: may you trade brilliantly.*

[![Chinese Documentation](https://img.shields.io/badge/lang-中文-red.svg)](docs/README_CN.org)
[![Newcomer Guide](https://img.shields.io/badge/doc-User_Guide-blueviolet.svg)](doc/USER_GUIDE.org)
[![Go Version](https://img.shields.io/badge/go-1.24.0-blue.svg)](go.mod)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

## Project Background

Brale Core is designed to turn subjective trading judgment into an engineering workflow that is verifiable, executable, and auditable. The goal is to reduce emotional interference and improve execution consistency.

In code, this is reflected by:

- Driving decisions with rule chains and state machines instead of ad-hoc manual judgment.
- Separating decision-making, execution, risk control, and reconciliation into independent modules orchestrated at runtime.
- Prioritizing a "plan first, execute second" workflow with continuous monitoring during the position lifecycle.

## Core Features

- Multi-agent collaboration: Indicator / Structure / Mechanics evaluate independently across indicators, market structure, and trading mechanics, then cross-validate to reduce false signals from any single source.
- Three-path consensus decisioning: Indicator / Structure / Mechanics analysis must pass through Gate evaluation before execution.
- Explainable rule flow: RuleFlow outputs Gate / Plan / FSM artifacts, enabling full traceability of each decision cycle.
- Structured decision outputs: standardized Gate, Plan, and FSM outputs improve auditability, cross-module integration, automated replay, and secondary analysis.
- Controlled tighten risk logic: when the system enters the TIGHTEN path, it applies layered pre-execution thresholds (monitor hits, volatility limits, score thresholds, debounce windows, sentiment gates, etc.) to avoid overreacting to noise.
- Plan-first execution: generates entry / stop / take-profits / position-size / leverage before opening positions.
- Clear execution order layer: decouples decision logic from exchange connectivity using a unified adapter for order placement, query, and cancellation.
- Closed-loop position risk control: runs risk monitoring on price ticks and supports tighten / exit actions.
- Reconciliation and recovery: periodic reconcile + startup recovery to reduce state drift.
- Engineering-grade configuration: layered system / symbol / strategy config with hash binding.

![Flow](docs/flow.png)

## Quick Start

### Prerequisites

- Docker ≥ 24 + Docker Compose V2
- Git

For manual CLI operations after onboarding, the host needs GNU Make and Docker Compose V2.

### Directory Layout

```
brale-core/
├── .env                          # Environment variables and secrets
├── configs/
│   ├── system.toml               # System-level config (execution, notifications, LLM)
│   ├── symbols-index.toml        # Symbol index (symbol → config/strategy mapping)
│   ├── symbols/*.toml            # Per-symbol parameters
│   ├── strategies/*.toml         # Per-symbol strategy (risk, TP/SL, etc.)
│   ├── rules/*.json              # Rule chain definitions
│   └── freqtrade/                # Freqtrade config templates & strategy files
├── data/                         # Runtime data (bind mount, persistent)
│   ├── brale/                    # Brale database & system config copy
│   └── freqtrade/user_data/      # Freqtrade runtime data
├── docker-compose.yml
├── Dockerfile
└── Makefile
```

### First-Time Deployment

```bash
# Recommended: thin bootstrap (clone/update repo -> start onboarding container -> open page)
curl -fsSL https://raw.githubusercontent.com/laukkw/brale-core/master/scripts/bootstrap.sh | bash

# Or if you already cloned the repo locally
make init
```

`make init` starts the always-on onboarding service on `http://127.0.0.1:9992` inside the same Compose project as `brale` and `freqtrade`.
Complete the setup in the onboarding page, then use the page's "Apply config and restart services"
action to run the equivalent of `make stop && make start`.
`make init` is the step that builds the onboarding image; later `make check` / `make prepare` reuse it instead of rebuilding on every run.

### Manual CLI Deployment After Onboarding

Once onboarding has generated `.env` and the `configs/` files, you can manage the stack directly:

```bash
make start
```

### Command Reference

| Command | Description |
|---|---|
| `make init` | Build/start the always-on onboarding service on `127.0.0.1:9992` |
| `make init-stop` | Stop the onboarding container |
| `make init-logs` | Tail onboarding container logs |
| `make start` | First-time / full start (freqtrade → brale) |
| `make apply-config` | Re-prepare config and restart `freqtrade` + `brale` with latest files |
| `make stop` | Stop `freqtrade` + `brale` only (`onboarding` stays up) |
| `make restart` | Same as `make start` (runs check + prepare first) |
| `make rebuild` | Rebuild brale image after code changes and restart |
| `make status` | Show service status |
| `make logs` | Tail freqtrade + brale logs |
| `make down` | Destroy the whole Compose project, including `onboarding` (`data/` directory unaffected) |

### Restart After Code Changes

```bash
# Rebuild brale image only — historical data is not affected
make rebuild
```

`make rebuild` automatically runs `check` + `prepare`, then `docker compose up -d --build brale`.
Since `data/brale` and `configs/` are bind-mounted, rebuilding never loses runtime data.

### Restart After Config Changes

- After modifying `.env` or files under `configs/`, **no rebuild is needed**.

```bash
make stop && make start
```

- Changed Go source code → use `make rebuild`.

The onboarding UI uses this same `stop + start` semantics when you apply updated config, while the onboarding service itself stays up as the control plane.

### Data Persistence

| Host Path | Container Path | Description |
|---|---|---|
| `./data/brale/` | `/app/data/` | SQLite database, system config |
| `./data/freqtrade/user_data/` | `/freqtrade/user_data/` | Freqtrade trade data & logs |
| `./configs/` | `/app/configs/` (read-only) | Strategy & rule configuration |

All runtime data is stored on the host under `data/`. Neither `make down` nor container rebuilds will delete it.

### Service Ports

| Service | Port | Description |
|---|---|---|
| Onboarding | `127.0.0.1:9992` | Local setup UI and stack control plane |
| Brale | `127.0.0.1:9991` | Runtime API + Decision View |
| Freqtrade | `127.0.0.1:8080` | Freqtrade REST API |

## Architecture Overview

This project uses a layered collaborative runtime architecture that can be summarized into five parts: bootstrap assembly, runtime orchestration, decision layer, execution & risk layer, and data & interface layer.

### 1) Bootstrap Assembly Layer

The bootstrap assembly layer handles initialization and dependency wiring. The app enters from `cmd/brale-core/main.go`, then `bootstrap.Run(...)` loads configuration, builds core dependencies, and starts scheduling plus service interfaces.

### 2) Runtime Orchestration Layer

The runtime orchestration layer builds isolated runtime units per symbol, ensuring each instrument has an independent decision and execution context under the same framework. The core entry is `runtime.BuildSymbolRuntime(...)`, which assembles SnapshotFetcher, Compressor, Agent/Provider, Runner, and Pipeline.

### 3) Decision Layer

The decision layer is responsible for turning information into judgments. In each cycle, the system sequentially performs market snapshot fetching, feature compression, Agent/Provider evaluation, and RuleFlow inference, then produces structured outputs such as Gate, Plan, and FSM.

### 4) Execution and Risk Layer

The execution and risk layer converts decisions into trading actions and continuously controls risk during the position lifecycle. The execution side sends trade instructions through the Freqtrade adapter; the risk side enforces plan execution and dynamic monitoring around ExecutionPlan and RiskPlan.

### 5) Data and Interface Layer

The data and interface layer provides persistent state storage and external interaction. SQLite + GORM persist key records at the storage layer, while Runtime API, Webhook, and notification channels expose runtime control and status feedback.

![Path](docs/path.png)

## Data Sources

| Data Category | Primary Source / Implementation |
|---|---|
| Klines | Binance Futures market API |
| Open Interest (OI) | Binance Futures |
| Funding Rate | Binance Futures |
| Long/Short Ratio | Binance Futures |
| Fear & Greed | Dedicated FearGreed service |
| Liquidations | Binance Futures (window aggregation) |
| Mark Price | Binance MarkPriceStream |
| Trade execution and account | Freqtrade API |

Note: At symbol level, configuration can decide whether OI/Funding/LongShort/FearGreed/Liquidations are strict requirements (see `SymbolRequire` in `internal/config/types.go`).

## Configuration Structure

The configuration system uses a three-layer structure: system-level, index-level, and strategy-level, so global behavior is decoupled from per-symbol strategy settings.

- System-level config: `configs/system.toml`, defines global capabilities such as execution system, Webhook, notifications, and LLM models.
- Index-level config: `configs/symbols-index.toml`, maintains mappings from symbols to config and strategy files.
- Strategy-level config: `configs/symbols/*.toml` and `configs/strategies/*.toml`, describes per-instrument parameters, risk controls, and strategy details.

For full configuration fields and structure definitions, see `internal/config/types.go`.

## ⚠️ Risk Disclaimer

- This project is for technical research, system development, and process validation only, and does not constitute investment advice.
- Digital asset trading is highly volatile and risky, and may result in partial or total loss of funds.
- Users must independently evaluate strategy, parameters, risk controls, and counterparty risks, and assume all consequences.
- Historical performance, backtest results, and sample configurations do not guarantee future returns.
- Before enabling production trading, perform thorough testing, monitoring, and contingency planning in an isolated environment.

## 📄 License

This project is licensed under the MIT License - see [LICENSE](LICENSE) for details.

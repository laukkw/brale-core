SHELL := /bin/bash
.SHELLFLAGS := -euo pipefail -c

COMPOSE_FILE ?= docker-compose.yml
COMPOSE_PROJECT_NAME ?= brale-core
SETUP_LANG ?=
SETUP_ARGS ?=
WAIT_FREQTRADE_TIMEOUT ?= 90
WAIT_BRALE_TIMEOUT ?= 90

ENV_ENABLE_MCP := $(strip $(shell awk -F= '/^[[:space:]]*ENABLE_MCP[[:space:]]*=/{gsub(/^[[:space:]]+|[[:space:]]+$$/,"",$$2); print $$2; exit}' .env 2>/dev/null))
ENV_NOTIFICATION_TELEGRAM_TOKEN := $(strip $(shell awk -F= '/^[[:space:]]*NOTIFICATION_TELEGRAM_TOKEN[[:space:]]*=/{gsub(/^[[:space:]]+|[[:space:]]+$$/,"",$$2); print $$2; exit}' .env 2>/dev/null))
ENV_NOTIFICATION_TELEGRAM_CHAT_ID := $(strip $(shell awk -F= '/^[[:space:]]*NOTIFICATION_TELEGRAM_CHAT_ID[[:space:]]*=/{gsub(/^[[:space:]]+|[[:space:]]+$$/,"",$$2); print $$2; exit}' .env 2>/dev/null))
ifeq ($(origin ENABLE_MCP), undefined)
ifneq ($(ENV_ENABLE_MCP),)
ENABLE_MCP := $(ENV_ENABLE_MCP)
else
ENABLE_MCP := 0
endif
endif

BRALE_CONFIG_ROOT ?= $(CURDIR)/configs
BRALE_SYSTEM_IN ?= $(BRALE_CONFIG_ROOT)/system.toml
BRALE_SYMBOL_INDEX_IN ?= $(BRALE_CONFIG_ROOT)/symbols-index.toml
BRALE_SYSTEM_PATH_IN_CONTAINER ?= /app/configs/system.toml
BRALE_SYMBOL_INDEX_PATH_IN_CONTAINER ?= /app/configs/symbols-index.toml
BRALE_DATA_ROOT ?= $(CURDIR)/data/brale
PGDATA_ROOT ?= $(CURDIR)/data/pgdata
FREQTRADE_CONFIG_ROOT ?= $(CURDIR)/configs/freqtrade
FREQTRADE_RUNTIME_ROOT ?= $(CURDIR)/data/freqtrade/user_data
FREQTRADE_CONFIG_FILE ?= $(FREQTRADE_RUNTIME_ROOT)/config.json
STACK_PROXY_ENV_FILE ?= $(CURDIR)/data/freqtrade/proxy.env
POSTGRES_HOST_PORT ?= 5432
FREQTRADE_HOST_PORT ?= 8080
BRALE_HOST_PORT ?= 9991
MCP_HOST_PORT ?= 8765
HOST_UID ?= $(shell id -u)
HOST_GID ?= $(shell id -g)
HOST_REPO_ROOT ?= $(CURDIR)
BRALECTL_BIN ?= $(BRALE_DATA_ROOT)/bin/bralectl
OUTPUT_ROOT ?= $(CURDIR)/_output
BRALECTL_OUTPUT_BIN ?= $(OUTPUT_ROOT)/bralectl
BRALECTL_INSTALL_DIR ?= $(HOME)/.local/bin
BRALECTL_INSTALL_BIN ?= $(BRALECTL_INSTALL_DIR)/bralectl
BRALECTL_DOCKER_IMAGE ?= brale-core-go-builder
GO_BUILD_FLAGS ?= -buildvcs=false
HOST_UNAME_S := $(shell uname -s 2>/dev/null | tr '[:upper:]' '[:lower:]')
HOST_UNAME_M := $(shell uname -m 2>/dev/null)
GO_BUILD_GOOS ?= $(if $(filter darwin,$(HOST_UNAME_S)),darwin,$(if $(filter linux,$(HOST_UNAME_S)),linux,$(HOST_UNAME_S)))
GO_BUILD_GOARCH ?= $(if $(filter x86_64 amd64,$(HOST_UNAME_M)),amd64,$(if $(filter arm64 aarch64,$(HOST_UNAME_M)),arm64,$(HOST_UNAME_M)))
GO_BUILD_ENV = CGO_ENABLED=0 GOOS="$(GO_BUILD_GOOS)" GOARCH="$(GO_BUILD_GOARCH)"
TEMPLATE_SYMBOL ?=
VERIFY_CTL ?= 1
CTL_SYMBOL ?= $(strip $(shell awk -F'"' '/^[[:space:]]*symbol[[:space:]]*=[[:space:]]*"/ { print $$2; exit }' "$(BRALE_SYMBOL_INDEX_IN)" 2>/dev/null))
CTL_TIMEOUT ?= 3m
CTL_REPORT_DIR ?= $(OUTPUT_ROOT)/smoke

ENABLE_MCP_NORM := $(if $(filter 1 true TRUE yes YES on ON,$(ENABLE_MCP)),1,0)
VERIFY_CTL_NORM := $(if $(filter 1 true TRUE yes YES on ON,$(VERIFY_CTL)),1,0)
OPTIONAL_STACK_SERVICES := $(if $(filter 1,$(ENABLE_MCP_NORM)),mcp,)
REBUILD_SERVICES := brale $(if $(filter 1,$(ENABLE_MCP_NORM)),mcp,)

STACK_EXPORTS = export HOST_UID="$(HOST_UID)"; export HOST_GID="$(HOST_GID)"; export COMPOSE_PROJECT_NAME="$(COMPOSE_PROJECT_NAME)"; export HOST_REPO_ROOT="$(HOST_REPO_ROOT)"; export BRALE_CONFIG_ROOT="$(BRALE_CONFIG_ROOT)"; export BRALE_SYSTEM_PATH_IN_CONTAINER="$(BRALE_SYSTEM_PATH_IN_CONTAINER)"; export BRALE_SYMBOL_INDEX_PATH_IN_CONTAINER="$(BRALE_SYMBOL_INDEX_PATH_IN_CONTAINER)"; export BRALE_DATA_ROOT="$(BRALE_DATA_ROOT)"; export PGDATA_ROOT="$(PGDATA_ROOT)"; export FREQTRADE_CONFIG_ROOT="$(FREQTRADE_CONFIG_ROOT)"; export FREQTRADE_RUNTIME_ROOT="$(FREQTRADE_RUNTIME_ROOT)"; export FREQTRADE_CONFIG_FILE="$(FREQTRADE_CONFIG_FILE)"; export STACK_PROXY_ENV_FILE="$(STACK_PROXY_ENV_FILE)"; export POSTGRES_HOST_PORT="$(POSTGRES_HOST_PORT)"; export FREQTRADE_HOST_PORT="$(FREQTRADE_HOST_PORT)"; export BRALE_HOST_PORT="$(BRALE_HOST_PORT)"; export MCP_HOST_PORT="$(MCP_HOST_PORT)";
STACK_PROXY_SOURCE = if [ -f "$$STACK_PROXY_ENV_FILE" ]; then \
	while IFS= read -r line || [ -n "$$line" ]; do \
		trimmed="$$line"; \
		trimmed="$$(printf '%s' "$$trimmed" | sed 's/^[[:space:]]*//')"; \
		if [ -z "$$trimmed" ]; then \
			continue; \
		fi; \
		if [ "$${trimmed:0:1}" = "$$(printf '\043')" ]; then \
			continue; \
		fi; \
		if [[ "$$trimmed" =~ ^(HTTP_PROXY|HTTPS_PROXY|NO_PROXY|http_proxy|https_proxy|no_proxy)=(.*)$$ ]]; then \
			key="$${BASH_REMATCH[1]}"; \
			value="$${BASH_REMATCH[2]}"; \
			if printf '%s\n' "$$value" | grep -q '[`$$]'; then \
				echo "[ERR] proxy env values must be literal in $$STACK_PROXY_ENV_FILE"; \
				exit 1; \
			fi; \
			if [[ "$$key" =~ ^(HTTP_PROXY|HTTPS_PROXY|http_proxy|https_proxy)$$ ]] && [[ "$$value" =~ ^socks5?h?:// ]]; then \
				echo "[WARN] skipping $$key from $$STACK_PROXY_ENV_FILE for Docker build: apt does not support SOCKS proxies"; \
				continue; \
			fi; \
			export "$$key=$$value"; \
		else \
			echo "[ERR] invalid proxy env entry in $$STACK_PROXY_ENV_FILE: $$trimmed"; \
			exit 1; \
		fi; \
	done < "$$STACK_PROXY_ENV_FILE"; \
fi;
COMPOSE = $(STACK_EXPORTS) $(STACK_PROXY_SOURCE) docker compose -f "$(COMPOSE_FILE)"

# PREPARE_STACK runs the prepare-stack logic locally (via Go) or in Docker.
PREPARE_STACK_ARGS = -env-file .env -config-in "$(FREQTRADE_CONFIG_ROOT)/config.base.json" -config-out "$(FREQTRADE_CONFIG_FILE)" -proxy-env-out "$(STACK_PROXY_ENV_FILE)" -system-in "$(BRALE_SYSTEM_IN)"

.PHONY: help env-init setup init check prepare start apply-config start-freqtrade wait-freqtrade wait-brale ctl-smoke post-start-verify start-brale mcp-start mcp-stop mcp-logs stop-freqtrade stop-brale stop restart rebuild down status logs logs-all build bralectl-build install-bralectl bralectl-builder-image add-symbol llm-probe migrate-up e2e-start e2e-stop e2e-reset e2e-status e2e-test e2e-logs

help: ## Show the main make targets and optional component switches
	@printf '%-22s %s\n' "env-init" "Create .env from .env.example if missing"; \
	printf '%-22s %s\n' "build" "Build bralectl into _output/bralectl"; \
	printf '%-22s %s\n' "install-bralectl" "Build and install bralectl into ~/.local/bin (override BRALECTL_INSTALL_DIR)"; \
	printf '%-22s %s\n' "setup" "Run bralectl setup (env init + optional MCP client config)"; \
	printf '%-22s %s\n' "init" "Interactive CLI wizard: configure .env + generate runtime configs"; \
	printf '%-22s %s\n' "start" "Start freqtrade + brale, then run ctl smoke verification by default"; \
	printf '%-22s %s\n' "mcp-start" "Start only the MCP service (dependencies auto-start)"; \
	printf '%-22s %s\n' "rebuild" "Rebuild brale (and mcp when ENABLE_MCP=1), then run ctl smoke verification"; \
	printf '%-22s %s\n' "stop / down" "Stop services / remove the full compose stack"; \
	printf '%-22s %s\n' "logs / logs-all" "Tail brale logs / tail brale + freqtrade logs"; \
	printf '%-22s %s\n' "mcp-logs" "Tail MCP logs"; \
	echo ""; \
	echo "Optional switches:"; \
	printf '  %-18s %s\n' "ENABLE_MCP=1" "Include the mcp service"; \
	printf '  %-18s %s\n' "VERIFY_CTL=0" "Skip the post-start ctl smoke verification"; \
	printf '  %-18s %s\n' "CTL_SYMBOL=..." "Override the symbol used by ctl smoke verification"; \
	printf '  %-18s %s\n' "SETUP_LANG=zh|en" "Preselect the setup wizard language"; \
	printf '  %-18s %s\n' "SETUP_ARGS='...'" "Pass extra flags to bralectl setup"; \
	printf '  %-18s %s\n' "BRALECTL_INSTALL_DIR=..." "Install bralectl into a custom bin directory"

env-init: ## Create .env from .env.example when missing, then validate required variables
	@set -e; \
	if [ -f ".env" ]; then \
		echo "[OK] .env already exists"; \
	elif [ -f ".env.example" ]; then \
		cp .env.example .env; \
		echo "[OK] created .env from .env.example"; \
	else \
		echo "[ERR] .env.example not found"; \
		exit 1; \
	fi; \
	missing=""; \
	for var in POSTGRES_USER POSTGRES_PASSWORD EXEC_USERNAME EXEC_SECRET; do \
		val=$$(awk -F= "/^[[:space:]]*$$var[[:space:]]*=/{gsub(/^[[:space:]]+|[[:space:]]+$$/,\"\",\$$2); print \$$2; exit}" .env 2>/dev/null); \
		if [ -z "$$val" ]; then \
			missing="$$missing $$var"; \
		fi; \
	done; \
	if [ -n "$$missing" ]; then \
		echo "[WARN] .env is missing required values:$$missing"; \
		echo "       Please edit .env before running 'make start'."; \
	fi

setup: bralectl-build env-init ## Run the local setup wizard (env init + optional MCP install)
	@"$(BRALECTL_BIN)" setup --repo "$(CURDIR)" $(if $(SETUP_LANG),--lang $(SETUP_LANG),) $(SETUP_ARGS)

init: bralectl-build env-init ## Interactive CLI wizard: configure .env + generate runtime configs
	@"$(BRALECTL_BIN)" init --repo "$(CURDIR)"

check: bralectl-build
	@if [ ! -f ".env" ]; then \
		echo "[ERR] .env not found in project root"; \
		exit 1; \
	fi
	@if [ ! -f "$(COMPOSE_FILE)" ]; then \
		echo "[ERR] compose file not found: $(COMPOSE_FILE)"; \
		exit 1; \
	fi
	@if [ ! -d "$(BRALE_CONFIG_ROOT)" ]; then \
		echo "[ERR] configs dir not found: $(BRALE_CONFIG_ROOT)"; \
		exit 1; \
	fi
	@if [ ! -d "$(FREQTRADE_CONFIG_ROOT)" ]; then \
		echo "[ERR] freqtrade config dir not found: $(FREQTRADE_CONFIG_ROOT)"; \
		exit 1; \
	fi
	@if [ ! -f "$(FREQTRADE_CONFIG_ROOT)/config.base.json" ]; then \
		echo "[ERR] freqtrade base config not found: $(FREQTRADE_CONFIG_ROOT)/config.base.json"; \
		exit 1; \
	fi
	@if [ ! -f "$(FREQTRADE_CONFIG_ROOT)/brale_shared_strategy.py" ]; then \
		echo "[ERR] strategy file not found: $(FREQTRADE_CONFIG_ROOT)/brale_shared_strategy.py"; \
		exit 1; \
	fi
	@"$(BRALECTL_BIN)" prepare-stack $(PREPARE_STACK_ARGS) -check-only

prepare: bralectl-build
	@mkdir -p "$(BRALE_DATA_ROOT)" \
		"$(PGDATA_ROOT)" \
		"$(FREQTRADE_RUNTIME_ROOT)" \
		"$(FREQTRADE_RUNTIME_ROOT)/backtest_results" \
		"$(FREQTRADE_RUNTIME_ROOT)/data" \
		"$(FREQTRADE_RUNTIME_ROOT)/freqaimodels" \
		"$(FREQTRADE_RUNTIME_ROOT)/hyperopt_results" \
		"$(FREQTRADE_RUNTIME_ROOT)/hyperopts" \
		"$(FREQTRADE_RUNTIME_ROOT)/logs" \
		"$(FREQTRADE_RUNTIME_ROOT)/notebooks" \
		"$(FREQTRADE_RUNTIME_ROOT)/plot" \
		"$(FREQTRADE_RUNTIME_ROOT)/strategies" \
		"$(dir $(FREQTRADE_CONFIG_FILE))" \
		"$(dir $(STACK_PROXY_ENV_FILE))"
	@"$(BRALECTL_BIN)" prepare-stack $(PREPARE_STACK_ARGS)
	@cp -f "$(FREQTRADE_CONFIG_ROOT)/brale_shared_strategy.py" "$(FREQTRADE_RUNTIME_ROOT)/strategies/BraleSharedStrategy.py"
	@if [ -d "$(BRALE_DATA_ROOT)/pgdata" ] && [ -z "$$(find "$(PGDATA_ROOT)" -mindepth 1 -maxdepth 1 2>/dev/null)" ]; then \
		echo "[WARN] detected legacy PostgreSQL data at $(BRALE_DATA_ROOT)/pgdata"; \
		echo "[WARN] new database data root is $(PGDATA_ROOT)"; \
		echo "[WARN] copy or migrate existing DB files before restarting if you need current database contents"; \
	fi
	@if [ "$$(id -u)" = "0" ] && [ -n "$(HOST_UID)" ] && [ -n "$(HOST_GID)" ]; then \
		chown -R "$(HOST_UID):$(HOST_GID)" "$(BRALE_DATA_ROOT)" "$(PGDATA_ROOT)" "$(FREQTRADE_RUNTIME_ROOT)" "$(dir $(STACK_PROXY_ENV_FILE))"; \
	fi

start: check prepare ## Start the core stack; use ENABLE_MCP=1 for optional MCP service
	@$(MAKE) start-freqtrade
	@$(MAKE) wait-freqtrade
	@$(COMPOSE) up -d --build brale $(OPTIONAL_STACK_SERVICES)
	@$(MAKE) post-start-verify

apply-config: check prepare stop ## Regenerate configs and restart the stack
	@$(MAKE) start ENABLE_MCP="$(ENABLE_MCP)"

start-freqtrade: ## Start the freqtrade service
	@$(COMPOSE) up -d --build freqtrade

wait-freqtrade: ## Wait for freqtrade health to turn green
	@ \
	cid=$$($(COMPOSE) ps -q freqtrade); \
	if [ -z "$$cid" ]; then \
		echo "[ERR] freqtrade container id not found"; \
		exit 1; \
	fi; \
	echo "[INFO] waiting freqtrade health (timeout=$(WAIT_FREQTRADE_TIMEOUT)s)..."; \
	for i in $$(seq 1 $(WAIT_FREQTRADE_TIMEOUT)); do \
		status=$$(docker inspect --format='{{if .State.Health}}{{.State.Health.Status}}{{else}}starting{{end}}' "$$cid" 2>/dev/null || true); \
		if [ "$$status" = "healthy" ]; then \
			echo "[OK] freqtrade healthy"; \
			exit 0; \
		fi; \
		sleep 2; \
	done; \
	echo "[ERR] freqtrade did not become healthy in time — recent logs:"; \
	$(COMPOSE) logs --tail=30 freqtrade 2>&1 || true; \
	exit 1

wait-brale: ## Wait for brale health to turn green
	@ \
	cid=$$($(COMPOSE) ps -q brale); \
	if [ -z "$$cid" ]; then \
		echo "[ERR] brale container id not found"; \
		exit 1; \
	fi; \
	echo "[INFO] waiting brale health (timeout=$(WAIT_BRALE_TIMEOUT)s)..."; \
	for i in $$(seq 1 $(WAIT_BRALE_TIMEOUT)); do \
		status=$$(docker inspect --format='{{if .State.Health}}{{.State.Health.Status}}{{else}}starting{{end}}' "$$cid" 2>/dev/null || true); \
		if [ "$$status" = "healthy" ]; then \
			echo "[OK] brale healthy"; \
			exit 0; \
		fi; \
		sleep 2; \
	done; \
	echo "[ERR] brale did not become healthy in time — recent logs:"; \
	$(COMPOSE) logs --tail=50 brale 2>&1 || true; \
	exit 1

ctl-smoke: wait-brale ## Run ctl suite against the live stack
	@if [ ! -x "$(BRALECTL_BIN)" ]; then \
		$(MAKE) bralectl-build; \
	fi
	@if [ -z "$(CTL_SYMBOL)" ]; then \
		echo "[ERR] CTL_SYMBOL is empty"; \
		echo "[ERR] add at least one symbol to $(BRALE_SYMBOL_INDEX_IN) or pass CTL_SYMBOL=..."; \
		exit 1; \
	fi
	@mkdir -p "$(CTL_REPORT_DIR)"
	@"$(BRALECTL_BIN)" test run \
		--endpoint "http://127.0.0.1:$(BRALE_HOST_PORT)" \
		--profile "main-stack" \
		--suites "ctl" \
		--symbol "$(CTL_SYMBOL)" \
		--timeout "$(CTL_TIMEOUT)" \
		--report-dir "$(CTL_REPORT_DIR)"

post-start-verify: ## Run post-start smoke verification unless VERIFY_CTL=0
	@if [ "$(VERIFY_CTL_NORM)" != "1" ]; then \
		echo "[INFO] ctl smoke verification skipped (VERIFY_CTL=$(VERIFY_CTL))"; \
	else \
		$(MAKE) ctl-smoke; \
	fi

start-brale: ## Start only the brale runtime service
	@$(COMPOSE) up -d brale

mcp-start: check prepare ## Start the MCP service (brings dependencies up as needed)
	@$(COMPOSE) up -d --build mcp

mcp-stop: ## Stop the MCP service
	@$(COMPOSE) stop mcp

mcp-logs: ## Tail MCP logs
	@$(COMPOSE) logs -f --tail=200 mcp

stop-freqtrade:
	@$(COMPOSE) stop freqtrade

stop-brale:
	@$(COMPOSE) stop brale

rebuild: check prepare ## Rebuild brale (and mcp when ENABLE_MCP=1)
	@$(COMPOSE) up -d --build $(REBUILD_SERVICES)
	@$(MAKE) post-start-verify

stop: ## Stop the running compose services
	@$(COMPOSE) stop brale freqtrade mcp

restart: apply-config

down: ## Remove the compose stack and orphans
	@$(COMPOSE) --profile mcp down --remove-orphans

status: ## Show compose service status
	@$(COMPOSE) ps

logs: ## Tail brale logs only
	@$(COMPOSE) logs -f --tail=200 brale

logs-all: ## Tail brale + freqtrade logs
	@$(COMPOSE) logs -f --tail=200 freqtrade brale

build: ## Build bralectl into _output/bralectl
	@if ! command -v go >/dev/null 2>&1; then \
		echo "[ERR] go command not found"; \
		exit 1; \
	fi
	@mkdir -p "$(dir $(BRALECTL_OUTPUT_BIN))"
	$(GO_BUILD_ENV) go build $(GO_BUILD_FLAGS) -o "$(BRALECTL_OUTPUT_BIN)" ./cmd/bralectl

bralectl-build: ## Build the local bralectl binary
	@mkdir -p "$(dir $(BRALECTL_BIN))"
	@if command -v go >/dev/null 2>&1; then \
		if $(GO_BUILD_ENV) go build $(GO_BUILD_FLAGS) -o "$(BRALECTL_BIN)" ./cmd/bralectl; then \
			exit 0; \
		fi; \
		echo "[WARN] local go build failed, building bralectl in Docker"; \
	else \
		echo "[INFO] go not found, building bralectl in Docker"; \
	fi; \
	$(MAKE) bralectl-builder-image; \
	docker run --rm \
		-e GOPROXY="$(GOPROXY)" \
		-e GOSUMDB="$(GOSUMDB)" \
		-e CGO_ENABLED=0 \
		-e GOOS="$(GO_BUILD_GOOS)" \
		-e GOARCH="$(GO_BUILD_GOARCH)" \
		-v "$(CURDIR):/src" \
		-v "$(abspath $(dir $(BRALECTL_BIN))):/out" \
		-w /src \
		"$(BRALECTL_DOCKER_IMAGE)" \
		go build $(GO_BUILD_FLAGS) -o "/out/$(notdir $(BRALECTL_BIN))" ./cmd/bralectl

install-bralectl: bralectl-build ## Build latest bralectl and install it into a PATH directory
	@set -e; \
	mkdir -p "$(BRALECTL_INSTALL_DIR)"; \
	tmp_bin="$$(mktemp "$(BRALECTL_INSTALL_DIR)/.bralectl.tmp.XXXXXX")"; \
	trap 'rm -f "$$tmp_bin"' EXIT; \
	cp -f "$(BRALECTL_BIN)" "$$tmp_bin"; \
	chmod 755 "$$tmp_bin"; \
	mv -f "$$tmp_bin" "$(BRALECTL_INSTALL_BIN)"; \
	echo "[OK] installed bralectl to $(BRALECTL_INSTALL_BIN)"; \
	case ":$$PATH:" in \
		*:"$(BRALECTL_INSTALL_DIR)":*) ;; \
		*) \
			echo "[WARN] $(BRALECTL_INSTALL_DIR) is not in PATH"; \
			echo "[WARN] add it to PATH or invoke $(BRALECTL_INSTALL_BIN) directly"; \
			;; \
	esac

bralectl-builder-image:
	@docker build --target bralectl-builder -f "$(CURDIR)/Dockerfile" -t "$(BRALECTL_DOCKER_IMAGE)" "$(CURDIR)"

add-symbol:
	@if [ -z "$(SYMBOL)" ]; then \
		echo "[ERR] SYMBOL is required. Usage: make add-symbol SYMBOL=XAGUSDT [TEMPLATE_SYMBOL=ETHUSDT] [DRY_RUN=1]"; \
		exit 1; \
	fi
	@set -e; \
	extra_args=""; \
	if [ -n "$(TEMPLATE_SYMBOL)" ]; then \
		extra_args="$$extra_args --template-symbol $(TEMPLATE_SYMBOL)"; \
	fi; \
	if [ -n "$(DRY_RUN)" ]; then \
		extra_args="$$extra_args --dry-run"; \
	fi; \
	if command -v go >/dev/null 2>&1; then \
		mkdir -p "$(dir $(BRALECTL_BIN))"; \
		$(GO_BUILD_ENV) go build $(GO_BUILD_FLAGS) -o "$(BRALECTL_BIN)" ./cmd/bralectl; \
		"$(BRALECTL_BIN)" add-symbol "$(SYMBOL)" --repo "$(CURDIR)" $$extra_args; \
	else \
		tty_args="-i"; \
		if [ -t 0 ] && [ -t 1 ]; then tty_args="-it"; fi; \
		echo "[INFO] go not found, running bralectl in Docker"; \
		docker build --target builder -f "$(CURDIR)/Dockerfile" -t "$(BRALECTL_DOCKER_IMAGE)" "$(CURDIR)"; \
		docker run --rm $$tty_args \
			-e GOPROXY="${GOPROXY}" \
			-e GOSUMDB="${GOSUMDB}" \
			-v "$(CURDIR):/src" \
			-w /src \
			"$(BRALECTL_DOCKER_IMAGE)" \
			go run ./cmd/bralectl add-symbol "$(SYMBOL)" --repo /src $$extra_args; \
	fi

llm-probe:
	@if command -v go >/dev/null 2>&1; then \
		go run ./cmd/bralectl llm probe --repo "$(CURDIR)" $(if $(STAGE),--stage $(STAGE),); \
	else \
		echo "[ERR] go command not found"; \
		exit 1; \
	fi

migrate-up: ## Run database migrations
	@if command -v go >/dev/null 2>&1; then \
		go run ./cmd/brale-core -migrate-up; \
	else \
		echo "[INFO] go not found, running migrations in Docker"; \
		$(COMPOSE) up -d timescaledb >/dev/null; \
		$(COMPOSE) run --rm --no-deps brale -migrate-up; \
	fi

# ======================================================================
# E2E Testing — Isolated stack for integration/lifecycle tests
# ======================================================================
# Usage:
#   make e2e-start  PROFILE=quick-lifecycle
#   make e2e-test   PROFILE=quick-lifecycle E2E_SUITE=ctl
#   make e2e-stop   PROFILE=quick-lifecycle
#   make e2e-reset  PROFILE=quick-lifecycle
#   make e2e-status PROFILE=quick-lifecycle
# ======================================================================

PROFILE ?= quick-lifecycle
E2E_COMPOSE_PROJECT  = brale-e2e-$(PROFILE)
E2E_BRALE_SYSTEM_IN  = $(CURDIR)/configs/e2e/$(PROFILE)/system.toml
E2E_SYSTEM_CONTAINER = /app/configs/e2e/$(PROFILE)/system.toml
E2E_INDEX_CONTAINER  = /app/configs/e2e/$(PROFILE)/symbols-index.toml
E2E_DATA_ROOT        = $(CURDIR)/data/e2e/$(PROFILE)
E2E_BRALE_DATA       = $(E2E_DATA_ROOT)/brale
E2E_PGDATA           = $(E2E_DATA_ROOT)/pgdata
E2E_FT_RUNTIME       = $(E2E_DATA_ROOT)/freqtrade/user_data
E2E_FT_CONFIG        = $(E2E_FT_RUNTIME)/config.json
E2E_PROXY_ENV        = $(E2E_DATA_ROOT)/freqtrade/proxy.env
E2E_PG_PORT         ?= 15432
E2E_FT_PORT         ?= 18080
E2E_BRALE_PORT      ?= 19991
E2E_MCP_PORT        ?= 18765
E2E_REPORT_DIR      ?= $(E2E_DATA_ROOT)/reports
E2E_SUITE           ?= ctl
E2E_SYMBOL          ?= SOLUSDT
E2E_TIMEOUT         ?= 2h
E2E_FORCE_CLOSE     ?= 30m
E2E_TELEGRAM_NOTIFY ?= 0
E2E_TELEGRAM_TOKEN  ?= $(ENV_NOTIFICATION_TELEGRAM_TOKEN)
E2E_TELEGRAM_CHAT_ID ?= $(ENV_NOTIFICATION_TELEGRAM_CHAT_ID)
E2E_TELEGRAM_NOTIFY_NORM := $(if $(filter 1 true TRUE yes YES on ON,$(E2E_TELEGRAM_NOTIFY)),1,0)
E2E_TEST_NOTIFY_ENV := $(if $(filter 1,$(E2E_TELEGRAM_NOTIFY_NORM)),BRALE_E2E_TELEGRAM_TOKEN="$(E2E_TELEGRAM_TOKEN)" BRALE_E2E_TELEGRAM_CHAT_ID="$(E2E_TELEGRAM_CHAT_ID)",)
E2E_STACK_EXPORTS    = export COMPOSE_PROJECT_NAME="$(E2E_COMPOSE_PROJECT)"; export BRALE_CONFIG_ROOT="$(BRALE_CONFIG_ROOT)"; export BRALE_SYSTEM_PATH_IN_CONTAINER="$(E2E_SYSTEM_CONTAINER)"; export BRALE_SYMBOL_INDEX_PATH_IN_CONTAINER="$(E2E_INDEX_CONTAINER)"; export BRALE_DATA_ROOT="$(E2E_BRALE_DATA)"; export PGDATA_ROOT="$(E2E_PGDATA)"; export FREQTRADE_RUNTIME_ROOT="$(E2E_FT_RUNTIME)"; export FREQTRADE_CONFIG_FILE="$(E2E_FT_CONFIG)"; export STACK_PROXY_ENV_FILE="$(E2E_PROXY_ENV)"; export POSTGRES_HOST_PORT="$(E2E_PG_PORT)"; export FREQTRADE_HOST_PORT="$(E2E_FT_PORT)"; export BRALE_HOST_PORT="$(E2E_BRALE_PORT)"; export MCP_HOST_PORT="$(E2E_MCP_PORT)"; export HOST_UID="$(HOST_UID)"; export HOST_GID="$(HOST_GID)"; export HOST_REPO_ROOT="$(HOST_REPO_ROOT)"; export FREQTRADE_CONFIG_ROOT="$(FREQTRADE_CONFIG_ROOT)";

.PHONY: e2e-start e2e-stop e2e-reset e2e-status e2e-test e2e-logs

e2e-start: bralectl-build ## Start isolated E2E stack
	@echo "─── E2E start (profile=$(PROFILE)) ───"
	@if [ ! -f "$(E2E_BRALE_SYSTEM_IN)" ]; then \
		echo "[ERR] E2E system config not found: $(E2E_BRALE_SYSTEM_IN)"; \
		echo "[ERR] Run: ls configs/e2e/ to see available profiles"; \
		exit 1; \
	fi
	@mkdir -p "$(E2E_BRALE_DATA)" \
		"$(E2E_PGDATA)" \
		"$(E2E_FT_RUNTIME)" \
		"$(E2E_FT_RUNTIME)/backtest_results" \
		"$(E2E_FT_RUNTIME)/data" \
		"$(E2E_FT_RUNTIME)/freqaimodels" \
		"$(E2E_FT_RUNTIME)/hyperopt_results" \
		"$(E2E_FT_RUNTIME)/hyperopts" \
		"$(E2E_FT_RUNTIME)/logs" \
		"$(E2E_FT_RUNTIME)/notebooks" \
		"$(E2E_FT_RUNTIME)/plot" \
		"$(E2E_FT_RUNTIME)/strategies" \
		"$(dir $(E2E_FT_CONFIG))" \
		"$(dir $(E2E_PROXY_ENV))" \
		"$(E2E_REPORT_DIR)"
	@"$(BRALECTL_BIN)" prepare-stack \
		-env-file .env \
		-config-in "$(FREQTRADE_CONFIG_ROOT)/config.base.json" \
		-config-out "$(E2E_FT_CONFIG)" \
		-proxy-env-out "$(E2E_PROXY_ENV)" \
		-system-in "$(E2E_BRALE_SYSTEM_IN)"
	@cp -f "$(FREQTRADE_CONFIG_ROOT)/brale_shared_strategy.py" "$(E2E_FT_RUNTIME)/strategies/BraleSharedStrategy.py"
	@$(MAKE) --no-print-directory _e2e-compose-up

_e2e-compose-up:
	@$(E2E_COMPOSE) up -d --build timescaledb freqtrade
	@echo "[INFO] waiting E2E freqtrade health..."
	@cid=$$($(E2E_COMPOSE) ps -q freqtrade); \
	for i in $$(seq 1 45); do \
		status=$$(docker inspect --format='{{if .State.Health}}{{.State.Health.Status}}{{else}}starting{{end}}' "$$cid" 2>/dev/null || true); \
		if [ "$$status" = "healthy" ]; then \
			echo "[OK] E2E freqtrade healthy"; \
			break; \
		fi; \
		if [ "$$i" = "45" ]; then \
			echo "[ERR] E2E freqtrade did not become healthy"; \
			exit 1; \
		fi; \
		sleep 2; \
	done
	@$(E2E_COMPOSE) up -d --build brale
	@echo "[INFO] waiting E2E brale health..."
	@cid=$$($(E2E_COMPOSE) ps -q brale); \
	for i in $$(seq 1 45); do \
		status=$$(docker inspect --format='{{if .State.Health}}{{.State.Health.Status}}{{else}}starting{{end}}' "$$cid" 2>/dev/null || true); \
		if [ "$$status" = "healthy" ]; then \
			echo "[OK] E2E brale healthy"; \
			break; \
		fi; \
		if [ "$$i" = "45" ]; then \
			echo "[ERR] E2E brale did not become healthy"; \
			$(E2E_COMPOSE) logs --tail=50 brale 2>&1 || true; \
			exit 1; \
		fi; \
		sleep 2; \
	done
	@$(E2E_COMPOSE) --profile mcp up -d --build mcp
	@echo "[INFO] waiting E2E mcp ready..."
	@sleep 3
	@echo "[OK] E2E stack started (profile=$(PROFILE))"
	@echo "     Brale API:     http://127.0.0.1:$(E2E_BRALE_PORT)"
	@echo "     Freqtrade API: http://127.0.0.1:$(E2E_FT_PORT)"
	@echo "     MCP HTTP:      http://127.0.0.1:$(E2E_MCP_PORT)"
	@echo "     PostgreSQL:    127.0.0.1:$(E2E_PG_PORT)"

# Helper: set all E2E env vars for a compose command
define E2E_COMPOSE
$(E2E_STACK_EXPORTS) $(STACK_PROXY_SOURCE) docker compose -f "$(COMPOSE_FILE)"
endef

e2e-stop: ## Stop the E2E stack
	@echo "─── E2E stop (profile=$(PROFILE)) ───"
	@$(E2E_COMPOSE) stop brale freqtrade timescaledb mcp 2>/dev/null || true
	@echo "[OK] E2E stack stopped"

e2e-reset: e2e-stop ## Stop E2E stack and remove all data
	@echo "─── E2E reset (profile=$(PROFILE)) ───"
	@$(E2E_COMPOSE) down --remove-orphans 2>/dev/null || true
	@rm -rf "$(E2E_DATA_ROOT)"
	@echo "[OK] E2E data removed: $(E2E_DATA_ROOT)"

e2e-status: ## Show E2E stack status
	@$(E2E_COMPOSE) ps 2>/dev/null || echo "[INFO] E2E stack not running"

e2e-logs: ## Tail E2E stack logs
	@$(E2E_COMPOSE) logs -f --tail=200 freqtrade brale

e2e-test: bralectl-build ## Run E2E test suite
	@echo "─── E2E test (profile=$(PROFILE) suite=$(E2E_SUITE)) ───"
	@if [ "$(E2E_TELEGRAM_NOTIFY_NORM)" = "1" ] && { [ -z "$(E2E_TELEGRAM_TOKEN)" ] || [ -z "$(E2E_TELEGRAM_CHAT_ID)" ]; }; then \
		echo "[ERR] E2E Telegram notify enabled but token/chat_id missing"; \
		echo "[ERR] Set NOTIFICATION_TELEGRAM_TOKEN / NOTIFICATION_TELEGRAM_CHAT_ID in .env or override E2E_TELEGRAM_TOKEN / E2E_TELEGRAM_CHAT_ID"; \
		exit 1; \
	fi
	@$(E2E_TEST_NOTIFY_ENV) "$(BRALECTL_BIN)" test run \
		--endpoint "http://127.0.0.1:$(E2E_BRALE_PORT)" \
		--profile "$(PROFILE)" \
		--suites "$(E2E_SUITE)" \
		--symbol "$(E2E_SYMBOL)" \
		--timeout "$(E2E_TIMEOUT)" \
		--force-close-after "$(E2E_FORCE_CLOSE)" \
		--report-dir "$(E2E_REPORT_DIR)" \
		--ft-endpoint "http://127.0.0.1:$(E2E_FT_PORT)" \
		--pg-port "$(E2E_PG_PORT)" \
		--mcp-endpoint "http://127.0.0.1:$(E2E_MCP_PORT)"

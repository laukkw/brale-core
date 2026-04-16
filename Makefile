SHELL := /bin/bash

COMPOSE_FILE ?= docker-compose.yml
COMPOSE_PROJECT_NAME ?= brale-core
ENABLE_MCP ?= 0
SETUP_LANG ?=
SETUP_ARGS ?=

BRALE_CONFIG_ROOT ?= $(CURDIR)/configs
BRALE_DATA_ROOT ?= $(CURDIR)/data/brale
PGDATA_ROOT ?= $(CURDIR)/data/pgdata
FREQTRADE_CONFIG_ROOT ?= $(CURDIR)/configs/freqtrade
FREQTRADE_RUNTIME_ROOT ?= $(CURDIR)/data/freqtrade/user_data
FREQTRADE_CONFIG_FILE ?= $(FREQTRADE_RUNTIME_ROOT)/config.json
STACK_PROXY_ENV_FILE ?= $(CURDIR)/data/freqtrade/proxy.env
HOST_UID ?= $(shell id -u)
HOST_GID ?= $(shell id -g)
HOST_REPO_ROOT ?= $(CURDIR)
BRALECTL_BIN ?= $(BRALE_DATA_ROOT)/bin/bralectl
OUTPUT_ROOT ?= $(CURDIR)/_output
BRALECTL_OUTPUT_BIN ?= $(OUTPUT_ROOT)/bralectl
BRALECTL_DOCKER_IMAGE ?= brale-core-go-builder
TEMPLATE_SYMBOL ?=

ENABLE_MCP_NORM := $(if $(filter 1 true TRUE yes YES on ON,$(ENABLE_MCP)),1,0)
OPTIONAL_STACK_SERVICES := $(if $(filter 1,$(ENABLE_MCP_NORM)),mcp-sse,)
REBUILD_SERVICES := brale $(if $(filter 1,$(ENABLE_MCP_NORM)),mcp-sse,)

STACK_EXPORTS = export HOST_UID="$(HOST_UID)"; export HOST_GID="$(HOST_GID)"; export COMPOSE_PROJECT_NAME="$(COMPOSE_PROJECT_NAME)"; export HOST_REPO_ROOT="$(HOST_REPO_ROOT)"; export BRALE_CONFIG_ROOT="$(BRALE_CONFIG_ROOT)"; export BRALE_DATA_ROOT="$(BRALE_DATA_ROOT)"; export PGDATA_ROOT="$(PGDATA_ROOT)"; export FREQTRADE_CONFIG_ROOT="$(FREQTRADE_CONFIG_ROOT)"; export FREQTRADE_RUNTIME_ROOT="$(FREQTRADE_RUNTIME_ROOT)"; export FREQTRADE_CONFIG_FILE="$(FREQTRADE_CONFIG_FILE)"; export STACK_PROXY_ENV_FILE="$(STACK_PROXY_ENV_FILE)";
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
			value="$${BASH_REMATCH[2]}"; \
			if printf '%s\n' "$$value" | grep -q '[`$$]'; then \
				echo "[ERR] proxy env values must be literal in $$STACK_PROXY_ENV_FILE"; \
				exit 1; \
			fi; \
			export "$${BASH_REMATCH[1]}=$$value"; \
		else \
			echo "[ERR] invalid proxy env entry in $$STACK_PROXY_ENV_FILE: $$trimmed"; \
			exit 1; \
		fi; \
	done < "$$STACK_PROXY_ENV_FILE"; \
fi;
COMPOSE = $(STACK_EXPORTS) $(STACK_PROXY_SOURCE) docker compose -f "$(COMPOSE_FILE)"

# PREPARE_STACK runs the prepare-stack logic locally (via Go) or in Docker.
PREPARE_STACK_ARGS = -env-file .env -config-in "$(FREQTRADE_CONFIG_ROOT)/config.base.json" -config-out "$(FREQTRADE_CONFIG_FILE)" -proxy-env-out "$(STACK_PROXY_ENV_FILE)" -system-in "$(BRALE_CONFIG_ROOT)/system.toml"

.PHONY: help env-init setup init check prepare start apply-config start-freqtrade wait-freqtrade start-brale mcp-start mcp-stop mcp-logs stop-freqtrade stop-brale stop restart rebuild down status logs build bralectl-build bralectl-builder-image add-symbol llm-probe migrate-up migrate-down

help: ## Show the main make targets and optional component switches
	@printf '%-22s %s\n' "env-init" "Create .env from .env.example if missing"; \
	printf '%-22s %s\n' "build" "Build bralectl into _output/bralectl"; \
	printf '%-22s %s\n' "setup" "Run bralectl setup (env init + optional MCP client config)"; \
	printf '%-22s %s\n' "init" "Interactive CLI wizard: configure .env + generate runtime configs"; \
	printf '%-22s %s\n' "start" "Start freqtrade + brale; add ENABLE_MCP=1 for MCP service"; \
	printf '%-22s %s\n' "mcp-start" "Start only the MCP SSE service (dependencies auto-start)"; \
	printf '%-22s %s\n' "rebuild" "Rebuild brale (and mcp-sse when ENABLE_MCP=1)"; \
	printf '%-22s %s\n' "stop / down" "Stop services / remove the full compose stack"; \
	printf '%-22s %s\n' "logs / mcp-logs" "Tail core logs / tail MCP logs"; \
	echo ""; \
	echo "Optional switches:"; \
	printf '  %-18s %s\n' "ENABLE_MCP=1" "Include the mcp-sse service"; \
	printf '  %-18s %s\n' "SETUP_LANG=zh|en" "Preselect the setup wizard language"; \
	printf '  %-18s %s\n' "SETUP_ARGS='...'" "Pass extra flags to bralectl setup"

env-init: ## Create .env from .env.example when missing
	@set -e; \
	if [ -f ".env" ]; then \
		echo "[OK] .env already exists"; \
	elif [ -f ".env.example" ]; then \
		cp .env.example .env; \
		echo "[OK] created .env from .env.example"; \
	else \
		echo "[ERR] .env.example not found"; \
		exit 1; \
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
	echo "[INFO] waiting freqtrade health..."; \
	for i in $$(seq 1 45); do \
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

start-brale: ## Start only the brale runtime service
	@$(COMPOSE) up -d brale

mcp-start: check prepare ## Start the MCP SSE service (brings dependencies up as needed)
	@$(COMPOSE) up -d --build mcp-sse

mcp-stop: ## Stop the MCP SSE service
	@$(COMPOSE) stop mcp-sse

mcp-logs: ## Tail MCP SSE logs
	@$(COMPOSE) logs -f --tail=200 mcp-sse

stop-freqtrade:
	@$(COMPOSE) stop freqtrade

stop-brale:
	@$(COMPOSE) stop brale

rebuild: check prepare ## Rebuild brale (and mcp-sse when ENABLE_MCP=1)
	@$(COMPOSE) up -d --build $(REBUILD_SERVICES)

stop: ## Stop the running compose services
	@$(COMPOSE) stop brale freqtrade mcp-sse

restart: apply-config

down: ## Remove the compose stack and orphans
	@$(COMPOSE) down --remove-orphans

status: ## Show compose service status
	@$(COMPOSE) ps

logs: ## Tail the core stack logs
	@$(COMPOSE) logs -f --tail=200 freqtrade brale

build: ## Build bralectl into _output/bralectl
	@if ! command -v go >/dev/null 2>&1; then \
		echo "[ERR] go command not found"; \
		exit 1; \
	fi
	@mkdir -p "$(dir $(BRALECTL_OUTPUT_BIN))"
	go build -o "$(BRALECTL_OUTPUT_BIN)" ./cmd/bralectl

bralectl-build: ## Build the local bralectl binary
	@if ! command -v go >/dev/null 2>&1; then \
		echo "[ERR] go command not found"; \
		exit 1; \
	fi
	@mkdir -p "$(dir $(BRALECTL_BIN))"
	go build -o "$(BRALECTL_BIN)" ./cmd/bralectl

bralectl-builder-image:
	@docker build --target builder -f "$(CURDIR)/Dockerfile" -t "$(BRALECTL_DOCKER_IMAGE)" "$(CURDIR)"

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
		go build -o "$(BRALECTL_BIN)" ./cmd/bralectl; \
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

migrate-down: ## Roll back the last database migration
	@if command -v go >/dev/null 2>&1; then \
		go run ./cmd/brale-core -migrate-down; \
	else \
		echo "[INFO] go not found, rolling back migrations in Docker"; \
		$(COMPOSE) up -d timescaledb >/dev/null; \
		$(COMPOSE) run --rm --no-deps brale -migrate-down; \
	fi

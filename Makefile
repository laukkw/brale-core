SHELL := /bin/bash

COMPOSE_FILE ?= docker-compose.yml
COMPOSE_PROJECT_NAME ?= brale-core
ENABLE_MCP ?= 0
ENABLE_ONBOARDING ?= 0
SETUP_LANG ?=
SETUP_ARGS ?=

BRALE_CONFIG_ROOT ?= $(CURDIR)/configs
BRALE_DATA_ROOT ?= $(CURDIR)/data/brale
FREQTRADE_CONFIG_ROOT ?= $(CURDIR)/configs/freqtrade
FREQTRADE_RUNTIME_ROOT ?= $(CURDIR)/data/freqtrade/user_data
FREQTRADE_CONFIG_FILE ?= $(FREQTRADE_RUNTIME_ROOT)/config.json
STACK_PROXY_ENV_FILE ?= $(CURDIR)/data/freqtrade/proxy.env
HOST_UID ?= $(shell id -u)
HOST_GID ?= $(shell id -g)
HOST_REPO_ROOT ?= $(CURDIR)
ONBOARDING_ADDR ?= 127.0.0.1:9992
ONBOARDING_URL ?= http://$(ONBOARDING_ADDR)
BRALECTL_BIN ?= $(BRALE_DATA_ROOT)/bin/bralectl
BRALECTL_DOCKER_IMAGE ?= brale-core-go-builder
TEMPLATE_SYMBOL ?=

ENABLE_MCP_NORM := $(if $(filter 1 true TRUE yes YES on ON,$(ENABLE_MCP)),1,0)
ENABLE_ONBOARDING_NORM := $(if $(filter 1 true TRUE yes YES on ON,$(ENABLE_ONBOARDING)),1,0)
OPTIONAL_STACK_SERVICES := $(if $(filter 1,$(ENABLE_MCP_NORM)),mcp-sse,) $(if $(filter 1,$(ENABLE_ONBOARDING_NORM)),onboarding,)
REBUILD_SERVICES := brale $(if $(filter 1,$(ENABLE_MCP_NORM)),mcp-sse,)

COMPOSE = HOST_UID="$(HOST_UID)" HOST_GID="$(HOST_GID)" COMPOSE_PROJECT_NAME="$(COMPOSE_PROJECT_NAME)" docker compose -f "$(COMPOSE_FILE)"
STACK_ENV = HOST_UID="$(HOST_UID)" HOST_GID="$(HOST_GID)" HOST_REPO_ROOT="$(HOST_REPO_ROOT)" BRALE_CONFIG_ROOT="$(BRALE_CONFIG_ROOT)" BRALE_DATA_ROOT="$(BRALE_DATA_ROOT)" FREQTRADE_CONFIG_ROOT="$(FREQTRADE_CONFIG_ROOT)" FREQTRADE_RUNTIME_ROOT="$(FREQTRADE_RUNTIME_ROOT)" FREQTRADE_CONFIG_FILE="$(FREQTRADE_CONFIG_FILE)" STACK_PROXY_ENV_FILE="$(STACK_PROXY_ENV_FILE)"
ONBOARDING_PREPARE = $(STACK_ENV) $(COMPOSE) run --rm --no-deps onboarding prepare-stack

.PHONY: help env-init setup init init-stop init-status init-logs check prepare start apply-config onboarding-start onboarding-pull onboarding-refresh-brale start-freqtrade wait-freqtrade start-brale mcp-start mcp-stop mcp-logs stop-freqtrade stop-brale stop restart rebuild down status logs bralectl-build bralectl-builder-image add-symbol llm-probe migrate-up migrate-down

help: ## Show the main make targets and optional component switches
	@printf '%-22s %s\n' "env-init" "Create .env from .env.example if missing"; \
	printf '%-22s %s\n' "setup" "Run bralectl setup (env init + optional MCP client config)"; \
	printf '%-22s %s\n' "init" "Start the onboarding UI only"; \
	printf '%-22s %s\n' "start" "Start freqtrade + brale; add ENABLE_MCP=1 and/or ENABLE_ONBOARDING=1"; \
	printf '%-22s %s\n' "mcp-start" "Start only the MCP SSE service (dependencies auto-start)"; \
	printf '%-22s %s\n' "rebuild" "Rebuild brale (and mcp-sse when ENABLE_MCP=1)"; \
	printf '%-22s %s\n' "stop / down" "Stop services / remove the full compose stack"; \
	printf '%-22s %s\n' "logs / mcp-logs" "Tail core logs / tail MCP logs"; \
	echo ""; \
	echo "Optional switches:"; \
	printf '  %-18s %s\n' "ENABLE_MCP=1" "Include the mcp-sse service"; \
	printf '  %-18s %s\n' "ENABLE_ONBOARDING=1" "Keep onboarding UI running with the stack"; \
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

init: ## Start the onboarding UI only
	@set -e; \
	if ! command -v docker >/dev/null 2>&1; then \
		echo "[ERR] docker command not found. Please install Docker first."; \
		echo "[TIP] macOS: ./scripts/install_docker_mac.sh"; \
		echo "[TIP] Linux: ./scripts/install_docker_linux.sh"; \
		exit 1; \
	fi; \
	if ! docker compose version >/dev/null 2>&1; then \
		echo "[ERR] docker compose command not found. Please install Docker Compose first."; \
		echo "[TIP] macOS: install/start Docker Desktop, then run: docker compose version"; \
		echo "[TIP] Linux: ./scripts/install_docker_linux.sh"; \
		echo "[TIP] Verify: docker compose version"; \
		exit 1; \
	fi; \
	if ! docker info >/dev/null 2>&1; then \
		echo "[ERR] Docker daemon is not running. Please start Docker Desktop / dockerd first."; \
		exit 1; \
	fi; \
	status_json="$$(curl -fsS "$(ONBOARDING_URL)/api/status" 2>/dev/null || true)"; \
	if [ -n "$$status_json" ]; then \
		echo "[OK] Onboarding already running at $(ONBOARDING_URL)"; \
		echo "[OPEN] $(ONBOARDING_URL)"; \
		exit 0; \
	fi; \
	host="$(ONBOARDING_ADDR)"; host="$${host%:*}"; \
	port="$(ONBOARDING_ADDR)"; port="$${port##*:}"; \
	if [ -z "$$host" ]; then host="127.0.0.1"; fi; \
	if (echo >"/dev/tcp/$$host/$$port") >/dev/null 2>&1; then \
		echo "[ERR] port $$host:$$port is already in use by another process"; \
		echo "[TIP] free this port or run: make init ONBOARDING_ADDR=127.0.0.1:9993"; \
		exit 1; \
	fi; \
	echo "[OK] Docker is ready"; \
	echo "[INFO] Starting onboarding container at $(ONBOARDING_URL)"; \
	$(STACK_ENV) $(COMPOSE) up -d --build onboarding; \
	for i in $$(seq 1 60); do \
		if curl -fsS "$(ONBOARDING_URL)/api/status" >/dev/null 2>&1; then \
			echo "[OK] onboarding running at $(ONBOARDING_URL)"; \
			echo "[OPEN] $(ONBOARDING_URL)"; \
			exit 0; \
		fi; \
		sleep 1; \
	done; \
	echo "[ERR] onboarding did not become ready in time"; \
	$(STACK_ENV) $(COMPOSE) logs --tail=200 onboarding; \
	exit 1

init-stop: ## Stop the onboarding UI container
	@set -e; \
	$(STACK_ENV) $(COMPOSE) stop onboarding >/dev/null 2>&1 || true; \
	echo "[OK] stopped onboarding container"

init-status: ## Show onboarding UI status
	@set -e; \
	if curl -fsS "$(ONBOARDING_URL)/api/status" >/dev/null 2>&1; then \
		echo "[OK] onboarding running at $(ONBOARDING_URL)"; \
		$(STACK_ENV) $(COMPOSE) ps onboarding; \
		exit 0; \
	fi; \
	echo "[INFO] onboarding not running"

init-logs: ## Tail onboarding UI logs
	@$(STACK_ENV) $(COMPOSE) logs -f --tail=200 onboarding

check:
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
	@$(ONBOARDING_PREPARE) --env-file .env --config-in "$(FREQTRADE_CONFIG_ROOT)/config.base.json" --config-out "$(FREQTRADE_CONFIG_FILE)" --proxy-env-out "$(STACK_PROXY_ENV_FILE)" --system-in "$(BRALE_CONFIG_ROOT)/system.toml" --check-only

prepare:
	@mkdir -p "$(BRALE_DATA_ROOT)" \
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
	@$(ONBOARDING_PREPARE) --env-file .env --config-in "$(FREQTRADE_CONFIG_ROOT)/config.base.json" --config-out "$(FREQTRADE_CONFIG_FILE)" --proxy-env-out "$(STACK_PROXY_ENV_FILE)" --system-in "$(BRALE_CONFIG_ROOT)/system.toml"
	@cp -f "$(FREQTRADE_CONFIG_ROOT)/brale_shared_strategy.py" "$(FREQTRADE_RUNTIME_ROOT)/strategies/BraleSharedStrategy.py"
	@if [ "$$(id -u)" = "0" ] && [ -n "$(HOST_UID)" ] && [ -n "$(HOST_GID)" ]; then \
		chown -R "$(HOST_UID):$(HOST_GID)" "$(BRALE_DATA_ROOT)" "$(FREQTRADE_RUNTIME_ROOT)" "$(dir $(STACK_PROXY_ENV_FILE))"; \
	fi

start: check prepare ## Start the core stack; use ENABLE_MCP=1 and/or ENABLE_ONBOARDING=1 for optional services
	@$(MAKE) start-freqtrade
	@$(MAKE) wait-freqtrade
	@$(STACK_ENV) $(COMPOSE) up -d --build brale $(OPTIONAL_STACK_SERVICES)

apply-config: check prepare stop ## Regenerate configs and restart the stack
	@$(MAKE) start ENABLE_MCP="$(ENABLE_MCP)" ENABLE_ONBOARDING="$(ENABLE_ONBOARDING)"

onboarding-start: apply-config

onboarding-pull:
	@$(STACK_ENV) $(COMPOSE) pull freqtrade

onboarding-refresh-brale:
	@echo "[INFO] onboarding-refresh-brale: start"
	@set -e; \
	if [ -n "$$(git status --porcelain)" ]; then \
		echo "[WARN] 检测到本地修改，跳过 git pull，直接执行 make rebuild。"; \
	else \
		echo "[INFO] 工作区干净，开始拉取 brale-core 最新代码..."; \
		git pull --ff-only --no-rebase; \
	fi
	@echo "[INFO] 开始执行 make rebuild..."
	@$(MAKE) rebuild
	@echo "[OK] onboarding-refresh-brale: done"

start-freqtrade: ## Start the freqtrade service
	@$(STACK_ENV) $(COMPOSE) up -d --build freqtrade

wait-freqtrade: ## Wait for freqtrade health to turn green
	@ \
	cid=$$($(STACK_ENV) $(COMPOSE) ps -q freqtrade); \
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
	echo "[ERR] freqtrade did not become healthy in time"; \
	exit 1

start-brale: ## Start only the brale runtime service
	@$(STACK_ENV) $(COMPOSE) up -d brale

mcp-start: check prepare ## Start the MCP SSE service (brings dependencies up as needed)
	@$(STACK_ENV) $(COMPOSE) up -d --build mcp-sse

mcp-stop: ## Stop the MCP SSE service
	@$(STACK_ENV) $(COMPOSE) stop mcp-sse

mcp-logs: ## Tail MCP SSE logs
	@$(STACK_ENV) $(COMPOSE) logs -f --tail=200 mcp-sse

stop-freqtrade:
	@$(STACK_ENV) $(COMPOSE) stop freqtrade

stop-brale:
	@$(STACK_ENV) $(COMPOSE) stop brale

rebuild: check prepare ## Rebuild brale (and mcp-sse when ENABLE_MCP=1)
	@$(STACK_ENV) $(COMPOSE) up -d --build $(REBUILD_SERVICES)

stop: ## Stop the running compose services
	@$(STACK_ENV) $(COMPOSE) stop brale freqtrade mcp-sse onboarding

restart: apply-config

down: ## Remove the compose stack and orphans
	@$(STACK_ENV) $(COMPOSE) down --remove-orphans

status: ## Show compose service status
	@$(STACK_ENV) $(COMPOSE) ps

logs: ## Tail the core stack logs
	@$(STACK_ENV) $(COMPOSE) logs -f --tail=200 freqtrade brale

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
	@go run ./cmd/brale-core -migrate-up

migrate-down: ## Roll back the last database migration
	@go run ./cmd/brale-core -migrate-down

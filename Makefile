SHELL := /bin/bash

COMPOSE_FILE ?= docker-compose.yml
COMPOSE_PROJECT_NAME ?= brale-core

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
ONBOARDING_PID_FILE ?= $(BRALE_DATA_ROOT)/onboarding.pid
ONBOARDING_LOG_FILE ?= $(FREQTRADE_RUNTIME_ROOT)/logs/onboarding.log
ONBOARDING_BIN ?= $(BRALE_DATA_ROOT)/bin/onboarding

COMPOSE = HOST_UID="$(HOST_UID)" HOST_GID="$(HOST_GID)" COMPOSE_PROJECT_NAME="$(COMPOSE_PROJECT_NAME)" docker compose -f "$(COMPOSE_FILE)"
INIT_COMPOSE = HOST_UID="$(HOST_UID)" HOST_GID="$(HOST_GID)" COMPOSE_PROJECT_NAME="$(COMPOSE_PROJECT_NAME)" docker compose -f "$(COMPOSE_FILE)"
STACK_ENV = HOST_UID="$(HOST_UID)" HOST_GID="$(HOST_GID)" HOST_REPO_ROOT="$(HOST_REPO_ROOT)" BRALE_CONFIG_ROOT="$(BRALE_CONFIG_ROOT)" BRALE_DATA_ROOT="$(BRALE_DATA_ROOT)" FREQTRADE_CONFIG_ROOT="$(FREQTRADE_CONFIG_ROOT)" FREQTRADE_RUNTIME_ROOT="$(FREQTRADE_RUNTIME_ROOT)" FREQTRADE_CONFIG_FILE="$(FREQTRADE_CONFIG_FILE)" STACK_PROXY_ENV_FILE="$(STACK_PROXY_ENV_FILE)"
ONBOARDING_PREPARE = $(STACK_ENV) $(COMPOSE) run --rm --no-deps onboarding prepare-stack

.PHONY: init init-stop init-status init-logs check prepare start apply-config onboarding-start onboarding-pull onboarding-refresh-brale start-freqtrade wait-freqtrade start-brale stop-freqtrade stop-brale stop restart rebuild down status logs

init:
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
	mkdir -p "$(dir $(ONBOARDING_PID_FILE))" "$(dir $(ONBOARDING_LOG_FILE))"; \
	mkdir -p "$(dir $(ONBOARDING_BIN))"; \
	echo "[OK] Docker is ready"; \
	echo "[INFO] Starting onboarding container at $(ONBOARDING_URL)"; \
	HOST_REPO_ROOT="$(CURDIR)" $(INIT_COMPOSE) up -d --build onboarding; \
	for i in $$(seq 1 60); do \
		if curl -fsS "$(ONBOARDING_URL)/api/status" >/dev/null 2>&1; then \
			echo "[OK] onboarding running at $(ONBOARDING_URL)"; \
			echo "[OPEN] $(ONBOARDING_URL)"; \
			exit 0; \
		fi; \
		sleep 1; \
	done; \
	echo "[ERR] onboarding did not become ready in time"; \
	$(INIT_COMPOSE) logs --tail=200 onboarding; \
	exit 1

init-stop:
	@set -e; \
	$(INIT_COMPOSE) stop onboarding >/dev/null 2>&1 || true; \
	echo "[OK] stopped onboarding container"

init-status:
	@set -e; \
	if curl -fsS "$(ONBOARDING_URL)/api/status" >/dev/null 2>&1; then \
		echo "[OK] onboarding running at $(ONBOARDING_URL)"; \
		$(INIT_COMPOSE) ps onboarding; \
		exit 0; \
	fi; \
	echo "[INFO] onboarding not running"

init-logs:
	@$(INIT_COMPOSE) logs -f --tail=200 onboarding

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

start: check prepare start-freqtrade wait-freqtrade start-brale

apply-config: check prepare stop start-freqtrade wait-freqtrade start-brale

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

start-freqtrade:
	@$(STACK_ENV) $(COMPOSE) up -d --build freqtrade

wait-freqtrade:
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

start-brale:
	@$(STACK_ENV) $(COMPOSE) up -d brale

stop-freqtrade:
	@$(STACK_ENV) $(COMPOSE) stop freqtrade

stop-brale:
	@$(STACK_ENV) $(COMPOSE) stop brale

rebuild: check prepare
	@$(STACK_ENV) $(COMPOSE) up -d --build brale

stop:
	@$(STACK_ENV) $(COMPOSE) stop brale freqtrade

restart: start

down:
	@$(STACK_ENV) $(COMPOSE) down --remove-orphans

status:
	@$(STACK_ENV) $(COMPOSE) ps

logs:
	@$(STACK_ENV) $(COMPOSE) logs -f --tail=200 freqtrade brale

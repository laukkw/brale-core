SHELL := /bin/bash

COMPOSE_FILE ?= docker-compose.yml
PROJECT_NAME ?= brale-stack

BRALE_CONFIG_ROOT ?= $(CURDIR)/configs
BRALE_DATA_ROOT ?= $(CURDIR)/data/brale
BRALE_SYSTEM_FILE ?= $(BRALE_DATA_ROOT)/system.toml
FREQTRADE_CONFIG_ROOT ?= $(CURDIR)/configs/freqtrade
FREQTRADE_RUNTIME_ROOT ?= $(CURDIR)/data/freqtrade/user_data
FREQTRADE_CONFIG_FILE ?= $(FREQTRADE_RUNTIME_ROOT)/config.json
STACK_PROXY_ENV_FILE ?= $(CURDIR)/data/freqtrade/proxy.env
ONBOARDING_ADDR ?= 127.0.0.1:9992
ONBOARDING_URL ?= http://$(ONBOARDING_ADDR)
ONBOARDING_PID_FILE ?= $(BRALE_DATA_ROOT)/onboarding.pid
ONBOARDING_LOG_FILE ?= $(FREQTRADE_RUNTIME_ROOT)/logs/onboarding.log
ONBOARDING_BIN ?= $(BRALE_DATA_ROOT)/bin/onboarding

COMPOSE = docker compose -p "$(PROJECT_NAME)" -f "$(COMPOSE_FILE)" --env-file ".env"
STACK_ENV = BRALE_CONFIG_ROOT="$(BRALE_CONFIG_ROOT)" BRALE_DATA_ROOT="$(BRALE_DATA_ROOT)" BRALE_SYSTEM_FILE="$(BRALE_SYSTEM_FILE)" FREQTRADE_CONFIG_ROOT="$(FREQTRADE_CONFIG_ROOT)" FREQTRADE_RUNTIME_ROOT="$(FREQTRADE_RUNTIME_ROOT)" FREQTRADE_CONFIG_FILE="$(FREQTRADE_CONFIG_FILE)" STACK_PROXY_ENV_FILE="$(STACK_PROXY_ENV_FILE)"

.PHONY: init init-stop init-status init-logs check prepare start onboarding-start onboarding-pull onboarding-refresh-brale start-freqtrade wait-freqtrade start-brale stop-freqtrade stop-brale stop restart rebuild down status logs

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
	if [ -n "$$status_json" ] && printf '%s' "$$status_json" | python3 -c 'import json,sys; d=json.load(sys.stdin); sys.exit(0 if isinstance(d, dict) and "ready" in d else 1)' >/dev/null 2>&1; then \
		echo "[OK] Onboarding already running at $(ONBOARDING_URL)"; \
		echo "[OPEN] $(ONBOARDING_URL)"; \
		exit 0; \
	fi; \
	host="$(ONBOARDING_ADDR)"; host="$${host%:*}"; \
	port="$(ONBOARDING_ADDR)"; port="$${port##*:}"; \
	if [ -z "$$host" ]; then host="127.0.0.1"; fi; \
	if python3 -c 'import socket,sys; h=sys.argv[1]; p=int(sys.argv[2]); s=socket.socket(); s.settimeout(0.5); rc=s.connect_ex((h,p)); s.close(); sys.exit(0 if rc==0 else 1)' "$$host" "$$port"; then \
		echo "[ERR] port $$host:$$port is already in use by another process"; \
		echo "[TIP] free this port or run: make init ONBOARDING_ADDR=127.0.0.1:9993"; \
		exit 1; \
	fi; \
	mkdir -p "$(dir $(ONBOARDING_PID_FILE))" "$(dir $(ONBOARDING_LOG_FILE))"; \
	mkdir -p "$(dir $(ONBOARDING_BIN))"; \
	echo "[OK] Docker is ready"; \
	echo "[INFO] Starting onboarding server at $(ONBOARDING_URL)"; \
	echo "[INFO] Running in foreground. Press Ctrl+C to stop."; \
	echo "[OPEN] $(ONBOARDING_URL)"; \
	go build -o "$(ONBOARDING_BIN)" ./cmd/onboarding; \
	exec "$(ONBOARDING_BIN)" -addr $(ONBOARDING_ADDR)

init-stop:
	@set -e; \
	if [ ! -f "$(ONBOARDING_PID_FILE)" ]; then \
		echo "[OK] onboarding is not running (pid file not found)"; \
		exit 0; \
	fi; \
	pid="$$(cat "$(ONBOARDING_PID_FILE)" 2>/dev/null || true)"; \
	if [ -n "$$pid" ] && kill -0 "$$pid" >/dev/null 2>&1; then \
		kill "$$pid"; \
		sleep 1; \
		if kill -0 "$$pid" >/dev/null 2>&1; then kill -9 "$$pid" >/dev/null 2>&1 || true; fi; \
		echo "[OK] stopped onboarding (pid=$$pid)"; \
	else \
		echo "[WARN] stale onboarding pid file"; \
	fi; \
	rm -f "$(ONBOARDING_PID_FILE)"

init-status:
	@set -e; \
	if curl -fsS "$(ONBOARDING_URL)/api/status" >/dev/null 2>&1; then \
		echo "[OK] onboarding running at $(ONBOARDING_URL)"; \
		if [ -f "$(ONBOARDING_PID_FILE)" ]; then \
			echo "[PID] $$(cat "$(ONBOARDING_PID_FILE)" 2>/dev/null || true)"; \
		fi; \
		exit 0; \
	fi; \
	echo "[INFO] onboarding not running"

init-logs:
	@mkdir -p "$(dir $(ONBOARDING_LOG_FILE))"
	@if [ ! -f "$(ONBOARDING_LOG_FILE)" ]; then \
		touch "$(ONBOARDING_LOG_FILE)"; \
	fi
	@tail -f "$(ONBOARDING_LOG_FILE)"

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
	@python3 scripts/prepare_stack.py --env-file .env --config-in "$(FREQTRADE_CONFIG_ROOT)/config.base.json" --config-out "$(FREQTRADE_CONFIG_FILE)" --proxy-env-out "$(STACK_PROXY_ENV_FILE)" --system-in "$(BRALE_CONFIG_ROOT)/system.toml" --system-out "$(BRALE_SYSTEM_FILE)" --check-only

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
	@python3 scripts/prepare_stack.py --env-file .env --config-in "$(FREQTRADE_CONFIG_ROOT)/config.base.json" --config-out "$(FREQTRADE_CONFIG_FILE)" --proxy-env-out "$(STACK_PROXY_ENV_FILE)" --system-in "$(BRALE_CONFIG_ROOT)/system.toml" --system-out "$(BRALE_SYSTEM_FILE)"
	@cp -f "$(FREQTRADE_CONFIG_ROOT)/brale_shared_strategy.py" "$(FREQTRADE_RUNTIME_ROOT)/strategies/BraleSharedStrategy.py"

start: check prepare start-freqtrade wait-freqtrade start-brale

onboarding-start: check prepare onboarding-pull start-freqtrade wait-freqtrade start-brale

onboarding-pull:
	@set -a; if [ -f "$(STACK_PROXY_ENV_FILE)" ]; then . "$(STACK_PROXY_ENV_FILE)"; fi; set +a; $(STACK_ENV) $(COMPOSE) pull freqtrade

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
	@set -a; if [ -f "$(STACK_PROXY_ENV_FILE)" ]; then . "$(STACK_PROXY_ENV_FILE)"; fi; set +a; $(STACK_ENV) $(COMPOSE) up -d freqtrade

wait-freqtrade:
	@set -a; if [ -f "$(STACK_PROXY_ENV_FILE)" ]; then . "$(STACK_PROXY_ENV_FILE)"; fi; set +a; \
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
	@set -a; if [ -f "$(STACK_PROXY_ENV_FILE)" ]; then . "$(STACK_PROXY_ENV_FILE)"; fi; set +a; $(STACK_ENV) $(COMPOSE) up -d brale

stop-freqtrade:
	@set -a; if [ -f "$(STACK_PROXY_ENV_FILE)" ]; then . "$(STACK_PROXY_ENV_FILE)"; fi; set +a; $(STACK_ENV) $(COMPOSE) stop freqtrade

stop-brale:
	@set -a; if [ -f "$(STACK_PROXY_ENV_FILE)" ]; then . "$(STACK_PROXY_ENV_FILE)"; fi; set +a; $(STACK_ENV) $(COMPOSE) stop brale

rebuild: check prepare
	@set -a; if [ -f "$(STACK_PROXY_ENV_FILE)" ]; then . "$(STACK_PROXY_ENV_FILE)"; fi; set +a; $(STACK_ENV) $(COMPOSE) up -d --build brale

stop:
	@set -a; if [ -f "$(STACK_PROXY_ENV_FILE)" ]; then . "$(STACK_PROXY_ENV_FILE)"; fi; set +a; $(STACK_ENV) $(COMPOSE) stop brale freqtrade

restart: start

down:
	@set -a; if [ -f "$(STACK_PROXY_ENV_FILE)" ]; then . "$(STACK_PROXY_ENV_FILE)"; fi; set +a; $(STACK_ENV) $(COMPOSE) down --remove-orphans

status:
	@set -a; if [ -f "$(STACK_PROXY_ENV_FILE)" ]; then . "$(STACK_PROXY_ENV_FILE)"; fi; set +a; $(STACK_ENV) $(COMPOSE) ps

logs:
	@set -a; if [ -f "$(STACK_PROXY_ENV_FILE)" ]; then . "$(STACK_PROXY_ENV_FILE)"; fi; set +a; $(STACK_ENV) $(COMPOSE) logs -f --tail=200 freqtrade brale

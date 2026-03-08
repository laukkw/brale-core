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

COMPOSE = docker compose -p "$(PROJECT_NAME)" -f "$(COMPOSE_FILE)" --env-file ".env"
STACK_ENV = BRALE_CONFIG_ROOT="$(BRALE_CONFIG_ROOT)" BRALE_DATA_ROOT="$(BRALE_DATA_ROOT)" BRALE_SYSTEM_FILE="$(BRALE_SYSTEM_FILE)" FREQTRADE_CONFIG_ROOT="$(FREQTRADE_CONFIG_ROOT)" FREQTRADE_RUNTIME_ROOT="$(FREQTRADE_RUNTIME_ROOT)" FREQTRADE_CONFIG_FILE="$(FREQTRADE_CONFIG_FILE)" STACK_PROXY_ENV_FILE="$(STACK_PROXY_ENV_FILE)"

.PHONY: check prepare start start-freqtrade wait-freqtrade start-brale stop restart rebuild down status logs

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

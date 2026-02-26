SHELL := /bin/bash

COMPOSE_FILE ?= docker-compose.yml
PROJECT_NAME ?= brale-freqtrade
SERVICE ?= freqtrade
FREQTRADE_CONFIG_ROOT ?= $(CURDIR)/configs
FREQTRADE_RUNTIME_ROOT ?= $(CURDIR)/user_data

COMPOSE_BASE = docker compose -p "$(PROJECT_NAME)" -f "$(COMPOSE_FILE)"
COMPOSE = FREQTRADE_CONFIG_ROOT="$(FREQTRADE_CONFIG_ROOT)" FREQTRADE_RUNTIME_ROOT="$(FREQTRADE_RUNTIME_ROOT)" $(COMPOSE_BASE)

.PHONY: start stop restart delete check prepare

start: check prepare
	@$(COMPOSE) up -d $(SERVICE)

stop: check
	@$(COMPOSE) stop $(SERVICE)

restart: check prepare
	@$(COMPOSE) up -d --force-recreate $(SERVICE)

delete: check
	@$(COMPOSE) down --volumes --remove-orphans

check:
	@if [ ! -f "$(COMPOSE_FILE)" ]; then \
		echo "[ERR] compose file not found: $(COMPOSE_FILE)"; \
		exit 1; \
	fi
	@if [ ! -d "$(FREQTRADE_CONFIG_ROOT)" ]; then \
		echo "[ERR] FREQTRADE_CONFIG_ROOT not found: $(FREQTRADE_CONFIG_ROOT)"; \
		exit 1; \
	fi
	@mkdir -p "$(FREQTRADE_RUNTIME_ROOT)"

prepare:
	@if [ ! -f "$(FREQTRADE_CONFIG_ROOT)/config.json" ] && [ -f "$(FREQTRADE_CONFIG_ROOT)/freqtrade-config.json" ]; then \
		cp "$(FREQTRADE_CONFIG_ROOT)/freqtrade-config.json" "$(FREQTRADE_CONFIG_ROOT)/config.json"; \
		echo "[INFO] generated config.json from freqtrade-config.json"; \
	fi
	@if [ ! -f "$(FREQTRADE_CONFIG_ROOT)/brale_shared_strategy.py" ]; then \
		echo "[ERR] strategy file not found: $(FREQTRADE_CONFIG_ROOT)/brale_shared_strategy.py"; \
		exit 1; \
	fi
	@mkdir -p "$(FREQTRADE_RUNTIME_ROOT)/logs" "$(FREQTRADE_RUNTIME_ROOT)/data" "$(FREQTRADE_RUNTIME_ROOT)/backtest_results"

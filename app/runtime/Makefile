# Lyra dev server — build + run the JSON-RPC/HTTP backend.
#
# `make restart` is the headline target: rebuild the binary, stop the old
# server, start the new one. The server MUST run from THIS directory so viper
# finds ./config/config.yaml (config.Load adds "config" as a search path
# relative to cwd) — launching the binary from elsewhere loads the wrong (or
# no) config. Every recipe runs in the Makefile's directory, so that holds.

# PORT must match server.listen in config/config.yaml — used for the health
# probe and the stop-by-port fallback. Override: make restart PORT=17171
PORT   ?= 17171
BIN    := lyra
PKG    := ./cmd/lyra
PID    := lyra.pid
LOG    := lyra.log
HEALTH := http://127.0.0.1:$(PORT)/v2/health

# LYRA_HOME holds the runtime's persistent state; DB is the single SQLite store
# `make reset-db` / `make fresh` drop (dev stage = drop + rebuild, never
# migrate). Only the db + its WAL/SHM sidecars are removed — local-token,
# skills/, plugins/, and any user-edited LYRA.md are preserved.
LYRA_HOME ?= $(HOME)/.lyra
DB        := $(LYRA_HOME)/lyra.db

.DEFAULT_GOAL := help
.PHONY: help build start stop restart status health logs run clean reset-db fresh

help: ## List targets
	@grep -hE '^[a-z-]+:.*?## ' $(MAKEFILE_LIST) | sort | \
		awk -F':.*?## ' '{printf "  \033[36m%-9s\033[0m %s\n", $$1, $$2}'

build: ## Compile the dev binary (./lyra)
	go build -o $(BIN) $(PKG)
	@echo "[make] built ./$(BIN)"

start: ## Launch the server in the background (loads ./config/config.yaml)
	@if [ -f $(PID) ] && kill -0 $$(cat $(PID)) 2>/dev/null; then \
		echo "[make] already running (pid $$(cat $(PID))) — use 'make restart'"; exit 1; \
	fi
	@nohup ./$(BIN) > $(LOG) 2>&1 & echo $$! > $(PID)
	@echo "[make] started pid $$(cat $(PID)), logging to ./$(LOG)"
	@$(MAKE) --no-print-directory health

stop: ## Stop the background server (by pidfile, then by port)
	@if [ -f $(PID) ]; then \
		kill $$(cat $(PID)) 2>/dev/null && echo "[make] stopped pid $$(cat $(PID))" || echo "[make] pidfile stale"; \
		rm -f $(PID); \
	fi
	@pids=$$(lsof -ti tcp:$(PORT) 2>/dev/null); \
	if [ -n "$$pids" ]; then kill $$pids 2>/dev/null && echo "[make] freed port $(PORT)"; fi; \
	true

restart: ## Rebuild, then stop the old server and start the new one
	@$(MAKE) --no-print-directory build
	@$(MAKE) --no-print-directory stop
	@$(MAKE) --no-print-directory start
	@echo "[make] restart complete"

status: ## Report whether the server is running
	@if [ -f $(PID) ] && kill -0 $$(cat $(PID)) 2>/dev/null; then \
		echo "[make] running (pid $$(cat $(PID))) on $(PORT)"; \
	else echo "[make] not running"; fi

health: ## Poll /v2/health until it answers (5s)
	@for i in 1 2 3 4 5; do \
		if out=$$(curl -fsS -m 2 $(HEALTH) 2>/dev/null); then \
			echo "[make] healthy: $$out"; exit 0; \
		fi; \
		sleep 1; \
	done; \
	echo "[make] health check failed after 5s — see ./$(LOG)"; exit 1

logs: ## Tail the server log
	@tail -f $(LOG)

run: build ## Build + run in the foreground (Ctrl-C to stop)
	./$(BIN)

clean: stop ## Stop the server and remove build/runtime artifacts
	rm -f $(BIN) $(PID) $(LOG)
	@echo "[make] cleaned"

reset-db: stop ## Stop the server and drop the SQLite db + log (no migration)
	rm -f $(DB) $(DB)-wal $(DB)-shm $(LOG)
	@echo "[make] dropped $(DB) (+wal/shm) + log — kept token/skills/plugins"

fresh: ## One-key clean slate: rebuild, drop the db + log, start fresh
	@$(MAKE) --no-print-directory build
	@$(MAKE) --no-print-directory reset-db
	@$(MAKE) --no-print-directory start
	@echo "[make] fresh start complete"

SHELL := /bin/bash
COMPOSE := docker compose
APP := cron-weather-app
DB  := postgres

.PHONY: up down restart logs ps build pull app-shell db-shell db-reset clean help

help:
	@echo "Targets:"
	@echo "  make up        - start (build if needed)"
	@echo "  make down      - stop containers"
	@echo "  make restart   - restart app service"
	@echo "  make logs      - follow logs"
	@echo "  make ps        - show containers"
	@echo "  make build     - build images"
	@echo "  make pull      - pull images"
	@echo "  make app-shell - shell into app container"
	@echo "  make db-shell  - psql into postgres"
	@echo "  make db-reset  - FULL WIPE: remove containers + volumes + orphans"
	@echo "  make clean     - alias to db-reset"

up:
	$(COMPOSE) up -d --build

down:
	$(COMPOSE) down

restart:
	$(COMPOSE) restart $(APP)

logs:
	$(COMPOSE) logs -f --tail=200

ps:
	$(COMPOSE) ps

build:
	$(COMPOSE) build --no-cache

pull:
	$(COMPOSE) pull

app-shell:
	$(COMPOSE) exec $(APP) sh

db-shell:
	$(COMPOSE) exec $(DB) psql -U $${PG_USER:-app} -d $${PG_DB:-app}

db-reset:
	$(COMPOSE) down -v --remove-orphans
	docker volume prune -f

clean: db-reset
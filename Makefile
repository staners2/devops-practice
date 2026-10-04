COMPOSE = docker compose --env-file .env -f deploy/compose/compose.yaml

.PHONY: up down migrate smoke logs ps config build

up: build
	$(COMPOSE) up -d

down:
	$(COMPOSE) down

migrate: build
	$(COMPOSE) --profile tools run --rm migrate

smoke: migrate
	$(COMPOSE) --profile tools run --rm smoke
	@api_id=$$($(COMPOSE) ps -q api); \
	worker_id=$$($(COMPOSE) ps -q worker); \
	test -n "$$api_id"; test -n "$$worker_id"; \
	$(COMPOSE) stop -t 10 api worker; \
	test "$$(docker inspect -f '{{.State.ExitCode}}' $$api_id)" = 0; \
	test "$$(docker inspect -f '{{.State.ExitCode}}' $$worker_id)" = 0; \
	$(COMPOSE) start api worker

logs:
	$(COMPOSE) logs -f api worker

ps:
	$(COMPOSE) ps

config:
	$(COMPOSE) config --quiet

build:
	docker build -t devops-practice:module-00 .

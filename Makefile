COMPOSE = docker compose --env-file .env -f deploy/compose/compose.yaml
RUNNER_COMPOSE = docker compose --env-file deploy/runner/.env -f deploy/runner/compose.yaml

.PHONY: up down migrate smoke logs ps config build format-check vet test compose-check ci-local runner-up runner-register runner-status runner-logs notifier-logs runner-down

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

format-check:
	test -z "$$(gofmt -l .)"

vet:
	go vet ./...

test:
	go test -race -cover ./...

compose-check:
	docker compose --env-file .env.example -f deploy/compose/compose.yaml config --quiet
	TELEGRAM_BOT_TOKEN=ci-validation TELEGRAM_CHAT_ID=ci-validation docker compose --env-file deploy/runner/.env.example -f deploy/runner/compose.yaml config --quiet

ci-local: format-check vet test compose-check

runner-up:
	$(RUNNER_COMPOSE) up -d --build

runner-register:
	./scripts/runner/register.sh

runner-status:
	$(RUNNER_COMPOSE) ps
	$(RUNNER_COMPOSE) exec -T runner gitlab-runner verify

runner-logs:
	$(RUNNER_COMPOSE) logs -f runner

notifier-logs:
	$(RUNNER_COMPOSE) logs -f notifier

runner-down:
	$(RUNNER_COMPOSE) down

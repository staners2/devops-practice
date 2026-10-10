COMPOSE = docker compose --env-file .env -f deploy/compose/compose.yaml
NOTIFIER_COMPOSE = docker compose --env-file deploy/notifier/.env -f deploy/notifier/compose.yaml

.PHONY: up down migrate smoke logs ps config build format-check vet test compose-check helm-check gitops-check ci-local notifier-up notifier-logs notifier-down

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
	TELEGRAM_BOT_TOKEN=ci-validation TELEGRAM_CHAT_ID=ci-validation docker compose --env-file deploy/notifier/.env.example -f deploy/notifier/compose.yaml config --quiet

helm-check:
	./scripts/ci/validate-helm.sh

gitops-check:
	GITOPS_REPO_URL="$${GITOPS_REPO_URL:?GITOPS_REPO_URL is required}" \
		./scripts/ci/validate-gitops.sh
	./scripts/ci/tests/promote-development_test.sh
	./scripts/ci/tests/verify-development_test.sh

ci-local: format-check vet test compose-check helm-check

notifier-up:
	$(NOTIFIER_COMPOSE) up -d --build

notifier-logs:
	$(NOTIFIER_COMPOSE) logs -f notifier

notifier-down:
	$(NOTIFIER_COMPOSE) down

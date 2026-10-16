.PHONY: up down reset status fmt hooks hooks-check verify integration

fmt:
	cd backend && gofmt -w cmd internal

hooks:
	chmod +x .githooks/pre-commit .githooks/pre-push
	git config --local core.hooksPath .githooks

hooks-check:
	.githooks/pre-commit

up:
	docker compose up --build --wait --wait-timeout 120

down:
	docker compose down

# Destructive: explicitly run this only to discard the local database.
reset:
	docker compose down --volumes

status:
	docker compose run --rm migrate status

verify:
	docker build --target verify --tag dubious-doubles-verify backend

integration:
	python3 scripts/integration.py

.PHONY: build build-ko push run test clean up down

build:
	docker compose build

build-ko:
	ko build --local ./cmd/bot

push:
	ko build ./cmd/bot

run:
	@if [ -f .env ]; then set -a && . ./.env && set +a; fi && go run ./cmd/bot

test:
	go test ./...

clean:
	rm -f quest.db

up: down
	docker compose up --build -d

down:
	docker compose down

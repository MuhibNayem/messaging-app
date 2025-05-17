.PHONY: build test deploy

build:
	docker-compose build

up:
	docker-compose up -d

down:
	docker-compose down

test:
	go test -v -race ./...

benchmark:
	./scripts/benchmark.sh

deploy:
	./scripts/deploy.sh

migrate:
	go run cmd/migrate/main.go

monitor:
	open http://localhost:3000
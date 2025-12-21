.PHONY: build build-events docker-build-events test deploy

build:
	docker-compose build

build-events:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/events ./cmd/events

docker-build-events:
	docker build -f Dockerfile.events -t messaging-events-service:latest .

up:
	chmod 400 docker/mongodb/mongodb-keyfile
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

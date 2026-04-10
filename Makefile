BINARY_NAME=s3nfsgw
BUILD_DIR=bin
GO=go
DOCKER_COMPOSE=docker compose -f deployments/docker/docker-compose.yml

.PHONY: build test lint clean docker up down integration fmt vet

build:
	$(GO) build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/s3nfsgw

test:
	$(GO) test ./... -v

lint:
	golangci-lint run ./...

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

clean:
	rm -rf $(BUILD_DIR)

docker:
	docker build -t $(BINARY_NAME) -f deployments/docker/Dockerfile .

up:
	$(DOCKER_COMPOSE) up -d

down:
	$(DOCKER_COMPOSE) down

integration:
	$(DOCKER_COMPOSE) -f deployments/docker/docker-compose.test.yml up --abort-on-container-exit

all: fmt vet lint test build

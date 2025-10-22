GO_CMD = CGO_ENABLED=0 GOOS=linux go
PREFIX_SUBSCRIBER = sbsc-
YUGE_SUBSCRIBER_VERSION=$(shell cat ./cmd/yuge_subscriber/version.txt)
# Run tests
.PHONY: test
test:
	@echo "Running tests..."
	$(GO_CMD) test github.com/nus25/yuge/...

# Build subscriber
.PHONY: $(PREFIX_SUBSCRIBER)build
$(PREFIX_SUBSCRIBER)build:
	@echo "Building yuge subscriber Go binary..."
	$(GO_CMD) build -ldflags="-s -w" -trimpath -o bin/yuge_subscriber cmd/yuge_subscriber/*.go

# Run subscriber
.PHONY: $(PREFIX_SUBSCRIBER)run
$(PREFIX_SUBSCRIBER)run: .env
	@echo "Running yuge subscriber..."
	set -a && . $(PWD)/.env && set +a && $(GO_CMD) run ./cmd/yuge_subscriber/... run

# Build subscriber docker image for amd64
.PHONY: $(PREFIX_SUBSCRIBER)build-amd64
$(PREFIX_SUBSCRIBER)build-amd64:
	@echo "Building yuge subscriber docker image for amd64..."
	docker buildx build --platform linux/amd64 -f build/Dockerfile.subscriber -t yuge-subscriber:$(YUGE_SUBSCRIBER_VERSION)-amd64 --load .

# Run subscriber in docker
.PHONY: $(PREFIX_SUBSCRIBER)up
$(PREFIX_SUBSCRIBER)up:
	@echo "Starting yuge subscriber..."
	YUGE_SUBSCRIBER_VERSION=$(YUGE_SUBSCRIBER_VERSION) docker compose -f docker-compose.subscriber.yaml up -d --build

# Stop subscriber in docker
.PHONY: $(PREFIX_SUBSCRIBER)down
$(PREFIX_SUBSCRIBER)down:
	@echo "Stopping yuge subscriber..."
	docker compose -f docker-compose.subscriber.yaml down

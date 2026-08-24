set dotenv-load
set default-list

subscriber-test-go-cmd := "CGO_ENABLED=1 go"
subscriber-build-go-cmd := "CGO_ENABLED=1 GOOS=linux go"
subscriber-run-go-cmd := "CGO_ENABLED=1 go"
cli-build-go-cmd := "CGO_ENABLED=0 GOOS=linux go"
yuge-subscriber-version := `cat cmd/yuge_subscriber/version.txt`

test:
    @echo "Running tests..."
    {{ subscriber-test-go-cmd }} test github.com/nus25/yuge/...

sbsc-build:
    @echo "Building yuge subscriber Go binary..."
    {{ subscriber-build-go-cmd }} build -ldflags="-s -w" -trimpath -o bin/yuge_subscriber cmd/yuge_subscriber/*.go

sbsc-run:
    @echo "Running yuge subscriber..."
    {{ subscriber-run-go-cmd }} run ./cmd/yuge_subscriber/... run

sbsc-build-image-amd64:
    @echo "Building yuge subscriber docker image for amd64..."
    docker buildx build --platform linux/amd64 -f build/Dockerfile.subscriber -t yuge-subscriber:{{ yuge-subscriber-version }}-amd64 --load .

sbsc-up:
    @echo "Starting yuge subscriber..."
    YUGE_SUBSCRIBER_VERSION={{ yuge-subscriber-version }} docker compose -f docker-compose.subscriber.yaml up -d --build

sbsc-down:
    @echo "Stopping yuge subscriber..."
    docker compose -f docker-compose.subscriber.yaml down

cli-build:
    @echo "Building yuge CLI Go binary..."
    {{ cli-build-go-cmd }} build -ldflags="-s -w" -trimpath -o bin/yuge cmd/yuge_cli/*.go

APP := cserver
BIN_DIR := bin

.PHONY: build run test test-race fmt vet check clean

build:
	go build -o $(BIN_DIR)/$(APP) ./cmd/cserver

run:
	go run ./cmd/cserver

test:
	go test ./...

test-race:
	go test -race ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

check:
	@test -z "$$(gofmt -l cmd internal)" || \
		(echo "Go files require formatting"; exit 1)
	go vet ./...
	go test ./...
	go build ./cmd/cserver

clean:
	rm -rf $(BIN_DIR)

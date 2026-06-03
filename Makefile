.PHONY: test lint fmt build tidy
test:
	go test ./...
lint:
	golangci-lint run
fmt:
	golangci-lint fmt
build:
	go build ./...
tidy:
	go mod tidy

.PHONY: test build tidy

test:
	CGO_ENABLED=0 go test ./...

build:
	CGO_ENABLED=0 go build -o bin/muxcore-operator ./cmd

tidy:
	go mod tidy

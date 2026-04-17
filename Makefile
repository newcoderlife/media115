.PHONY: build test lint clean install

build:
	go build -o bin/cloud115 ./cmd/cloud115
	go build -o bin/media115 ./cmd/media115

test:
	go test ./... -timeout 120s -count=1 -race -gcflags="all=-l" -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html

lint:
	golangci-lint fmt --diff ./...
	golangci-lint run ./...

clean:
	rm -rf bin/ coverage.out coverage.html

install:
	go install ./cmd/cloud115
	go install ./cmd/media115

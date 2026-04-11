.PHONY: build test test-go test-py clean install

build:
	go build -o bin/cloud115 ./cmd/cloud115
	go build -o bin/media115 ./cmd/media115-go

test-go:
	go test ./... -timeout 10s -count=1

test-py:
	.venv/bin/pytest tests/ --timeout=10 -q

test: test-go test-py

clean:
	rm -rf bin/

install:
	go install ./cmd/cloud115
	go install ./cmd/media115-go

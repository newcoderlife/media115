.PHONY: build test clean install

build:
	go build -o bin/cloud115 ./cmd/cloud115
	go build -o bin/media115 ./cmd/media115

test:
	go test ./... -timeout 120s -count=1 -gcflags="all=-l"

clean:
	rm -rf bin/

install:
	go install ./cmd/cloud115
	go install ./cmd/media115

.PHONY: build test docker clean

BINARY := bin/straumheim
MODULE := github.com/deepskydatahq/straumheim

build:
	CGO_ENABLED=0 go build -o $(BINARY) ./cmd/straumheim

test:
	go test ./...

docker:
	docker build -t straumheim .

clean:
	rm -rf bin/

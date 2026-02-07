.PHONY: build install clean test

VERSION := 0.1.0
LDFLAGS := -ldflags "-X github.com/eshe-huli/pier/internal/cli.Version=$(VERSION)"

build:
	go build $(LDFLAGS) -o bin/pier ./cmd/pier

install: build
	cp bin/pier /usr/local/bin/pier

clean:
	rm -rf bin/

test:
	go test ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

lint: fmt vet

dev: build
	./bin/pier $(ARGS)

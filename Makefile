.PHONY: build test lint verify acceptance

build:
	go build -o bin/mure ./cmd/mure

test:
	go test ./...

lint:
	go vet ./...
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt:"; echo "$$out"; exit 1; fi

acceptance: build
	bash test/acceptance.sh

verify: build lint test acceptance

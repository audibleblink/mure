.PHONY: sync-piext build test lint verify tmux-test

sync-piext:
	rsync -a --delete --exclude=node_modules --exclude=test --exclude=package-lock.json pi-mure/ internal/piext/assets/

build: sync-piext
	go build -o bin/mure ./cmd/mure

test:
	go test ./...
	@if command -v shellcheck >/dev/null 2>&1; then \
		set -e; \
		files=$$(ls tmux-mure/scripts/*.sh 2>/dev/null || true); \
		if [ -f tmux-mure/tmux-mure.tmux ]; then files="$$files tmux-mure/tmux-mure.tmux"; fi; \
		if [ -n "$$files" ]; then shellcheck $$files; fi; \
	else \
		echo "shellcheck not found; skipping"; \
	fi

lint:
	go vet ./...
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt:"; echo "$$out"; exit 1; fi

tmux-test:
	@if ! command -v tmux >/dev/null 2>&1; then \
		echo "tmux not found; skipping tmux-test"; \
	else \
		bash tmux-mure/test/hooks_test.sh; \
	fi

verify: sync-piext build lint test tmux-test

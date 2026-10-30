APP := apps/pocket-youtube-video
BACKEND := $(APP)/backend
FRONTEND := $(APP)/frontend

.PHONY: build test test-unit test-integration dev frontend-deps screenshots verify-screenshots clean

build: frontend-deps
	cd $(BACKEND) && go build ./cmd/videod
	cd $(FRONTEND) && npm run build

test:
	cd $(BACKEND) && go test ./...
	cd $(BACKEND) && go test ./tests -count=1 -v 2>&1 | grep -E '^(=== RUN|--- PASS|--- FAIL|PASS|FAIL|ok)' 

test-unit:
	cd $(BACKEND) && go test ./...

test-integration:
	cd $(BACKEND) && go test ./tests -count=1 -v

frontend-deps:
	cd $(FRONTEND) && npm install --no-audit --no-fund

screenshots:
	node $(APP)/scripts/capture-screenshots.mjs

# Re-check the committed matrix without overwriting it.
verify-screenshots:
	node $(APP)/scripts/capture-screenshots.mjs --verify-only

dev:
	@set -eu; trap 'kill 0' INT TERM EXIT; \
	  (cd $(BACKEND) && go run ./cmd/videod) & \
	  (cd $(FRONTEND) && npm run dev -- --host 127.0.0.1)

clean:
	rm -rf $(FRONTEND)/dist $(BACKEND)/videod

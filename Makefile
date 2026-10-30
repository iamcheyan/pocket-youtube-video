APP := apps/pocket-youtube-video
BACKEND := $(APP)/backend
FRONTEND := $(APP)/frontend

.PHONY: build test dev frontend-deps clean

build: frontend-deps
	cd $(BACKEND) && go build ./cmd/videod
	cd $(FRONTEND) && npm run build

test:
	cd $(BACKEND) && go test ./...

frontend-deps:
	cd $(FRONTEND) && npm install --no-audit --no-fund

dev:
	@set -eu; trap 'kill 0' INT TERM EXIT; \
	  (cd $(BACKEND) && go run ./cmd/videod) & \
	  (cd $(FRONTEND) && npm run dev -- --host 127.0.0.1)

clean:
	rm -rf $(FRONTEND)/dist $(BACKEND)/videod

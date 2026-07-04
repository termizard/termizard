FRONTEND_DIR         := frontend
BINARY               := termizard
NODE_IMAGE           := node:22-alpine
# Named volume keeps node_modules out of the host filesystem.
NODE_MODULES_VOLUME  := termizard-node-modules

.PHONY: all build frontend build-go clean clean-volumes

all: build

## frontend: build the React/xterm.js app inside Docker — no local Node required.
## node_modules are stored in a named Docker volume so they never land on the host.
frontend:
	docker run --rm \
		-v $(NODE_MODULES_VOLUME):/app/node_modules \
		-v "$(CURDIR)/$(FRONTEND_DIR):/app" \
		-w /app \
		$(NODE_IMAGE) \
		sh -c "npm ci && npm run build"

## build: build the Go binary (runs `frontend` first).
build: frontend
	go build -tags production -o $(BINARY) ./cmd/termizard

## build-go: build the Go binary, skipping the frontend step.
## Use this when the dist/ is already up to date.
build-go:
	go build -tags production -o $(BINARY) ./cmd/termizard

## clean: remove the binary, dist/, and any host-local node_modules.
clean:
	rm -f $(BINARY)
	rm -rf $(FRONTEND_DIR)/dist $(FRONTEND_DIR)/node_modules

## clean-volumes: remove the Docker volume used to cache node_modules.
clean-volumes:
	docker volume rm -f $(NODE_MODULES_VOLUME)

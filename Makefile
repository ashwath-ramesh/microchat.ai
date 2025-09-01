# Naming Convention: <env>-<component>-<model>-<options>
# Examples: dev-server, prod-server, dev-client-echo-metrics, prod-client-gemini-detail

# =============================================================================
# SERVER TARGETS
# =============================================================================

# Development server (allows Echo provider)
dev-server:
	cd cmd/server && APP_ENV=development TLS_CERT_FILE=../../certs/server.crt TLS_KEY_FILE=../../certs/server.key go run .

# Production server (no Echo provider access)
prod-server:
	cd cmd/server && APP_ENV=production TLS_CERT_FILE=../../certs/server.crt TLS_KEY_FILE=../../certs/server.key go run .

# =============================================================================
# CLIENT TARGETS
# =============================================================================

# Development clients with Echo model
dev-client-echo:
	cd cmd/client && go run . -model=echo

dev-client-echo-metrics:
	cd cmd/client && go run . -model=echo --metrics

dev-client-echo-detail:
	cd cmd/client && go run . -model=echo --metrics-detail

# Production clients with Gemini model
prod-client-gemini:
	cd cmd/client && go run . -model=gemini

prod-client-gemini-metrics:
	cd cmd/client && go run . -model=gemini --metrics

prod-client-gemini-detail:
	cd cmd/client && go run . -model=gemini --metrics-detail

# =============================================================================
# DEVELOPMENT TOOLS
# =============================================================================

proto:
	protoc --go_out=. --go_opt=paths=source_relative \
	       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
	       proto/chat.proto

test:
	go test -v ./...

build:
	go build ./...

audit:
	go fmt ./...
	go vet ./...
	go mod tidy
	go mod verify
	go test ./...
	go build ./...

# =============================================================================
# PHONY TARGETS
# =============================================================================

.PHONY: dev-server prod-server client \
        dev-client-echo dev-client-echo-metrics dev-client-echo-detail \
        prod-client-gemini prod-client-gemini-metrics prod-client-gemini-detail \
        proto test build audit

# Development-only Makefile for local testing

# =============================================================================
# SERVERS
# =============================================================================

server:
	cd cmd/server && APP_ENV=development TLS_CERT_FILE=../../certs/server.crt TLS_KEY_FILE=../../certs/server.key go run .

# =============================================================================
# CLIENTS  
# =============================================================================

client-echo:
	cd cmd/client && go run . -model=echo

client-gemini:
	cd cmd/client && go run . -model=gemini

client-gemini-metrics:
	cd cmd/client && go run . -model=gemini -metrics

client-gemini-detail:
	cd cmd/client && go run . -model=gemini -metrics-detail

# =============================================================================
# TOOLS
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
	govulncheck ./...
	go test ./...
	go build ./...

# =============================================================================
# PHONY TARGETS
# =============================================================================

.PHONY: server \
        client-echo client-gemini client-gemini-metrics client-gemini-detail \
        proto test build audit

# Development-only Makefile for local testing

# =============================================================================
# SERVERS
# =============================================================================

server:
	cd cmd/server && APP_ENV=development TLS_CERT_FILE=../../certs/server.crt TLS_KEY_FILE=../../certs/server.key go run .

# =============================================================================
# CLIENTS  
# =============================================================================

client:
	cd cmd/client && go run . -model=echo

client-echo:
	cd cmd/client && go run . -model=echo

client-gemini:
	cd cmd/client && go run . -model=gemini

client-gemini-metrics:
	cd cmd/client && go run . -model=gemini -metrics

client-gemini-detail:
	cd cmd/client && go run . -model=gemini -metrics-detail

client-gemini-total:
	cd cmd/client && go run . -model=gemini -metrics-total
# =============================================================================
# ADMIN TOOLS
# =============================================================================

admin-metrics:
	grpcurl -H "Authorization: Bearer admin-key-1" \
		-insecure \
		-d '{}' \
		localhost:4000 \
		chat.ChatService/GetMetrics

# =============================================================================
# PROFILING
# =============================================================================

pprof-cpu:
	@if [ -z "$$ADMIN_KEY" ]; then echo "Error: Set ADMIN_KEY environment variable"; exit 1; fi
	curl -H "Authorization: Bearer $$ADMIN_KEY" -o /tmp/cpu.prof 'http://127.0.0.1:6060/debug/pprof/profile?seconds=30'
	go tool pprof /tmp/cpu.prof

pprof-heap:
	@if [ -z "$$ADMIN_KEY" ]; then echo "Error: Set ADMIN_KEY environment variable"; exit 1; fi
	curl -H "Authorization: Bearer $$ADMIN_KEY" -o /tmp/heap.prof http://127.0.0.1:6060/debug/pprof/heap
	go tool pprof /tmp/heap.prof

pprof-goroutines:
	@if [ -z "$$ADMIN_KEY" ]; then echo "Error: Set ADMIN_KEY environment variable"; exit 1; fi
	curl -H "Authorization: Bearer $$ADMIN_KEY" -o /tmp/goroutine.prof http://127.0.0.1:6060/debug/pprof/goroutine
	go tool pprof /tmp/goroutine.prof

# =============================================================================
# TOOLS
# =============================================================================

proto:
	protoc --go_out=. --go_opt=paths=source_relative \
	       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
	       proto/chat.proto

test:
	go test -v ./...

test-server:
	cd cmd/server && go test -v .

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
        client client-echo client-gemini client-gemini-metrics client-gemini-detail \
        admin-metrics \
        pprof-cpu pprof-heap pprof-goroutines \
        proto test test-server build audit

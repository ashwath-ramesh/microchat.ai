proto:
	protoc --go_out=. --go_opt=paths=source_relative \
	       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
	       proto/chat.proto

server:
	cd cmd/server && go run .

client:
	cd cmd/client && go run .

test-client:
	@echo "Is your server live?"
	cd cmd/client && go test -v

audit:
	@echo "Starting audit ..."
	@echo "Is your server live?"
	go fmt ./...
	go vet ./...
	go mod tidy
	go mod verify
	go test ./...
	go build ./...
	@echo "Audit complete"

.PHONY: proto server client test-client audit

.PHONY: proto
proto:
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		api/proto/dex/v1/dex.proto

.PHONY: build
build:
	go build -o bin/server cmd/server/main.go

.PHONY: run
run:
	go run cmd/server/main.go 
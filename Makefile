.PHONY: proto
proto:
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		api/proto/option/v1/option.proto \
		api/proto/market/v1/market.proto

.PHONY: build
build:
	go build -o bin/server cmd/server/main.go

.PHONY: build-production
build-production:
	mkdir -p bin
	env GOOS=linux GOARCH=386 go build -v -o bin/jax-linux-386 cmd/server/main.go

.PHONY: package-production
package-production: build-production
	mkdir -p package
	tar -zcf package/jax-linux-386.tar.gz bin/jax-linux-386 scripts/

.PHONY: run
run:
	go run cmd/server/main.go 
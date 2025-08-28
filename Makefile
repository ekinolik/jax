# Version information
VERSION_MAJOR = 0
VERSION_MINOR = 1
BUILD_VERSION_FILE = .build-version
VERSION = $(VERSION_MAJOR).$(VERSION_MINOR)

# Directory where the packaged files will be stored
PACKAGE_DIR = package
BUILD_DIR = build_tmp

CURRENT_OS = "linux"
CURRENT_ARCH = "386"

# Get current build number or initialize to 00000
ifeq ($(wildcard $(BUILD_VERSION_FILE)),)
    BUILD_NUMBER = 00000
else
    BUILD_NUMBER = $(shell cat $(BUILD_VERSION_FILE))
endif

# Increment build number function
define increment_build
	$(eval BUILD_NUMBER := $(shell printf "%05d" $$(($(BUILD_NUMBER) + 1))))
    @echo "$(BUILD_NUMBER)" > $(BUILD_VERSION_FILE)
endef

define build-package
	mkdir -p $(BUILD_DIR)/$(CURRENT_OS)
	cp .env.example $(BUILD_DIR)/$(CURRENT_OS)/.env.example
	echo "$(FULL_VERSION)" > $(BUILD_DIR)/$(CURRENT_OS)/VERSION

	mkdir -p $(BUILD_DIR)/$(CURRENT_OS)/certs
	cp scripts/deploy/README-linux.md $(BUILD_DIR)/$(CURRENT_OS)/README.md

	mkdir -p $(BUILD_DIR)/$(CURRENT_OS)/scripts
	cp scripts/*.sh $(BUILD_DIR)/$(CURRENT_OS)/scripts/

	mkdir -p $(BUILD_DIR)/$(CURRENT_OS)/cache
	mkdir -p $(BUILD_DIR)/$(CURRENT_OS)/cache-configs
	cp -r cache-configs/example.yaml $(BUILD_DIR)/$(CURRENT_OS)/cache-configs/



	mkdir -p ${BUILD_DIR}/bin
	env GOOS=${CURRENT_OS} GOARCH=${CURRENT_ARCH} go build -v -o ${BUILD_DIR}/$(CURRENT_OS)/bin/jax-$(CURRENT_OS)-$(CURRENT_ARCH) cmd/server/main.go
endef

# Full version with build number
FULL_VERSION = $(VERSION).$(BUILD_NUMBER)

LINUX_TARBALL = $(PACKAGE_DIR)/jax-$(FULL_VERSION)-linux-x64.tar.gz
DARWIN_TARBALL = $(PACKAGE_DIR)/jax-$(FULL_VERSION)-darwin-arm64.tar.gz

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

.PHONY: start-packaging
start-packaging:
	mkdir -p $(BUILD_DIR)
	mkdir -p $(PACKAGE_DIR)

	$(call increment_build)

.PHONY: package-linux
package-linux:
	# Package for Linux x64
	@echo "Creating Linux x64 package..."

	$(call build-package)

	tar -zcf $(LINUX_TARBALL) -C $(BUILD_DIR) -s /^$(CURRENT_OS)/jax/ $(CURRENT_OS)

.PHONY: package-mac
package-mac:
	# Package for Mac ARM64
	@echo "Creating Mac ARM64 package..."

	$(call build-package)

	tar -zcf $(DARWIN_TARBALL) -C $(BUILD_DIR) -s /^$(CURRENT_OS)/jax/ $(CURRENT_OS)

.PHONY: finish-packaging
finish-packaging:
	rm -rf $(BUILD_DIR)

.PHONY: package-all
package-all: start-packaging
	$(MAKE) package-linux CURRENT_OS=linux CURRENT_ARCH=386
	$(MAKE) package-mac CURRENT_OS=darwin CURRENT_ARCH=arm64
	$(MAKE) finish-packaging
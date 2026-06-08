# Version information
VERSION_MAJOR = 0
VERSION_MINOR = 2
BUILD_VERSION_FILE = .build-version
VERSION = $(VERSION_MAJOR).$(VERSION_MINOR)

# Directory where the packaged files will be stored
PACKAGE_DIR = package
BUILD_DIR = build_tmp

CURRENT_OS = linux
CURRENT_ARCH = amd64

# Pure Go — safe for cross-compilation (no CGO in this project)
GO_BUILD_ENV = CGO_ENABLED=0

# Get current build number or initialize to 00000
ifeq ($(wildcard $(BUILD_VERSION_FILE)),)
    BUILD_NUMBER = 00000
else
    BUILD_NUMBER = $(shell cat $(BUILD_VERSION_FILE))
endif

# Increment build number function (10# prefix avoids octal mis-parse of e.g. 00008)
define increment_build
	$(eval BUILD_NUMBER := $(shell bn='$(BUILD_NUMBER)'; [ -z "$$bn" ] && bn=0; printf "%05d" $$((10#$$bn + 1))))
	@echo "$(BUILD_NUMBER)" > $(BUILD_VERSION_FILE)
endef

define build-package
	rm -rf $(BUILD_DIR)/$(CURRENT_OS)-$(CURRENT_ARCH)
	mkdir -p $(BUILD_DIR)/$(CURRENT_OS)-$(CURRENT_ARCH)/bin
	cp .env.example $(BUILD_DIR)/$(CURRENT_OS)-$(CURRENT_ARCH)/.env.example
	echo "$(FULL_VERSION)" > $(BUILD_DIR)/$(CURRENT_OS)-$(CURRENT_ARCH)/VERSION

	mkdir -p $(BUILD_DIR)/$(CURRENT_OS)-$(CURRENT_ARCH)/certs
	cp scripts/deploy/README-linux.md $(BUILD_DIR)/$(CURRENT_OS)-$(CURRENT_ARCH)/README.md

	mkdir -p $(BUILD_DIR)/$(CURRENT_OS)-$(CURRENT_ARCH)/scripts
	cp scripts/*.sh $(BUILD_DIR)/$(CURRENT_OS)-$(CURRENT_ARCH)/scripts/

	mkdir -p $(BUILD_DIR)/$(CURRENT_OS)-$(CURRENT_ARCH)/cache
	mkdir -p $(BUILD_DIR)/$(CURRENT_OS)-$(CURRENT_ARCH)/cache-configs
	cp -r cache-configs/example.yaml $(BUILD_DIR)/$(CURRENT_OS)-$(CURRENT_ARCH)/cache-configs/

	mkdir -p $(BUILD_DIR)/$(CURRENT_OS)-$(CURRENT_ARCH)/confluence-configs
	cp confluence-configs/*.yaml $(BUILD_DIR)/$(CURRENT_OS)-$(CURRENT_ARCH)/confluence-configs/

	env $(GO_BUILD_ENV) GOOS=$(CURRENT_OS) GOARCH=$(CURRENT_ARCH) go build -v $(VERSION_LDFLAGS) -o $(BUILD_DIR)/$(CURRENT_OS)-$(CURRENT_ARCH)/bin/jax ./cmd/server
	env $(GO_BUILD_ENV) GOOS=$(CURRENT_OS) GOARCH=$(CURRENT_ARCH) go build -v -o $(BUILD_DIR)/$(CURRENT_OS)-$(CURRENT_ARCH)/bin/confluence-test ./cmd/confluence-test
endef

# Full version with build number
FULL_VERSION = $(VERSION).$(BUILD_NUMBER)
VERSION_LDFLAGS = -ldflags "-X github.com/ekinolik/jax/internal/version.Version=$(FULL_VERSION)"

LINUX_AMD64_TARBALL = $(PACKAGE_DIR)/jax-$(FULL_VERSION)-linux-amd64.tar.gz
LINUX_ARM64_TARBALL = $(PACKAGE_DIR)/jax-$(FULL_VERSION)-linux-arm64.tar.gz
# Legacy 32-bit x86 tarball name (kept for backward compatibility)
LINUX_TARBALL = $(PACKAGE_DIR)/jax-$(FULL_VERSION)-linux-x64.tar.gz
DARWIN_TARBALL = $(PACKAGE_DIR)/jax-$(FULL_VERSION)-darwin-arm64.tar.gz

.PHONY: proto
proto:
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		api/proto/option/v1/option.proto \
		api/proto/market/v1/market.proto \
		api/proto/confluence/v1/confluence.proto

.PHONY: bump-version
bump-version:
	$(call increment_build)

.PHONY: build
build: bump-version
	mkdir -p bin
	go build $(VERSION_LDFLAGS) -o bin/server ./cmd/server

.PHONY: confluence-test
confluence-test:
	mkdir -p bin
	go build -o bin/confluence-test ./cmd/confluence-test

.PHONY: build-linux-amd64 build-linux-arm64 build-production build-production-arm64
build-linux-amd64: bump-version
	mkdir -p bin
	env $(GO_BUILD_ENV) GOOS=linux GOARCH=amd64 go build -v $(VERSION_LDFLAGS) -o bin/jax-linux-amd64 ./cmd/server
	env $(GO_BUILD_ENV) GOOS=linux GOARCH=amd64 go build -v -o bin/confluence-test-linux-amd64 ./cmd/confluence-test

build-linux-arm64: bump-version
	mkdir -p bin
	env $(GO_BUILD_ENV) GOOS=linux GOARCH=arm64 go build -v $(VERSION_LDFLAGS) -o bin/jax-linux-arm64 ./cmd/server
	env $(GO_BUILD_ENV) GOOS=linux GOARCH=arm64 go build -v -o bin/confluence-test-linux-arm64 ./cmd/confluence-test

build-production: build-linux-amd64

build-production-arm64: build-linux-arm64

.PHONY: package-production package-production-arm64
package-production: build-production
	rm -rf $(BUILD_DIR)/production-linux-amd64
	mkdir -p $(BUILD_DIR)/production-linux-amd64/bin
	cp bin/jax-linux-amd64 $(BUILD_DIR)/production-linux-amd64/bin/jax
	cp bin/confluence-test-linux-amd64 $(BUILD_DIR)/production-linux-amd64/bin/confluence-test
	cp -r scripts $(BUILD_DIR)/production-linux-amd64/
	mkdir -p package
	tar -zcf package/jax-linux-amd64.tar.gz -C $(BUILD_DIR)/production-linux-amd64 .

package-production-arm64: build-production-arm64
	rm -rf $(BUILD_DIR)/production-linux-arm64
	mkdir -p $(BUILD_DIR)/production-linux-arm64/bin
	cp bin/jax-linux-arm64 $(BUILD_DIR)/production-linux-arm64/bin/jax
	cp bin/confluence-test-linux-arm64 $(BUILD_DIR)/production-linux-arm64/bin/confluence-test
	cp -r scripts $(BUILD_DIR)/production-linux-arm64/
	mkdir -p package
	tar -zcf package/jax-linux-arm64.tar.gz -C $(BUILD_DIR)/production-linux-arm64 .

.PHONY: run
run:
	go run ./cmd/server

.PHONY: start-packaging
start-packaging: bump-version
	mkdir -p $(BUILD_DIR)
	mkdir -p $(PACKAGE_DIR)

.PHONY: package-linux package-linux-amd64 package-linux-arm64
package-linux: CURRENT_OS = linux
package-linux: CURRENT_ARCH = 386
package-linux:
	# Legacy alias — 32-bit x86 (386)
	@echo "Creating Linux x86 (386) package..."
	$(call build-package)
	tar -zcf $(LINUX_TARBALL) -C $(BUILD_DIR)/$(CURRENT_OS)-$(CURRENT_ARCH) .

package-linux-amd64: CURRENT_OS = linux
package-linux-amd64: CURRENT_ARCH = amd64
package-linux-amd64:
	@echo "Creating Linux AMD64 package (t3.nano)..."
	$(call build-package)
	tar -zcf $(LINUX_AMD64_TARBALL) -C $(BUILD_DIR)/$(CURRENT_OS)-$(CURRENT_ARCH) .

package-linux-arm64: CURRENT_OS = linux
package-linux-arm64: CURRENT_ARCH = arm64
package-linux-arm64:
	@echo "Creating Linux ARM64 package (t4g.nano)..."
	$(call build-package)
	tar -zcf $(LINUX_ARM64_TARBALL) -C $(BUILD_DIR)/$(CURRENT_OS)-$(CURRENT_ARCH) .

.PHONY: package-mac
package-mac: CURRENT_OS = darwin
package-mac: CURRENT_ARCH = arm64
package-mac:
	@echo "Creating Mac ARM64 package..."
	$(call build-package)
	tar -zcf $(DARWIN_TARBALL) -C $(BUILD_DIR)/$(CURRENT_OS)-$(CURRENT_ARCH) .

.PHONY: finish-packaging
finish-packaging:
	rm -rf $(BUILD_DIR)

.PHONY: package-all
package-all: start-packaging
	$(MAKE) package-linux-amd64
	$(MAKE) package-linux-arm64
	$(MAKE) package-mac
	$(MAKE) finish-packaging

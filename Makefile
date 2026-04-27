HOST_GOOS ?= $(shell go env GOOS)
HOST_GOARCH ?= $(shell go env GOARCH)

WHISPER_DIR := third_party/whisper.cpp
WHISPER_BUILD := $(WHISPER_DIR)/build
CURL ?= curl
VERSION_FILE := VERSION
include scripts/make/version.mk

ifeq ($(HOST_GOOS),darwin)
CONFIG_ROOT := $(HOME)/Library/Application Support
else ifeq ($(HOST_GOOS),windows)
CONFIG_ROOT := $(APPDATA)
else
CONFIG_ROOT := $(if $(XDG_CONFIG_HOME),$(XDG_CONFIG_HOME),$(HOME)/.config)
endif
APP_SUPPORT_DIR := $(CONFIG_ROOT)/JoiceTyper
MODEL_DIR := $(APP_SUPPORT_DIR)/models
MODEL_FILE := $(MODEL_DIR)/ggml-small.bin
MODEL_URL := https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small.bin
BUILD_DIR := build/$(HOST_GOOS)-$(HOST_GOARCH)
BIN_PATH := $(BUILD_DIR)/voicetype

UI_DIR := ui
FRONTEND_INSTALL_STAMP := $(UI_DIR)/node_modules/.package-lock.stamp
FRONTEND_VITE_BIN := $(UI_DIR)/node_modules/.bin/vite
FRONTEND_REACT_PKG := $(UI_DIR)/node_modules/react/package.json
FRONTEND_REACT_DOM_PKG := $(UI_DIR)/node_modules/react-dom/package.json
FRONTEND_TYPESCRIPT_PKG := $(UI_DIR)/node_modules/typescript/package.json

.PHONY: all setup build clean download-model whisper test frontend-build bridge-contract bridge-contract-check whisper-ready
.PHONY: verify verify-buildinfra verify-mac verify-windows
.PHONY: app dmg release-check build-windows-amd64 build-windows-amd64-no-version-bump build-windows-amd64-release
.PHONY: build-windows-runtime-amd64 build-windows-runtime-amd64-no-version-bump build-windows-runtime-amd64-release
.PHONY: package-windows package-windows-no-version-bump package-windows-release package-windows-runtime
.PHONY: package-windows-msix package-windows-msix-no-version-bump package-windows-msix-test-sign package-windows-msix-release
.PHONY: windows-preflight windows-runtime-prereqs windows-runtime-stage-check windows-portaudio-static windows-whisper-runtime-stage

all: whisper build

bridge-contract:
	go run ./scripts/generate_bridge_contract

bridge-contract-check:
	go run ./scripts/generate_bridge_contract -check

version-bump:
	@current=$$(tr -d '[:space:]' < "$(VERSION_FILE)"); \
	major=$${current%%.*}; \
	rest=$${current#*.}; \
	minor=$${rest%%.*}; \
	patch=$${rest##*.}; \
	next_patch=$$((patch + 1)); \
	next="$$major.$$minor.$$next_patch"; \
	printf '%s\n' "$$next" > "$(VERSION_FILE)"; \
	echo "Version bumped: $$current -> $$next"

$(FRONTEND_VITE_BIN) $(FRONTEND_REACT_PKG) $(FRONTEND_REACT_DOM_PKG) $(FRONTEND_TYPESCRIPT_PKG): $(UI_DIR)/package-lock.json $(UI_DIR)/package.json
	cd $(UI_DIR) && npm ci

$(FRONTEND_INSTALL_STAMP): $(UI_DIR)/package-lock.json $(UI_DIR)/package.json $(FRONTEND_VITE_BIN) $(FRONTEND_REACT_PKG) $(FRONTEND_REACT_DOM_PKG) $(FRONTEND_TYPESCRIPT_PKG)
	@mkdir -p "$(dir $@)"
	@touch $@

frontend-build: bridge-contract-check $(FRONTEND_INSTALL_STAMP)
	cd $(UI_DIR) && npm run build

download-model:
	mkdir -p "$(MODEL_DIR)"
	@if [ -f "$(MODEL_FILE)" ]; then \
		echo "Model already present at $(MODEL_FILE)"; \
	else \
		$(CURL) -L --progress-bar -o "$(MODEL_FILE)" "$(MODEL_URL)"; \
	fi

whisper-ready:
	@test -f "$(WHISPER_DIR)/include/whisper.h" || (echo "fatal: missing whisper.cpp submodule headers; run 'git submodule update --init --recursive'" && exit 1)
	@test -d "$(WHISPER_BUILD)" || (echo "fatal: missing whisper.cpp build; run 'make whisper'" && exit 1)

clean:
	rm -rf build
	rm -rf $(WHISPER_BUILD)
	rm -rf $(UI_DIR)/node_modules

test: bridge-contract-check whisper-ready
	go test -v -count=1 ./...

# vet-cross is the bridge contract check. Both bridge.Platform itself
# and the funcPlatform test adapter are pure Go (no cgo), so vet'ing
# the bridge package under both GOOS targets verifies the interface is
# well-formed and self-consistent on either side. The platform-specific
# adapters (darwin/windows) are checked by their respective real builds
# — full Windows-side typechecking from a Mac dev box would require a
# CGO_ENABLED=0 graph the project doesn't yet maintain.
.PHONY: vet-cross
vet-cross:
	@echo "vetting bridge contract under GOOS=windows..."
	GOOS=windows CGO_ENABLED=0 go vet ./internal/core/bridge/...
	@echo "vetting bridge contract under GOOS=darwin..."
	GOOS=darwin CGO_ENABLED=0 go vet ./internal/core/bridge/...

RELEASE_TAG ?= $(shell git describe --tags --exact-match 2>/dev/null || true)

release-check:
	@test -n "$(RELEASE_TAG)" || (echo "fatal: no release tag provided or checked out" && exit 1)
	@test "v$(VERSION)" = "$(RELEASE_TAG)" || (echo "fatal: release tag $(RELEASE_TAG) does not match VERSION $(VERSION)" && exit 1)
	@test "$$(git rev-parse HEAD)" = "$$(git rev-list -n 1 "$(RELEASE_TAG)")" || (echo "fatal: release tag $(RELEASE_TAG) does not point at HEAD" && exit 1)
	@test -z "$$(git status --porcelain)" || (echo "fatal: release requires a clean working tree" && exit 1)
	@echo "Release tag $(RELEASE_TAG) matches VERSION $(VERSION)"

include scripts/make/macos.mk
include scripts/make/windows.mk
include scripts/make/verify.mk

HOST_GOOS ?= $(shell go env GOOS)
HOST_GOARCH ?= $(shell go env GOARCH)

WHISPER_DIR := third_party/whisper.cpp
WHISPER_BUILD := $(WHISPER_DIR)/build
CURL ?= curl
VERSION_FILE := VERSION
VERSION := $(shell tr -d '[:space:]' < $(VERSION_FILE))
GO_LDFLAGS := -X 'voicetype/internal/core/version.Version=$(VERSION)'
WINDOWS_GO_LDFLAGS := $(GO_LDFLAGS) -H=windowsgui -extldflags=-Wl,--subsystem,windows

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

.PHONY: all setup build clean download-model whisper test frontend-build bridge-contract bridge-contract-check
.PHONY: app dmg release-check build-windows-amd64 build-windows-runtime-amd64 package-windows package-windows-runtime
.PHONY: windows-runtime-prereqs windows-runtime-stage-check windows-portaudio-static windows-whisper-runtime-stage

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

frontend-build: bridge-contract $(FRONTEND_INSTALL_STAMP)
	cd $(UI_DIR) && npm run build

download-model:
	mkdir -p "$(MODEL_DIR)"
	@if [ -f "$(MODEL_FILE)" ]; then \
		echo "Model already present at $(MODEL_FILE)"; \
	else \
		$(CURL) -L --progress-bar -o "$(MODEL_FILE)" "$(MODEL_URL)"; \
	fi

clean:
	rm -rf build
	rm -rf $(WHISPER_BUILD)
	rm -rf $(UI_DIR)/node_modules

test: bridge-contract
	go test -v -count=1 ./...

RELEASE_TAG ?= $(shell git describe --tags --exact-match 2>/dev/null || true)

release-check:
	@test -n "$(RELEASE_TAG)" || (echo "fatal: no release tag provided or checked out" && exit 1)
	@test "v$(VERSION)" = "$(RELEASE_TAG)" || (echo "fatal: release tag $(RELEASE_TAG) does not match VERSION $(VERSION)" && exit 1)
	@echo "Release tag $(RELEASE_TAG) matches VERSION $(VERSION)"

include scripts/make/macos.mk
include scripts/make/windows.mk

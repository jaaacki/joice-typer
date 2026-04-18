.PHONY: all setup build clean download-model whisper test app dmg release-check build-windows-amd64

WHISPER_DIR := third_party/whisper.cpp
WHISPER_BUILD := $(WHISPER_DIR)/build
CURL ?= curl
VERSION_FILE := VERSION
VERSION := $(shell tr -d '[:space:]' < $(VERSION_FILE))
GO_LDFLAGS := -X 'voicetype/internal/core/version.Version=$(VERSION)'
HOST_GOOS ?= $(shell go env GOOS)
HOST_GOARCH ?= $(shell go env GOARCH)
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

all: whisper build

setup:
	brew install portaudio cmake

whisper:
	cd $(WHISPER_DIR) && cmake -B build \
		-DWHISPER_METAL=ON \
		-DBUILD_SHARED_LIBS=OFF \
		-DWHISPER_BUILD_TESTS=OFF \
		-DWHISPER_BUILD_EXAMPLES=OFF \
		-DCMAKE_BUILD_TYPE=Release
	cd $(WHISPER_DIR) && cmake --build build --config Release -j$$(sysctl -n hw.ncpu)

build: whisper
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=1 go build -ldflags "$(GO_LDFLAGS)" -o $(BIN_PATH) ./cmd/joicetyper

download-model:
	mkdir -p "$(MODEL_DIR)"
	@if [ -f "$(MODEL_FILE)" ]; then \
		echo "Model already present at $(MODEL_FILE)"; \
	else \
		$(CURL) -L --progress-bar -o "$(MODEL_FILE)" "$(MODEL_URL)"; \
	fi

APP_NAME := JoiceTyper
APP_BUNDLE := $(APP_NAME).app
PLIST_TEMPLATE := assets/macos/Info.plist.tmpl
APP_ICON := assets/icons/icon.icns
PORTAUDIO_PREFIX ?= $(shell brew --prefix portaudio 2>/dev/null || echo /opt/homebrew/opt/portaudio)
PORTAUDIO_DYLIB := $(PORTAUDIO_PREFIX)/lib/libportaudio.2.dylib

clean:
	rm -rf build
	rm -rf $(WHISPER_BUILD)
	rm -rf $(APP_BUNDLE)

test:
	go test -v -count=1 ./...

app: build
	rm -rf $(APP_BUNDLE)
	mkdir -p $(APP_BUNDLE)/Contents/MacOS
	mkdir -p $(APP_BUNDLE)/Contents/Resources
	mkdir -p $(APP_BUNDLE)/Contents/Frameworks
	cp $(BIN_PATH) $(APP_BUNDLE)/Contents/MacOS/$(APP_NAME)
	sed "s/{{VERSION}}/$(VERSION)/g" $(PLIST_TEMPLATE) > $(APP_BUNDLE)/Contents/Info.plist
	@if [ -f "$(APP_ICON)" ]; then cp "$(APP_ICON)" $(APP_BUNDLE)/Contents/Resources/; fi
	@# Bundle PortAudio dylib and fix load path
	cp "$(PORTAUDIO_DYLIB)" $(APP_BUNDLE)/Contents/Frameworks/
	install_name_tool -change "$(PORTAUDIO_DYLIB)" \
		@executable_path/../Frameworks/libportaudio.2.dylib \
		$(APP_BUNDLE)/Contents/MacOS/$(APP_NAME)
	codesign --force --sign - $(APP_BUNDLE)/Contents/Frameworks/libportaudio.2.dylib
	codesign --force --sign - $(APP_BUNDLE)
	@echo "Built $(APP_BUNDLE)"

DMG_NAME := $(APP_NAME)-$(VERSION).dmg
DMG_STAGING := dmg-staging

dmg: app
	@echo "Creating $(DMG_NAME)..."
	rm -rf $(DMG_STAGING) $(DMG_NAME)
	mkdir -p $(DMG_STAGING)
	cp -R $(APP_BUNDLE) $(DMG_STAGING)/
	ln -s /Applications $(DMG_STAGING)/Applications
	hdiutil create -volname "$(APP_NAME)" \
		-srcfolder $(DMG_STAGING) \
		-ov -format UDZO \
		$(DMG_NAME)
	rm -rf $(DMG_STAGING)
	@echo "Built $(DMG_NAME)"

RELEASE_TAG ?= $(shell git describe --tags --exact-match 2>/dev/null || true)

release-check:
	@test -n "$(RELEASE_TAG)" || (echo "fatal: no release tag provided or checked out" && exit 1)
	@test "v$(VERSION)" = "$(RELEASE_TAG)" || (echo "fatal: release tag $(RELEASE_TAG) does not match VERSION $(VERSION)" && exit 1)
	@echo "Release tag $(RELEASE_TAG) matches VERSION $(VERSION)"

build-windows-amd64:
	mkdir -p build/windows-amd64
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(GO_LDFLAGS)" -o build/windows-amd64/joicetyper.exe ./cmd/joicetyper

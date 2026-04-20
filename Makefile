.PHONY: all setup build clean download-model whisper test app dmg release-check build-windows-amd64 build-windows-runtime-amd64 package-windows package-windows-runtime frontend-build bridge-contract bridge-contract-check windows-runtime-prereqs windows-runtime-stage-check

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
WINDOWS_BUILD_DIR := build/windows-amd64
WINDOWS_BIN_PATH := $(WINDOWS_BUILD_DIR)/joicetyper.exe
WINDOWS_RUNTIME_DIR := $(WHISPER_DIR)/build/bin/Release
WINDOWS_RUNTIME_IMPORT_DIR := $(WHISPER_DIR)/build/src/Release
WINDOWS_RUNTIME_DLLS := whisper.dll ggml.dll ggml-base.dll ggml-cpu.dll
WINDOWS_CC ?= x86_64-w64-mingw32-gcc
WINDOWS_CXX ?= x86_64-w64-mingw32-g++
WINDOWS_PORTAUDIO_DLL ?= $(shell find "$(CURDIR)/third_party/portaudio-windows-src/lib/.libs" -name 'libportaudio-2.dll' -print -quit 2>/dev/null)
WINDOWS_LIBGCC_DLL ?= $(shell $(WINDOWS_CC) -print-file-name=libgcc_s_seh-1.dll)
WINDOWS_LIBSTDCXX_DLL ?= $(shell $(WINDOWS_CXX) -print-file-name=libstdc++-6.dll)
WINDOWS_WINPTHREAD_DLL ?= $(shell find "$(dir $(WINDOWS_LIBGCC_DLL))/.." -name 'libwinpthread-1.dll' -print -quit 2>/dev/null)
WINDOWS_EXTRA_RUNTIME_DLLS := libwhisper.dll libportaudio-2.dll libgcc_s_seh-1.dll libstdc++-6.dll
WINDOWS_RUNTIME_STAGE_FILES := joicetyper.exe $(WINDOWS_RUNTIME_DLLS) $(WINDOWS_EXTRA_RUNTIME_DLLS)
WINDOWS_INSTALLER_SCRIPT := packaging/windows/joicetyper.iss
WINDOWS_INSTALLER_NAME := JoiceTyper-$(VERSION)-setup.exe
WINDOWS_INSTALLER_PATH := $(WINDOWS_BUILD_DIR)/$(WINDOWS_INSTALLER_NAME)
ISCC ?= iscc
UI_DIR := ui
FRONTEND_INSTALL_STAMP := $(UI_DIR)/node_modules/.package-lock.stamp
FRONTEND_VITE_BIN := $(UI_DIR)/node_modules/.bin/vite
FRONTEND_REACT_PKG := $(UI_DIR)/node_modules/react/package.json
FRONTEND_REACT_DOM_PKG := $(UI_DIR)/node_modules/react-dom/package.json
FRONTEND_TYPESCRIPT_PKG := $(UI_DIR)/node_modules/typescript/package.json

all: whisper build

bridge-contract:
	go run ./scripts/generate_bridge_contract

bridge-contract-check:
	go run ./scripts/generate_bridge_contract -check

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

$(FRONTEND_VITE_BIN) $(FRONTEND_REACT_PKG) $(FRONTEND_REACT_DOM_PKG) $(FRONTEND_TYPESCRIPT_PKG): $(UI_DIR)/package-lock.json $(UI_DIR)/package.json
	cd $(UI_DIR) && npm ci

$(FRONTEND_INSTALL_STAMP): $(UI_DIR)/package-lock.json $(UI_DIR)/package.json $(FRONTEND_VITE_BIN) $(FRONTEND_REACT_PKG) $(FRONTEND_REACT_DOM_PKG) $(FRONTEND_TYPESCRIPT_PKG)
	@mkdir -p "$(dir $@)"
	@touch $@

frontend-build: bridge-contract $(FRONTEND_INSTALL_STAMP)
	cd $(UI_DIR) && npm run build

build: bridge-contract whisper frontend-build
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
	rm -rf $(UI_DIR)/node_modules

test: bridge-contract
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

build-windows-amd64: bridge-contract frontend-build
	mkdir -p $(WINDOWS_BUILD_DIR)
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(GO_LDFLAGS)" -o $(WINDOWS_BIN_PATH) ./cmd/joicetyper

windows-runtime-prereqs:
	@command -v $(WINDOWS_CC) >/dev/null 2>&1 || (echo "fatal: missing Windows C compiler $(WINDOWS_CC)" && exit 1)
	@command -v $(WINDOWS_CXX) >/dev/null 2>&1 || (echo "fatal: missing Windows C++ compiler $(WINDOWS_CXX)" && exit 1)
	@test -d "$(WINDOWS_RUNTIME_DIR)" || (echo "fatal: missing Windows runtime directory $(WINDOWS_RUNTIME_DIR)" && exit 1)
	@test -d "$(WINDOWS_RUNTIME_IMPORT_DIR)" || (echo "fatal: missing Windows import library directory $(WINDOWS_RUNTIME_IMPORT_DIR)" && exit 1)
	@test -f "$(WINDOWS_PORTAUDIO_DLL)" || (echo "fatal: missing Windows PortAudio runtime $(WINDOWS_PORTAUDIO_DLL)" && exit 1)
	@test -f "$(WINDOWS_LIBGCC_DLL)" || (echo "fatal: missing MinGW runtime $(WINDOWS_LIBGCC_DLL)" && exit 1)
	@test -f "$(WINDOWS_LIBSTDCXX_DLL)" || (echo "fatal: missing MinGW runtime $(WINDOWS_LIBSTDCXX_DLL)" && exit 1)
	@for dll in $(WINDOWS_RUNTIME_DLLS); do \
		test -f "$(WINDOWS_RUNTIME_DIR)/$$dll" || (echo "fatal: missing Windows runtime payload $(WINDOWS_RUNTIME_DIR)/$$dll" && exit 1); \
	done

windows-runtime-stage-check:
	@for artifact in $(WINDOWS_RUNTIME_STAGE_FILES); do \
		test -f "$(WINDOWS_BUILD_DIR)/$$artifact" || (echo "fatal: missing staged Windows runtime artifact $(WINDOWS_BUILD_DIR)/$$artifact" && exit 1); \
	done

build-windows-runtime-amd64: bridge-contract frontend-build windows-runtime-prereqs
	mkdir -p $(WINDOWS_BUILD_DIR)
	CC=$(WINDOWS_CC) CXX=$(WINDOWS_CXX) GOOS=windows GOARCH=amd64 CGO_ENABLED=1 go build -ldflags "$(GO_LDFLAGS)" -o $(WINDOWS_BIN_PATH) ./cmd/joicetyper
	@for dll in $(WINDOWS_RUNTIME_DLLS); do \
		cp "$(WINDOWS_RUNTIME_DIR)/$$dll" "$(WINDOWS_BUILD_DIR)/$$dll"; \
	done
	cp "$(WINDOWS_RUNTIME_DIR)/whisper.dll" "$(WINDOWS_BUILD_DIR)/libwhisper.dll"
	cp "$(WINDOWS_PORTAUDIO_DLL)" "$(WINDOWS_BUILD_DIR)/libportaudio-2.dll"
	cp "$(WINDOWS_LIBGCC_DLL)" "$(WINDOWS_BUILD_DIR)/libgcc_s_seh-1.dll"
	cp "$(WINDOWS_LIBSTDCXX_DLL)" "$(WINDOWS_BUILD_DIR)/libstdc++-6.dll"
	@if [ -n "$(WINDOWS_WINPTHREAD_DLL)" ] && [ -f "$(WINDOWS_WINPTHREAD_DLL)" ]; then \
		cp "$(WINDOWS_WINPTHREAD_DLL)" "$(WINDOWS_BUILD_DIR)/libwinpthread-1.dll"; \
	fi
	@$(MAKE) windows-runtime-stage-check

package-windows: build-windows-runtime-amd64 windows-runtime-stage-check
	@test -f "$(WINDOWS_INSTALLER_SCRIPT)" || (echo "fatal: missing $(WINDOWS_INSTALLER_SCRIPT)" && exit 1)
	$(ISCC) /DAppVersion=$(VERSION) /DRepoRoot="$(CURDIR)" /DOutputDir="$(CURDIR)/$(WINDOWS_BUILD_DIR)" "$(WINDOWS_INSTALLER_SCRIPT)"

package-windows-runtime: package-windows

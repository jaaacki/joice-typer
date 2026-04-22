APP_NAME := JoiceTyper
APP_BUNDLE := $(APP_NAME).app
PLIST_TEMPLATE := assets/macos/Info.plist.tmpl
APP_ICON := assets/icons/icon.icns
PORTAUDIO_PREFIX ?= $(shell brew --prefix portaudio 2>/dev/null || echo /opt/homebrew/opt/portaudio)
PORTAUDIO_DYLIB := $(PORTAUDIO_PREFIX)/lib/libportaudio.2.dylib
DMG_NAME := $(APP_NAME)-$(VERSION).dmg
DMG_STAGING := dmg-staging
MACOS_RELEASE_DIR := build/macos-release
MACOS_RELEASE_ENV_FILE ?= packaging/macos/release.env.local
MACOS_RELEASE_ENV_SCRIPT := scripts/release/macos_release_env.sh
MACOS_PREFLIGHT_SCRIPT := scripts/release/macos_preflight.sh
MACOS_RELEASE_ARCHIVE_SCRIPT := scripts/release/macos_archive.sh
MACOS_APPCAST_SCRIPT := scripts/release/macos_appcast.py
MACOS_PLIST_RENDER_SCRIPT := scripts/release/macos_render_info_plist.py
MACOS_SPARKLE_STAGE_SCRIPT := scripts/release/macos_stage_sparkle.sh
MACOS_PREPARE_RELEASE_APP_SCRIPT := scripts/release/macos_prepare_release_app.sh
MACOS_NOTARIZE_SCRIPT := scripts/release/macos_notarize.sh
MACOS_PUBLISH_GITHUB_SCRIPT := scripts/release/macos_publish_github.sh
MACOS_APPCAST_TEMPLATE := packaging/macos/sparkle-appcast.xml.tmpl
MACOS_RELEASE_APP_BUNDLE := $(MACOS_RELEASE_DIR)/$(APP_BUNDLE)
MACOS_RELEASE_ARCHIVE := $(MACOS_RELEASE_DIR)/JoiceTyper-$(VERSION)-macos.zip
MACOS_RELEASE_METADATA := $(MACOS_RELEASE_DIR)/JoiceTyper-$(VERSION)-macos.env
MACOS_APPCAST_PATH := $(MACOS_RELEASE_DIR)/appcast.xml
MACOS_RELEASE_DMG := $(MACOS_RELEASE_DIR)/JoiceTyper-$(VERSION).dmg
MACOS_SPARKLE_STAGE_DIR := $(MACOS_RELEASE_DIR)/sparkle
MACOS_RELEASE_DMG_STAGE := $(MACOS_RELEASE_DIR)/dmg-staging
MACOS_SPARKLE_FEED_URL ?=
MACOS_SPARKLE_PUBLIC_ED_KEY ?=

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

build: version-bump bridge-contract whisper frontend-build
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=1 go build -ldflags "$(GO_LDFLAGS)" -o $(BIN_PATH) ./cmd/joicetyper

app: build
	rm -rf $(APP_BUNDLE)
	mkdir -p $(APP_BUNDLE)/Contents/MacOS
	mkdir -p $(APP_BUNDLE)/Contents/Resources
	mkdir -p $(APP_BUNDLE)/Contents/Frameworks
	cp $(BIN_PATH) $(APP_BUNDLE)/Contents/MacOS/$(APP_NAME)
	@python3 "$(MACOS_PLIST_RENDER_SCRIPT)" "$(PLIST_TEMPLATE)" "$(APP_BUNDLE)/Contents/Info.plist" "$(VERSION)" "" "" "false"
	@if [ -f "$(APP_ICON)" ]; then cp "$(APP_ICON)" $(APP_BUNDLE)/Contents/Resources/; fi
	cp "$(PORTAUDIO_DYLIB)" $(APP_BUNDLE)/Contents/Frameworks/
	install_name_tool -change "$(PORTAUDIO_DYLIB)" \
		@executable_path/../Frameworks/libportaudio.2.dylib \
		$(APP_BUNDLE)/Contents/MacOS/$(APP_NAME)
	codesign --force --sign - $(APP_BUNDLE)/Contents/Frameworks/libportaudio.2.dylib
	codesign --force --sign - $(APP_BUNDLE)
	@echo "Built $(APP_BUNDLE)"

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

mac-stage-sparkle:
	@sh "$(MACOS_RELEASE_ENV_SCRIPT)" sparkle
	@sh "$(MACOS_SPARKLE_STAGE_SCRIPT)" "$(MACOS_SPARKLE_STAGE_DIR)"

mac-release-preflight:
	@sh "$(MACOS_PREFLIGHT_SCRIPT)" archive

mac-notarize-preflight:
	@sh "$(MACOS_PREFLIGHT_SCRIPT)" notarize

mac-publish-preflight: release-check
	@. "$(MACOS_RELEASE_ENV_FILE)"; \
		RELEASE_TAG="$(RELEASE_TAG)" sh "$(MACOS_PREFLIGHT_SCRIPT)" publish

mac-release-app: mac-release-preflight app mac-stage-sparkle
	@sh "$(MACOS_RELEASE_ENV_SCRIPT)" archive
	@sh "$(MACOS_RELEASE_ENV_SCRIPT)" appcast
	mkdir -p "$(MACOS_RELEASE_DIR)"
	@. "$(MACOS_RELEASE_ENV_FILE)"; \
		feed_url="$${MACOS_APPCAST_BASE_URL%/}/appcast.xml"; \
		sh "$(MACOS_PREPARE_RELEASE_APP_SCRIPT)" "$(APP_BUNDLE)" "$(MACOS_RELEASE_APP_BUNDLE)" "$(MACOS_SPARKLE_STAGE_DIR)" "$(VERSION)" "$$feed_url" "$${MACOS_SPARKLE_PUBLIC_ED_KEY}" "$${MACOS_CODESIGN_IDENTITY}" "$(PLIST_TEMPLATE)" "$(MACOS_PLIST_RENDER_SCRIPT)"

mac-release-archive: mac-release-app
	@sh "$(MACOS_RELEASE_ENV_SCRIPT)" archive
	mkdir -p "$(MACOS_RELEASE_DIR)"
	@. "$(MACOS_RELEASE_ENV_FILE)"; \
		sign_tool="$(MACOS_SPARKLE_STAGE_DIR)/bin/sign_update"; \
		sh "$(MACOS_RELEASE_ARCHIVE_SCRIPT)" "$(MACOS_RELEASE_APP_BUNDLE)" "$(MACOS_RELEASE_ARCHIVE)" "$(VERSION)" "$(MACOS_RELEASE_METADATA)" "$$sign_tool" "$${MACOS_SPARKLE_PRIVATE_KEY_FILE}"

mac-appcast: mac-release-archive
	@sh "$(MACOS_RELEASE_ENV_SCRIPT)" appcast
	mkdir -p "$(MACOS_RELEASE_DIR)"
	@. "$(MACOS_RELEASE_ENV_FILE)"; \
		download_url="$${MACOS_APPCAST_BASE_URL%/}/$(notdir $(MACOS_RELEASE_ARCHIVE))"; \
		appcast_url="$${MACOS_APPCAST_BASE_URL%/}/appcast.xml"; \
		python3 "$(MACOS_APPCAST_SCRIPT)" "$(MACOS_APPCAST_TEMPLATE)" "$(MACOS_APPCAST_PATH)" "$(APP_NAME)" "$$appcast_url" "$$download_url" "$${MACOS_SPARKLE_PUBLIC_ED_KEY}" "$(MACOS_RELEASE_METADATA)"

mac-release-dmg: mac-release-app
	@echo "Creating $(MACOS_RELEASE_DMG)..."
	rm -rf "$(MACOS_RELEASE_DMG_STAGE)" "$(MACOS_RELEASE_DMG)"
	mkdir -p "$(MACOS_RELEASE_DMG_STAGE)"
	cp -R "$(MACOS_RELEASE_APP_BUNDLE)" "$(MACOS_RELEASE_DMG_STAGE)/"
	ln -s /Applications "$(MACOS_RELEASE_DMG_STAGE)/Applications"
	hdiutil create -volname "$(APP_NAME)" \
		-srcfolder "$(MACOS_RELEASE_DMG_STAGE)" \
		-ov -format UDZO \
		"$(MACOS_RELEASE_DMG)"
	rm -rf "$(MACOS_RELEASE_DMG_STAGE)"
	@echo "Built $(MACOS_RELEASE_DMG)"

mac-notarize-release: mac-notarize-preflight mac-release-archive
	@sh "$(MACOS_RELEASE_ENV_SCRIPT)" notarize
	@. "$(MACOS_RELEASE_ENV_FILE)"; \
		sh "$(MACOS_NOTARIZE_SCRIPT)" "$(MACOS_RELEASE_ARCHIVE)" "$${MACOS_NOTARYTOOL_PROFILE}"

mac-release-artifacts: mac-appcast mac-release-dmg
	mkdir -p "$(MACOS_RELEASE_DIR)"

mac-publish-github-release: mac-publish-preflight mac-release-artifacts
	@. "$(MACOS_RELEASE_ENV_FILE)"; \
		RELEASE_TAG="$(RELEASE_TAG)" sh "$(MACOS_PUBLISH_GITHUB_SCRIPT)" "$(MACOS_RELEASE_ARCHIVE)" "$(MACOS_RELEASE_DMG)" "$(MACOS_APPCAST_PATH)"

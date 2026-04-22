APP_NAME := JoiceTyper
APP_BUNDLE := $(APP_NAME).app
PLIST_TEMPLATE := assets/macos/Info.plist.tmpl
APP_ICON := assets/icons/icon.icns
PORTAUDIO_PREFIX ?= $(shell brew --prefix portaudio 2>/dev/null || echo /opt/homebrew/opt/portaudio)
PORTAUDIO_DYLIB := $(PORTAUDIO_PREFIX)/lib/libportaudio.2.dylib
DMG_NAME := $(APP_NAME)-$(VERSION).dmg
DMG_STAGING := dmg-staging

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
	sed "s/{{VERSION}}/$(VERSION)/g" $(PLIST_TEMPLATE) > $(APP_BUNDLE)/Contents/Info.plist
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

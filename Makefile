.PHONY: all setup build clean download-model whisper test app

WHISPER_DIR := third_party/whisper.cpp
WHISPER_BUILD := $(WHISPER_DIR)/build
MODEL_DIR := $(HOME)/.config/voicetype/models
MODEL_FILE := $(MODEL_DIR)/ggml-small.bin
MODEL_URL := https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small.bin

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
	CGO_ENABLED=1 go build -o voicetype .

download-model: $(MODEL_FILE)

$(MODEL_FILE):
	mkdir -p $(MODEL_DIR)
	curl -L --progress-bar -o $(MODEL_FILE) $(MODEL_URL)

APP_NAME := JoiceTyper
APP_BUNDLE := $(APP_NAME).app

clean:
	rm -f voicetype
	rm -rf $(WHISPER_BUILD)
	rm -rf $(APP_BUNDLE)

test:
	go test -v -count=1 ./...

app: build
	rm -rf $(APP_BUNDLE)
	mkdir -p $(APP_BUNDLE)/Contents/MacOS
	mkdir -p $(APP_BUNDLE)/Contents/Resources
	cp voicetype $(APP_BUNDLE)/Contents/MacOS/$(APP_NAME)
	cp Info.plist $(APP_BUNDLE)/Contents/
	@if [ -f icon.icns ]; then cp icon.icns $(APP_BUNDLE)/Contents/Resources/; fi
	codesign --force --sign - $(APP_BUNDLE)
	@echo "Built $(APP_BUNDLE)"

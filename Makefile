.PHONY: all setup build clean download-model whisper test

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

clean:
	rm -f voicetype
	rm -rf $(WHISPER_BUILD)

test:
	go test -v -count=1 ./...

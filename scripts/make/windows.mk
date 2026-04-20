WINDOWS_BUILD_DIR := build/windows-amd64
WINDOWS_BIN_PATH := $(WINDOWS_BUILD_DIR)/joicetyper.exe
WINDOWS_RUNTIME_DIR := $(WHISPER_DIR)/build/bin/Release
WINDOWS_RUNTIME_IMPORT_DIR := $(WHISPER_DIR)/build/src/Release
WINDOWS_RUNTIME_DLLS := whisper.dll ggml.dll ggml-base.dll ggml-cpu.dll
WINDOWS_CC ?= x86_64-w64-mingw32-gcc
WINDOWS_CXX ?= x86_64-w64-mingw32-g++
WINDOWS_PORTAUDIO_SRC_DIR := third_party/portaudio-windows-src
WINDOWS_PORTAUDIO_BUILD_DIR := $(WINDOWS_PORTAUDIO_SRC_DIR)/build-windows-static
WINDOWS_PORTAUDIO_INSTALL_DIR := third_party/portaudio-windows-static-install
WINDOWS_PORTAUDIO_STATIC_LIB := $(WINDOWS_PORTAUDIO_INSTALL_DIR)/lib/libportaudio.a
WINDOWS_PORTAUDIO_PKGCONFIG_DIR := $(WINDOWS_PORTAUDIO_INSTALL_DIR)/lib/pkgconfig
WINDOWS_PORTAUDIO_PC := $(WINDOWS_PORTAUDIO_PKGCONFIG_DIR)/portaudio-2.0.pc
WINDOWS_LIBGCC_DLL ?= $(shell $(WINDOWS_CC) -print-file-name=libgcc_s_seh-1.dll)
WINDOWS_LIBSTDCXX_DLL ?= $(shell $(WINDOWS_CXX) -print-file-name=libstdc++-6.dll)
WINDOWS_LIBWINPTHREAD_DLL ?= $(shell find "$(dir $(WINDOWS_LIBGCC_DLL))/.." -name 'libwinpthread-1.dll' -print -quit 2>/dev/null)
WINDOWS_LIBGOMP_DLL ?= $(shell find "$(dir $(WINDOWS_LIBGCC_DLL))/.." -name 'libgomp-1.dll' -print -quit 2>/dev/null)
WINDOWS_LIBDL_DLL ?= $(shell find "$(dir $(WINDOWS_LIBGCC_DLL))/.." -name 'libdl.dll' -print -quit 2>/dev/null)
WINDOWS_EXTRA_RUNTIME_DLLS := libwhisper.dll libgcc_s_seh-1.dll libstdc++-6.dll libgomp-1.dll libdl.dll
WINDOWS_OPTIONAL_RUNTIME_DLLS := libwinpthread-1.dll
WINDOWS_RUNTIME_STAGE_FILES := joicetyper.exe $(WINDOWS_RUNTIME_DLLS) $(WINDOWS_EXTRA_RUNTIME_DLLS)
WINDOWS_INSTALLER_SCRIPT := packaging/windows/joicetyper.iss
WINDOWS_INSTALLER_NAME := JoiceTyper-$(VERSION)-setup.exe
WINDOWS_INSTALLER_PATH := $(WINDOWS_BUILD_DIR)/$(WINDOWS_INSTALLER_NAME)
ISCC ?= iscc

build-windows-amd64: bridge-contract frontend-build
	mkdir -p $(WINDOWS_BUILD_DIR)
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(WINDOWS_GO_LDFLAGS)" -o $(WINDOWS_BIN_PATH) ./cmd/joicetyper

windows-portaudio-static:
	@test -d "$(WINDOWS_PORTAUDIO_SRC_DIR)" || (echo "fatal: missing Windows PortAudio source directory $(WINDOWS_PORTAUDIO_SRC_DIR)" && exit 1)
	rm -rf "$(WINDOWS_PORTAUDIO_BUILD_DIR)" "$(WINDOWS_PORTAUDIO_INSTALL_DIR)"
	cmake -G "MinGW Makefiles" -DCMAKE_MAKE_PROGRAM="$(dir $(WINDOWS_CC))/mingw32-make.exe" -S "$(WINDOWS_PORTAUDIO_SRC_DIR)" -B "$(WINDOWS_PORTAUDIO_BUILD_DIR)" \
		-DCMAKE_SYSTEM_NAME=Windows \
		-DCMAKE_C_COMPILER="$(WINDOWS_CC)" \
		-DCMAKE_CXX_COMPILER="$(WINDOWS_CXX)" \
		-DCMAKE_INSTALL_PREFIX="$(CURDIR)/$(WINDOWS_PORTAUDIO_INSTALL_DIR)" \
		-DPA_BUILD_SHARED_LIBS=OFF \
		-DPA_BUILD_TESTS=OFF \
		-DPA_BUILD_EXAMPLES=OFF \
		-DPA_USE_WMME=OFF \
		-DPA_USE_WASAPI=ON \
		-DPA_USE_DS=OFF \
		-DPA_USE_WDMKS=OFF \
		-DPA_USE_WDMKS_DEVICE_INFO=OFF \
		-DCMAKE_BUILD_TYPE=Release
	cmake --build "$(WINDOWS_PORTAUDIO_BUILD_DIR)" --config Release --parallel 8
	cmake --install "$(WINDOWS_PORTAUDIO_BUILD_DIR)"
	@mkdir -p "$(WINDOWS_PORTAUDIO_PKGCONFIG_DIR)"
	@printf '%s\n' \
		'prefix=$(CURDIR)/$(WINDOWS_PORTAUDIO_INSTALL_DIR)' \
		'exec_prefix=$${prefix}' \
		'libdir=$${prefix}/lib' \
		'includedir=$${prefix}/include' \
		'' \
		'Name: PortAudio' \
		'Description: Portable audio I/O' \
		'Requires:' \
		'Version: 19.8' \
		'' \
		'Libs: -L$${libdir} -lportaudio -lole32 -luuid -lksuser -lmfplat -lmfuuid -lavrt' \
		'Cflags: -I$${includedir} -DPA_USE_WASAPI=1' > "$(WINDOWS_PORTAUDIO_PC)"
	@test -f "$(WINDOWS_PORTAUDIO_STATIC_LIB)" || (echo "fatal: missing static PortAudio library $(WINDOWS_PORTAUDIO_STATIC_LIB)" && exit 1)
	@test -f "$(WINDOWS_PORTAUDIO_PC)" || (echo "fatal: missing Windows PortAudio pkg-config file $(WINDOWS_PORTAUDIO_PC)" && exit 1)

windows-whisper-runtime-stage:
	rm -rf "$(WHISPER_BUILD)"
	cmake -G "MinGW Makefiles" -DCMAKE_MAKE_PROGRAM="$(dir $(WINDOWS_CC))/mingw32-make.exe" -S "$(WHISPER_DIR)" -B "$(WHISPER_BUILD)" \
		-DCMAKE_C_COMPILER="$(WINDOWS_CC)" \
		-DCMAKE_CXX_COMPILER="$(WINDOWS_CXX)" \
		-DBUILD_SHARED_LIBS=ON \
		-DWHISPER_BUILD_TESTS=OFF \
		-DWHISPER_BUILD_EXAMPLES=OFF \
		-DWHISPER_BUILD_SERVER=OFF \
		-DGGML_NATIVE=OFF \
		-DGGML_AVX=ON \
		-DGGML_AVX2=ON \
		-DGGML_FMA=ON \
		-DGGML_F16C=ON \
		-DCMAKE_BUILD_TYPE=Release
	cmake --build "$(WHISPER_BUILD)" --config Release --parallel 8
	mkdir -p "$(WINDOWS_RUNTIME_DIR)" "$(WINDOWS_RUNTIME_IMPORT_DIR)" "$(WHISPER_BUILD)/ggml/src/Release" "$(WHISPER_BUILD)/ggml/src/ggml-cpu/Release"
	cp "$(WHISPER_BUILD)/bin/libwhisper.dll" "$(WINDOWS_RUNTIME_DIR)/whisper.dll"
	cp "$(WHISPER_BUILD)/bin/ggml.dll" "$(WINDOWS_RUNTIME_DIR)/ggml.dll"
	cp "$(WHISPER_BUILD)/bin/ggml-base.dll" "$(WINDOWS_RUNTIME_DIR)/ggml-base.dll"
	cp "$(WHISPER_BUILD)/bin/ggml-cpu.dll" "$(WINDOWS_RUNTIME_DIR)/ggml-cpu.dll"
	cp "$(WHISPER_BUILD)/src/libwhisper.dll.a" "$(WINDOWS_RUNTIME_IMPORT_DIR)/libwhisper.dll.a"
	cp "$(WHISPER_BUILD)/ggml/src/libggml.dll.a" "$(WHISPER_BUILD)/ggml/src/Release/libggml.dll.a"
	cp "$(WHISPER_BUILD)/ggml/src/libggml-base.dll.a" "$(WHISPER_BUILD)/ggml/src/Release/libggml-base.dll.a"
	cp "$(WHISPER_BUILD)/ggml/src/libggml-cpu.dll.a" "$(WHISPER_BUILD)/ggml/src/ggml-cpu/Release/libggml-cpu.dll.a"

windows-runtime-prereqs: windows-portaudio-static windows-whisper-runtime-stage
	@command -v $(WINDOWS_CC) >/dev/null 2>&1 || (echo "fatal: missing Windows C compiler $(WINDOWS_CC)" && exit 1)
	@command -v $(WINDOWS_CXX) >/dev/null 2>&1 || (echo "fatal: missing Windows C++ compiler $(WINDOWS_CXX)" && exit 1)
	@test -d "$(WINDOWS_RUNTIME_DIR)" || (echo "fatal: missing Windows runtime directory $(WINDOWS_RUNTIME_DIR)" && exit 1)
	@test -d "$(WINDOWS_RUNTIME_IMPORT_DIR)" || (echo "fatal: missing Windows import library directory $(WINDOWS_RUNTIME_IMPORT_DIR)" && exit 1)
	@test -f "$(WINDOWS_PORTAUDIO_STATIC_LIB)" || (echo "fatal: missing static PortAudio library $(WINDOWS_PORTAUDIO_STATIC_LIB)" && exit 1)
	@test -f "$(WINDOWS_PORTAUDIO_PC)" || (echo "fatal: missing Windows PortAudio pkg-config file $(WINDOWS_PORTAUDIO_PC)" && exit 1)
	@test -f "$(WINDOWS_LIBGCC_DLL)" || (echo "fatal: missing MinGW runtime $(WINDOWS_LIBGCC_DLL)" && exit 1)
	@test -f "$(WINDOWS_LIBSTDCXX_DLL)" || (echo "fatal: missing MinGW runtime $(WINDOWS_LIBSTDCXX_DLL)" && exit 1)
	@test -f "$(WINDOWS_LIBGOMP_DLL)" || (echo "fatal: missing MinGW runtime $(WINDOWS_LIBGOMP_DLL)" && exit 1)
	@test -f "$(WINDOWS_LIBDL_DLL)" || (echo "fatal: missing MinGW runtime $(WINDOWS_LIBDL_DLL)" && exit 1)
	@for dll in $(WINDOWS_RUNTIME_DLLS); do \
		test -f "$(WINDOWS_RUNTIME_DIR)/$$dll" || (echo "fatal: missing Windows runtime payload $(WINDOWS_RUNTIME_DIR)/$$dll" && exit 1); \
	done

windows-runtime-stage-check:
	@for artifact in $(WINDOWS_RUNTIME_STAGE_FILES); do \
		test -f "$(WINDOWS_BUILD_DIR)/$$artifact" || (echo "fatal: missing staged Windows runtime artifact $(WINDOWS_BUILD_DIR)/$$artifact" && exit 1); \
	done

build-windows-runtime-amd64: bridge-contract frontend-build windows-runtime-prereqs
	mkdir -p $(WINDOWS_BUILD_DIR)
	rm -f "$(WINDOWS_BUILD_DIR)/libportaudio-2.dll"
	go clean -cache
	PKG_CONFIG="pkg-config" PKG_CONFIG_PATH= PKG_CONFIG_LIBDIR="$(CURDIR)/$(WINDOWS_PORTAUDIO_PKGCONFIG_DIR)" \
		CC=$(WINDOWS_CC) CXX=$(WINDOWS_CXX) CGO_LDFLAGS="-lwinmm -lole32 -luuid" GOOS=windows GOARCH=amd64 CGO_ENABLED=1 \
		go build -ldflags "$(WINDOWS_GO_LDFLAGS)" -o $(WINDOWS_BIN_PATH) ./cmd/joicetyper
	@for dll in $(WINDOWS_RUNTIME_DLLS); do \
		cp "$(WINDOWS_RUNTIME_DIR)/$$dll" "$(WINDOWS_BUILD_DIR)/$$dll"; \
	done
	cp "$(WHISPER_BUILD)/bin/libwhisper.dll" "$(WINDOWS_BUILD_DIR)/libwhisper.dll"
	cp "$(WINDOWS_LIBGCC_DLL)" "$(WINDOWS_BUILD_DIR)/libgcc_s_seh-1.dll"
	cp "$(WINDOWS_LIBSTDCXX_DLL)" "$(WINDOWS_BUILD_DIR)/libstdc++-6.dll"
	cp "$(WINDOWS_LIBGOMP_DLL)" "$(WINDOWS_BUILD_DIR)/libgomp-1.dll"
	cp "$(WINDOWS_LIBDL_DLL)" "$(WINDOWS_BUILD_DIR)/libdl.dll"
	@if [ -n "$(WINDOWS_LIBWINPTHREAD_DLL)" ] && [ -f "$(WINDOWS_LIBWINPTHREAD_DLL)" ]; then \
		cp "$(WINDOWS_LIBWINPTHREAD_DLL)" "$(WINDOWS_BUILD_DIR)/libwinpthread-1.dll"; \
	fi
	@$(MAKE) windows-runtime-stage-check

package-windows: build-windows-runtime-amd64 windows-runtime-stage-check
	@test -f "$(WINDOWS_INSTALLER_SCRIPT)" || (echo "fatal: missing $(WINDOWS_INSTALLER_SCRIPT)" && exit 1)
	$(ISCC) /DAppVersion=$(VERSION) /DRepoRoot="$(CURDIR)" /DOutputDir="$(CURDIR)/$(WINDOWS_BUILD_DIR)" "$(WINDOWS_INSTALLER_SCRIPT)"

package-windows-runtime: package-windows

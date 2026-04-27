WINDOWS_BUILD_DIR := build/windows-amd64
WINDOWS_BIN_PATH := $(WINDOWS_BUILD_DIR)/joicetyper.exe
WINDOWS_RUNTIME_DIR := $(WHISPER_DIR)/build/bin/Release
WINDOWS_RUNTIME_IMPORT_DIR := $(WHISPER_DIR)/build/src/Release
WINDOWS_RUNTIME_DLLS := whisper.dll ggml.dll ggml-base.dll ggml-cpu.dll ggml-vulkan.dll
WINDOWS_CC ?= x86_64-w64-mingw32-gcc
WINDOWS_CXX ?= x86_64-w64-mingw32-g++
WINDOWS_PORTAUDIO_SRC_DIR := third_party/portaudio-windows-src
WINDOWS_PORTAUDIO_BUILD_DIR := $(WINDOWS_PORTAUDIO_SRC_DIR)/build-windows-static
WINDOWS_PORTAUDIO_INSTALL_DIR := third_party/portaudio-windows-static-install
WINDOWS_PORTAUDIO_STATIC_LIB := $(WINDOWS_PORTAUDIO_INSTALL_DIR)/lib/libportaudio.a
WINDOWS_PORTAUDIO_PKGCONFIG_DIR := $(WINDOWS_PORTAUDIO_INSTALL_DIR)/lib/pkgconfig
WINDOWS_PORTAUDIO_PC := $(WINDOWS_PORTAUDIO_PKGCONFIG_DIR)/portaudio-2.0.pc
WINDOWS_MINGW_BIN_DIR ?= $(dir $(shell command -v $(WINDOWS_CXX) 2>/dev/null))
WINDOWS_LIBGCC_DLL ?= $(WINDOWS_MINGW_BIN_DIR)libgcc_s_seh-1.dll
WINDOWS_LIBSTDCXX_DLL ?= $(WINDOWS_MINGW_BIN_DIR)libstdc++-6.dll
WINDOWS_LIBWINPTHREAD_DLL ?= $(WINDOWS_MINGW_BIN_DIR)libwinpthread-1.dll
WINDOWS_LIBGOMP_DLL ?= $(WINDOWS_MINGW_BIN_DIR)libgomp-1.dll
WINDOWS_LIBDL_DLL ?= $(WINDOWS_MINGW_BIN_DIR)libdl.dll
WINDOWS_EXTRA_RUNTIME_DLLS := libwhisper.dll libgcc_s_seh-1.dll libstdc++-6.dll libgomp-1.dll libdl.dll
WINDOWS_OPTIONAL_RUNTIME_DLLS := libwinpthread-1.dll
WINDOWS_APP_ICON := assets/windows/joicetyper.ico
WINDOWS_RUNTIME_STAGE_FILES := joicetyper.exe joicetyper.ico $(WINDOWS_RUNTIME_DLLS) $(WINDOWS_EXTRA_RUNTIME_DLLS)
WINDOWS_INSTALLER_SCRIPT := packaging/windows/joicetyper.iss
WINDOWS_INSTALLER_NAME = JoiceTyper-$(PACKAGE_VERSION)-setup.exe
WINDOWS_INSTALLER_PATH = $(WINDOWS_BUILD_DIR)/$(WINDOWS_INSTALLER_NAME)
ISCC ?= $(shell command -v iscc 2>/dev/null || command -v ISCC.exe 2>/dev/null || for d in '/c/Users/Eko04/AppData/Local/Programs/Inno Setup 6/ISCC.exe' '/c/Program Files/Inno Setup 6/ISCC.exe' '/c/Program Files (x86)/Inno Setup 6/ISCC.exe'; do if [ -f "$$d" ]; then printf '%s\n' "$$d"; break; fi; done)
WINDOWS_HAS_ISCC := $(if $(strip $(ISCC)),1,)
WINDOWS_PKG_CONFIG ?= $(shell command -v pkg-config 2>/dev/null || command -v pkgconf 2>/dev/null)
WINDOWS_MAKE_PROGRAM ?= $(shell command -v mingw32-make.exe 2>/dev/null || command -v mingw32-make 2>/dev/null)
WINDOWS_HAS_MINGW_MAKE := $(if $(strip $(WINDOWS_MAKE_PROGRAM)),1,)
WINDOWS_VULKAN_SDK_ROOT ?= $(shell for d in /c/VulkanSDK/*; do if [ -d "$$d/Include/vulkan" ] && [ -f "$$d/Lib/vulkan-1.lib" ]; then echo $$d; break; fi; done)
WINDOWS_HAS_PKG_CONFIG := $(if $(strip $(WINDOWS_PKG_CONFIG)),1,)
WINDOWS_HAS_VULKAN_SDK := $(if $(strip $(WINDOWS_VULKAN_SDK_ROOT)),1,)
WINDOWS_VULKAN_INCLUDE_DIR ?= $(if $(WINDOWS_VULKAN_SDK_ROOT),$(WINDOWS_VULKAN_SDK_ROOT)/Include)
WINDOWS_VULKAN_LIBRARY ?= $(if $(WINDOWS_VULKAN_SDK_ROOT),$(WINDOWS_VULKAN_SDK_ROOT)/Lib/vulkan-1.lib)

WINDOWS_WEBVIEW2_BOOTSTRAPPER := packaging/windows/MicrosoftEdgeWebview2Setup.exe

WINDOWS_MSIX_DIR := packaging/windows/msix
WINDOWS_MSIX_MANIFEST_TEMPLATE := $(WINDOWS_MSIX_DIR)/AppxManifest.xml.tmpl
WINDOWS_MSIX_ASSETS_DIR := $(WINDOWS_MSIX_DIR)/Assets
WINDOWS_MSIX_IDENTITY_ENV := $(WINDOWS_MSIX_DIR)/identity.local.env
WINDOWS_MSIX_SCRIPT := scripts/release/windows_package_msix.ps1
ifneq ("$(wildcard $(WINDOWS_MSIX_IDENTITY_ENV))","")
include $(WINDOWS_MSIX_IDENTITY_ENV)
export WINDOWS_MSIX_IDENTITY_NAME WINDOWS_MSIX_PUBLISHER WINDOWS_MSIX_PUBLISHER_DISPLAY_NAME WINDOWS_MSIX_DISPLAY_NAME
endif

build-windows-amd64: build-windows-amd64-no-version-bump

build-windows-amd64-no-version-bump: bridge-contract-check frontend-build
	mkdir -p $(WINDOWS_BUILD_DIR)
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(WINDOWS_GO_LDFLAGS)" -o $(WINDOWS_BIN_PATH) ./cmd/joicetyper

build-windows-amd64-release: BUILD_TYPE := release
build-windows-amd64-release: release-check build-windows-amd64-no-version-bump

windows-preflight:
	@echo "checking supported Win11 build environment..."
	@command -v $(WINDOWS_CC) >/dev/null 2>&1 || (echo "fatal: missing Windows C compiler $(WINDOWS_CC)" && exit 1)
	@command -v $(WINDOWS_CXX) >/dev/null 2>&1 || (echo "fatal: missing Windows C++ compiler $(WINDOWS_CXX)" && exit 1)
	@command -v cmake >/dev/null 2>&1 || (echo "fatal: missing cmake" && exit 1)
	@test -n "$(WINDOWS_HAS_MINGW_MAKE)" || (echo "fatal: missing mingw32-make.exe" && exit 1)
	@test -n "$(WINDOWS_HAS_PKG_CONFIG)" || (echo "fatal: missing pkg-config or pkgconf" && exit 1)
	@test -n "$(WINDOWS_HAS_VULKAN_SDK)" || (echo "fatal: missing Vulkan SDK root under /c/VulkanSDK" && exit 1)
	@test -d "$(WINDOWS_VULKAN_INCLUDE_DIR)" || (echo "fatal: missing Vulkan include dir $(WINDOWS_VULKAN_INCLUDE_DIR)" && exit 1)
	@test -f "$(WINDOWS_VULKAN_LIBRARY)" || (echo "fatal: missing Vulkan library $(WINDOWS_VULKAN_LIBRARY)" && exit 1)
	@test -d "$(WINDOWS_PORTAUDIO_SRC_DIR)" || (echo "fatal: missing Windows PortAudio source directory $(WINDOWS_PORTAUDIO_SRC_DIR)" && exit 1)
	@test -n "$(WINDOWS_HAS_ISCC)" || (echo "fatal: missing Inno Setup compiler (ISCC.exe)" && exit 1)
	@echo "Windows C compiler: $(WINDOWS_CC)"
	@echo "Windows C++ compiler: $(WINDOWS_CXX)"
	@echo "MinGW make: $(WINDOWS_MAKE_PROGRAM)"
	@echo "pkg-config: $(WINDOWS_PKG_CONFIG)"
	@echo "Vulkan SDK: $(WINDOWS_VULKAN_SDK_ROOT)"
	@echo "Inno Setup: $(ISCC)"
	@if [ -f "$(WINDOWS_WEBVIEW2_BOOTSTRAPPER)" ]; then \
		echo "WebView2 bootstrapper: bundled"; \
	else \
		echo "note: WebView2 bootstrapper not bundled; installer will require an existing WebView2 runtime"; \
	fi

windows-portaudio-static:
	@test -d "$(WINDOWS_PORTAUDIO_SRC_DIR)" || (echo "fatal: missing Windows PortAudio source directory $(WINDOWS_PORTAUDIO_SRC_DIR)" && exit 1)
	@test -n "$(WINDOWS_HAS_MINGW_MAKE)" || (echo "fatal: missing mingw32-make.exe" && exit 1)
	rm -rf "$(WINDOWS_PORTAUDIO_BUILD_DIR)" "$(WINDOWS_PORTAUDIO_INSTALL_DIR)"
	cmake -G "MinGW Makefiles" -DCMAKE_MAKE_PROGRAM="$(WINDOWS_MAKE_PROGRAM)" -S "$(WINDOWS_PORTAUDIO_SRC_DIR)" -B "$(WINDOWS_PORTAUDIO_BUILD_DIR)" \
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
	@test -n "$(WINDOWS_HAS_MINGW_MAKE)" || (echo "fatal: missing mingw32-make.exe" && exit 1)
	@test -n "$(WINDOWS_HAS_VULKAN_SDK)" || (echo "fatal: missing Vulkan SDK root under /c/VulkanSDK" && exit 1)
	@test -f "$(WINDOWS_VULKAN_LIBRARY)" || (echo "fatal: missing Vulkan library $(WINDOWS_VULKAN_LIBRARY)" && exit 1)
	@test -d "$(WINDOWS_VULKAN_INCLUDE_DIR)" || (echo "fatal: missing Vulkan include dir $(WINDOWS_VULKAN_INCLUDE_DIR)" && exit 1)
	rm -rf "$(WHISPER_BUILD)"
	cmake -G "MinGW Makefiles" -DCMAKE_MAKE_PROGRAM="$(WINDOWS_MAKE_PROGRAM)" -S "$(WHISPER_DIR)" -B "$(WHISPER_BUILD)" \
		-DCMAKE_C_COMPILER="$(WINDOWS_CC)" \
		-DCMAKE_CXX_COMPILER="$(WINDOWS_CXX)" \
		-DVulkan_INCLUDE_DIR="$(WINDOWS_VULKAN_INCLUDE_DIR)" \
		-DVulkan_LIBRARY="$(WINDOWS_VULKAN_LIBRARY)" \
		-DBUILD_SHARED_LIBS=ON \
		-DWHISPER_BUILD_TESTS=OFF \
		-DWHISPER_BUILD_EXAMPLES=OFF \
		-DWHISPER_BUILD_SERVER=OFF \
		-DGGML_NATIVE=OFF \
		-DGGML_AVX=ON \
		-DGGML_AVX2=ON \
		-DGGML_FMA=ON \
		-DGGML_F16C=ON \
		-DGGML_VULKAN=ON \
		-DCMAKE_BUILD_TYPE=Release
	cmake --build "$(WHISPER_BUILD)" --config Release --parallel 8
	mkdir -p "$(WINDOWS_RUNTIME_DIR)" "$(WINDOWS_RUNTIME_IMPORT_DIR)" "$(WHISPER_BUILD)/ggml/src/Release" "$(WHISPER_BUILD)/ggml/src/ggml-cpu/Release"
	cp "$(WHISPER_BUILD)/bin/libwhisper.dll" "$(WINDOWS_RUNTIME_DIR)/whisper.dll"
	cp "$(WHISPER_BUILD)/bin/ggml.dll" "$(WINDOWS_RUNTIME_DIR)/ggml.dll"
	cp "$(WHISPER_BUILD)/bin/ggml-base.dll" "$(WINDOWS_RUNTIME_DIR)/ggml-base.dll"
	cp "$(WHISPER_BUILD)/bin/ggml-cpu.dll" "$(WINDOWS_RUNTIME_DIR)/ggml-cpu.dll"
	cp "$(WHISPER_BUILD)/bin/ggml-vulkan.dll" "$(WINDOWS_RUNTIME_DIR)/ggml-vulkan.dll"
	cp "$(WHISPER_BUILD)/src/libwhisper.dll.a" "$(WINDOWS_RUNTIME_IMPORT_DIR)/libwhisper.dll.a"
	cp "$(WHISPER_BUILD)/ggml/src/libggml.dll.a" "$(WHISPER_BUILD)/ggml/src/Release/libggml.dll.a"
	cp "$(WHISPER_BUILD)/ggml/src/libggml-base.dll.a" "$(WHISPER_BUILD)/ggml/src/Release/libggml-base.dll.a"
	cp "$(WHISPER_BUILD)/ggml/src/libggml-cpu.dll.a" "$(WHISPER_BUILD)/ggml/src/ggml-cpu/Release/libggml-cpu.dll.a"

windows-runtime-prereqs: windows-preflight windows-portaudio-static windows-whisper-runtime-stage
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

build-windows-runtime-amd64: build-windows-runtime-amd64-no-version-bump

build-windows-runtime-amd64-no-version-bump: bridge-contract-check frontend-build windows-runtime-prereqs
	mkdir -p $(WINDOWS_BUILD_DIR)
	rm -f "$(WINDOWS_BUILD_DIR)/libportaudio-2.dll"
	go clean -cache
	PKG_CONFIG="$(WINDOWS_PKG_CONFIG)" PKG_CONFIG_PATH= PKG_CONFIG_LIBDIR="$(CURDIR)/$(WINDOWS_PORTAUDIO_PKGCONFIG_DIR)" \
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
	cp "$(WINDOWS_APP_ICON)" "$(WINDOWS_BUILD_DIR)/joicetyper.ico"
	@for artifact in $(WINDOWS_RUNTIME_STAGE_FILES); do \
		test -f "$(WINDOWS_BUILD_DIR)/$$artifact" || (echo "fatal: missing staged Windows runtime artifact $(WINDOWS_BUILD_DIR)/$$artifact" && exit 1); \
	done

build-windows-runtime-amd64-release: BUILD_TYPE := release
build-windows-runtime-amd64-release: release-check build-windows-runtime-amd64-no-version-bump

package-windows: package-windows-no-version-bump

package-windows-no-version-bump: windows-runtime-stage-check
	@test -f "$(WINDOWS_INSTALLER_SCRIPT)" || (echo "fatal: missing $(WINDOWS_INSTALLER_SCRIPT)" && exit 1)
	@test -n "$(WINDOWS_HAS_ISCC)" || (echo "fatal: missing Inno Setup compiler (ISCC.exe)" && exit 1)
	powershell.exe -ExecutionPolicy Bypass -NoProfile -NonInteractive -File "$(CURDIR)/scripts/release/windows_package_installer.ps1" -IsccPath "$(subst /,\,$(ISCC))" -AppVersion "$(PACKAGE_VERSION)" -RepoRoot "$(subst /,\,$(CURDIR))" -OutputDir "$(subst /,\,$(CURDIR)/$(WINDOWS_BUILD_DIR))" -ScriptPath "$(subst /,\,$(CURDIR)/$(WINDOWS_INSTALLER_SCRIPT))"

package-windows-release: BUILD_TYPE := release
package-windows-release: build-windows-runtime-amd64-release package-windows-no-version-bump

package-windows-runtime: package-windows

package-windows-msix: package-windows-msix-no-version-bump

package-windows-msix-no-version-bump: windows-runtime-stage-check
	@test -f "$(WINDOWS_MSIX_MANIFEST_TEMPLATE)" || (echo "fatal: missing $(WINDOWS_MSIX_MANIFEST_TEMPLATE)" && exit 1)
	@test -f "$(WINDOWS_MSIX_SCRIPT)" || (echo "fatal: missing $(WINDOWS_MSIX_SCRIPT)" && exit 1)
	@test -n "$(WINDOWS_MSIX_IDENTITY_NAME)" || (echo "fatal: WINDOWS_MSIX_IDENTITY_NAME unset (set in $(WINDOWS_MSIX_IDENTITY_ENV) or env)" && exit 1)
	@test -n "$(WINDOWS_MSIX_PUBLISHER)" || (echo "fatal: WINDOWS_MSIX_PUBLISHER unset" && exit 1)
	@test -n "$(WINDOWS_MSIX_PUBLISHER_DISPLAY_NAME)" || (echo "fatal: WINDOWS_MSIX_PUBLISHER_DISPLAY_NAME unset" && exit 1)
	@test -n "$(WINDOWS_MSIX_DISPLAY_NAME)" || (echo "fatal: WINDOWS_MSIX_DISPLAY_NAME unset" && exit 1)
	powershell.exe -ExecutionPolicy Bypass -NoProfile -NonInteractive -File "$(CURDIR)/$(WINDOWS_MSIX_SCRIPT)" \
		-RepoRoot "$(subst /,\,$(CURDIR))" \
		-AppVersion "$(PACKAGE_VERSION)" \
		-IdentityName "$(WINDOWS_MSIX_IDENTITY_NAME)" \
		-Publisher "$(WINDOWS_MSIX_PUBLISHER)" \
		-PublisherDisplayName "$(WINDOWS_MSIX_PUBLISHER_DISPLAY_NAME)" \
		-DisplayName "$(WINDOWS_MSIX_DISPLAY_NAME)" \
		-StagedBuildDir "$(subst /,\,$(CURDIR)/$(WINDOWS_BUILD_DIR))" \
		-OutputDir "$(subst /,\,$(CURDIR)/$(WINDOWS_BUILD_DIR))" \
		-ManifestTemplate "$(subst /,\,$(CURDIR)/$(WINDOWS_MSIX_MANIFEST_TEMPLATE))" \
		-AssetsDir "$(subst /,\,$(CURDIR)/$(WINDOWS_MSIX_ASSETS_DIR))"

package-windows-msix-test-sign: windows-runtime-stage-check
	@test -n "$(WINDOWS_MSIX_TEST_PFX)" || (echo "fatal: WINDOWS_MSIX_TEST_PFX unset (path to self-signed .pfx for sideload testing)" && exit 1)
	@test -f "$(WINDOWS_MSIX_TEST_PFX)" || (echo "fatal: pfx not found: $(WINDOWS_MSIX_TEST_PFX)" && exit 1)
	@test -n "$(WINDOWS_MSIX_IDENTITY_NAME)" || (echo "fatal: WINDOWS_MSIX_IDENTITY_NAME unset" && exit 1)
	@test -n "$(WINDOWS_MSIX_PUBLISHER)" || (echo "fatal: WINDOWS_MSIX_PUBLISHER unset" && exit 1)
	@test -n "$(WINDOWS_MSIX_PUBLISHER_DISPLAY_NAME)" || (echo "fatal: WINDOWS_MSIX_PUBLISHER_DISPLAY_NAME unset" && exit 1)
	@test -n "$(WINDOWS_MSIX_DISPLAY_NAME)" || (echo "fatal: WINDOWS_MSIX_DISPLAY_NAME unset" && exit 1)
	powershell.exe -ExecutionPolicy Bypass -NoProfile -NonInteractive -File "$(CURDIR)/$(WINDOWS_MSIX_SCRIPT)" \
		-RepoRoot "$(subst /,\,$(CURDIR))" \
		-AppVersion "$(PACKAGE_VERSION)" \
		-IdentityName "$(WINDOWS_MSIX_IDENTITY_NAME)" \
		-Publisher "$(WINDOWS_MSIX_PUBLISHER)" \
		-PublisherDisplayName "$(WINDOWS_MSIX_PUBLISHER_DISPLAY_NAME)" \
		-DisplayName "$(WINDOWS_MSIX_DISPLAY_NAME)" \
		-StagedBuildDir "$(subst /,\,$(CURDIR)/$(WINDOWS_BUILD_DIR))" \
		-OutputDir "$(subst /,\,$(CURDIR)/$(WINDOWS_BUILD_DIR))" \
		-ManifestTemplate "$(subst /,\,$(CURDIR)/$(WINDOWS_MSIX_MANIFEST_TEMPLATE))" \
		-AssetsDir "$(subst /,\,$(CURDIR)/$(WINDOWS_MSIX_ASSETS_DIR))" \
		-TestSign \
		-PfxPath "$(subst /,\,$(WINDOWS_MSIX_TEST_PFX))" \
		-PfxPassword "$(WINDOWS_MSIX_TEST_PFX_PASSWORD)"

package-windows-msix-release: BUILD_TYPE := release
package-windows-msix-release: build-windows-runtime-amd64-release package-windows-msix-no-version-bump

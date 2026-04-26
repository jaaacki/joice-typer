# Windows Packaging

This directory contains Windows packaging scripts, templates, manifests, and installer assets.

Current contents:
- `joicetyper.iss`: Inno Setup installer definition for the Windows build output
- optional `MicrosoftEdgeWebview2Setup.exe`: packaged Evergreen WebView2 bootstrapper for installer-time runtime provisioning

The root `Makefile` remains the packaging entrypoint:
- `make windows-preflight`
- `make build-windows-amd64`
- `make build-windows-runtime-amd64`
- `make package-windows`
- `make package-windows-runtime`
- `make build-windows-runtime-amd64-release`
- `make package-windows-release`

Supported standard Win11 local-dev contract:
- MinGW `x86_64-w64-mingw32-gcc`
- MinGW `x86_64-w64-mingw32-g++`
- `mingw32-make.exe`
- `cmake`
- `pkg-config` or `pkgconf`
- Vulkan SDK installed in the supported Win11 layout
- Inno Setup compiler (`ISCC.exe`), including per-user installs under `AppData\Local\Programs\Inno Setup 6`
- local `third_party/portaudio-windows-src` checkout

Local-dev Windows targets are now deterministic and do not bump `VERSION`:
- `build-windows-runtime-amd64` builds/stages the native runtime into `build/windows-amd64`
- `package-windows` packages the already-staged runtime bundle into a setup executable

Release-only version mutation is explicit:
- `build-windows-runtime-amd64-release`
- `package-windows-release`

Runtime packaging is fail-closed:
- `windows-preflight` validates the supported Win11 toolchain before the expensive runtime build runs
- `build-windows-runtime-amd64` requires a Windows CGO toolchain and a local `third_party/portaudio-windows-src` checkout
- the runtime build stages whisper/ggml Windows artifacts into the expected `build/.../Release` layout automatically
- missing `whisper.dll`, `ggml.dll`, `ggml-base.dll`, `ggml-cpu.dll`, or `ggml-vulkan.dll` now fails the build before installer generation
- the packaged runtime bundle includes the MinGW support DLLs required by the whisper/ggml stack (`libwhisper.dll`, `libgcc_s_seh-1.dll`, `libstdc++-6.dll`, `libgomp-1.dll`, `libdl.dll`, and `libwinpthread-1.dll` when present)
- `package-windows` now packages only the staged runtime build path instead of forcing a fresh rebuild
- if the target machine might not already have WebView2 Runtime, place `MicrosoftEdgeWebview2Setup.exe` in this directory before building the installer so setup can install WebView2 silently when needed

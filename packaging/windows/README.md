# Windows Packaging

This directory contains Windows packaging scripts, templates, manifests, and installer assets.

Current contents:
- `joicetyper.iss`: Inno Setup installer definition for the Windows build output
- optional `MicrosoftEdgeWebview2Setup.exe`: packaged Evergreen WebView2 bootstrapper for installer-time runtime provisioning

The root `Makefile` remains the packaging entrypoint:
- `make build-windows-amd64`
- `make build-windows-runtime-amd64`
- `make package-windows`
- `make package-windows-runtime`

These artifact-producing build targets now bump the checked-in patch version in `VERSION` once per invocation before building, so Windows installers and binaries always get a newer semantic version.

Runtime packaging is fail-closed:
- `build-windows-runtime-amd64` requires a Windows CGO toolchain and a local `third_party/portaudio-windows-src` checkout
- the runtime build now stages whisper/ggml Windows artifacts into the expected `build/.../Release` layout automatically
- missing `whisper.dll`, `ggml.dll`, `ggml-base.dll`, or `ggml-cpu.dll` now fails the build before installer generation
- the packaged runtime bundle includes the MinGW support DLLs required by the whisper/ggml stack (`libwhisper.dll`, `libgcc_s_seh-1.dll`, `libstdc++-6.dll`, `libgomp-1.dll`, `libdl.dll`, and `libwinpthread-1.dll` when present)
- `package-windows` now packages only the runtime build path; `build-windows-amd64` remains a bootstrap/non-CGO shell build
- if the target machine might not already have WebView2 Runtime, place `MicrosoftEdgeWebview2Setup.exe` in this directory before building the installer so setup can install WebView2 silently when needed

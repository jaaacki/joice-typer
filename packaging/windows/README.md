# Windows Packaging

This directory contains Windows packaging scripts, templates, manifests, and installer assets.

Current contents:
- `joicetyper.iss`: Inno Setup installer definition for the Windows build output

The root `Makefile` remains the packaging entrypoint:
- `make build-windows-amd64`
- `make build-windows-runtime-amd64`
- `make package-windows`
- `make package-windows-runtime`

Runtime packaging is fail-closed:
- `build-windows-runtime-amd64` requires a Windows CGO toolchain and staged whisper/ggml runtime DLLs
- missing `whisper.dll`, `ggml.dll`, `ggml-base.dll`, or `ggml-cpu.dll` now fails the build before installer generation

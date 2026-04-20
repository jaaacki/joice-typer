# Windows Packaging

This directory contains Windows packaging scripts, templates, manifests, and installer assets.

Current contents:
- `joicetyper.iss`: Inno Setup installer definition for the Windows build output

The root `Makefile` remains the packaging entrypoint:
- `make build-windows-amd64`
- `make package-windows`

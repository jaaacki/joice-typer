# Worktree setup (Windows dev)

Git worktrees share `.git` but not the working-tree files. Our build pipeline
depends on three pieces that live *outside* of source control — after creating
a worktree you must populate them before the first build succeeds.

## One-time bootstrap

From inside the new worktree directory:

```bash
# 1. Init the whisper.cpp submodule (headers + source)
#    Without this, cgo can't find <whisper.h>.
git submodule update --init third_party/whisper.cpp

# 2. Install JS deps and build the embedded web UI
(cd ui && npm install && npm run build)

# 3. Either rebuild portaudio + whisper DLLs from scratch (slow, ~15 min)...
#    via `make windows-runtime-prereqs`
#    ...or copy the prebuilt artifacts from your primary clone:
cp -r ../joice-typer/third_party/whisper.cpp/build third_party/whisper.cpp/build
cp -r ../joice-typer/third_party/portaudio-windows-static-install third_party/portaudio-windows-static-install
```

## Compiling the exe

Once bootstrapped, a lightweight rebuild only touches Go + UI.

First: resolve your MinGW toolchain path. On Windows, MinGW is typically installed
via winget, scoop, or MSYS2 — find where `x86_64-w64-mingw32-gcc.exe` lives and
export it as `MINGW_BIN`:

```bash
# Resolve dynamically from PATH (works in Git Bash with MinGW on PATH):
MINGW_BIN="$(dirname "$(command -v x86_64-w64-mingw32-gcc)")"
# Or set it explicitly to wherever your MinGW lives, e.g.:
#   MINGW_BIN="C:/Users/<you>/AppData/Local/.../mingw64/bin"
#   MINGW_BIN="C:/msys64/mingw64/bin"
```

Then build:

```bash
# 1. Rebuild the web UI if any ui/ files changed
(cd ui && npm run build)

# 2. Cross-compile the Go binary with CGO against the prebuilt DLLs
VERSION=$(cat VERSION)
PKG_CONFIG="pkg-config" PKG_CONFIG_PATH= \
PKG_CONFIG_LIBDIR="$(pwd)/third_party/portaudio-windows-static-install/lib/pkgconfig" \
CC="$MINGW_BIN/x86_64-w64-mingw32-gcc.exe" \
CXX="$MINGW_BIN/x86_64-w64-mingw32-g++.exe" \
CGO_LDFLAGS="-lwinmm -lole32 -luuid" \
GOOS=windows GOARCH=amd64 CGO_ENABLED=1 \
go build -ldflags "-X 'voicetype/internal/core/version.Version=$VERSION' -H=windowsgui -extldflags=-Wl,--subsystem,windows" \
  -o build/windows-amd64/joicetyper.exe ./cmd/joicetyper
```

## Embedded icon resource

The app icon is embedded via a `.syso` file compiled from `packaging/windows/joicetyper.rc`.
Regenerate only when the icon file changes (requires `$MINGW_BIN` as above):

```bash
"$MINGW_BIN/windres.exe" \
  packaging/windows/joicetyper.rc \
  -O coff \
  -o cmd/joicetyper/joicetyper_windows.syso
```

Go automatically links any `*_windows.syso` file in the main package on Windows builds.

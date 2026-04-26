BASE_VERSION = $(shell tr -d '[:space:]' < '$(VERSION_FILE)')
GIT_COMMIT = $(shell git rev-parse --short HEAD)
GIT_DIRTY_SUFFIX = $(shell if [ -n "$$(git status --porcelain)" ]; then printf '.dirty'; fi)

DEV_VERSION = $(BASE_VERSION)-dev+$(GIT_COMMIT)$(GIT_DIRTY_SUFFIX)
RELEASE_VERSION = $(BASE_VERSION)
BUILD_TYPE ?= dev
DISPLAY_VERSION = $(if $(filter release,$(BUILD_TYPE)),$(RELEASE_VERSION),$(DEV_VERSION))
PACKAGE_VERSION = $(subst +,-,$(DISPLAY_VERSION))

VERSION = $(BASE_VERSION)
GO_LDFLAGS = -X 'voicetype/internal/core/version.Version=$(DISPLAY_VERSION)'
WINDOWS_GO_LDFLAGS = $(GO_LDFLAGS) -H=windowsgui -extldflags=-Wl,--subsystem,windows

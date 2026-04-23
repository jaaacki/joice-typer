//go:build windows && cgo

package transcription

/*
#cgo windows CFLAGS: -I${SRCDIR}/../../../third_party/whisper.cpp/include -I${SRCDIR}/../../../third_party/whisper.cpp/ggml/include
#cgo windows LDFLAGS: -L${SRCDIR}/../../../third_party/whisper.cpp/build/src/Release -L${SRCDIR}/../../../third_party/whisper.cpp/build/ggml/src/Release -L${SRCDIR}/../../../third_party/whisper.cpp/build/ggml/src/ggml-cpu/Release -lwhisper -lggml -lggml-base -lggml-cpu -lstdc++
#include <stdlib.h>
#include <string.h>
#include <whisper.h>
#include <ggml-backend.h>

extern void goJoiceTyperWhisperLogCallback(int level, char * text);

static void joicetyper_whisper_log_callback(enum ggml_log_level level, const char * text, void * user_data) {
    goJoiceTyperWhisperLogCallback((int) level, (char *) text);
}

static void joicetyper_install_whisper_log_callback(void) {
    whisper_log_set(joicetyper_whisper_log_callback, NULL);
}

static void joicetyper_reset_whisper_log_callback(void) {
    whisper_log_set(NULL, NULL);
}

static char * joicetyper_backend_inventory(const char * dir_path) {
    ggml_backend_load_all_from_path(dir_path);

    size_t count = ggml_backend_dev_count();
    size_t cap = 8192;
    char * out = (char *) malloc(cap);
    if (!out) {
        return NULL;
    }
    out[0] = '\0';

    for (size_t i = 0; i < count; ++i) {
        ggml_backend_dev_t dev = ggml_backend_dev_get(i);
        const char * name = ggml_backend_dev_name(dev);
        const char * desc = ggml_backend_dev_description(dev);
        enum ggml_backend_dev_type type = ggml_backend_dev_type(dev);
        const char * type_name = type == GGML_BACKEND_DEVICE_TYPE_GPU ? "gpu" :
                                 type == GGML_BACKEND_DEVICE_TYPE_IGPU ? "igpu" :
                                 type == GGML_BACKEND_DEVICE_TYPE_ACCEL ? "accel" : "cpu";
        char line[1024];
        snprintf(line, sizeof(line), "%s\t%s\t%s\n", type_name, name ? name : "", desc ? desc : "");
        if (strlen(out) + strlen(line) + 1 >= cap) {
            cap *= 2;
            char * resized = (char *) realloc(out, cap);
            if (!resized) {
                free(out);
                return NULL;
            }
            out = resized;
        }
        strcat(out, line);
    }
    return out;
}
*/
import "C"

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	bridgepkg "voicetype/internal/core/bridge"
)

func windowsRuntimeDir() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve executable path: %w", err)
	}
	return filepath.Dir(exePath), nil
}

func windowsBackendInventory() []bridgepkg.MachineBackendSnapshot {
	dir, err := windowsRuntimeDir()
	if err != nil {
		return nil
	}
	cDir := C.CString(dir)
	defer C.free(unsafe.Pointer(cDir))
	inventory := C.joicetyper_backend_inventory(cDir)
	if inventory == nil {
		return nil
	}
	defer C.free(unsafe.Pointer(inventory))
	text := strings.TrimSpace(C.GoString(inventory))
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	backends := make([]bridgepkg.MachineBackendSnapshot, 0, len(lines))
	for _, line := range lines {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			continue
		}
		backends = append(backends, bridgepkg.MachineBackendSnapshot{
			Kind:        strings.TrimSpace(parts[0]),
			Name:        strings.TrimSpace(parts[1]),
			Description: strings.TrimSpace(parts[2]),
		})
	}
	return backends
}

func WindowsBackendInventory() []bridgepkg.MachineBackendSnapshot {
	return windowsBackendInventory()
}

func windowsSelectedGPUDevice() (int, bridgepkg.MachineBackendSnapshot, bool) {
	backends := windowsBackendInventory()
	selected := bridgepkg.MachineBackendSnapshot{}
	selectedIndex := -1
	fallbackIndex := -1
	for i, backend := range backends {
		if backend.Kind != "igpu" && backend.Kind != "gpu" {
			continue
		}
		name := strings.ToLower(backend.Name + " " + backend.Description)
		if fallbackIndex == -1 {
			fallbackIndex = i
		}
		if backend.Kind == "igpu" && strings.Contains(name, "amd") {
			selectedIndex = i
			selected = backend
			break
		}
		if selectedIndex == -1 && backend.Kind == "igpu" {
			selectedIndex = i
			selected = backend
		}
	}
	if selectedIndex >= 0 {
		return selectedIndex, selected, true
	}
	if fallbackIndex >= 0 {
		return fallbackIndex, backends[fallbackIndex], true
	}
	return -1, bridgepkg.MachineBackendSnapshot{}, false
}

func logWindowsBackendInventory(logger *slog.Logger) {
	backends := windowsBackendInventory()
	if len(backends) == 0 {
		return
	}
	var lines []string
	for i, backend := range backends {
		lines = append(lines, backend.Kind+"["+strconv.Itoa(i)+"]: "+backend.Name+" | "+backend.Description)
	}
	logger.Info("ggml backend inventory", "operation", "NewTranscriber", "inventory", strings.Join(lines, "\n")+"\n")
}

type windowsWhisperBackendState struct {
	usingBackend string
	usingVulkan  bool
	noGPUFound   bool
}

var (
	windowsWhisperLogMu     sync.Mutex
	windowsWhisperLogLogger *slog.Logger
	windowsWhisperLogState  windowsWhisperBackendState
)

func beginWindowsWhisperBackendLogging(logger *slog.Logger) {
	windowsWhisperLogMu.Lock()
	windowsWhisperLogLogger = logger
	windowsWhisperLogState = windowsWhisperBackendState{}
	windowsWhisperLogMu.Unlock()
	C.joicetyper_install_whisper_log_callback()
}

func endWindowsWhisperBackendLogging() windowsWhisperBackendState {
	windowsWhisperLogMu.Lock()
	state := windowsWhisperLogState
	windowsWhisperLogLogger = nil
	windowsWhisperLogState = windowsWhisperBackendState{}
	windowsWhisperLogMu.Unlock()
	C.joicetyper_reset_whisper_log_callback()
	return state
}

//export goJoiceTyperWhisperLogCallback
func goJoiceTyperWhisperLogCallback(level C.int, text *C.char) {
	message := strings.TrimSpace(C.GoString(text))
	if message == "" {
		return
	}

	windowsWhisperLogMu.Lock()
	logger := windowsWhisperLogLogger
	if strings.Contains(message, "whisper_backend_init_gpu: using ") {
		windowsWhisperLogState.usingBackend = message
		windowsWhisperLogState.usingVulkan = strings.Contains(message, "Vulkan")
	}
	if strings.Contains(message, "whisper_backend_init_gpu: no GPU found") {
		windowsWhisperLogState.noGPUFound = true
	}
	windowsWhisperLogMu.Unlock()

	if logger != nil {
		logger.Info("whisper backend log", "operation", "NewTranscriber", "message", message, "level", int(level))
	}
}

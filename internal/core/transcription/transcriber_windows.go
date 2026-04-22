//go:build windows && cgo

package transcription

/*
#cgo windows CFLAGS: -I${SRCDIR}/../../../third_party/whisper.cpp/include -I${SRCDIR}/../../../third_party/whisper.cpp/ggml/include
#cgo windows LDFLAGS: -L${SRCDIR}/../../../third_party/whisper.cpp/build/src/Release -L${SRCDIR}/../../../third_party/whisper.cpp/build/ggml/src/Release -L${SRCDIR}/../../../third_party/whisper.cpp/build/ggml/src/ggml-cpu/Release -lwhisper -lggml -lggml-base -lggml-cpu -lstdc++
#include <stdlib.h>
#include <string.h>
#include <whisper.h>
#include <ggml-backend.h>

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
        snprintf(line, sizeof(line), "%s[%zu]: %s | %s\n", type_name, i, name ? name : "<nil>", desc ? desc : "<nil>");
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
	"log/slog"
	"path/filepath"
	"unsafe"
)

func logWindowsBackendInventory(modelPath string, logger *slog.Logger) {
	dir := filepath.Dir(modelPath)
	cDir := C.CString(dir)
	defer C.free(unsafe.Pointer(cDir))
	inventory := C.joicetyper_backend_inventory(cDir)
	if inventory == nil {
		return
	}
	defer C.free(unsafe.Pointer(inventory))
	logger.Info("ggml backend inventory", "operation", "NewTranscriber", "inventory", C.GoString(inventory))
}

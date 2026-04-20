//go:build darwin

package transcription

/*
#cgo CFLAGS: -I${SRCDIR}/../../../third_party/whisper.cpp/include -I${SRCDIR}/../../../third_party/whisper.cpp/ggml/include
#cgo LDFLAGS: -L${SRCDIR}/../../../third_party/whisper.cpp/build/src -L${SRCDIR}/../../../third_party/whisper.cpp/build/ggml/src -L${SRCDIR}/../../../third_party/whisper.cpp/build/ggml/src/ggml-metal -L${SRCDIR}/../../../third_party/whisper.cpp/build/ggml/src/ggml-blas -lwhisper -lggml -lggml-base -lggml-cpu -lggml-metal -lggml-blas -lstdc++ -framework Accelerate -framework Metal -framework Foundation -framework CoreML
*/
import "C"

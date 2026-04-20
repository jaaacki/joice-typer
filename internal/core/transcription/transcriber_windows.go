//go:build windows && cgo

package transcription

/*
#cgo windows CFLAGS: -I${SRCDIR}/../../../third_party/whisper.cpp/include -I${SRCDIR}/../../../third_party/whisper.cpp/ggml/include
#cgo windows LDFLAGS: -L${SRCDIR}/../../../third_party/whisper.cpp/build/src/Release -L${SRCDIR}/../../../third_party/whisper.cpp/build/ggml/src/Release -L${SRCDIR}/../../../third_party/whisper.cpp/build/ggml/src/ggml-cpu/Release -lwhisper -lggml -lggml-base -lggml-cpu -lstdc++
*/
import "C"

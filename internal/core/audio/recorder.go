//go:build darwin || (windows && cgo)

package audio

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"runtime"
	"sync"
	"time"

	config "voicetype/internal/core/config"
	apppkg "voicetype/internal/core/runtime"

	"github.com/gordonklaus/portaudio"
)

type portaudioRecorder struct {
	sampleRate     float64 // target sample rate for whisper (e.g. 16000)
	captureRate    float64 // actual capture rate used by the open stream
	deviceName     string
	stream         *portaudio.Stream
	activeStream   *portaudio.Stream // guarded by mu; used by Stop to force-abort on timeout
	warmStream     *portaudio.Stream // pre-opened stream for instant start
	warmStreamRate float64
	buffer         []float32
	audio          []float32
	mu             sync.Mutex
	recording      bool
	done           chan struct{}
	sessionID      uint64
	logger         *slog.Logger
	maxSamples     int
	totalSamples   int
	unhealthy      bool
}

func NewRecorder(sampleRate int, deviceName string, logger *slog.Logger) apppkg.Recorder {
	return &portaudioRecorder{
		sampleRate:  float64(sampleRate),
		captureRate: float64(sampleRate), // updated when stream opens; may differ from sampleRate
		deviceName:  deviceName,
		logger:      logger.With("component", "recorder"),
		maxSamples:  sampleRate * 90, // updated at stream-open time if captureRate differs
	}
}

// resampleLinear resamples audio between sample rates.
// For downsampling, it uses a band-limited windowed-sinc kernel to preserve
// speech quality. For upsampling, it falls back to linear interpolation.
func resampleLinear(src []float32, srcRate, dstRate float64) []float32 {
	if srcRate == dstRate || len(src) == 0 {
		return src
	}
	outLen := int(math.Round(float64(len(src)) * dstRate / srcRate))
	if outLen <= 0 {
		return nil
	}
	if dstRate >= srcRate {
		ratio := srcRate / dstRate
		out := make([]float32, outLen)
		for i := range out {
			pos := float64(i) * ratio
			lo := int(pos)
			hi := lo + 1
			frac := float32(pos - float64(lo))
			if hi >= len(src) {
				out[i] = src[lo]
			} else {
				out[i] = src[lo]*(1-frac) + src[hi]*frac
			}
		}
		return out
	}

	const filterRadius = 16.0
	cutoff := dstRate / srcRate
	out := make([]float32, outLen)
	for i := range out {
		center := float64(i) * srcRate / dstRate
		start := int(math.Floor(center - filterRadius + 1))
		end := int(math.Ceil(center + filterRadius))
		var sum float64
		var norm float64
		for j := start; j <= end; j++ {
			if j < 0 || j >= len(src) {
				continue
			}
			x := center - float64(j)
			if math.Abs(x) > filterRadius {
				continue
			}
			weight := cutoff * windowedSinc(x*cutoff) * (0.5 + 0.5*math.Cos(math.Pi*x/filterRadius))
			sum += float64(src[j]) * weight
			norm += weight
		}
		if norm != 0 {
			sum /= norm
		}
		out[i] = float32(sum)
	}
	return out
}

func windowedSinc(x float64) float64 {
	if x == 0 {
		return 1
	}
	pix := math.Pi * x
	return math.Sin(pix) / pix
}

func InitAudio() error {
	return portaudio.Initialize()
}

func TerminateAudio() error {
	return portaudio.Terminate()
}

// RefreshDevices safely closes any warm or active streams, re-initializes
// PortAudio to pick up newly connected devices (e.g. Bluetooth microphones),
// and re-warms the stream for instant recording.
func (r *portaudioRecorder) RefreshDevices() error {
	r.mu.Lock()
	// Reject if recording is active
	if r.recording {
		r.mu.Unlock()
		return fmt.Errorf("recorder.RefreshDevices: cannot refresh while recording")
	}
	// Wait for any in-flight session cleanup to complete.
	// Stop() clears r.recording before done is closed, so checking
	// recording alone is insufficient — readLoop cleanup may still
	// be in flight.
	done := r.done
	r.mu.Unlock()

	if done != nil {
		select {
		case <-done:
			// Previous session fully exited
		case <-time.After(2 * time.Second):
			return fmt.Errorf("recorder.RefreshDevices: previous session still cleaning up")
		}
	}

	r.mu.Lock()
	// Re-check: a new recording may have started while we waited
	if r.recording {
		r.mu.Unlock()
		return fmt.Errorf("recorder.RefreshDevices: recording started during wait")
	}
	if r.warmStream != nil {
		if err := r.warmStream.Close(); err != nil {
			r.logger.Warn("failed to close warm stream during refresh",
				"operation", "RefreshDevices", "error", err)
		}
		r.warmStream = nil
	}
	r.mu.Unlock()

	if err := portaudio.Terminate(); err != nil {
		return fmt.Errorf("recorder.RefreshDevices: terminate: %w", err)
	}
	if err := portaudio.Initialize(); err != nil {
		return fmt.Errorf("recorder.RefreshDevices: initialize: %w", err)
	}
	r.mu.Lock()
	r.unhealthy = false
	r.mu.Unlock()

	// Re-warm after refresh so next recording starts instantly
	r.Warm()
	return nil
}

func (r *portaudioRecorder) MarkStale(reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.unhealthy = true
	if r.warmStream != nil {
		if err := r.warmStream.Close(); err != nil {
			r.logger.Warn("failed to close warm stream while marking stale",
				"operation", "MarkStale", "reason", reason, "error", err)
		}
		r.warmStream = nil
	}
	r.logger.Info("recorder marked stale",
		"operation", "MarkStale", "reason", reason)
}

func listDevicesConfigHint() string {
	cfgPath, err := config.DefaultConfigPath()
	if err != nil {
		switch runtime.GOOS {
		case "windows":
			return `%APPDATA%\JoiceTyper\config.yaml`
		default:
			return "~/Library/Application Support/JoiceTyper/config.yaml"
		}
	}
	return cfgPath
}

func findInputDevice(name string) (*portaudio.DeviceInfo, error) {
	devices, err := portaudio.Devices()
	if err != nil {
		return nil, &apppkg.ErrDependencyUnavailable{Component: "recorder", Operation: "findInputDevice", Wrapped: err}
	}
	for _, d := range devices {
		if d.MaxInputChannels > 0 && d.Name == name {
			return d, nil
		}
	}
	return nil, &apppkg.ErrDependencyUnavailable{Component: "recorder", Operation: "findInputDevice", Wrapped: fmt.Errorf("input device %q not found", name)}
}

func findInputDeviceByID(id string) (*portaudio.DeviceInfo, error) {
	if runtime.GOOS != "windows" {
		return findInputDevice(id)
	}
	devices, err := portaudio.Devices()
	if err != nil {
		return nil, &apppkg.ErrDependencyUnavailable{Component: "recorder", Operation: "findInputDeviceByID", Wrapped: err}
	}
	snapshots, snapErr := ListInputDeviceSnapshots()
	if snapErr != nil {
		return nil, &apppkg.ErrDependencyUnavailable{Component: "recorder", Operation: "findInputDeviceByID", Wrapped: snapErr}
	}
	nameByID := make(map[string]string, len(snapshots))
	for _, snapshot := range snapshots {
		nameByID[snapshot.ID] = snapshot.Name
	}
	name, ok := nameByID[id]
	if !ok {
		return nil, &apppkg.ErrDependencyUnavailable{Component: "recorder", Operation: "findInputDeviceByID", Wrapped: fmt.Errorf("input device %q not found", id)}
	}
	for _, d := range devices {
		if d.MaxInputChannels > 0 && d.Name == name {
			return d, nil
		}
	}
	return nil, &apppkg.ErrDependencyUnavailable{Component: "recorder", Operation: "findInputDeviceByID", Wrapped: fmt.Errorf("input device %q resolved to %q but portaudio device was not found", id, name)}
}

// openStream creates and returns a PortAudio stream for the configured device.
// Returns the stream and the actual sample rate used (may differ from r.sampleRate
// if the device does not support the requested rate and a fallback was used).
func (r *portaudioRecorder) openStream(buf []float32) (*portaudio.Stream, float64, error) {
	device, devErr := r.resolveDevice()
	if devErr != nil {
		// No device info — try requested rate directly.
		stream, err := portaudio.OpenDefaultStream(1, 0, r.sampleRate, len(buf), buf)
		if err != nil {
			return nil, 0, err
		}
		return stream, r.sampleRate, nil
	}

	for _, rate := range []float64{r.sampleRate, device.DefaultSampleRate} {
		if rate <= 0 {
			continue
		}
		params := portaudio.StreamParameters{
			Input: portaudio.StreamDeviceParameters{
				Device:   device,
				Channels: 1,
				Latency:  device.DefaultLowInputLatency,
			},
			SampleRate:      rate,
			FramesPerBuffer: len(buf),
		}
		stream, err := portaudio.OpenStream(params, buf)
		if err == nil {
			if rate != r.sampleRate {
				r.logger.Info("opened audio stream at native device rate",
					"operation", "openStream",
					"requested_rate", r.sampleRate,
					"actual_rate", rate)
			}
			return stream, rate, nil
		}
		r.logger.Debug("openStream rate not supported, trying next",
			"operation", "openStream", "rate", rate, "error", err)
	}
	return nil, 0, fmt.Errorf("device does not support rates %v or %v", r.sampleRate, device.DefaultSampleRate)
}

func (r *portaudioRecorder) resolveDevice() (*portaudio.DeviceInfo, error) {
	if r.deviceName != "" {
		device, err := findInputDeviceByID(r.deviceName)
		if err != nil {
			r.logger.Warn("configured device not found, using default input",
				"operation", "openStream",
				"device", r.deviceName, "error", err)
		} else {
			r.logger.Info("using configured input device", "operation", "openStream", "device", r.deviceName, "portaudio_name", device.Name)
			return device, nil
		}
	}
	device, err := portaudio.DefaultInputDevice()
	if err == nil && device != nil {
		r.logger.Info("using default input device", "operation", "openStream", "portaudio_name", device.Name)
	}
	return device, err
}

// Warm pre-opens the audio stream so Start() is near-instant.
// Call after the app is ready. Safe to call multiple times.
func (r *portaudioRecorder) Warm() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.warmStream != nil {
		return // already warmed
	}
	if r.unhealthy {
		r.logger.Warn("skipping warm while recorder backend is unhealthy",
			"operation", "Warm")
		return
	}

	buf := make([]float32, 256)
	stream, actualRate, err := r.openStream(buf)
	if err != nil {
		r.logger.Warn("failed to pre-warm audio stream",
			"operation", "Warm", "error", err)
		return
	}
	r.warmStream = stream
	r.warmStreamRate = actualRate
	r.captureRate = actualRate
	r.maxSamples = int(actualRate * 90.0)
	r.buffer = buf
	r.logger.Info("audio stream pre-warmed", "operation", "Warm")
}

func (r *portaudioRecorder) Start(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	startTime := time.Now()

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.recording {
		return fmt.Errorf("recorder.Start: already recording")
	}

	// Block new sessions until previous one is fully cleaned up.
	// If a previous readLoop leaked (done not nil and not closed), wait up to
	// 6s for the 5s watchdog in readLoop to close it.
	if r.done != nil {
		prevDone := r.done
		r.mu.Unlock()
		select {
		case <-prevDone:
			// Previous session exited
		case <-time.After(6 * time.Second):
			r.mu.Lock() // re-acquire so defer Unlock is balanced
			return fmt.Errorf("recorder.Start: previous session leaked, cannot start")
		}
		r.mu.Lock()
		// Re-check after re-acquiring lock
		if r.recording {
			return fmt.Errorf("recorder.Start: already recording")
		}
	}
	if r.unhealthy {
		r.mu.Unlock()
		refreshErr := r.RefreshDevices()
		r.mu.Lock()
		if refreshErr != nil {
			return &apppkg.ErrDependencyUnavailable{
				Component: "recorder",
				Operation: "Start",
				Wrapped:   fmt.Errorf("refresh unhealthy backend: %w", refreshErr),
			}
		}
		if r.recording {
			return fmt.Errorf("recorder.Start: already recording")
		}
	}

	r.logger.Debug("starting", "operation", "Start",
		"sample_rate", r.sampleRate, "device", r.deviceName)

	r.audio = nil
	r.totalSamples = 0
	r.recording = true
	r.sessionID++

	var stream *portaudio.Stream
	var err error
	wasWarm := false

	// Use pre-warmed stream if available (near-instant start)
	if r.warmStream != nil {
		stream = r.warmStream
		r.warmStream = nil
		if r.warmStreamRate > 0 {
			r.captureRate = r.warmStreamRate
			r.maxSamples = int(r.warmStreamRate * 90.0)
		}
		r.warmStreamRate = 0
		wasWarm = true
		r.logger.Debug("using pre-warmed stream", "operation", "Start",
			"elapsed_us", time.Since(startTime).Microseconds())
	} else {
		// Cold path: open stream now (slow — can take 1-2 seconds)
		coldStart := time.Now()
		r.buffer = make([]float32, 256)
		var actualRate float64
		stream, actualRate, err = r.openStream(r.buffer)
		if err != nil {
			r.recording = false
			r.unhealthy = true
			return &apppkg.ErrDependencyUnavailable{Component: "recorder", Operation: "Start", Wrapped: fmt.Errorf("open stream: %w", err)}
		}
		r.captureRate = actualRate
		r.maxSamples = int(actualRate * 90.0)
		r.logger.Debug("cold stream opened", "operation", "Start",
			"open_ms", time.Since(coldStart).Milliseconds())
	}

	streamStartTime := time.Now()
	if err := stream.Start(); err != nil {
		r.recording = false
		r.unhealthy = true
		if closeErr := stream.Close(); closeErr != nil {
			r.logger.Error("failed to close stream after start error",
				"operation", "Start", "error", closeErr)
		}
		return &apppkg.ErrDependencyUnavailable{Component: "recorder", Operation: "Start", Wrapped: fmt.Errorf("start stream: %w", err)}
	}
	r.logger.Debug("stream started", "operation", "Start",
		"stream_start_ms", time.Since(streamStartTime).Milliseconds())

	// Flush one buffer on warm-stream reuse to discard stale WASAPI frames
	// that may have accumulated while the stream sat idle. Done before
	// readLoop starts so the goroutine never sees the stale samples.
	if wasWarm {
		_ = stream.Read()
	}

	r.stream = stream
	r.activeStream = stream
	r.unhealthy = false

	done := make(chan struct{})
	r.done = done
	go r.readLoop(stream, r.buffer, done, r.sessionID)

	r.logger.Debug("recording started", "operation", "Start",
		"total_start_ms", time.Since(startTime).Milliseconds())
	return nil
}

// readLoop reads audio chunks from the stream until stopped.
// Owns the stream reference passed by value — closes it on exit.
// Uses sessionID to detect if it has been superseded by a new session.
func (r *portaudioRecorder) readLoop(stream *portaudio.Stream, buffer []float32, done chan struct{}, sessionID uint64) {
	defer func() {
		stream.Close()
		r.mu.Lock()
		// Only clear activeStream if this session still owns it.
		// A late zombie readLoop must not clobber a newer session's handle.
		if r.sessionID == sessionID {
			r.activeStream = nil
		}
		r.mu.Unlock()
		close(done)
	}()
	for {
		// Watchdog: if stream.Read() doesn't return in 5s, the OS audio callback is stuck.
		// One goroutine per iteration is safe — the loop is sequential (waits for Read to return).
		readDone := make(chan error, 1)
		go func() {
			readDone <- stream.Read()
		}()

		var readErr error
		select {
		case readErr = <-readDone:
			// normal path
		case <-time.After(5 * time.Second):
			r.logger.Error("stream.Read() hung for 5s, aborting recording",
				"operation", "readLoop")
			r.mu.Lock()
			if r.sessionID == sessionID {
				r.unhealthy = true
			}
			r.mu.Unlock()
			stream.Abort()
			return
		}

		if readErr != nil {
			// stream.Read returns error when stream is stopped — expected
			return
		}
		r.mu.Lock()
		if !r.recording || r.sessionID != sessionID {
			r.mu.Unlock()
			return
		}
		r.audio = append(r.audio, buffer...)
		r.totalSamples += len(buffer)
		if r.totalSamples >= r.maxSamples {
			r.recording = false
			r.mu.Unlock()
			r.logger.Warn("max recording duration reached",
				"operation", "readLoop",
				"total_samples", r.totalSamples,
				"max_samples", r.maxSamples)
			return
		}
		r.mu.Unlock()
	}
}

func (r *portaudioRecorder) Snapshot() []float32 {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.audio) == 0 {
		return nil
	}
	return append([]float32(nil), r.audio...)
}

func (r *portaudioRecorder) Stop() ([]float32, error) {
	r.mu.Lock()
	r.recording = false
	audio := append([]float32(nil), r.audio...)
	r.audio = nil
	done := r.done
	r.mu.Unlock()

	if done == nil {
		return nil, fmt.Errorf("recorder.Stop: not recording")
	}

	r.logger.Debug("stopping", "operation", "Stop")

	// Don't call stream.Stop() — it deadlocks with stream.Read() in PortAudio.
	// readLoop checks r.recording after each Read() (~64ms buffer cycle) and exits.
	// readLoop's defer calls stream.Close() which implicitly stops and releases.
	select {
	case <-done:
		// readLoop exited cleanly
	case <-time.After(500 * time.Millisecond):
		r.logger.Warn("readLoop did not exit in time, force-aborting stream",
			"operation", "Stop")
		r.mu.Lock()
		if r.activeStream != nil {
			r.activeStream.Abort()
			r.activeStream = nil
		}
		r.mu.Unlock()
		// Wait a bit more for goroutine to notice
		select {
		case <-done:
		case <-time.After(500 * time.Millisecond):
			r.logger.Error("readLoop goroutine leaked after force-abort, watchdog will clean up",
				"operation", "Stop")
			// Leave done in place — Start() will wait with timeout for the 5s watchdog
		}
	}

	total := len(audio)
	r.mu.Lock()
	unhealthy := r.unhealthy
	r.mu.Unlock()
	if total == 0 {
		r.logger.Warn("no audio captured", "operation", "Stop")
		if unhealthy {
			if refreshErr := r.RefreshDevices(); refreshErr != nil {
				return nil, &apppkg.ErrDependencyUnavailable{
					Component: "recorder",
					Operation: "Stop",
					Wrapped:   fmt.Errorf("recover unhealthy backend: %w", refreshErr),
				}
			}
			return nil, &apppkg.ErrDependencyUnavailable{
				Component: "recorder",
				Operation: "Stop",
				Wrapped:   fmt.Errorf("recording backend became unhealthy"),
			}
		}
		return []float32{}, nil
	}

	captureRate := r.sampleRate
	r.mu.Lock()
	if r.captureRate > 0 {
		captureRate = r.captureRate
	}
	r.mu.Unlock()

	if captureRate != r.sampleRate && captureRate > 0 {
		audio = resampleLinear(audio, captureRate, r.sampleRate)
		r.logger.Info("resampled audio", "operation", "Stop",
			"from_rate", captureRate, "to_rate", r.sampleRate,
			"input_samples", total, "output_samples", len(audio))
	}

	r.logger.Debug("recording stopped", "operation", "Stop",
		"samples", len(audio),
		"duration_sec", float64(len(audio))/r.sampleRate)

	// Re-warm the stream in background so next recording starts instantly
	go r.Warm()

	return audio, nil
}

func (r *portaudioRecorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logger.Info("closing", "operation", "Close")
	if r.warmStream != nil {
		if err := r.warmStream.Close(); err != nil {
			r.logger.Warn("failed to close warm stream",
				"operation", "Close", "error", err)
		}
		r.warmStream = nil
	}
	// Active stream cleanup is handled by readLoop's defer.
	// Each readLoop owns its stream by value and closes it on exit.
	return nil
}

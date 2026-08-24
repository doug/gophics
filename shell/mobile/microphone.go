package mobile

import (
	"encoding/binary"
	"errors"
	"math"
	"sync"
	"time"

	"github.com/doug/gophics/internal/dsp"
	"github.com/doug/gophics/shell"
)

// The Bridge opts into audio input by implementing shell.MicrophoneWindow;
// this is the compile-time check that it still does.
var _ shell.MicrophoneWindow = (*Bridge)(nil)

// MonitorHost is implemented by the native host (Android/iOS) and registered
// via Bridge.SetMonitorHost. It is deliberately separate from MediaHost, the
// same way shell.Microphone is separate from shell.Audio: recording a clip is a
// one-shot transaction that ends with a result, while monitoring is an open
// stream with no end state. A host can provide either, both, or neither.
//
// Go calls these to start and stop capture; the host reports back through the
// Bridge.DeliverMonitor… and FailMonitoring methods, correlating by reqID.
//
// Threading is the one place this differs from MediaHost, and deliberately:
//
//   - AuthorizeMic, DeliverMonitorReady and FailMonitoring MUST be delivered on
//     the host's UI thread, because they run app callbacks that mutate the
//     widget tree.
//   - DeliverMonitorPCM MUST NOT be. It is called from the audio callback
//     hundreds of times a second, touches no app code and no widget tree — only
//     a mutex-guarded ring buffer — and marshaling every block through the UI
//     thread would add a frame of latency to a path whose whole purpose is to
//     be current.
type MonitorHost interface {
	// AuthorizeMic requests microphone permission. → DeliverPermission(reqID, granted)
	AuthorizeMic(reqID int)
	// StartMonitoring opens the microphone and begins streaming.
	// → DeliverMonitorReady(reqID, sampleRate) | FailMonitoring(reqID, msg),
	// then DeliverMonitorPCM(reqID, pcm) repeatedly until stopped.
	StartMonitoring(reqID int)
	// StopMonitoring closes the microphone and ends the stream.
	StopMonitoring(reqID int)
}

// SetMonitorHost registers the native live-capture backend. Until it is set,
// Microphone() returns nil and the app degrades to no input.
func (b *Bridge) SetMonitorHost(h MonitorHost) {
	b.monMu.Lock()
	defer b.monMu.Unlock()
	b.monHost = h
}

// Microphone returns audio input, or nil until a host that can provide it is
// set.
//
// The two halves arrive over different hosts — Listen over MonitorHost, Record
// over MediaHost — and a host may register one without the other. One device
// is one capability, so this is non-nil when either is present and the half
// with no host reports an error when called: an app that can record should not
// lose recording because nothing implements monitoring.
func (b *Bridge) Microphone() shell.Microphone {
	b.monMu.Lock()
	mon := b.monHost
	b.monMu.Unlock()
	if mon == nil && b.media.host == nil {
		return nil
	}
	return &mobileMicrophone{b: b, mobileRecording: mobileRecording{m: b.media}}
}

type mobileMicrophone struct {
	b *Bridge
	mobileRecording
}

func (m *mobileMicrophone) Authorize(cb func(shell.Permission)) {
	b := m.b
	b.monMu.Lock()
	host := b.monHost
	b.monMu.Unlock()
	id := b.media.newReq()
	b.media.perm[id] = cb
	switch {
	case host != nil:
		host.AuthorizeMic(id)
	case b.media.host != nil:
		// Same permission, either route: whichever host is registered can ask.
		b.media.host.AuthorizeMic(id)
	default:
		delete(b.media.perm, id)
		cb(shell.PermissionDenied)
	}
}

func (m *mobileMicrophone) Listen(done func(shell.Monitor, error)) {
	b := m.b
	b.monMu.Lock()
	host := b.monHost
	if host == nil {
		b.monMu.Unlock()
		done(nil, errors.New("no microphone on this device"))
		return
	}
	id := b.media.newReq()
	b.monCb[id] = done
	b.monMu.Unlock()

	host.StartMonitoring(id)
}

// mobileMonitor is a live monitor backed by the shared analyzer. Everything
// except opening and closing the device is platform-independent.
type mobileMonitor struct {
	b       *Bridge
	id      int
	an      *dsp.Analyzer
	stopped bool
	mu      sync.Mutex
}

func (m *mobileMonitor) Level() float32 {
	if m.done() {
		return 0
	}
	return m.an.Level()
}

func (m *mobileMonitor) Bands(dst []float32) int {
	if m.done() {
		return 0
	}
	return m.an.Bands(dst)
}

func (m *mobileMonitor) Samples(dst []float32) int {
	if m.done() {
		return 0
	}
	return m.an.Samples(dst)
}

func (m *mobileMonitor) WindowSize() int { return m.an.WindowSize() }
func (m *mobileMonitor) SampleRate() int { return m.an.SampleRate() }

func (m *mobileMonitor) done() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopped
}

func (m *mobileMonitor) Stop() {
	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		return
	}
	m.stopped = true
	m.mu.Unlock()

	b := m.b
	b.monMu.Lock()
	host := b.monHost
	delete(b.monitors, m.id)
	b.monMu.Unlock()

	if host != nil {
		host.StopMonitoring(m.id)
	}
}

// DeliverMonitorReady signals that capture is live at the given rate; the
// Listen callback fires with the Monitor. Call on the host's UI thread.
func (b *Bridge) DeliverMonitorReady(reqID int, sampleRate int) {
	b.monMu.Lock()
	cb := b.monCb[reqID]
	delete(b.monCb, reqID)
	if cb == nil {
		b.monMu.Unlock()
		return
	}
	if sampleRate <= 0 {
		sampleRate = 44100
	}
	m := &mobileMonitor{b: b, id: reqID, an: dsp.New(sampleRate, dsp.DefaultWindow)}
	b.monitors[reqID] = m
	b.monMu.Unlock()

	cb(m, nil)
}

// FailMonitoring reports that monitoring could not start — permission denied,
// the device is in use, or there is no input at all. Call on the UI thread.
func (b *Bridge) FailMonitoring(reqID int, msg string) {
	b.monMu.Lock()
	cb := b.monCb[reqID]
	delete(b.monCb, reqID)
	b.monMu.Unlock()
	if cb != nil {
		cb(nil, errors.New(msg))
	}
}

// DeliverMonitorPCM feeds a block of captured audio: signed 16-bit
// little-endian mono PCM, the format Android's AudioRecord and iOS's
// AVAudioEngine are configured to produce.
//
// Unlike the other Deliver methods this is safe to call from the audio thread,
// and should be — see MonitorHost.
func (b *Bridge) DeliverMonitorPCM(reqID int, pcm []byte) {
	b.monMu.Lock()
	m := b.monitors[reqID]
	b.monMu.Unlock()
	if m == nil || m.done() {
		return
	}
	m.an.WriteInt16(pcmToInt16(pcm))
}

// DeliverMonitorFloat32 is DeliverMonitorPCM for hosts that already hold float
// samples in [-1,1] — iOS's AVAudioEngine delivers these natively, and routing
// them through int16 would quantize for no reason.
//
// The data is raw little-endian IEEE-754 float32, passed as bytes rather than
// as a []float32 because gomobile binds only byte slices; a []float32 parameter
// would make the whole package unbindable rather than merely awkward.
func (b *Bridge) DeliverMonitorFloat32(reqID int, data []byte) {
	b.monMu.Lock()
	m := b.monitors[reqID]
	b.monMu.Unlock()
	if m == nil || m.done() {
		return
	}
	n := len(data) / 4
	if n == 0 {
		return
	}
	out := make([]float32, n)
	for i := 0; i < n; i++ {
		bits := binary.LittleEndian.Uint32(data[i*4:])
		out[i] = math.Float32frombits(bits)
	}
	m.an.Write(out)
}

// --- Recording ---------------------------------------------------------------
//
// The clip half of the microphone. It runs over MediaHost rather than
// MonitorHost — see the note on Bridge.Microphone for why that matters.

// mobileRecording is the microphone's recording half. It is a distinct type
// only because the two halves arrive over different hosts; mobileMicrophone
// embeds it so an app sees one device.
type mobileRecording struct{ m *mediaBridge }

func (a *mobileRecording) Record(_ shell.RecordOptions, done func(shell.Recorder, error)) {
	if a.m == nil || a.m.host == nil {
		done(nil, errors.New("recording is not available on this device"))
		return
	}
	id := a.m.newReq()
	a.m.recCb[id] = done
	a.m.recs[id] = &mobileRecorder{m: a.m, id: id, start: time.Now()}
	a.m.host.StartRecording(id)
}

type mobileRecorder struct {
	m      *mediaBridge
	id     int
	start  time.Time
	level  float32
	stopCb func(shell.Clip, error)
}

func (r *mobileRecorder) Level() float32 { return r.level }

func (r *mobileRecorder) Elapsed() time.Duration { return time.Since(r.start) }

func (r *mobileRecorder) Stop(done func(shell.Clip, error)) {
	r.stopCb = done
	r.m.host.StopRecording(r.id)
}

func (r *mobileRecorder) Cancel() {
	delete(r.m.recs, r.id)
	r.m.host.StopRecording(r.id)
}

// DeliverRecorderReady signals the mic is live; the Record callback fires.
func (b *Bridge) DeliverRecorderReady(reqID int) {
	cb := b.media.recCb[reqID]
	r := b.media.recs[reqID]
	if cb == nil || r == nil {
		return
	}
	delete(b.media.recCb, reqID)
	r.start = time.Now()
	cb(r, nil)
}

// FailRecording reports that recording could not start (e.g. permission denied).
func (b *Bridge) FailRecording(reqID int, msg string) {
	cb := b.media.recCb[reqID]
	delete(b.media.recCb, reqID)
	delete(b.media.recs, reqID)
	if cb != nil {
		cb(nil, errors.New(msg))
	}
}

// SetAudioLevel updates a live recording's input level (0..1) for the meter.
func (b *Bridge) SetAudioLevel(reqID int, level float32) {
	if r := b.media.recs[reqID]; r != nil {
		r.level = level
		b.dirty.Store(true)
	}
}

// DeliverPCM finalizes a recording: raw 16-bit LE mono PCM + sample rate. Go
// encodes the WAV Clip and computes its waveform envelope.
func (b *Bridge) DeliverPCM(reqID int, pcm []byte, sampleRate int, durationMs int) {
	r := b.media.recs[reqID]
	if r == nil {
		return
	}
	delete(b.media.recs, reqID)
	samples := pcmToInt16(pcm)
	dur := time.Duration(durationMs) * time.Millisecond
	if dur == 0 && sampleRate > 0 {
		dur = time.Duration(len(samples)) * time.Second / time.Duration(sampleRate)
	}
	if r.stopCb != nil {
		r.stopCb(shell.Clip{
			Data:     shell.EncodeWAV(samples, sampleRate),
			Mime:     "audio/wav",
			Duration: dur,
			Envelope: shell.Envelope(samples, 120),
		}, nil)
	}
}

func pcmToInt16(b []byte) []int16 {
	n := len(b) / 2
	out := make([]int16, n)
	for i := 0; i < n; i++ {
		out[i] = int16(binary.LittleEndian.Uint16(b[i*2:]))
	}
	return out
}

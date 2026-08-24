package mobile

import (
	"encoding/binary"
	"math"
	"sync"
	"testing"

	"github.com/doug/gophics/shell"
)

// fakeMonitorHost stands in for the Kotlin/Swift side.
type fakeMonitorHost struct {
	mu       sync.Mutex
	started  []int
	stopped  []int
	authReqs []int
}

func (h *fakeMonitorHost) AuthorizeMic(id int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.authReqs = append(h.authReqs, id)
}

func (h *fakeMonitorHost) StartMonitoring(id int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.started = append(h.started, id)
}

func (h *fakeMonitorHost) StopMonitoring(id int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.stopped = append(h.stopped, id)
}

func (h *fakeMonitorHost) lastStarted() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.started) == 0 {
		return -1
	}
	return h.started[len(h.started)-1]
}

// pcm16 encodes float samples as the signed 16-bit LE bytes a native host sends.
func pcm16(samples []float32) []byte {
	out := make([]byte, len(samples)*2)
	for i, v := range samples {
		binary.LittleEndian.PutUint16(out[i*2:], uint16(int16(v*32767)))
	}
	return out
}

func newTestBridge() *Bridge { return NewBridge(nil) }

func TestMicrophoneNilWithoutHost(t *testing.T) {
	b := newTestBridge()
	if b.Microphone() != nil {
		t.Error("Microphone() is non-nil with no MonitorHost — the app would " +
			"show an affordance that cannot work")
	}
}

func TestMicrophoneAvailableWithHost(t *testing.T) {
	b := newTestBridge()
	b.SetMonitorHost(&fakeMonitorHost{})
	if b.Microphone() == nil {
		t.Fatal("Microphone() is nil after SetMonitorHost")
	}
	// CameraPreview is deliberately unimplemented; it must say so rather than
	// panicking, since LiveMediaWindow pairs the two.
	if b.CameraPreview() != nil {
		t.Error("CameraPreview() should report unavailable")
	}
}

func TestListenDeliversAMonitor(t *testing.T) {
	b := newTestBridge()
	h := &fakeMonitorHost{}
	b.SetMonitorHost(h)

	var got shell.Monitor
	var gotErr error
	b.Microphone().Listen(func(m shell.Monitor, err error) { got, gotErr = m, err })

	id := h.lastStarted()
	if id < 0 {
		t.Fatal("Listen did not ask the host to start monitoring")
	}
	if got != nil || gotErr != nil {
		t.Fatal("the callback fired before the host reported ready")
	}

	b.DeliverMonitorReady(id, 48000)
	if gotErr != nil {
		t.Fatalf("Listen failed: %v", gotErr)
	}
	if got == nil {
		t.Fatal("no monitor delivered")
	}
	if got.SampleRate() != 48000 {
		t.Errorf("SampleRate = %d, want 48000", got.SampleRate())
	}
	if got.WindowSize() <= 0 {
		t.Errorf("WindowSize = %d", got.WindowSize())
	}
}

func TestFailMonitoringReportsTheError(t *testing.T) {
	b := newTestBridge()
	h := &fakeMonitorHost{}
	b.SetMonitorHost(h)

	var gotErr error
	b.Microphone().Listen(func(_ shell.Monitor, err error) { gotErr = err })
	b.FailMonitoring(h.lastStarted(), "permission denied")

	if gotErr == nil || gotErr.Error() != "permission denied" {
		t.Errorf("got %v, want a permission-denied error", gotErr)
	}
}

// TestPCMReachesSamples is the end-to-end path the whole capability exists for:
// bytes from the native audio callback come back out as analysable samples.
func TestPCMReachesSamples(t *testing.T) {
	b := newTestBridge()
	h := &fakeMonitorHost{}
	b.SetMonitorHost(h)

	var mon shell.Monitor
	b.Microphone().Listen(func(m shell.Monitor, _ error) { mon = m })
	b.DeliverMonitorReady(h.lastStarted(), 48000)
	if mon == nil {
		t.Fatal("no monitor")
	}

	// A 440 Hz tone, delivered in realistic 512-sample blocks.
	const rate = 48000
	total := mon.WindowSize()
	phase := 0.0
	for sent := 0; sent < total*2; sent += 512 {
		block := make([]float32, 512)
		for i := range block {
			block[i] = float32(0.8 * math.Sin(2*math.Pi*phase))
			phase += 440.0 / rate
		}
		b.DeliverMonitorPCM(h.lastStarted(), pcm16(block))
	}

	out := make([]float32, total)
	n := mon.Samples(out)
	if n != total {
		t.Fatalf("Samples returned %d, want %d", n, total)
	}
	if lvl := mon.Level(); lvl < 0.5 {
		t.Errorf("Level = %.3f for a loud tone", lvl)
	}

	// The samples must actually be the tone: count zero crossings and check
	// the implied frequency. This is what catches byte-order and scaling bugs
	// that a level check would sail past.
	crossings := 0
	for i := 1; i < n; i++ {
		if (out[i-1] < 0) != (out[i] < 0) {
			crossings++
		}
	}
	freq := float64(crossings) / 2 * rate / float64(n)
	if math.Abs(freq-440) > 25 {
		t.Errorf("recovered %.0f Hz from the samples, want 440", freq)
	}

	bands := make([]float32, 32)
	if got := mon.Bands(bands); got != 32 {
		t.Errorf("Bands wrote %d, want 32", got)
	}
}

func TestFloatDeliveryPath(t *testing.T) {
	b := newTestBridge()
	h := &fakeMonitorHost{}
	b.SetMonitorHost(h)

	var mon shell.Monitor
	b.Microphone().Listen(func(m shell.Monitor, _ error) { mon = m })
	b.DeliverMonitorReady(h.lastStarted(), 44100)

	block := make([]byte, 1024*4)
	for i := 0; i < 1024; i++ {
		binary.LittleEndian.PutUint32(block[i*4:], math.Float32bits(0.5))
	}
	b.DeliverMonitorFloat32(h.lastStarted(), block)

	if lvl := mon.Level(); math.Abs(float64(lvl-0.5)) > 0.01 {
		t.Errorf("Level = %.3f, want 0.5", lvl)
	}
}

func TestStopReleasesTheDevice(t *testing.T) {
	b := newTestBridge()
	h := &fakeMonitorHost{}
	b.SetMonitorHost(h)

	var mon shell.Monitor
	b.Microphone().Listen(func(m shell.Monitor, _ error) { mon = m })
	id := h.lastStarted()
	b.DeliverMonitorReady(id, 48000)

	mon.Stop()

	h.mu.Lock()
	stopped := append([]int(nil), h.stopped...)
	h.mu.Unlock()
	if len(stopped) != 1 || stopped[0] != id {
		t.Errorf("stopped = %v, want [%d] — the mic would stay open", stopped, id)
	}

	// A stopped monitor must go quiet rather than keep reporting the last
	// buffer, and late PCM from a callback still in flight must not revive it.
	b.DeliverMonitorPCM(id, pcm16([]float32{0.9, 0.9, 0.9, 0.9}))
	if lvl := mon.Level(); lvl != 0 {
		t.Errorf("a stopped monitor reports level %.3f", lvl)
	}
	if n := mon.Samples(make([]float32, 64)); n != 0 {
		t.Errorf("a stopped monitor returned %d samples", n)
	}

	mon.Stop() // idempotent
	h.mu.Lock()
	again := len(h.stopped)
	h.mu.Unlock()
	if again != 1 {
		t.Errorf("Stop called the host %d times, want 1", again)
	}
}

func TestAuthorizeRoutesThroughTheHost(t *testing.T) {
	b := newTestBridge()
	h := &fakeMonitorHost{}
	b.SetMonitorHost(h)

	var perm shell.Permission
	b.Microphone().Authorize(func(p shell.Permission) { perm = p })

	h.mu.Lock()
	reqs := append([]int(nil), h.authReqs...)
	h.mu.Unlock()
	if len(reqs) != 1 {
		t.Fatalf("host saw %d authorize requests, want 1", len(reqs))
	}
	b.DeliverPermission(reqs[0], true)
	if perm != shell.PermissionGranted {
		t.Errorf("permission = %v, want granted", perm)
	}
}

// TestConcurrentPCMAndPolling mirrors the real threading: the audio thread
// delivers while the UI polls. Run with -race.
func TestConcurrentPCMAndPolling(t *testing.T) {
	b := newTestBridge()
	h := &fakeMonitorHost{}
	b.SetMonitorHost(h)

	var mon shell.Monitor
	b.Microphone().Listen(func(m shell.Monitor, _ error) { mon = m })
	id := h.lastStarted()
	b.DeliverMonitorReady(id, 48000)

	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(1)
	go func() { // the audio thread
		defer wg.Done()
		block := make([]float32, 256)
		for {
			select {
			case <-stop:
				return
			default:
			}
			b.DeliverMonitorPCM(id, pcm16(block))
		}
	}()

	out := make([]float32, mon.WindowSize())
	bands := make([]float32, 24)
	for i := 0; i < 300; i++ { // the UI goroutine
		mon.Samples(out)
		mon.Level()
		mon.Bands(bands)
	}
	close(stop)
	wg.Wait()
	mon.Stop()
}

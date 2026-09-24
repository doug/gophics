//go:build !js

package devmedia

import (
	"testing"

	"github.com/doug/gophics/internal/audio"
	"github.com/doug/gophics/shell"
)

// noRateCapture opens but reports no sample rate, which the device is
// entitled to do and which both entry points treat as a failure.
type noRateCapture struct{ countingCapture }

func (*noRateCapture) Open(int) (int, error) { return 0, nil }

// Every path after a successful Open has to close the device, because the
// open is what turns on macOS's recording indicator. Record closed on the
// no-rate path; Listen reported the error and left the microphone open.
func TestListenAndRecordCloseTheDeviceWhenItReportsNoRate(t *testing.T) {
	for _, tc := range []struct {
		name string
		call func(shell.Microphone) error
	}{
		{"Listen", func(m shell.Microphone) error {
			var got error
			m.Listen(func(_ shell.Monitor, err error) { got = err })
			return got
		}},
		{"Record", func(m shell.Microphone) error {
			var got error
			m.Record(shell.RecordOptions{}, func(_ shell.Recorder, err error) { got = err })
			return got
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cap := &noRateCapture{}
			old := defaultCapture
			defaultCapture = func() audio.Capture { return cap }
			t.Cleanup(func() { defaultCapture = old })

			if err := tc.call(deviceMic{}); err == nil {
				t.Fatal("a device with no sample rate was reported as opened")
			}
			if cap.closes != 1 {
				t.Errorf("the device was closed %d times after the failure, want 1 — "+
					"the recording indicator stays on", cap.closes)
			}
		})
	}
}

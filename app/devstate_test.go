package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/widget"
	"golang.org/x/image/font/gofont/goregular"
)

// devCounter is a stateful widget with an exported, serializable field. Init
// publishes the live state pointer so the test can drive and assert it without
// reaching into widget internals.
type devCounter struct {
	Ref **devCounterState
}

func (devCounter) CreateState() widget.State { return &devCounterState{} }

type devCounterState struct {
	widget.StateBase[devCounter]
	N int // exported → captured by the plain-json path
}

func (s *devCounterState) Init(widget.Ctx) {
	if r := s.W().Ref; r != nil {
		*r = s
	}
}
func (s *devCounterState) Build(widget.Ctx) widget.Widget {
	return widget.Sized{W: float32(s.N), H: 10}
}

// TestDevStateFileRoundTrip exercises the exact path a hot-restart takes: an
// app snapshots its state to a file, a freshly booted app reads that file and
// restores — through the real Core (which wraps the root in OverlayHost).
func TestDevStateFileRoundTrip(t *testing.T) {
	cfg := Config{Size: geom.Size{W: 100, H: 100}, Font: goregular.TTF}

	// First app: drive the counter to 42, then snapshot to a file.
	var ref1 *devCounterState
	h1, err := NewHeadless(devCounter{Ref: &ref1}, cfg, 1)
	if err != nil {
		t.Fatal(err)
	}
	if ref1 == nil {
		t.Fatal("state not mounted")
	}
	ref1.SetState(func() { ref1.N = 42 })
	h1.Render()

	file := filepath.Join(t.TempDir(), "dev-state.json")
	if err := writeDevSnapshot(file, h1.Core.Owner.SnapshotState()); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}

	// Second app (the relaunched process): fresh tree at N=0, then restore.
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	var snap widget.StateSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}

	var ref2 *devCounterState
	h2, err := NewHeadless(devCounter{Ref: &ref2}, cfg, 1)
	if err != nil {
		t.Fatal(err)
	}
	if ref2.N != 0 {
		t.Fatalf("fresh app N = %d, want 0", ref2.N)
	}

	h2.Core.Owner.RestoreState(snap)

	if ref2.N != 42 {
		t.Errorf("restored N = %d, want 42", ref2.N)
	}
}

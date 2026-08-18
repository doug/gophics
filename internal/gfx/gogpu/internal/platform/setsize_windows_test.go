//go:build windows

package platform

import (
	"os"
	"testing"
	"unsafe"
)

// TestSetSizeOnRealWindow resizes an actual Win32 window and asks Windows what
// happened.
//
// The mock test pins the interface contract; this pins the arithmetic, which is
// where the real risk is. SetSize takes a *content* size, so the frame has to be
// added back before SetWindowPos — and at anything but 100% scaling the border
// and caption thickness are DPI-dependent, so getting that wrong leaves the
// client area short by a few pixels rather than failing outright.
//
// Opt-in: it creates a real window, which is rude in a headless run and needs a
// window station that permits it.
func TestSetSizeOnRealWindow(t *testing.T) {
	if os.Getenv("GOPHICS_WINDOW_TESTS") == "" {
		t.Skip("set GOPHICS_WINDOW_TESTS=1 to create a real window")
	}
	p := newPlatformManager()
	if err := p.Init(); err != nil {
		t.Skipf("platform init: %v", err)
	}
	w, err := p.CreateWindow(Config{Width: 400, Height: 300, Title: "gophics setsize"})
	if err != nil {
		t.Skipf("create window: %v", err)
	}

	if !w.SetSize(640, 480) {
		t.Fatal("SetSize reported failure on Windows, which supports it")
	}

	// GetClientRect is the authority: it reports the content area, which is
	// what SetSize promises, with the frame already excluded.
	win := w.(*win32Window)
	var r rect
	procGetClientRect.Call(uintptr(win.hwnd), uintptr(unsafe.Pointer(&r)))
	gotW, gotH := int(r.right-r.left), int(r.bottom-r.top)
	if gotW != 640 || gotH != 480 {
		t.Errorf("client area = %dx%d, want 640x480 — the frame adjustment is wrong", gotW, gotH)
	}

	// A degenerate request must be refused rather than producing a zero window.
	if w.SetSize(0, 100) || w.SetSize(100, -1) {
		t.Error("SetSize accepted a non-positive size")
	}
}

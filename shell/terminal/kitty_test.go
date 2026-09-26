package terminal

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestFullFrameCmdInline(t *testing.T) {
	// 2×1 RGBA → 8 bytes → inline base64 in one chunk.
	pixels := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	cmds := (&encoder{}).fullFrame(1, 2, 1, 0, 0, pixels) // no dir → inline
	if len(cmds) != 1 {
		t.Fatalf("want 1 command, got %d", len(cmds))
	}
	cmd := string(cmds[0])
	if !strings.HasPrefix(cmd, "\x1b_G") || !strings.HasSuffix(cmd, "\x1b\\") {
		t.Fatalf("bad envelope: %q", cmd)
	}
	for _, want := range []string{"a=T", "f=32", "s=2", "v=1", "i=1", "C=1", "m=0"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("missing %q in %q", want, cmd)
		}
	}
}

func TestFullFrameCmdFile(t *testing.T) {
	dir := t.TempDir()
	enc := &encoder{dir: dir}
	cmds := enc.fullFrame(1, 2, 1, 0, 0, []byte{0, 0, 0, 0, 0, 0, 0, 0})
	if len(cmds) != 1 {
		t.Fatalf("file transfer should be one command, got %d", len(cmds))
	}
	if !strings.Contains(string(cmds[0]), "t=t") {
		t.Errorf("file transfer missing t=t: %q", cmds[0])
	}
	// The terminal only deletes a t=t file whose path carries the marker;
	// without it every frame left a file behind.
	files, _ := filepath.Glob(filepath.Join(dir, "*"))
	if len(files) != 1 || !strings.Contains(filepath.Base(files[0]), "tty-graphics-protocol") {
		t.Errorf("temp file %v should carry the tty-graphics-protocol marker", files)
	}
	// And the encoder unlinks it itself once the next frame goes out, for
	// the terminals that ignore t=t and would never delete it.
	enc.reap()
	if files, _ := filepath.Glob(filepath.Join(dir, "*")); len(files) != 0 {
		t.Errorf("temp files left after reap: %v", files)
	}
}

// A temp file that cannot be written must not lose the frame: the pixels go
// inline for that frame, and the sent frame becomes the diff baseline. Before,
// nothing was sent yet the frame was saved, so the terminal kept stale pixels
// until they happened to change again.
func TestTempWriteFailureFallsBackInline(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "gone")
	var out bytes.Buffer
	ts := &termState{out: &out, imageID: 1, enc: encoder{dir: missing}}
	base := solid(40, 30, 10, 10, 10)
	ts.present(base)
	if s := out.String(); !strings.Contains(s, "a=T") || strings.Contains(s, "t=t") {
		t.Fatalf("frame with an unwritable temp dir should go inline, got %q", firstBytes(s))
	}
	term := &kittyTerm{}
	term.apply(t, out.Bytes())
	if !bytes.Equal(term.pixels(), tightPixels(base)) {
		t.Fatal("inline fallback did not reconstruct the frame")
	}
	if ts.lastFrame == nil {
		t.Fatal("a frame that was sent must become the diff baseline")
	}
}

func TestComposeCmd(t *testing.T) {
	cmd := string((&encoder{}).compose(1, 10, 20, 4, 3, make([]byte, 4*3*4))[0])
	for _, want := range []string{"a=f", "r=1", "x=10", "y=20", "s=4", "v=3", "i=1"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("compose cmd missing %q in %q", want, cmd)
		}
	}
}

func TestInlineChunking(t *testing.T) {
	// A payload whose (compressed) base64 exceeds one chunk must split, with m=1
	// on all but the last and control keys only on the first. Use incompressible
	// data so zlib (o=z) doesn't shrink it below one chunk.
	payload := make([]byte, 64*1024)
	for i := range payload {
		payload[i] = byte((i * 2654435761) >> 17)
	}
	cmds := (&encoder{}).cmds("a=T,f=32", "", payload)
	if len(cmds) < 2 {
		t.Fatalf("want >=2 chunks, got %d", len(cmds))
	}
	if !strings.Contains(string(cmds[0]), "a=T") || !strings.Contains(string(cmds[0]), "m=1") {
		t.Errorf("first chunk should carry control + m=1: %q", cmds[0])
	}
	last := string(cmds[len(cmds)-1])
	if strings.Contains(last, "a=T") || !strings.Contains(last, "m=0") {
		t.Errorf("last chunk should be a bare m=0 continuation: %q", last)
	}
}

// Frame-compose chunks repeat a=f: the protocol requires it on every chunk of
// animation-frame data, and a bare m= continuation is read as part of an
// ordinary transmission.
func TestComposeChunksRepeatFrameKey(t *testing.T) {
	const n = 128
	payload := make([]byte, n*n*4) // incompressible, so it chunks
	for i := range payload {
		payload[i] = byte((i * 2654435761) >> 17)
	}
	cmds := (&encoder{}).compose(1, 0, 0, n, n, payload)
	if len(cmds) < 2 {
		t.Fatalf("want >=2 chunks, got %d", len(cmds))
	}
	for i, c := range cmds {
		if !strings.HasPrefix(string(c), "\x1b_Ga=f,") {
			t.Errorf("chunk %d lacks a=f: %q", i, firstBytes(string(c)))
		}
	}
	// The whole transfer must still reconstruct through the decoder, which
	// now rejects a bare continuation mid-frame.
	term := &kittyTerm{w: n, h: n, buf: make([]byte, n*n*4)}
	term.apply(t, bytes.Join(cmds, nil))
	if !bytes.Equal(term.pixels(), payload) {
		t.Fatal("chunked compose did not reconstruct")
	}
}

// The decoder is the stand-in terminal, so it has to be strict where the
// protocol is: a continuation chunk without a=f during a frame transfer.
func TestDecoderRejectsBareFrameContinuation(t *testing.T) {
	term := &kittyTerm{w: 1, h: 1, buf: make([]byte, 4)}
	term.command(t, "a=f,f=32,s=1,v=1,i=1,r=1,m=1;AAAA")
	term.command(t, "m=0;AAAA")
	if len(term.violations) == 0 {
		t.Fatal("bare continuation during a=f transfer went unnoticed")
	}
}

func TestDeleteImageCmd(t *testing.T) {
	cmd := string(deleteImageCmd(1))
	for _, want := range []string{"\x1b_G", "a=d", "d=I", "i=1", "\x1b\\"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("delete cmd missing %q in %q", want, cmd)
		}
	}
}

package terminal

import (
	"strings"
	"testing"
)

func TestFullFrameCmdInline(t *testing.T) {
	// 2×1 RGBA → 8 bytes → inline base64 in one chunk.
	pixels := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	cmds := fullFrameCmds(1, 2, 1, 0, 0, pixels, "") // dir="" → inline
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
	cmds := fullFrameCmds(1, 2, 1, 0, 0, []byte{0, 0, 0, 0, 0, 0, 0, 0}, t.TempDir())
	if len(cmds) != 1 {
		t.Fatalf("file transfer should be one command, got %d", len(cmds))
	}
	if !strings.Contains(string(cmds[0]), "t=t") {
		t.Errorf("file transfer missing t=t: %q", cmds[0])
	}
}

func TestComposeCmd(t *testing.T) {
	cmd := string(composeCmds(1, 10, 20, 4, 3, make([]byte, 4*3*4), "")[0])
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
	cmds := graphicsCmds("a=T,f=32", payload, "")
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

func TestDeleteImageCmd(t *testing.T) {
	cmd := string(deleteImageCmd(1))
	for _, want := range []string{"\x1b_G", "a=d", "d=I", "i=1", "\x1b\\"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("delete cmd missing %q in %q", want, cmd)
		}
	}
}

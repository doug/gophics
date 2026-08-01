package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/doug/gossamer/app"
	"github.com/doug/gossamer/layout"
	"github.com/doug/gossamer/widget"
)

func writeNote(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func labels(h *app.Headless) []string {
	var out []string
	for _, n := range layout.FlattenSemantics(h.Core.Semantics()) {
		if n.Label != "" {
			out = append(out, n.Label)
		}
	}
	return out
}

func hasLabel(h *app.Headless, substr string) bool {
	for _, l := range labels(h) {
		if strings.Contains(l, substr) {
			return true
		}
	}
	return false
}

func mountNotes(t *testing.T, dir string) (*app.Headless, *workspaceState) {
	t.Helper()
	v, err := LoadVault(dir)
	if err != nil {
		t.Fatal(err)
	}
	var st *workspaceState
	stateHook = func(s *workspaceState) { st = s }
	defer func() { stateHook = nil }()
	h, err := app.NewHeadless(Workspace{Vault: v}, Config(), 2)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	if st == nil {
		t.Fatal("workspace state not mounted")
	}
	return h, st
}

func TestNotesOpenRenderAndWikilink(t *testing.T) {
	dir := t.TempDir()
	alpha := writeNote(t, dir, "Alpha.md", "# Alpha Heading\n\nGo to [[Beta]] for more.")
	writeNote(t, dir, "Beta.md", "# Beta Heading\n\nback to [[Alpha]].")

	h, st := mountNotes(t, dir)
	if !hasLabel(h, "Select a note") {
		t.Fatalf("expected empty state; labels=%v", labels(h))
	}

	// Open Alpha — its markdown heading should render.
	st.open(alpha)
	h.Render()
	if !hasLabel(h, "Alpha Heading") {
		t.Fatalf("Alpha heading not rendered; labels=%v", labels(h))
	}

	// Follow the [[Beta]] wikilink → Beta's heading renders.
	st.followNote(st.W().Vault, "Beta")
	h.Render()
	if !hasLabel(h, "Beta Heading") {
		t.Fatalf("wikilink did not open Beta; labels=%v", labels(h))
	}
}

func TestNotesLivePreview(t *testing.T) {
	dir := t.TempDir()
	path := writeNote(t, dir, "Note.md", "# Note\n")

	h, st := mountNotes(t, dir)
	st.open(path)
	note, _ := st.W().Vault.Get(path)
	st.startEdit(note)
	h.Render()

	// Type markdown with a wikilink. The rendered preview strips the brackets
	// ("Go to Beta now"); that text can only come from live-rendering the draft,
	// not the raw editor, so it proves the preview updates as you type.
	st.SetState(func() { st.Draft = "# Live\n\nGo to [[Beta]] now" })
	h.Render()
	if !hasLabel(h, "Go to Beta now") {
		t.Fatalf("preview did not render the draft live; labels=%v", labels(h))
	}
}

func TestNotesEditSaveWritesDisk(t *testing.T) {
	dir := t.TempDir()
	path := writeNote(t, dir, "Note.md", "# Note\n\noriginal body")

	h, st := mountNotes(t, dir)
	st.open(path)
	h.Render()

	note, _ := st.W().Vault.Get(path)
	st.startEdit(note)
	h.Render()
	st.SetState(func() { st.Draft = "# Note\n\nedited body" })
	st.save(st.W().Vault)
	h.Render()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "edited body") {
		t.Fatalf("save did not write edit to disk: %q", got)
	}
	if st.Editing {
		t.Error("still editing after save")
	}
}

func TestNotesSessionRestore(t *testing.T) {
	dir := t.TempDir()
	writeNote(t, dir, "Alpha.md", "# Alpha Heading\n")
	beta := writeNote(t, dir, "Beta.md", "# Beta Heading\n")

	// Drive to Beta, in edit mode with an unsaved draft.
	h, st := mountNotes(t, dir)
	st.open(beta)
	note, _ := st.W().Vault.Get(beta)
	st.startEdit(note)
	st.SetState(func() { st.Draft = "# Beta Heading\n\nunsaved words" })
	h.Render()

	// Snapshot → JSON → fresh app → restore.
	blob, err := json.Marshal(h.Core.Owner.SnapshotState())
	if err != nil {
		t.Fatal(err)
	}
	var snap widget.StateSnapshot
	if err := json.Unmarshal(blob, &snap); err != nil {
		t.Fatal(err)
	}

	h2, st2 := mountNotes(t, dir)
	if st2.OpenPath != "" {
		t.Fatalf("fresh app should have nothing open, got %q", st2.OpenPath)
	}
	h2.Core.Owner.RestoreState(snap)
	h2.Render()

	if st2.OpenPath != beta {
		t.Errorf("restored open note = %q, want %q", st2.OpenPath, beta)
	}
	if !st2.Editing {
		t.Error("restored app not in edit mode")
	}
	if !strings.Contains(st2.Draft, "unsaved words") {
		t.Errorf("restored draft lost unsaved edit: %q", st2.Draft)
	}
}

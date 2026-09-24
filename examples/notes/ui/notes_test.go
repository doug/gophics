package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
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
	for _, n := range layout.FlattenSemantics(h.Semantics()) {
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

func TestNotesSearchFilters(t *testing.T) {
	dir := t.TempDir()
	writeNote(t, dir, "Alpha.md", "# Alpha\n\njust alpha")
	writeNote(t, dir, "Beta.md", "# Beta\n\nmentions unicorn")

	h, st := mountNotes(t, dir)
	if !hasLabel(h, "Alpha") || !hasLabel(h, "Beta") {
		t.Fatalf("both notes should list initially; labels=%v", labels(h))
	}
	// Query matches Beta by body only.
	st.SetState(func() { st.Query = "unicorn" })
	h.Render()
	if hasLabel(h, "Alpha") {
		t.Errorf("Alpha should be filtered out; labels=%v", labels(h))
	}
	if !hasLabel(h, "Beta") {
		t.Errorf("Beta should match body search; labels=%v", labels(h))
	}
}

func TestNotesBacklinks(t *testing.T) {
	dir := t.TempDir()
	alpha := writeNote(t, dir, "Alpha.md", "# Alpha\n\nsee [[Beta]]")
	beta := writeNote(t, dir, "Beta.md", "# Beta\n")

	h, st := mountNotes(t, dir)

	st.open(beta) // Beta is linked from Alpha
	h.Render()
	if !hasLabel(h, "Linked references") {
		t.Fatalf("Beta should show backlinks; labels=%v", labels(h))
	}

	st.open(alpha) // nothing links to Alpha
	h.Render()
	if hasLabel(h, "Linked references") {
		t.Errorf("Alpha should have no backlinks; labels=%v", labels(h))
	}
}

func TestNotesOutline(t *testing.T) {
	dir := t.TempDir()
	path := writeNote(t, dir, "Doc.md", "# Top\n\nbody\n\n## Sub\n\nmore")
	h, st := mountNotes(t, dir)
	st.open(path)
	h.Render()
	if !hasLabel(h, "OUTLINE") {
		t.Fatalf("outline panel not shown; labels=%v", labels(h))
	}
}

func TestExtractHeadingsAndWikilinks(t *testing.T) {
	hs := extractHeadings("# A\n## B\n```\n# not a heading\n```\ntext\n### C")
	want := []headingItem{{1, "A"}, {2, "B"}, {3, "C"}}
	if len(hs) != len(want) {
		t.Fatalf("headings = %v, want %v", hs, want)
	}
	for i := range want {
		if hs[i] != want[i] {
			t.Errorf("heading %d = %v, want %v", i, hs[i], want[i])
		}
	}
	tgts := wikilinkTargets("see [[Beta]] and also [[Ideas Note]] here")
	if len(tgts) != 2 || tgts[0] != "Beta" || tgts[1] != "Ideas Note" {
		t.Errorf("wikilinkTargets = %v, want [Beta, Ideas Note]", tgts)
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
	blob, err := json.Marshal(h.Owner().SnapshotState())
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
	h2.Owner().RestoreState(snap)
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

func hasMonoSpan(blocks []widget.Widget) bool {
	found := false
	var walk func(w widget.Widget)
	walk = func(w widget.Widget) {
		switch x := w.(type) {
		case widget.Rich:
			for _, sp := range x.Spans {
				if sp.Font == "mono" {
					found = true
				}
			}
		case widget.Padding:
			walk(x.Child)
		case widget.Decorated:
			walk(x.Child)
		}
	}
	for _, b := range blocks {
		walk(b)
	}
	return found
}

func TestCodeBlocksRenderMono(t *testing.T) {
	if !hasMonoSpan(renderMarkdown("t\n\n```\nfenced\n```\n", mdTheme(theme.Light()), nil)) {
		t.Error("fenced code block should render mono")
	}
	if !hasMonoSpan(renderMarkdown("t\n\n    indented code\n", mdTheme(theme.Light()), nil)) {
		t.Error("indented code block should render mono")
	}
}

func TestNotesCreateAndDelete(t *testing.T) {
	dir := t.TempDir()
	h, st := mountNotes(t, dir)
	v := st.W().Vault

	st.SetState(func() { st.newName = "My New Note" })
	st.createNote(v)
	h.Render()

	path := filepath.Join(dir, "My New Note.md")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("note file not created: %v", err)
	}
	if st.OpenPath != path || !st.Editing {
		t.Errorf("new note should open in edit mode; OpenPath=%q Editing=%v", st.OpenPath, st.Editing)
	}

	st.confirmDelete = true
	st.deleteNote(v)
	h.Render()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("note file should be deleted from disk")
	}
	if st.OpenPath != "" {
		t.Errorf("pane should clear after delete, OpenPath=%q", st.OpenPath)
	}
}

// readOnly makes dir and every file in it unwritable for the rest of the test,
// so a save or delete fails the way a locked or full disk does. The skips are
// capability guards, not defects: root ignores mode bits, and Windows has no
// read-only directory to chmod into.
func readOnly(t *testing.T, dir string) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("capability: root bypasses directory permissions, the vault cannot be made read-only")
	}
	if runtime.GOOS == "windows" {
		t.Skip("capability: chmod cannot make a Windows directory read-only")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		if err := os.Chmod(p, 0o400); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
}

// editing opens path and puts the workspace in edit mode with draft typed.
func editing(t *testing.T, h *app.Headless, st *workspaceState, path, draft string) {
	t.Helper()
	st.open(path)
	note, ok := st.W().Vault.Get(path)
	if !ok {
		t.Fatalf("%s not in vault", path)
	}
	st.startEdit(note)
	st.SetState(func() { st.Draft = draft })
	h.Render()
}

// A save that fails must not look like one that worked. It used to: the error
// was dropped, edit mode closed, and the next Edit reseeded the draft from the
// body still on disk — the edits were gone without a word.
func TestNotesFailedSaveKeepsDraft(t *testing.T) {
	dir := t.TempDir()
	path := writeNote(t, dir, "Note.md", "# Note\n\noriginal")
	h, st := mountNotes(t, dir)
	editing(t, h, st, path, "# Note\n\nhours of edits")
	readOnly(t, dir)

	st.save(st.W().Vault)
	h.Render()

	if got, _ := os.ReadFile(path); strings.Contains(string(got), "hours of edits") {
		t.Skip("capability: this filesystem ignores mode bits, the write went through")
	}
	if !st.Editing {
		t.Error("edit mode closed after a failed save")
	}
	if !strings.Contains(st.Draft, "hours of edits") {
		t.Errorf("draft lost after a failed save: %q", st.Draft)
	}
	if n, _ := st.W().Vault.Get(path); n.Body != "# Note\n\noriginal" {
		t.Errorf("vault body became %q although the write failed", n.Body)
	}
	if !hasLabel(h, "Could not save") {
		t.Errorf("the failure is not shown; labels=%v", labels(h))
	}
}

// Navigating away mid-edit saves the draft rather than dropping it. The route
// driven here is the one easiest to hit by accident: a [[wikilink]] in the
// editor's own live preview.
func TestNotesNavigationSavesDraft(t *testing.T) {
	dir := t.TempDir()
	a := writeNote(t, dir, "A.md", "# A\n\nsee [[B]]")
	b := writeNote(t, dir, "B.md", "# B Heading\n")
	h, st := mountNotes(t, dir)
	editing(t, h, st, a, "# A\n\nunsaved [[B]]")

	st.followNote(st.W().Vault, "B")
	h.Render()

	if st.OpenPath != b {
		t.Fatalf("link did not open B; OpenPath=%q Editing=%v", st.OpenPath, st.Editing)
	}
	if got, _ := os.ReadFile(a); !strings.Contains(string(got), "unsaved") {
		t.Errorf("A's draft was not saved before navigating; disk=%q", got)
	}
	if !hasLabel(h, "B Heading") {
		t.Errorf("B not rendered; labels=%v", labels(h))
	}
}

// When the draft cannot be saved the navigation is refused: the editor stays
// where it is, draft intact, with the reason shown. Silently moving on would
// be the data loss the save was meant to prevent.
func TestNotesNavigationRefusedWhenSaveFails(t *testing.T) {
	dir := t.TempDir()
	a := writeNote(t, dir, "A.md", "# A\n\nsee [[B]]")
	writeNote(t, dir, "B.md", "# B\n")
	h, st := mountNotes(t, dir)
	editing(t, h, st, a, "# A\n\nunsaved [[B]]")
	readOnly(t, dir)

	st.followNote(st.W().Vault, "B")
	h.Render()

	if got, _ := os.ReadFile(a); strings.Contains(string(got), "unsaved") {
		t.Skip("capability: this filesystem ignores mode bits, the write went through")
	}
	if st.OpenPath != a || !st.Editing {
		t.Errorf("navigated away from a draft that could not be saved; OpenPath=%q Editing=%v", st.OpenPath, st.Editing)
	}
	if !strings.Contains(st.Draft, "unsaved") {
		t.Errorf("draft lost: %q", st.Draft)
	}
	if !hasLabel(h, "Could not save") {
		t.Errorf("the failure is not shown; labels=%v", labels(h))
	}
}

// A delete that fails leaves the note where it is and says so, rather than
// clearing the pane over a file the next launch will list again.
func TestNotesFailedDeleteKeepsNote(t *testing.T) {
	dir := t.TempDir()
	path := writeNote(t, dir, "Note.md", "# Note\n")
	h, st := mountNotes(t, dir)
	st.open(path)
	h.Render()
	readOnly(t, dir)

	st.SetState(func() { st.confirmDelete = true })
	st.deleteNote(st.W().Vault)
	h.Render()

	if _, err := os.Stat(path); err != nil {
		t.Skip("capability: this filesystem ignores mode bits, the delete went through")
	}
	if st.OpenPath != path {
		t.Errorf("pane cleared although the file is still there; OpenPath=%q", st.OpenPath)
	}
	if _, ok := st.W().Vault.Get(path); !ok {
		t.Error("note dropped from the vault although the file is still there")
	}
	if !hasLabel(h, "Could not delete") {
		t.Errorf("the failure is not shown; labels=%v", labels(h))
	}
}

// A store error is rendered with a folder open. That is where a late write
// failure on web arrives, and the no-folder prompt — the only place it used
// to be drawn — is long gone by then.
func TestNotesStoreErrorShownWithFolderOpen(t *testing.T) {
	dir := t.TempDir()
	writeNote(t, dir, "Note.md", "# Note\n")
	h, st := mountNotes(t, dir)
	if !st.W().Vault.HasStore() {
		t.Fatal("folder not open")
	}
	st.SetState(func() { st.storeErr = "Could not save to that folder." })
	h.Render()
	if !hasLabel(h, "Could not save to that folder.") {
		t.Errorf("store error not shown with a folder open; labels=%v", labels(h))
	}
}

// A note found as Foo.MD is saved back to Foo.MD. Rebuilding the name as
// Foo.md wrote a second file on a case-sensitive disk and left the original
// untouched, so the next launch listed two "Foo" notes.
func TestNotesSaveKeepsUpperCaseExtension(t *testing.T) {
	dir := t.TempDir()
	writeNote(t, dir, "Foo.MD", "# Foo\n")
	v, err := LoadVault(dir)
	if err != nil {
		t.Fatal(err)
	}
	n := v.Notes[0]
	if err := v.Save(n.Path, "# Foo\n\nedited"); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "Foo.MD" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("files after save = %v, want [Foo.MD]", names)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "Foo.MD")); !strings.Contains(string(got), "edited") {
		t.Errorf("Foo.MD not updated: %q", got)
	}
}

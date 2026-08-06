// Package ui is a local-first Markdown notes app — the driving example for
// gophics's text-editing and state-preservation stories. A vault is a folder
// of .md files; edit and read them side by side, follow [[wikilinks]], and —
// because all UI state is plain serializable data — a `gophics dev` hot-restart
// drops you back on the same note, in the same mode, with unsaved edits intact.
package ui

import (
	"strings"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// BG is the window background used before a widget context exists (Config.
// Background). Inside the tree every color comes from theme.Of(ctx), so the app
// follows the platform light/dark scheme for free; this matches the light Bg.
var BG = theme.Light().Bg

// mdTheme maps the active Theme onto the markdown renderer's style: neutrals to
// the text/muted/surface tokens, the link accent to Primary, and the body size
// to the type scale. The heading size ramp stays bespoke (a 6-level scale the
// TypeScale doesn't cover).
func mdTheme(th theme.Theme) mdStyle {
	return mdStyle{
		Text:    th.Text,
		Heading: th.Text,
		Code:    th.Text,
		CodeBG:  th.SurfaceHover,
		Link:    th.Primary,
		Meta:    th.Muted,
		Size:    th.Type.Body,
	}
}

// Root loads the vault and returns the workspace. The vault directory comes from
// $NOTES_DIR, else ./examples/notes/vault (when run from the repo root), else
// ./vault.
func Root() widget.Widget {
	// defaultStore is the OS filesystem on desktop (a folder is always open) and
	// nil on web (the user opens a folder via the sidebar; see openFolder).
	v, _ := OpenVault(defaultStore())
	return Workspace{Vault: v}
}

// Workspace is the whole UI: a note list beside a reader/editor pane.
type Workspace struct{ Vault *Vault }

func (Workspace) CreateState() widget.State { return &workspaceState{} }

// stateHook lets tests observe the mounted workspace state.
var stateHook func(*workspaceState)

func (s *workspaceState) Init(widget.Ctx) {
	if stateHook != nil {
		stateHook(s)
	}
}

// workspaceState's exported fields are session state: a hot-restart (or any
// snapshot/restore) puts you back on the same note, in the same mode, with the
// same unsaved draft.
type workspaceState struct {
	widget.StateBase[Workspace]
	OpenPath string // open note's absolute path
	Editing  bool   // edit vs read mode
	Draft    string // unsaved edit buffer
	Query    string // sidebar search filter

	// Transient (unexported → not persisted): note-management UI.
	creating      bool   // the new-note name input is showing
	newName       string // name being typed for a new note
	confirmDelete bool   // delete is armed (second click confirms)
	storeErr      string // last folder-open error (web), shown in the sidebar
}

func (s *workspaceState) Build(ctx widget.Ctx) widget.Widget {
	// Resolve the theme from the platform color scheme and provide it to the
	// tree, so every panel below reads colors with theme.Of(ctx) and the whole
	// app follows light/dark automatically.
	th := theme.Auto(ctx)
	v := s.W().Vault
	children := []widget.Widget{
		widget.Sized{W: 240, Child: s.sidebar(th, v)},
		widget.Sized{W: 1, Child: widget.Decorated{Color: th.Border}},
		widget.Expand(s.pane(ctx, th, v)),
	}
	// Outline is a third column, shown in read mode when the open note has
	// headings. It lives at the top level (not nested in the pane) so it shares
	// the proven outer split layout.
	if note, ok := v.Get(s.OpenPath); ok && !s.Editing && len(extractHeadings(note.Body)) > 0 {
		children = append(children,
			widget.Sized{W: 1, Child: widget.Decorated{Color: th.Border}},
			widget.Sized{W: 200, Child: outlinePanel(th, note)},
		)
	}
	row := widget.Row(children...)
	row.CrossAlign = layout.CrossStretch
	return widget.Provide[theme.Theme]{
		Value: th,
		Child: widget.Fill{Color: th.Bg, Child: row},
	}
}

func (s *workspaceState) sidebar(th theme.Theme, v *Vault) widget.Widget {
	// Web: no folder is open until the user picks one (File System Access needs
	// a user gesture). Desktop always has a folder, so this never shows there.
	if !v.HasStore() {
		return s.folderPrompt(th)
	}
	head := widget.Row(
		widget.Expand(widget.Text{S: "NOTES", Font: "bold", Size: th.Type.Label, Color: th.Muted}),
		widget.Interactive{
			Handler: widget.Handler{OnTap: func() { s.SetState(func() { s.creating = true; s.newName = "" }) }},
			Child:   widget.Text{S: "+ New", Size: th.Type.Label, Color: th.Primary},
		},
	)
	head.CrossAlign = layout.CrossCenter

	items := []widget.Widget{
		widget.Padding{Insets: geom.Insets{Left: 16, Right: 12, Top: 14, Bottom: 6}, Child: head},
	}
	if s.creating {
		items = append(items, widget.Padding{Insets: geom.Insets{Left: 12, Right: 12, Bottom: 6}, Child: theme.Field{
			Value:       s.newName,
			Placeholder: "Name, then Enter…",
			OnChange:    func(t string) { s.SetState(func() { s.newName = t }) },
			OnSubmit:    func(string) { s.createNote(v) },
		}})
	}
	items = append(items, widget.Padding{Insets: geom.Insets{Left: 12, Right: 12, Bottom: 6}, Child: theme.Field{
		Value:       s.Query,
		Placeholder: "Search notes…",
		OnChange:    func(t string) { s.SetState(func() { s.Query = t }) },
	}})

	for _, n := range v.Search(s.Query) {
		n := n
		bg := th.Bg
		if n.Path == s.OpenPath {
			bg = th.Selection
		}
		items = append(items, theme.Tappable{
			OnTap:      func() { s.open(n.Path) },
			Background: bg,
			Pad:        geom.InsetsSymmetric(16, 9),
			Child:      widget.Text{S: n.Name, Size: th.Type.Body, Color: th.Text},
		})
	}
	col := widget.Column(items...)
	col.CrossAlign = layout.CrossStretch
	return widget.Decorated{Color: th.Bg, Child: widget.Scroll{Child: col}}
}

// folderPrompt is the web starting state: invite the user to open a folder,
// which the File System Access API can only do from a click (openFolder).
func (s *workspaceState) folderPrompt(th theme.Theme) widget.Widget {
	items := []widget.Widget{
		widget.Padding{Insets: geom.Insets{Left: 16, Right: 12, Top: 14, Bottom: 10},
			Child: widget.Text{S: "NOTES", Font: "bold", Size: th.Type.Label, Color: th.Muted}},
		widget.Padding{Insets: geom.InsetsSymmetric(12, 4),
			Child: s.button(th, "Open folder…", func() { openFolder(s) })},
		widget.Padding{Insets: geom.InsetsSymmetric(16, 6),
			Child: widget.Text{S: "Pick a folder of .md files to read and edit them locally.",
				Size: th.Type.Caption, Color: th.Muted, Wrap: true}},
	}
	if s.storeErr != "" {
		items = append(items, widget.Padding{Insets: geom.InsetsSymmetric(16, 6),
			Child: widget.Text{S: s.storeErr, Size: th.Type.Caption, Color: th.Danger, Wrap: true}})
	}
	col := widget.Column(items...)
	col.CrossAlign = layout.CrossStart
	return widget.Decorated{Color: th.Bg, Child: col}
}

func (s *workspaceState) pane(ctx widget.Ctx, th theme.Theme, v *Vault) widget.Widget {
	note, ok := v.Get(s.OpenPath)
	if !ok {
		msg := "Select a note"
		if !v.HasStore() {
			msg = "Open a folder to start"
		}
		return widget.Fill{Color: th.Surface, Child: widget.Center(widget.Text{S: msg, Color: th.Muted})}
	}

	var action, body widget.Widget
	if s.Editing {
		action = s.button(th, "Save", func() { s.save(v) })
		body = s.editorSplit(ctx, th, v)
	} else {
		edit := s.button(th, "Edit", func() { s.startEdit(note) })
		row := widget.Row(s.deleteControl(th, v), widget.Sized{W: 8}, edit)
		row.CrossAlign = layout.CrossCenter
		action = row
		body = s.reader(ctx, th, v, note)
	}

	bar := widget.Row(
		widget.Expand(widget.Text{S: note.Name, Font: "bold", Size: th.Type.Heading, Color: th.Text}),
		action,
	)
	bar.CrossAlign = layout.CrossCenter

	content := widget.Column(
		widget.Padding{Insets: geom.InsetsSymmetric(20, 12), Child: bar},
		widget.Sized{H: 1, Child: widget.Decorated{Color: th.Border}},
		widget.Expand(body),
	)
	content.CrossAlign = layout.CrossStretch
	return widget.Decorated{Color: th.Surface, Child: content}
}

// editorSplit is the live-preview editor: a plain-text pane beside the rendered
// markdown, re-rendering as you type (OnChange updates Draft, which rebuilds).
// On a narrow pane it collapses to the editor alone.
func (s *workspaceState) editorSplit(ctx widget.Ctx, th theme.Theme, v *Vault) widget.Widget {
	editor := scrollPad(widget.TextField{
		Value:          s.Draft,
		Multiline:      true,
		Size:           th.Type.Body,
		TextColor:      th.Text,
		CaretColor:     th.Primary,
		SelectionColor: th.Selection,
		OnChange:       func(t string) { s.SetState(func() { s.Draft = t }) },
	})
	return widget.LayoutBuilder{Build: func(cs layout.Constraints) widget.Widget {
		if cs.BoundedW() && cs.Max.W < 560 {
			return editor // too narrow to split
		}
		row := widget.Row(
			widget.Expand(editor),
			widget.Sized{W: 1, Child: widget.Decorated{Color: th.Border}},
			widget.Expand(scrollPad(s.markdown(th, s.Draft, ctx, v))),
		)
		row.CrossAlign = layout.CrossStretch
		return row
	}}
}

// markdown renders src to a left-aligned Column of block widgets.
func (s *workspaceState) markdown(th theme.Theme, src string, ctx widget.Ctx, v *Vault) widget.Widget {
	col := widget.Column(renderMarkdown(src, mdTheme(th), func(url string) { s.onLink(ctx, v, url) })...)
	col.CrossAlign = layout.CrossStart
	return col
}

// reader is the read view: the rendered note followed by its backlinks section.
func (s *workspaceState) reader(ctx widget.Ctx, th theme.Theme, v *Vault, note Note) widget.Widget {
	blocks := renderMarkdown(note.Body, mdTheme(th), func(url string) { s.onLink(ctx, v, url) })
	blocks = append(blocks, s.backlinks(th, v, note)...)
	body := widget.Column(blocks...)
	body.CrossAlign = layout.CrossStart
	return scrollPad(body)
}

// backlinks renders the "Linked references" section — the notes that [[link]]
// to this one — appended beneath the note body. Empty when there are none.
func (s *workspaceState) backlinks(th theme.Theme, v *Vault, note Note) []widget.Widget {
	refs := v.Backlinks(note.Name)
	if len(refs) == 0 {
		return nil
	}
	out := []widget.Widget{
		block(widget.Padding{Insets: geom.Insets{Top: 12, Bottom: 8}, Child: widget.Sized{H: 1, Child: widget.Decorated{Color: th.Border}}}),
		block(widget.Text{S: "Linked references", Font: "bold", Size: th.Type.Label, Color: th.Muted}),
	}
	for _, n := range refs {
		n := n
		out = append(out, block(widget.Interactive{
			Handler: widget.Handler{OnTap: func() { s.open(n.Path) }},
			Child:   widget.Text{S: "← " + n.Name, Size: th.Type.Body, Color: th.Primary},
		}))
	}
	return out
}

// outlinePanel lists the note's headings, indented by level — a live document
// map. (Click-to-scroll awaits a public scroll-to-position API in the framework.)
func outlinePanel(th theme.Theme, note Note) widget.Widget {
	items := []widget.Widget{
		widget.Padding{Insets: geom.Insets{Left: 14, Right: 14, Top: 16, Bottom: 8},
			Child: widget.Text{S: "OUTLINE", Font: "bold", Size: th.Type.Label, Color: th.Muted}},
	}
	hs := extractHeadings(note.Body)
	if len(hs) == 0 {
		items = append(items, widget.Padding{Insets: geom.InsetsSymmetric(14, 4),
			Child: widget.Text{S: "No headings", Size: th.Type.Label, Color: th.Muted}})
	}
	for _, h := range hs {
		items = append(items, widget.Padding{
			Insets: geom.Insets{Left: 14 + float32(h.Level-1)*12, Right: 12, Top: 3, Bottom: 3},
			Child:  widget.Text{S: h.Text, Size: th.Type.Label, Color: th.Text, Wrap: true},
		})
	}
	col := widget.Column(items...)
	col.CrossAlign = layout.CrossStart
	return widget.Decorated{Color: th.Bg, Child: widget.Scroll{Child: col}}
}

// scrollPad wraps content in a scroll view with the standard reading padding.
func scrollPad(child widget.Widget) widget.Widget {
	return widget.Scroll{Child: widget.Padding{Insets: geom.InsetsSymmetric(20, 12), Child: child}}
}

func (s *workspaceState) open(path string) {
	s.SetState(func() { s.OpenPath, s.Editing, s.Draft, s.confirmDelete = path, false, "", false })
}

func (s *workspaceState) startEdit(n Note) {
	s.SetState(func() { s.Editing, s.Draft = true, n.Body })
}

func (s *workspaceState) save(v *Vault) {
	_ = v.Save(s.OpenPath, s.Draft)
	s.SetState(func() { s.Editing = false })
}

func (s *workspaceState) onLink(ctx widget.Ctx, v *Vault, url string) {
	if name, ok := strings.CutPrefix(url, "note:"); ok {
		s.followNote(v, name)
		return
	}
	_ = ctx.OpenURL(url)
}

// followNote opens the note a [[wikilink]] names, if it exists.
func (s *workspaceState) followNote(v *Vault, name string) {
	if n, ok := v.ByName(name); ok {
		s.open(n.Path)
	}
}

// createNote creates the note named in the new-note input and opens it in edit
// mode; a blank name just cancels.
func (s *workspaceState) createNote(v *Vault) {
	n, err := v.Create(s.newName)
	s.SetState(func() {
		s.creating, s.newName = false, ""
		if err == nil {
			s.OpenPath, s.Editing, s.Draft, s.confirmDelete = n.Path, true, n.Body, false
		}
	})
}

// deleteNote deletes the open note from disk and clears the pane.
func (s *workspaceState) deleteNote(v *Vault) {
	_ = v.Delete(s.OpenPath)
	s.SetState(func() { s.OpenPath, s.Editing, s.Draft, s.confirmDelete = "", false, "", false })
}

func (s *workspaceState) button(th theme.Theme, label string, onTap func()) widget.Widget {
	return widget.Interactive{
		Handler: widget.Handler{OnTap: onTap},
		Child: widget.Decorated{Color: th.Primary, Radius: 7, Child: widget.Padding{
			Insets: geom.InsetsSymmetric(16, 8),
			Child:  widget.Text{S: label, Size: th.Type.Label, Color: th.OnPrimary},
		}},
	}
}

// deleteControl is a two-click delete: the first click arms it (turning into a
// red "Delete?" with a Cancel), the second removes the note.
func (s *workspaceState) deleteControl(th theme.Theme, v *Vault) widget.Widget {
	if !s.confirmDelete {
		return widget.Interactive{
			Handler: widget.Handler{OnTap: func() { s.SetState(func() { s.confirmDelete = true }) }},
			Child:   widget.Padding{Insets: geom.InsetsSymmetric(10, 8), Child: widget.Text{S: "Delete", Size: th.Type.Label, Color: th.Muted}},
		}
	}
	cancel := widget.Interactive{
		Handler: widget.Handler{OnTap: func() { s.SetState(func() { s.confirmDelete = false }) }},
		Child:   widget.Padding{Insets: geom.InsetsSymmetric(10, 8), Child: widget.Text{S: "Cancel", Size: th.Type.Label, Color: th.Muted}},
	}
	confirm := widget.Interactive{
		Handler: widget.Handler{OnTap: func() { s.deleteNote(v) }},
		Child: widget.Decorated{Color: th.Danger, Radius: 7, Child: widget.Padding{
			Insets: geom.InsetsSymmetric(14, 8),
			Child:  widget.Text{S: "Delete?", Size: th.Type.Label, Color: th.OnPrimary},
		}},
	}
	r := widget.Row(cancel, widget.Sized{W: 4}, confirm)
	r.CrossAlign = layout.CrossCenter
	return r
}

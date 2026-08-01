// Package ui is a local-first Markdown notes app — the driving example for
// gossamer's text-editing and state-preservation stories. A vault is a folder
// of .md files; edit and read them side by side, follow [[wikilinks]], and —
// because all UI state is plain serializable data — a `gossamer dev` hot-restart
// drops you back on the same note, in the same mode, with unsaved edits intact.
package ui

import (
	"os"
	"strings"

	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/layout"
	"github.com/doug/gossamer/paint"
	"github.com/doug/gossamer/widget"
)

var (
	colBg      = paint.RGB(1, 1, 1)
	colSidebar = paint.RGB(0.965, 0.965, 0.975)
	colSel     = paint.RGB(0.87, 0.91, 0.99)
	colText    = paint.RGB(0.13, 0.13, 0.16)
	colHeading = paint.RGB(0.05, 0.05, 0.09)
	colMeta    = paint.RGB(0.52, 0.52, 0.57)
	colLink    = paint.RGB(0.13, 0.40, 0.86)
	colCode    = paint.RGB(0.16, 0.16, 0.22)
	colCodeBG  = paint.RGB(0.955, 0.955, 0.965)
	colAccent  = paint.RGB(0.20, 0.45, 0.95)
	colOnAcc   = paint.RGB(1, 1, 1)
	colBorder  = paint.RGB(0.89, 0.89, 0.91)
)

func mdTheme() mdStyle {
	return mdStyle{Text: colText, Heading: colHeading, Code: colCode, CodeBG: colCodeBG, Link: colLink, Meta: colMeta, Size: 15}
}

// Root loads the vault and returns the workspace. The vault directory comes from
// $NOTES_DIR, else ./examples/notes/vault (when run from the repo root), else
// ./vault.
func Root() widget.Widget {
	dir := vaultDir()
	v, err := LoadVault(dir)
	if err != nil {
		v = &Vault{Dir: dir}
	}
	return Workspace{Vault: v}
}

func vaultDir() string {
	if d := os.Getenv("NOTES_DIR"); d != "" {
		return d
	}
	if _, err := os.Stat("examples/notes/vault"); err == nil {
		return "examples/notes/vault"
	}
	return "vault"
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
}

func (s *workspaceState) Build(ctx widget.Ctx) widget.Widget {
	v := s.W().Vault
	row := widget.Row(
		widget.Sized{W: 240, Child: s.sidebar(v)},
		widget.Sized{W: 1, Child: widget.Decorated{Color: colBorder}},
		widget.Expand(s.pane(ctx, v)),
	)
	row.CrossAlign = layout.CrossStretch
	return widget.Decorated{Color: colBg, Child: row}
}

func (s *workspaceState) sidebar(v *Vault) widget.Widget {
	items := []widget.Widget{
		widget.Padding{Insets: geom.Insets{Left: 16, Right: 16, Top: 16, Bottom: 8},
			Child: widget.Text{S: "NOTES", Font: "bold", Size: 12, Color: colMeta}},
	}
	for _, n := range v.Notes {
		n := n
		bg := colSidebar
		if n.Path == s.OpenPath {
			bg = colSel
		}
		items = append(items, widget.Interactive{
			Handler: widget.Handler{OnTap: func() { s.open(n.Path) }},
			Child: widget.Decorated{Color: bg, Child: widget.Padding{
				Insets: geom.InsetsSymmetric(16, 9),
				Child:  widget.Text{S: n.Name, Size: 14, Color: colText},
			}},
		})
	}
	col := widget.Column(items...)
	col.CrossAlign = layout.CrossStretch
	return widget.Decorated{Color: colSidebar, Child: widget.Scroll{Child: col}}
}

func (s *workspaceState) pane(ctx widget.Ctx, v *Vault) widget.Widget {
	note, ok := v.Get(s.OpenPath)
	if !ok {
		return widget.Center(widget.Text{S: "Select a note", Color: colMeta})
	}

	var action, body widget.Widget
	if s.Editing {
		action = s.button("Save", func() { s.save(v) })
		body = s.editorSplit(ctx, v)
	} else {
		action = s.button("Edit", func() { s.startEdit(note) })
		body = scrollPad(s.markdown(note.Body, ctx, v))
	}

	bar := widget.Row(
		widget.Expand(widget.Text{S: note.Name, Font: "bold", Size: 16, Color: colHeading}),
		action,
	)
	bar.CrossAlign = layout.CrossCenter

	content := widget.Column(
		widget.Padding{Insets: geom.InsetsSymmetric(20, 12), Child: bar},
		widget.Sized{H: 1, Child: widget.Decorated{Color: colBorder}},
		widget.Expand(body),
	)
	content.CrossAlign = layout.CrossStretch
	return content
}

// editorSplit is the live-preview editor: a plain-text pane beside the rendered
// markdown, re-rendering as you type (OnChange updates Draft, which rebuilds).
// On a narrow pane it collapses to the editor alone.
func (s *workspaceState) editorSplit(ctx widget.Ctx, v *Vault) widget.Widget {
	editor := scrollPad(widget.TextField{
		Value:     s.Draft,
		Multiline: true,
		Size:      15,
		TextColor: colText,
		OnChange:  func(t string) { s.SetState(func() { s.Draft = t }) },
	})
	return widget.LayoutBuilder{Build: func(cs layout.Constraints) widget.Widget {
		if cs.BoundedW() && cs.Max.W < 560 {
			return editor // too narrow to split
		}
		row := widget.Row(
			widget.Expand(editor),
			widget.Sized{W: 1, Child: widget.Decorated{Color: colBorder}},
			widget.Expand(scrollPad(s.markdown(s.Draft, ctx, v))),
		)
		row.CrossAlign = layout.CrossStretch
		return row
	}}
}

// markdown renders src to a left-aligned Column of block widgets.
func (s *workspaceState) markdown(src string, ctx widget.Ctx, v *Vault) widget.Widget {
	col := widget.Column(renderMarkdown(src, mdTheme(), func(url string) { s.onLink(ctx, v, url) })...)
	col.CrossAlign = layout.CrossStart
	return col
}

// scrollPad wraps content in a scroll view with the standard reading padding.
func scrollPad(child widget.Widget) widget.Widget {
	return widget.Scroll{Child: widget.Padding{Insets: geom.InsetsSymmetric(20, 12), Child: child}}
}

func (s *workspaceState) open(path string) {
	s.SetState(func() { s.OpenPath, s.Editing, s.Draft = path, false, "" })
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

func (s *workspaceState) button(label string, onTap func()) widget.Widget {
	return widget.Interactive{
		Handler: widget.Handler{OnTap: onTap},
		Child: widget.Decorated{Color: colAccent, Radius: 7, Child: widget.Padding{
			Insets: geom.InsetsSymmetric(16, 8),
			Child:  widget.Text{S: label, Size: 14, Color: colOnAcc},
		}},
	}
}

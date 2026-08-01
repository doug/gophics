// Command journal is the M1 demo for gossamer's media-capture shell: a tiny
// private journal that captures a photo (shell.Camera) and a voice memo
// (shell.Audio record/playback with a live level + waveform), then lists
// entries. It proves the shell/media.go interfaces end to end in the web build;
// desktop, where those capabilities are nil today, degrades to text-only.
//
// Run on web (secure context / Chrome):  gossamer dev -p web ./examples/journal
package main

import (
	"fmt"
	"image"
	"log"
	"time"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gossamer/app"
	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/layout"
	"github.com/doug/gossamer/paint"
	"github.com/doug/gossamer/shell"
	"github.com/doug/gossamer/widget"
)

var (
	colBg     = paint.RGB(0.07, 0.08, 0.11)
	colCard   = paint.RGB(0.12, 0.14, 0.19)
	colText   = paint.RGB(0.92, 0.93, 0.95)
	colDim    = paint.RGB(0.52, 0.55, 0.62)
	colAccent = paint.RGB(0.36, 0.62, 0.98)
	colRec    = paint.RGB(0.92, 0.35, 0.38)
	colBorder = paint.RGB(0.20, 0.22, 0.28)
)

// Journal is the root widget.
type Journal struct{}

func (Journal) CreateState() widget.State { return &journalState{} }

type entry struct {
	text  string
	photo image.Image
	clip  *shell.Clip
}

type journalState struct {
	widget.StateBase[Journal]
	ctx   widget.Ctx
	draft string

	// Pending capture, not yet saved into an entry.
	photo image.Image
	clip  *shell.Clip

	// Live capture/playback.
	rec      shell.Recorder
	recLevel float32
	play     shell.Playback

	entries []entry
}

func (s *journalState) Init(ctx widget.Ctx) {
	s.ctx = ctx
	ctx.AddTicker(s)
}

func (s *journalState) Dispose() { s.ctx.RemoveTicker(s) }

// Tick keeps the level meter and playback cursor live while recording/playing.
func (s *journalState) Tick(dt float64) bool {
	active := false
	if s.rec != nil {
		s.recLevel = s.rec.Level()
		active = true
	}
	if s.play != nil {
		if s.play.Playing() {
			active = true
		} else {
			s.play = nil
		}
	}
	if active {
		s.SetState(nil)
	}
	return active
}

// --- actions -----------------------------------------------------------------

func (s *journalState) capturePhoto() {
	cam := s.ctx.Camera()
	if cam == nil {
		return
	}
	cam.Capture(shell.CaptureOptions{Facing: shell.FacingBack, MaxDim: 1600},
		func(img image.Image, err error) {
			if err != nil || img == nil {
				return
			}
			s.SetState(func() { s.photo = img })
		})
}

func (s *journalState) toggleRecord() {
	aud := s.ctx.Audio()
	if aud == nil {
		return
	}
	if s.rec != nil {
		r := s.rec
		s.SetState(func() { s.rec = nil })
		r.Stop(func(clip shell.Clip, err error) {
			if err != nil {
				return
			}
			c := clip
			s.SetState(func() { s.clip = &c })
		})
		return
	}
	aud.Record(shell.RecordOptions{}, func(r shell.Recorder, err error) {
		if err != nil {
			return
		}
		s.SetState(func() { s.rec = r; s.recLevel = 0 })
		s.ctx.Invalidate() // wake the ticker for the level meter
	})
}

func (s *journalState) togglePlay(clip *shell.Clip) {
	if s.play != nil {
		s.play.Stop()
		s.SetState(func() { s.play = nil })
		return
	}
	aud := s.ctx.Audio()
	if aud == nil || clip == nil {
		return
	}
	aud.Play(*clip, func(p shell.Playback, err error) {
		if err != nil {
			return
		}
		s.SetState(func() { s.play = p })
		s.ctx.Invalidate()
	})
}

func (s *journalState) save() {
	if s.draft == "" && s.photo == nil && s.clip == nil {
		return
	}
	s.SetState(func() {
		s.entries = append([]entry{{text: s.draft, photo: s.photo, clip: s.clip}}, s.entries...)
		s.draft, s.photo, s.clip = "", nil, nil
	})
}

// --- build -------------------------------------------------------------------

func (s *journalState) Build(ctx widget.Ctx) widget.Widget {
	hasMedia := ctx.Camera() != nil || ctx.Audio() != nil

	col := widget.Column(
		widget.Padding{Insets: geom.InsetsSymmetric(18, 16),
			Child: widget.Text{S: "Journal", Font: "bold", Size: 24, Color: colText}},
		widget.Padding{Insets: geom.Insets{Left: 16, Right: 16, Bottom: 12}, Child: s.composer(ctx, hasMedia)},
		widget.Sized{H: 1, Child: widget.Decorated{Color: colBorder}},
		widget.Expand(widget.Scroll{Child: s.timeline()}),
	)
	col.CrossAlign = layout.CrossStretch
	return widget.Decorated{Color: colBg, Child: col}
}

func (s *journalState) composer(ctx widget.Ctx, hasMedia bool) widget.Widget {
	field := widget.Decorated{
		Color: colCard, Radius: 10, BorderColor: colBorder, BorderWidth: 1,
		Child: widget.Padding{All: 12, Child: widget.Sized{H: 72, Child: widget.TextField{
			Value:            s.draft,
			Placeholder:      "What happened?",
			Multiline:        true,
			Size:             15,
			TextColor:        colText,
			PlaceholderColor: colDim,
			CaretColor:       colAccent,
			OnChange:         func(t string) { s.SetState(func() { s.draft = t }) },
		}}},
	}

	var controls []widget.Widget
	if hasMedia {
		if ctx.Camera() != nil {
			controls = append(controls, chip("📷 Photo", colCard, colText, s.capturePhoto))
		}
		if ctx.Audio() != nil {
			label, bg := "🎙 Record", colCard
			if s.rec != nil {
				label, bg = "⏹ Stop", colRec
			}
			controls = append(controls, chip(label, bg, colText, s.toggleRecord))
		}
	} else {
		controls = append(controls, widget.Padding{Insets: geom.InsetsSymmetric(4, 8),
			Child: widget.Text{S: "Photo/audio capture runs on the web or mobile build.", Size: 12, Color: colDim}})
	}
	controls = append(controls, widget.Expand(widget.Sized{}), chip("Save", colAccent, paint.RGB(1, 1, 1), s.save))
	bar := widget.Row(controls...)
	bar.CrossAlign = layout.CrossCenter

	items := []widget.Widget{field, widget.Sized{H: 10}, bar}

	// Live recording meter, or a preview of the pending photo/clip.
	if s.rec != nil {
		items = append(items, widget.Sized{H: 10}, s.levelMeter())
	}
	if s.photo != nil || s.clip != nil {
		items = append(items, widget.Sized{H: 10}, s.preview())
	}

	c := widget.Column(items...)
	c.CrossAlign = layout.CrossStretch
	return c
}

func (s *journalState) preview() widget.Widget {
	var row []widget.Widget
	if s.photo != nil {
		row = append(row, thumb(s.photo), widget.Sized{W: 12})
	}
	if s.clip != nil {
		row = append(row, widget.Expand(s.player(s.clip)))
	}
	r := widget.Row(row...)
	r.CrossAlign = layout.CrossCenter
	return r
}

// player draws the clip waveform with a play/stop button and a progress cursor.
func (s *journalState) player(clip *shell.Clip) widget.Widget {
	label := "▶"
	if s.play != nil {
		label = "⏸"
	}
	var progress float32
	if s.play != nil && clip.Duration > 0 {
		progress = float32(s.play.Position()) / float32(clip.Duration)
	}
	wave := widget.Canvas{H: 40, Draw: func(cv paint.Canvas, size geom.Size) {
		drawWave(cv, size, clip.Envelope, progress)
	}}
	dur := widget.Text{S: fmtDur(clip.Duration), Size: 12, Color: colDim}
	row := widget.Row(
		chip(label, colCard, colText, func() { s.togglePlay(clip) }),
		widget.Sized{W: 10},
		widget.Expand(wave),
		widget.Sized{W: 8},
		dur,
	)
	row.CrossAlign = layout.CrossCenter
	return widget.Decorated{Color: colCard, Radius: 10,
		Child: widget.Padding{Insets: geom.InsetsSymmetric(10, 8), Child: row}}
}

func (s *journalState) levelMeter() widget.Widget {
	lvl := s.recLevel
	el := s.rec.Elapsed()
	return widget.Decorated{Color: colCard, Radius: 10, Child: widget.Padding{Insets: geom.InsetsSymmetric(12, 10),
		Child: widget.Row(
			widget.Text{S: "● " + fmtDur(el), Size: 13, Color: colRec},
			widget.Sized{W: 12},
			widget.Expand(widget.Canvas{H: 24, Draw: func(cv paint.Canvas, size geom.Size) {
				w := size.W * lvl
				cv.FillRRect(geom.RectXYWH(0, size.H/2-4, size.W, 8), 4, colBorder)
				if w > 0 {
					cv.FillRRect(geom.RectXYWH(0, size.H/2-4, w, 8), 4, colRec)
				}
			}})),
	}}
}

func (s *journalState) timeline() widget.Widget {
	if len(s.entries) == 0 {
		return widget.Padding{All: 24, Child: widget.Text{S: "No entries yet.", Size: 14, Color: colDim}}
	}
	items := make([]widget.Widget, 0, len(s.entries))
	for i := range s.entries {
		e := &s.entries[i]
		var parts []widget.Widget
		if e.text != "" {
			parts = append(parts, widget.Text{S: e.text, Size: 15, Color: colText, Wrap: true})
		}
		if e.photo != nil {
			parts = append(parts, widget.Sized{H: 8}, thumb(e.photo))
		}
		if e.clip != nil {
			parts = append(parts, widget.Sized{H: 8}, s.player(e.clip))
		}
		body := widget.Column(parts...)
		body.CrossAlign = layout.CrossStart
		items = append(items, widget.Padding{Insets: geom.InsetsSymmetric(16, 8),
			Child: widget.Decorated{Color: colCard, Radius: 12,
				Child: widget.Padding{All: 14, Child: body}}})
	}
	c := widget.Column(items...)
	c.CrossAlign = layout.CrossStretch
	return c
}

// --- small widgets -----------------------------------------------------------

func chip(label string, bg, fg paint.Color, onTap func()) widget.Widget {
	return widget.Interactive{
		Handler: widget.Handler{OnTap: onTap},
		Child: widget.Decorated{Color: bg, Radius: 8, Child: widget.Padding{
			Insets: geom.InsetsSymmetric(12, 8),
			Child:  widget.Text{S: label, Size: 14, Color: fg},
		}},
	}
}

func thumb(img image.Image) widget.Widget {
	return widget.Decorated{Radius: 8, Child: widget.Sized{W: 72, H: 72, Child: widget.Image{Src: img, W: 72, H: 72}}}
}

func drawWave(cv paint.Canvas, size geom.Size, env []float32, progress float32) {
	if len(env) == 0 {
		cv.FillRRect(geom.RectXYWH(0, size.H/2-1, size.W, 2), 1, colBorder)
		return
	}
	n := len(env)
	bw := size.W / float32(n)
	for i, v := range env {
		h := v * size.H
		if h < 2 {
			h = 2
		}
		x := float32(i) * bw
		col := colDim
		if progress > 0 && float32(i)/float32(n) <= progress {
			col = colAccent
		}
		cv.FillRRect(geom.RectXYWH(x, size.H/2-h/2, bw*0.7, h), bw*0.35, col)
	}
}

func fmtDur(d time.Duration) string {
	s := int(d.Seconds())
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

func main() {
	err := app.Run(Journal{}, app.Config{
		Title:        "Journal",
		Size:         geom.Size{W: 440, H: 720},
		Background:   colBg,
		Font:         goregular.TTF,
		FontFamilies: map[string][]byte{"bold": gobold.TTF},
	})
	if err != nil {
		log.Fatal(err)
	}
}

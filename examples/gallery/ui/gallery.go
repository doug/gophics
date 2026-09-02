// Package ui is the gophics widget catalog: a sectioned, interactive
// showcase of the framework's higher-level components — controls, typography,
// charts, dialogs, layout primitives, implicit animations, and the
// Navigator/Hero/gesture stack — all built on procedural (network-free)
// content so it runs anywhere and stays testable headless.
//
// Structure: a Navigator whose home screen lists the catalog sections as
// tappable cards; tapping one pushes that section's page. A theme switcher in
// the home top bar cycles Light / Dark / Glass / GlassDark, held in root state
// and re-provided via widget.Provide[theme.Theme] so every section — which
// reads its colors through theme.Of(ctx) — follows the switch live.
//
// The sections live in sections_*.go; this file holds the root, the theme
// switcher, the home list, and the shared page chrome (scaffold + helpers).
package ui

import (
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// --- Theme mode & root -------------------------------------------------------

// themeMode selects which built-in theme the whole catalog runs under; the
// switcher in the home top bar cycles it and root state re-provides the theme.
type themeMode int

const (
	modeLight themeMode = iota
	modeDark
	modeGlass
	modeGlassDark
)

func (m themeMode) theme() theme.Theme {
	switch m {
	case modeDark:
		return theme.Dark()
	case modeGlass:
		return theme.Glass()
	case modeGlassDark:
		return theme.GlassDark()
	default:
		return theme.Light()
	}
}

func (m themeMode) label() string {
	switch m {
	case modeDark:
		return "Dark"
	case modeGlass:
		return "Glass"
	case modeGlassDark:
		return "Glass Dark"
	default:
		return "Light"
	}
}

var allModes = []themeMode{modeLight, modeDark, modeGlass, modeGlassDark}

// themeControl is provided alongside the theme so the switcher (and anything
// else) can read the active mode and request a new one, without threading a
// callback through every page.
type themeControl struct {
	mode themeMode
	set  func(themeMode)
}

// Gallery is the root widget: stateful so it can hold the selected theme mode.
type Gallery struct{}

func (Gallery) CreateState() widget.State { return &galleryState{} }

type galleryState struct {
	widget.StateBase[Gallery]
	mode themeMode
}

// rootHook lets tests observe (and drive) the root state.
var rootHook func(*galleryState)

func (s *galleryState) Init(ctx widget.Ctx) {
	// Seed from the platform scheme so first paint matches the OS; the switcher
	// takes over from there.
	if ctx.DarkMode() {
		s.mode = modeDark
	}
	if rootHook != nil {
		rootHook(s)
	}
	s.publishMenus(ctx)
}

// Menu item IDs. These cross into the platform's menu, which outlives the build
// that described it, so they are constants rather than indices into anything.
const (
	menuThemeLight = iota + 1
	menuThemeDark
	menuThemeGlass
	menuThemeGlassDark
)

// publishMenus installs the native menu bar, where the platform has one.
//
// The theme items are the interesting half: choosing one travels out to the OS
// menu, back through the capability's invoke, onto the UI goroutine, and into
// SetState — the same round trip any real app's menu makes. The roles are the
// other half: About and Quit are placed and performed by the platform, which on
// macOS is the difference between a menu bar that looks right and one that
// looks almost right.
func (s *galleryState) publishMenus(ctx widget.Ctx) {
	menus := ctx.Menus()
	if menus == nil {
		return // web, mobile, terminal: nothing to publish into
	}
	s.publishBar(menus)
}

// publishBar describes the bar and installs it. Split from publishMenus so the
// description can be tested without a platform menu to publish into.
func (s *galleryState) publishBar(menus shell.Menus) {
	menus.SetBar([]shell.Menu{
		{Title: "Gallery", Items: []shell.MenuItem{
			{Title: "About Gallery", Role: shell.RoleAbout},
			{Separator: true},
			{Title: "Hide Gallery", Role: shell.RoleHide},
			{Title: "Quit Gallery", Role: shell.RoleQuit},
		}},
		{Title: "View", Items: []shell.MenuItem{
			{ID: menuThemeLight, Title: "Light"},
			{ID: menuThemeDark, Title: "Dark"},
			{Separator: true},
			{ID: menuThemeGlass, Title: "Glass"},
			{ID: menuThemeGlassDark, Title: "Glass Dark"},
		}},
		{Title: "Window", Items: []shell.MenuItem{
			{Title: "Minimize", Role: shell.RoleMinimize},
			{Title: "Zoom", Role: shell.RoleZoom},
			{Separator: true},
			{Title: "Close Window", Role: shell.RoleClose},
		}},
	}, s.onMenu)
}

// onMenu handles an item the user chose. The capability delivers this on the UI
// goroutine, so SetState is the right call rather than PostState.
func (s *galleryState) onMenu(id int) {
	mode, ok := menuThemeMode(id)
	if !ok {
		return
	}
	s.SetState(func() { s.mode = mode })
}

// menuThemeMode maps a menu ID to the theme it selects. Separate from onMenu so
// the mapping can be tested without a platform menu to click.
func menuThemeMode(id int) (themeMode, bool) {
	switch id {
	case menuThemeLight:
		return modeLight, true
	case menuThemeDark:
		return modeDark, true
	case menuThemeGlass:
		return modeGlass, true
	case menuThemeGlassDark:
		return modeGlassDark, true
	}
	return 0, false
}

func (s *galleryState) Build(ctx widget.Ctx) widget.Widget {
	th := s.mode.theme()
	ctl := themeControl{mode: s.mode, set: func(m themeMode) {
		s.SetState(func() { s.mode = m })
	}}
	// Provide the theme and the switcher control to the whole tree, then run the
	// Navigator over the catalog home. appSurface paints the page background
	// (a flat fill, or a gradient backdrop under the glass themes so their
	// translucent surfaces have something to frost).
	return widget.Provide[theme.Theme]{
		Value: th,
		Child: widget.Provide[themeControl]{
			Value: ctl,
			Child: appSurface(th, widget.Navigator{Home: homePage{}}),
		},
	}
}

// --- Home: section catalog + theme switcher ----------------------------------

// section is one catalog entry: a title, a one-line summary, and a factory for
// the page tapping it pushes.
type section struct {
	title, subtitle string
	page            func() widget.Widget
}

// sections returns the catalog in display order. Each page factory returns a
// self-contained widget; most wrap their demo in sectionPage (adds the scaffold
// chrome + a scrolling body), while Navigator & Hero pushes its own full-page
// feed — a push transition cannot be demonstrated inside a scrolling list.
func sections() []section {
	sp := func(title, subtitle string, body widget.Widget) func() widget.Widget {
		return func() widget.Widget { return sectionPage{title: title, subtitle: subtitle, body: body} }
	}
	return []section{
		{"Buttons & tappables", "Button, Primary, press-feedback rows",
			sp("Buttons & tappables", "Button, Primary, and Tappable rows", buttonsSection{})},
		{"Form controls", "Switch, Checkbox, Radio, Slider — all live",
			sp("Form controls", "Switch, Checkbox, Radio group, Slider", formSection{})},
		{"Selection", "Dropdown, Segmented, Tabs — bound live",
			sp("Selection", "Dropdown, Segmented, and Tabs", selectionSection{})},
		{"Pickers", "Date & time pickers in a dialog",
			sp("Pickers", "Date and time pickers, echoing your pick", pickersSection{})},
		{"Text input", "Field with live caret, selection & echo",
			sp("Text input", "Field / TextField, echoing what you type", textInputSection{})},
		{"Typography", "The Display → Caption type scale",
			sp("Typography", "The theme's TypeScale, role by role", typographySection{})},
		{"Cards & surfaces", "Card, Decorated, Opacity",
			sp("Cards & surfaces", "Surfaces, borders, and group opacity", cardsSection{})},
		{"Charts", "Line, area, bar, pie & heatmap marks",
			sp("Charts", "Declarative marks over shared scales", chartsSection{})},
		{"Dialogs & menus", "ShowDialog and ShowMenu overlays",
			sp("Dialogs & menus", "Modal dialog and anchored menu", dialogsSection{})},
		{"Layout", "Grid, Wrap, Stack, AspectRatio",
			sp("Layout", "The core layout primitives, live", layoutSection{})},
		{"Animations", "AnimateColor, AnimateFloat, Scale, Rotation",
			sp("Animations", "Implicit animations you can trigger", animationsSection{})},
		{"Navigator & Hero", "Push a page; the swatch flies with it",
			func() widget.Widget { return feedPage{} }},
		{"Pull to refresh", "Drag a list down to reload it",
			sp("Pull to refresh", "LazyList with Refreshing / OnRefresh", refreshSection{})},
		{"Swipe to dismiss", "Swipe a row aside to remove it",
			sp("Swipe to dismiss", "Dismissible rows, keyed so the right one goes", dismissSection{})},
		{"Tree", "Fold a hierarchy; rows announce their state",
			sp("Tree", "Expandable rows, indented and announced as a tree", treeSection{})},
		{"Autocomplete", "Type-ahead over an in-memory list",
			sp("Autocomplete", "Suggestions filtered as you type", autocompleteSection{})},
		{"Reorderable list", "Drag rows into a new order",
			sp("Reorderable list", "Uniform rows, reordered by dragging", reorderSection{})},
		{"Drag & drop", "Carry chips between two bins",
			sp("Drag & drop", "Draggable payloads and targets that accept them", dragDropSection{})},
		{"Rich text & selection", "Styled spans, a link, and drag-to-select",
			sp("Rich text & selection", "Rich spans inside a SelectionArea", richTextSection{})},
		{"Transform", "Rotate and scale a still-live widget",
			sp("Transform", "A transformed subtree that still takes taps", transformSection{})},
		{"Right to left", "Mirror a layout, not its glyphs",
			sp("Right to left", "Directionality flipping a whole subtree", rtlSection{})},
	}
}

type homePage struct{}

// homeHook lets tests grab the Navigator handle and push pages directly.
var homeHook func(widget.Nav)

func (homePage) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)
	nav := ctx.MustOf[widget.Nav]()
	if homeHook != nil {
		homeHook(nav)
	}

	rows := make([]widget.Widget, 0, len(sections())*2)
	for _, sec := range sections() {
		if len(rows) > 0 {
			rows = append(rows, widget.Sized{H: 10})
		}
		rows = append(rows, sectionCard(th, sec, func() { nav.Push(sec.page()) }))
	}
	list := widget.Scroll{Child: widget.Padding{
		Insets: geom.Insets{Left: 16, Right: 16, Top: 4, Bottom: 28},
		Child:  sectionColumn(rows...),
	}}

	head := widget.Column(
		theme.Display("Catalog"),
		widget.Sized{H: 4},
		theme.Label("A tour of the gophics component set"),
		widget.Sized{H: 14},
		themeSwitcher(ctx),
	)
	head.CrossAlign = layout.CrossStart

	col := widget.Column(
		widget.Padding{Insets: geom.Insets{Left: 16, Right: 16, Top: 22, Bottom: 12}, Child: head},
		widget.Expand(list),
	)
	col.CrossAlign = layout.CrossStretch
	return appSurface(th, col)
}

// themeSwitcher is a row of buttons cycling the four built-in themes; the active
// one shows the filled Primary style.
func themeSwitcher(ctx widget.Ctx) widget.Widget {
	ctl := ctx.MustOf[themeControl]()
	btns := make([]widget.Widget, len(allModes))
	for i, m := range allModes {
		btns[i] = theme.Button{
			Label:   m.label(),
			Primary: m == ctl.mode,
			OnTap:   func() { ctl.set(m) },
		}
	}
	return widget.Wrap{Spacing: 8, RunSpacing: 8, Children: btns}
}

// sectionCard is one tappable catalog entry.
func sectionCard(th theme.Theme, sec section, onTap func()) widget.Widget {
	info := widget.Column(
		widget.Text{Value: sec.title, Font: theme.FontBold, Size: th.Type.Heading, Color: th.Text},
		widget.Sized{H: 3},
		widget.Text{Value: sec.subtitle, Size: th.Type.Label, Color: th.Muted, MaxLines: 1, Ellipsis: true},
	)
	info.CrossAlign = layout.CrossStart
	row := widget.Row(
		widget.Expand(info),
		widget.Sized{W: 8},
		widget.Text{Value: "›", Size: th.Type.Title, Color: th.Muted},
	)
	return theme.Tappable{
		Background: th.Surface,
		Radius:     th.Radius,
		Pad:        geom.InsetsSymmetric(14, 14),
		Haptic:     true,
		OnTap:      onTap,
		Child:      row,
	}
}

// --- Section page chrome -----------------------------------------------------

// sectionPage wraps a demo body in the shared scaffold and a scrolling area.
type sectionPage struct {
	title, subtitle string
	body            widget.Widget
}

func (p sectionPage) Build(ctx widget.Ctx) widget.Widget {
	scroll := widget.Scroll{Child: widget.Padding{
		Insets: geom.Insets{Left: 16, Right: 16, Top: 4, Bottom: 32},
		Child:  p.body,
	}}
	return scaffold(ctx, p.title, p.subtitle, widget.Expand(scroll))
}

// scaffold is the shared page frame: a title/subtitle header (with a Back button
// once there's something to pop) over the page body, on the app surface.
func scaffold(ctx widget.Ctx, title, subtitle string, body widget.Widget) widget.Widget {
	th := theme.Of(ctx)
	titleCol := widget.Column(
		theme.Title(title),
		widget.Sized{H: 2},
		theme.Label(subtitle),
	)
	titleCol.CrossAlign = layout.CrossStart

	var head widget.Widget = titleCol
	if nav, ok := ctx.Of[widget.Nav](); ok && nav.Depth() > 1 {
		back := theme.Button{Label: "← Back", OnTap: func() { nav.Pop() }}
		row := widget.Row(back, widget.Sized{W: 12}, widget.Expand(titleCol))
		head = row
	}

	col := widget.Column(
		widget.Padding{Insets: geom.Insets{Left: 16, Right: 16, Top: 20, Bottom: 8}, Child: head},
		body,
	)
	col.CrossAlign = layout.CrossStretch
	return appSurface(th, col)
}

// --- Shared helpers ----------------------------------------------------------

// appSurface paints the page background behind child. Opaque themes get a flat
// fill; the glass themes (Blur > 0) get a soft gradient backdrop so their
// translucent surfaces have real content to frost over. The tree shape is kept
// constant (a Stack over a background Canvas) across every theme, so switching
// themes never remounts the Navigator underneath it — page and scroll state
// survive the switch.
func appSurface(th theme.Theme, child widget.Widget) widget.Widget {
	bg := widget.Canvas{Draw: func(c paint.Canvas, size geom.Size) {
		if th.Blur > 0 {
			drawBackdrop(c, size, th.Dark)
		} else {
			c.FillRect(geom.Rect{Max: size.Pt()}, th.Bg)
		}
	}}
	return widget.Stack{Children: []widget.Widget{bg, capWidth(child)}}
}

// contentMaxW keeps the catalog content in a single readable column, centered
// over the full-bleed background, instead of sprawling across a wide window.
const contentMaxW = 760

// capWidth centers child and caps it at contentMaxW, filling narrower screens.
// It centers with symmetric padding (rather than a MainCenter flex) so it works
// under the loose constraints the Stack hands it — the pad fills the surplus and
// the child gets exactly contentMaxW.
//
// The Padding is always emitted, with zero insets when there is no surplus,
// because the shape of this subtree must not depend on the window width.
// Returning a bare child below the threshold and a wrapped one above it changed
// the tree's shape as the window crossed contentMaxW, and reconciliation
// matches by position: child got a new element, and the Navigator underneath it
// remounted and dropped its page stack. Resizing across 760px threw the reader
// back to the section list. Same reason appSurface keeps a constant shape
// across themes, one caller up.
func capWidth(child widget.Widget) widget.Widget {
	return widget.LayoutBuilder{Build: func(cs layout.Constraints) widget.Widget {
		pad := max((cs.Max.W-contentMaxW)/2, 0)
		return widget.Padding{Insets: geom.Insets{Left: pad, Right: pad}, Child: child}
	}}
}

// drawBackdrop fills the surface with a diagonal two-tone gradient plus a
// couple of soft color blooms — a photo-ish backdrop for the glass material.
func drawBackdrop(c paint.Canvas, size geom.Size, dark bool) {
	r := geom.Rect{Max: size.Pt()}
	var a, b paint.Color
	if dark {
		a, b = paint.RGB(0.10, 0.12, 0.20), paint.RGB(0.18, 0.10, 0.16)
	} else {
		a, b = paint.RGB(0.78, 0.84, 0.93), paint.RGB(0.93, 0.82, 0.78)
	}
	c.FillRRectGradient(r, 0, a, b, false)
	// Two blooms of the accent hues, painted translucent for a lit-glass feel.
	bloom := func(cx, cy, rad float32, col paint.Color) {
		c.FillRRect(geom.RectXYWH(cx-rad, cy-rad, rad*2, rad*2), rad, col)
	}
	if dark {
		bloom(size.W*0.2, size.H*0.18, size.W*0.5, paint.Color{R: 0.30, G: 0.20, B: 0.45, A: 0.35})
		bloom(size.W*0.85, size.H*0.7, size.W*0.5, paint.Color{R: 0.45, G: 0.22, B: 0.20, A: 0.30})
	} else {
		bloom(size.W*0.2, size.H*0.18, size.W*0.5, paint.Color{R: 0.55, G: 0.62, B: 0.95, A: 0.30})
		bloom(size.W*0.85, size.H*0.7, size.W*0.5, paint.Color{R: 0.95, G: 0.62, B: 0.45, A: 0.28})
	}
}

// sectionColumn is a cross-stretched vertical stack — the default body layout,
// so full-width controls (Field, Slider, dividers) fill the page width.
func sectionColumn(children ...widget.Widget) widget.Widget {
	col := widget.Column(children...)
	col.CrossAlign = layout.CrossStretch
	return col
}

// leftColumn stacks children left-aligned (for control groups like a checkbox or
// radio list that should sit at the start of the column, not centered).
func leftColumn(children ...widget.Widget) widget.Widget {
	col := widget.Column(children...)
	col.CrossAlign = layout.CrossStart
	return col
}

// groupLabel titles a group of demos within a section.
func groupLabel(s string) widget.Widget {
	return widget.Padding{Insets: geom.Insets{Top: 12, Bottom: 6}, Child: theme.Heading(s)}
}

// divider is a hairline rule in the theme's border color.
func divider(th theme.Theme) widget.Widget {
	return widget.Sized{H: 1, Child: widget.Fill{Color: th.Border}}
}

// Config is the app's window configuration, shared by every shell that runs
// it: the desktop binary, the web build, and the mobile bind package.
func Config() app.Config {
	return app.Config{
		Title:        "Gophics Catalog",
		AppID:        "com.gophics.gallery",
		Size:         geom.Size{W: 420, H: 760},
		Background:   theme.Light().Bg,
		Font:         goregular.TTF,
		FontFamilies: map[string][]byte{"bold": gobold.TTF},
	}
}

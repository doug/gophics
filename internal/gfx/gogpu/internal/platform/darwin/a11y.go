//go:build darwin

// macOS accessibility bridge. A gogpu window draws every pixel itself, so
// VoiceOver sees one opaque view unless the application publishes what it
// drew. This attaches a list of NSAccessibilityElements to the content view,
// which is exactly the mechanism AppKit provides for custom-drawn UI (and what
// a custom NSTableView cell or a canvas-based editor uses).
//
// Two design choices are worth stating, because both are deliberate:
//
// The published list is FLAT. The incoming tree has parents, but every element
// is attached directly to the content view with its frame in the view's own
// coordinate space. VoiceOver navigates a flat element list perfectly well —
// it is the documented shape for custom-drawn content — and flattening avoids
// per-parent coordinate conversion and the deep, mostly-meaningless nesting
// that a widget tree produces. An AT user stepping through a form wants the
// fields, not the six layout boxes around each one.
//
// Elements use NSAccessibilityElement's own stored properties rather than a
// hand-rolled class implementing the accessibility protocol. That matters for
// a practical reason: the protocol's accessibilityFrame returns an NSRect, and
// the Go callback trampoline this package relies on can return only a single
// scalar — no struct returns. NSAccessibilityElement stores the frame for us
// and answers that call in Objective-C, so the only method we implement in Go
// is accessibilityPerformPress, which returns BOOL.

package darwin

import (
	"sync"

	"github.com/go-webgpu/goffi/ffi"
)

// A11yNode mirrors platform.A11yNode. It is declared here rather than imported
// because the platform package imports this one — the adapter in
// platform_darwin.go converts across the boundary.
type A11yNode struct {
	ID         int
	ParentID   int
	Role       string
	Label      string
	Value      string
	Hint       string
	X, Y, W, H int
	Tappable   bool
	Focused    bool
	Disabled   bool
	Selected   bool
	Checkable  bool
	Checked    bool
	Expandable bool
	Expanded   bool
}

var (
	a11yElementClass     Class
	a11yElementClassOnce sync.Once
	errA11yElementClass  error

	// a11yRegistry maps a live element's pointer to the node id it stands
	// for, plus the callback to run when it is pressed. The map is the reason
	// the subclass needs no instance variables: the element identifies itself
	// by its own address, which is stable for its lifetime.
	a11yRegistry = struct {
		sync.Mutex
		nodes    map[uintptr]int
		activate func(id int)
	}{nodes: map[uintptr]int{}}
)

// a11yElementClassRef returns the registered GoGPUA11yElement class, creating
// it once.
func a11yElementClassRef() (Class, error) {
	a11yElementClassOnce.Do(func() {
		a11yElementClass, errA11yElementClass = registerA11yElementClass()
	})
	return a11yElementClass, errA11yElementClass
}

func registerA11yElementClass() (Class, error) {
	if err := initRuntime(); err != nil {
		return 0, err
	}
	super := GetClass("NSAccessibilityElement")
	if super == 0 {
		return 0, ErrClassNotFound
	}
	cls := AllocateClassPair(super, "GoGPUA11yElement")
	if cls == 0 {
		// Already registered by an earlier call in this process (the once
		// guard covers this package, not a second copy of it).
		if existing := GetClass("GoGPUA11yElement"); existing != 0 {
			return existing, nil
		}
		return 0, ErrClassNotFound
	}

	// -(BOOL)accessibilityPerformPress — VoiceOver's "activate" (VO-Space).
	// A scalar return, which is all the callback trampoline supports.
	//
	// YES means "accepted", not "completed". fn posts to the UI goroutine and
	// returns, so the widget handler has not run by the time this answers —
	// deliberately, since this is called on the AppKit thread and the handler
	// touches widget state. That is the ordinary dispatch-then-report-handled
	// pattern; VoiceOver wants to know the action was recognised, not to wait
	// for it.
	pressIMP := ffi.NewCallback(func(self, sel uintptr) uintptr {
		a11yRegistry.Lock()
		id, ok := a11yRegistry.nodes[self]
		fn := a11yRegistry.activate
		a11yRegistry.Unlock()
		if !ok || fn == nil {
			return 0 // NO: nothing happened
		}
		fn(id)
		return 1 // YES
	})
	ClassAddMethod(cls, RegisterSelector("accessibilityPerformPress"), pressIMP, "B@:")

	// -(BOOL)isAccessibilityElement — every element we publish is real.
	// NSAccessibilityElement answers YES already; stating it costs nothing and
	// makes the class correct on its own terms.
	isElemIMP := ffi.NewCallback(func(self, sel uintptr) uintptr { return 1 })
	ClassAddMethod(cls, RegisterSelector("isAccessibilityElement"), isElemIMP, "B@:")

	RegisterClassPair(cls)
	return cls, nil
}

// axRole maps the portable ARIA-style role onto the NSAccessibility role
// constant. The AppKit vocabulary is smaller, so several roles land on the
// same value; where there is no good match the element becomes a group, which
// is still navigable and labeled — better than an unrecognized role string,
// which VoiceOver reports as "unknown".
func axRole(role string, tappable bool) string {
	switch role {
	case "button":
		return "AXButton"
	case "checkbox", "switch", "toggle":
		return "AXCheckBox"
	case "radio", "tab":
		// AppKit has no tab-item role that VoiceOver narrates usefully; a tab
		// behaves like a radio button (one of a set, exactly one chosen), and
		// that is how AppKit's own tab views expose their tabs.
		return "AXRadioButton"
	case "slider":
		return "AXSlider"
	case "progressbar":
		return "AXProgressIndicator"
	case "textfield", "textbox":
		return "AXTextField"
	case "heading":
		return "AXHeading"
	case "link":
		return "AXLink"
	case "image", "img":
		return "AXImage"
	case "list":
		return "AXList"
	case "listitem":
		return "AXRow"
	case "text", "label":
		return "AXStaticText"
	}
	if tappable {
		return "AXButton"
	}
	return "AXGroup"
}

// a11ySels caches the selectors used per publish.
var (
	a11ySels struct {
		alloc                        SEL
		init                         SEL
		setAccessibilityRole         SEL
		setAccessibilityLabel        SEL
		setAccessibilityValue        SEL
		setAccessibilityHelp         SEL
		setAccessibilityParent       SEL
		setAccessibilityFrameInParen SEL
		setAccessibilityEnabled      SEL
		setAccessibilityDisclosing   SEL
		setAccessibilityChildren     SEL
		numberWithBool               SEL
		array                        SEL
		addObject                    SEL
		numberWithInteger            SEL
		arrayWithObjectsCount        SEL
		dictWithObjectsForKeys       SEL
	}
	a11ySelsOnce sync.Once
)

func initA11ySelectors() {
	a11ySelsOnce.Do(func() {
		a11ySels.alloc = RegisterSelector("alloc")
		a11ySels.init = RegisterSelector("init")
		a11ySels.setAccessibilityRole = RegisterSelector("setAccessibilityRole:")
		a11ySels.setAccessibilityLabel = RegisterSelector("setAccessibilityLabel:")
		a11ySels.setAccessibilityValue = RegisterSelector("setAccessibilityValue:")
		a11ySels.setAccessibilityHelp = RegisterSelector("setAccessibilityHelp:")
		a11ySels.setAccessibilityDisclosing = RegisterSelector("setAccessibilityDisclosing:")
		a11ySels.setAccessibilityParent = RegisterSelector("setAccessibilityParent:")
		a11ySels.setAccessibilityFrameInParen = RegisterSelector("setAccessibilityFrameInParentSpace:")
		a11ySels.setAccessibilityEnabled = RegisterSelector("setAccessibilityEnabled:")
		a11ySels.setAccessibilityChildren = RegisterSelector("setAccessibilityChildren:")
		a11ySels.numberWithBool = RegisterSelector("numberWithBool:")
		a11ySels.array = RegisterSelector("array")
		a11ySels.addObject = RegisterSelector("addObject:")
		a11ySels.numberWithInteger = RegisterSelector("numberWithInteger:")
		a11ySels.arrayWithObjectsCount = RegisterSelector("arrayWithObjects:count:")
		a11ySels.dictWithObjectsForKeys = RegisterSelector("dictionaryWithObjects:forKeys:")
	})
}

// SetA11yTree publishes nodes as accessibility elements on the content view.
func (w *Window) SetA11yTree(nodes []A11yNode, activate func(id int)) {
	w.mu.Lock()
	view := w.contentView
	_, hPoints := w.width, w.height
	w.mu.Unlock()

	if view.IsNil() {
		return
	}
	initA11ySelectors()

	if _, err := a11yElementClassRef(); err != nil {
		return
	}

	// Replacing the registry wholesale drops the previous elements' entries;
	// the elements themselves are released when the view's children array is
	// replaced below.
	a11yRegistry.Lock()
	a11yRegistry.nodes = make(map[uintptr]int, len(nodes))
	a11yRegistry.activate = activate
	a11yRegistry.Unlock()

	if len(nodes) == 0 {
		view.SendPtr(a11ySels.setAccessibilityChildren, 0)
		return
	}

	scale := w.BackingScaleFactor()
	if scale <= 0 {
		scale = 1
	}
	viewH := float64(hPoints)

	children := ID(GetClass("NSMutableArray")).Send(a11ySels.array)
	if children.IsNil() {
		return
	}

	for _, n := range nodes {
		// Nodes that name nothing and do nothing are layout scaffolding. They
		// would add a step to every VoiceOver traversal and say nothing when
		// reached, so they are not published.
		if !publishable(n) {
			continue
		}
		el := newA11yElement(n, viewH, scale)
		if el.IsNil() {
			continue
		}
		el.SendPtr(a11ySels.setAccessibilityParent, view.Ptr())
		if n.Tappable {
			a11yRegistry.Lock()
			a11yRegistry.nodes[el.Ptr()] = n.ID
			a11yRegistry.Unlock()
		}
		children.SendPtr(a11ySels.addObject, el.Ptr())
	}

	view.SendPtr(a11ySels.setAccessibilityChildren, children.Ptr())
	// The view itself is a container for the elements, not an element with
	// content of its own; saying so stops VoiceOver announcing the canvas.
	view.SendPtr(a11ySels.setAccessibilityRole, createNSString("AXGroup").Ptr())
}

// AnnounceA11y speaks message through VoiceOver without changing the tree —
// the live-region idiom, for things a user must be told that are not part of
// the UI's structure ("5 results", "upload failed").
//
// AppKit routes these through NSAccessibilityPostNotificationWithUserInfo, a
// plain C function rather than an Objective-C method, so the call is built with
// an FFI interface instead of the objc_msgSend path everything else here uses.
// See a11y_announce.go.
//
// Posted against the content view: the announcement needs an element that is
// part of the accessibility hierarchy, and the view is the one object that is
// always there whether or not a tree has been published.
func (w *Window) AnnounceA11y(message string, assertive bool) {
	w.mu.Lock()
	view := w.contentView
	w.mu.Unlock()
	if view == 0 {
		return
	}
	postAnnouncement(view, message, assertive)
}

// publishable reports whether a node is worth a step in a VoiceOver
// traversal. A node that names nothing and does nothing is layout
// scaffolding: reaching it would say nothing.
func publishable(n A11yNode) bool {
	return n.Label != "" || n.Value != "" || n.Tappable
}

// newA11yElement builds one configured accessibility element. viewH is the
// content view's height in points and scale its backing scale factor, which
// together convert gophics's top-left physical pixels into the bottom-left
// points an unflipped NSView uses.
//
// It is separate from SetA11yTree so the Objective-C path can be exercised by
// a test without a live window.
func newA11yElement(n A11yNode, viewH, scale float64) ID {
	initA11ySelectors()
	cls, err := a11yElementClassRef()
	if err != nil {
		return 0
	}
	if scale <= 0 {
		scale = 1
	}
	el := ID(cls).Send(a11ySels.alloc).Send(a11ySels.init)
	if el.IsNil() {
		return 0
	}

	el.SendPtr(a11ySels.setAccessibilityRole, createNSString(axRole(n.Role, n.Tappable)).Ptr())
	if n.Label != "" {
		el.SendPtr(a11ySels.setAccessibilityLabel, createNSString(n.Label).Ptr())
	}
	if n.Hint != "" {
		el.SendPtr(a11ySels.setAccessibilityHelp, createNSString(n.Hint).Ptr())
	}
	switch {
	case n.Checkable:
		// VoiceOver reads a checkbox's state from its value as a number.
		num := ID(GetClass("NSNumber")).SendBool(a11ySels.numberWithBool, n.Checked)
		el.SendPtr(a11ySels.setAccessibilityValue, num.Ptr())
	case n.Value != "":
		el.SendPtr(a11ySels.setAccessibilityValue, createNSString(n.Value).Ptr())
	}
	if n.Disabled {
		el.SendBool(a11ySels.setAccessibilityEnabled, false)
	}
	// AXDisclosing is how NSAccessibility reports an open/closed row —
	// outline rows and disclosure triangles both read it. Set only when the
	// node is expandable: on an element that cannot disclose, "not disclosing"
	// is announced as a closed branch rather than as a leaf.
	if n.Expandable {
		el.SendBool(a11ySels.setAccessibilityDisclosing, n.Expanded)
	}

	// Physical pixels, y down from the top — to points, y up from the bottom,
	// which is what an unflipped NSView's coordinate space uses.
	x := float64(n.X) / scale
	y := float64(n.Y) / scale
	wpt := float64(n.W) / scale
	hpt := float64(n.H) / scale
	el.SendRect(a11ySels.setAccessibilityFrameInParen, NSRect{
		Origin: NSPoint{X: x, Y: viewH - y - hpt},
		Size:   NSSize{Width: wpt, Height: hpt},
	})
	return el
}

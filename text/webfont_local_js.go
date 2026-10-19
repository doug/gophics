//go:build js

package text

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"syscall/js"
)

// LocalFontSource returns a WebFontSource backed by the Local Font Access API
// (window.queryLocalFonts), the only browser API that hands a page the actual
// bytes of an installed font — FontData.blob() returns the raw SFNT.
//
// It is not the default, and cannot be. The reasons are worth recording here
// because together they are the whole answer to "why doesn't the web target
// just use the system fonts":
//
//   - It exists only in desktop Chromium. MDN's compatibility data records
//     Chrome 103 and Edge 103 and an explicit false for Firefox, Safari,
//     Safari iOS, Chrome on Android and Android WebView. WebKit's reply to the
//     request for a position was that they are strongly against letting
//     scripts enumerate fonts at all and would rather supply a font picker;
//     Mozilla has recorded no position and shipped nothing. So this covers
//     desktop Chrome and Edge and no phone at all.
//   - It is permission-gated and needs a user gesture. The first call prompts
//     for the "local-fonts" permission and throws unless it is made from a
//     transient user activation, so a framework cannot reach for it during
//     startup: a page that tried would either throw or put a font permission
//     prompt in front of a user who asked for nothing. Calling it has to be
//     something the app does from a click it owns.
//   - Font enumeration is a fingerprinting surface — the installed set is close
//     to unique per machine and leaks locale — which is why the other two
//     engines refused, and why spending a permission prompt on it should be the
//     app's decision rather than a framework default.
//
// Where it does work it works properly: real system faces, no download. A
// desktop Chromium kiosk or an Electron-style shell is the case it is for.
//
// families names the candidate font families per script, best first; nil uses
// a built-in list of the usual macOS, Windows and Linux faces. Matching is
// case-insensitive against FontData.family, and the first family present on the
// machine wins. Only that one face is read, so the cost is one blob, not the
// whole font directory.
//
// Deliberately absent from the built-in list: colour emoji (Apple Color Emoji,
// Segoe UI Emoji, Noto Color Emoji). Their glyphs are bitmap or SVG data and
// AppendGlyphPath emits nothing for either, so resolving them would replace
// tofu with blank space — a worse bug, because it looks like a layout error
// rather than a missing font. A monochrome outline face (Noto Emoji) passed in
// explicitly does render.
func LocalFontSource(families map[Script][]string) WebFontSource {
	if families == nil {
		families = defaultLocalFamilies
	}
	m := make(map[Script][]string, len(families))
	for k, v := range families {
		m[k] = append([]string(nil), v...)
	}
	return &localFontSource{families: m}
}

// defaultLocalFamilies lists, per script, the families that are actually
// installed on a stock machine, in the order macOS, Windows, Linux — which is
// also roughly the order of how likely a desktop Chromium user is to have one,
// since this API exists on desktop only. Noto names appear last because a
// machine with them has usually been set up deliberately.
var defaultLocalFamilies = map[Script][]string{
	"Hani": {"PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", "SimSun", "Noto Sans CJK SC", "Noto Sans SC", "Source Han Sans SC", "WenQuanYi Micro Hei"},
	"Hira": {"Hiragino Sans", "Hiragino Kaku Gothic ProN", "Yu Gothic", "Meiryo", "MS Gothic", "Noto Sans CJK JP", "Noto Sans JP", "IPAGothic"},
	"Kana": {"Hiragino Sans", "Hiragino Kaku Gothic ProN", "Yu Gothic", "Meiryo", "MS Gothic", "Noto Sans CJK JP", "Noto Sans JP", "IPAGothic"},
	"Hang": {"Apple SD Gothic Neo", "Malgun Gothic", "Noto Sans CJK KR", "Noto Sans KR", "NanumGothic"},
	"Arab": {"Geeza Pro", "SF Arabic", "Segoe UI", "Tahoma", "Noto Sans Arabic", "Noto Naskh Arabic"},
	"Hebr": {"Arial Hebrew", "Segoe UI", "Noto Sans Hebrew", "DejaVu Sans"},
	"Deva": {"Devanagari Sangam MN", "Kohinoor Devanagari", "Nirmala UI", "Noto Sans Devanagari", "Lohit Devanagari"},
	"Thai": {"Thonburi", "Leelawadee UI", "Noto Sans Thai"},
	"Cyrl": {"Helvetica", "Arial", "Segoe UI", "DejaVu Sans", "Noto Sans"},
	"Grek": {"Helvetica", "Arial", "Segoe UI", "DejaVu Sans", "Noto Sans"},
	"Armn": {"Mshtakan", "Sylfaen", "Noto Sans Armenian"},
	"Geor": {"Noto Sans Georgian", "Sylfaen"},
	"Ethi": {"Kefa", "Nyala", "Noto Sans Ethiopic"},
}

type localFontSource struct {
	families map[Script][]string
	// listed caches the enumeration. The permission prompt and the
	// enumeration itself are the expensive half, and they answer for every
	// script at once, so asking again for the second missing script would
	// pay for both twice — and would put a second permission prompt in
	// front of the user.
	//
	// The Once is not just memoisation. A page missing both Han and Thai
	// starts two fetch goroutines, so FontForScript is genuinely called
	// concurrently; without it both would race on these fields and both
	// would call queryLocalFonts.
	once      sync.Once
	listed    map[string]js.Value // lowercased family -> FontData
	listedErr error
}

func (l *localFontSource) FontForScript(ctx context.Context, sc Script) ([]byte, error) {
	want := l.families[sc]
	if len(want) == 0 {
		return nil, fmt.Errorf("no local font families listed for script %s", sc)
	}
	if err := l.enumerate(ctx); err != nil {
		return nil, err
	}
	for _, name := range want {
		fd, ok := l.listed[strings.ToLower(name)]
		if !ok {
			continue
		}
		data, err := l.read(ctx, fd)
		if err != nil {
			return nil, fmt.Errorf("reading local font %q: %w", name, err)
		}
		return data, nil
	}
	return nil, fmt.Errorf("none of the %d candidate families for script %s is installed (tried %s)",
		len(want), sc, strings.Join(want, ", "))
}

func (l *localFontSource) enumerate(ctx context.Context) error {
	l.once.Do(func() { l.listedErr = l.enumerateOnce(ctx) })
	return l.listedErr
}

func (l *localFontSource) enumerateOnce(ctx context.Context) error {
	q := js.Global().Get("queryLocalFonts")
	if q.Type() != js.TypeFunction {
		// Stated in full rather than as "unsupported", because the next
		// question is always whether a flag or a polyfill would fix it, and
		// the answer is no.
		return errors.New("this browser has no Local Font Access API " +
			"(window.queryLocalFonts): it ships in desktop Chrome and Edge 103+ only — " +
			"not Firefox, not Safari, not any mobile browser. Use FetchFontSource instead")
	}
	arr, err := await(ctx, js.Global().Call("queryLocalFonts"))
	if err != nil {
		// The overwhelmingly common rejection is NotAllowedError, from a
		// denied permission or a call made outside a user gesture, and
		// those are a configuration mistake rather than a browser defect.
		return fmt.Errorf("queryLocalFonts was refused (%w); the local-fonts "+
			"permission must be granted and the call made from a user gesture", err)
	}
	n := arr.Get("length").Int()
	l.listed = make(map[string]js.Value, n)
	for i := 0; i < n; i++ {
		fd := arr.Index(i)
		fam := fd.Get("family")
		if fam.Type() != js.TypeString {
			continue
		}
		key := strings.ToLower(fam.String())
		// First wins: a family has one entry per style (regular, bold,
		// italic), and the regular one is listed first. Taking the last
		// would hand the shaper an italic face to draw upright text with.
		if _, dup := l.listed[key]; !dup {
			l.listed[key] = fd
		}
	}
	return nil
}

// read pulls one FontData's bytes: blob() for the SFNT, then arrayBuffer() to
// get at it, since there is no synchronous accessor for either.
func (l *localFontSource) read(ctx context.Context, fd js.Value) ([]byte, error) {
	blob, err := await(ctx, fd.Call("blob"))
	if err != nil {
		return nil, err
	}
	buf, err := await(ctx, blob.Call("arrayBuffer"))
	if err != nil {
		return nil, err
	}
	u8 := js.Global().Get("Uint8Array").New(buf)
	out := make([]byte, u8.Get("length").Int())
	js.CopyBytesToGo(out, u8)
	return out, nil
}

// await blocks until p settles. Safe only off the UI goroutine, which is where
// a WebFontSource runs: a promise cannot settle while a JS event listener is
// still on the stack, so awaiting one from inside an event handler deadlocks
// the tab — the hazard syscall/js documents for blocking inside a wrapped func
// and that the fetch package's doc comment spells out.
//
// ctx is checked before starting rather than raced against the promise. Letting
// a cancellation return early would mean releasing the js.Funcs while the
// browser still held references to them, and there is nothing to abort on the
// other side: unlike fetch(), neither queryLocalFonts nor Blob.arrayBuffer
// takes an AbortSignal.
func await(ctx context.Context, p js.Value) (js.Value, error) {
	if err := ctx.Err(); err != nil {
		return js.Undefined(), err
	}
	type outcome struct {
		v   js.Value
		err error
	}
	done := make(chan outcome, 1)
	var onOK, onErr js.Func
	onOK = js.FuncOf(func(_ js.Value, args []js.Value) any {
		var v js.Value
		if len(args) > 0 {
			v = args[0]
		}
		done <- outcome{v: v}
		return nil
	})
	onErr = js.FuncOf(func(_ js.Value, args []js.Value) any {
		msg := "rejected"
		if len(args) > 0 && !args[0].IsUndefined() && !args[0].IsNull() {
			msg = args[0].Call("toString").String()
		}
		done <- outcome{err: errors.New(msg)}
		return nil
	})
	p.Call("then", onOK).Call("catch", onErr)
	o := <-done
	// Released only once the chain has settled; the chain is single-shot, so
	// nothing can call these again, and releasing earlier would free funcs
	// the browser still holds.
	onOK.Release()
	onErr.Release()
	return o.v, o.err
}

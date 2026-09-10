//go:build js && wasm

package web

import (
	"strings"
	"syscall/js"
)

// hostUA is the user agent, or "" where the browser withholds one.
func hostUA() string {
	ua := js.Global().Get("navigator").Get("userAgent")
	if ua.Type() != js.TypeString {
		return ""
	}
	return ua.String()
}

// hostIsAndroid reports an Android browser — the one platform whose scroll
// physics and gesture thresholds differ from Apple's in the ways the widget
// layer models.
func hostIsAndroid() bool { return strings.Contains(hostUA(), "Android") }

// hostIsApple reports a Mac, iPhone or iPad browser, where a hardware
// keyboard follows Apple's editing conventions.
func hostIsApple() bool {
	u := hostUA()
	return strings.Contains(u, "Mac") || strings.Contains(u, "iPhone") || strings.Contains(u, "iPad")
}

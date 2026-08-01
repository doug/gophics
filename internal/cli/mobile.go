package cli

import (
	"fmt"
	"os"
	"path/filepath"
)

// buildMobile wraps `gomobile bind` to produce the native artifact the platform
// project links against: an .xcframework for iOS, an .aar for Android. o.pkg
// should be the gomobile bind package (the one exposing the mobile entry
// point), e.g. ./examples/hn/mobile.
func buildMobile(o buildOpts) (string, error) {
	if !have("gomobile") {
		return "", fmt.Errorf("gomobile not found — install with:\n" +
			"  go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init")
	}
	dir := outDir(o)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	var target, out string
	switch o.platform.name {
	case "ios":
		target, out = "ios", filepath.Join(dir, "App.xcframework")
	case "android":
		target, out = "android", filepath.Join(dir, "app.aar")
	}
	args := []string{"bind", "-target", target, "-o", out}
	if t := tagList(o.platform, o.tags); t != "" {
		args = append(args, "-tags", t)
	}
	args = append(args, o.pkg)
	if err := run("", nil, "gomobile", args...); err != nil {
		return "", fmt.Errorf("gomobile bind: %w", err)
	}
	return out, nil
}

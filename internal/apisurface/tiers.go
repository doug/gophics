package apisurface

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Tier is how much of a compatibility promise a package is proposed to carry.
//
// The module is pre-1.0 and the README says so: nothing is frozen, and renames
// land eagerly while the cost is low. A tier is therefore a *proposal* — the
// list of packages whose shape is believed settled enough to freeze at v1, and
// the list that is deliberately not. Writing it down costs nothing now and is
// the only way the "small surface, Go 1 ethos after 1.0" sentence can turn
// into a decision rather than a feeling. It also tells somebody evaluating the
// toolkit which imports will survive the tag, which is the question they
// actually have.
//
// The tiers are in Go, not in prose, because the tests read them: a new public
// package must be classified before the gate will pass, and a v1 package may
// not take an unstable package's types into its signatures.
type Tier string

const (
	// TierV1 is proposed to be frozen at v1: additive change only, no
	// signature change to anything already in the manifest.
	TierV1 Tier = "v1"

	// TierUnstable is explicitly not covered. It may break in any v0.x, and
	// after v1 it breaks in a minor release with a note. Importing it is a
	// choice to track the repo.
	TierUnstable Tier = "unstable"
)

// tiers classifies every public package. The reasoning, per package, is in
// README.md; the one-line notes here are the summary a reader of this file
// needs to not have to go and get it.
var tiers = map[string]Tier{
	// The framework an app is written against. These are the imports in every
	// gophics program, they have had the most use, and the shape of each one
	// is the thing the project is actually opinionated about.
	"anim":   TierV1, // curves and a controller; small, and the signatures are numbers
	"app":    TierV1, // app.Run and app.Headless are the entry points
	"geom":   TierV1, // value types and pure math, no churn pressure
	"input":  TierV1, // event types; a new event kind is additive
	"layout": TierV1, // the RenderObject protocol a custom widget implements
	"paint":  TierV1, // Color, Path, Canvas — see the Canvas note in README
	"shell":  TierV1, // the capability interfaces an app calls through ctx
	"text":   TierV1, // shaping types that appear in paint and widget signatures
	"theme":  TierV1, // design tokens and styled components
	"widget": TierV1, // the widget tree itself
	"intl":   TierV1, // not by plan; widget.Ctx.IntlLocale returns intl.Locale

	// Platform entry points. Three exported names between them, each called
	// once from a main package, and the signature is a window config in and an
	// error out. There is nothing there left to settle.
	"shell/desktop":  TierV1,
	"shell/terminal": TierV1,
	"shell/web":      TierV1,

	// Young, optional, or tooling. A break here is cheap to absorb: either it
	// is a compile error in a test, or it is a subsystem an app opted into.
	"apptest":             TierUnstable, // the golden-image tolerance model is still moving
	"chart":               TierUnstable, // a catalog that compounds; marks arrive and get renamed
	"fetch":               TierUnstable, // exists to dodge a linker problem that may get fixed
	"sound":               TierUnstable, // the audio subsystem is the least exercised layer
	"sound/device":        TierUnstable,
	"sound/mp3":           TierUnstable,
	"sound/ogg":           TierUnstable,
	"sound/pitch":         TierUnstable,
	"sound/procedural":    TierUnstable,
	"tools/uitrace/trace": TierUnstable, // a trace file format, versioned with its viewer
}

// tierExceptions records v1 keys whose signatures name something a v1 caller
// cannot commit to, with the reason each is tolerated.
//
// A v1 signature that mentions an unstable or unimportable type is a promise
// with a hole in it: the committed name cannot be used without the uncommitted
// one, so freezing the first and not the second is not a position anybody can
// act on. The right answer is usually to move the type, which is a change to
// somebody else's package — so the leak is recorded here with a reason and the
// test fails on a *new* one. An empty map was the goal, and is no longer quite
// the right target: one entry turned out to be the correct end state rather
// than an unfixed leak. "No entry without a reason that survives reading" is
// the standard the test can actually hold.
//
// Both entries here were the first thing the signature manifest found, and
// neither was visible while the manifest recorded names only. app.Headless.Scene
// has since been fixed — it returns the exported app.Scene projection — which
// leaves one. That one is deliberate and permanent rather than a to-do; its
// entry says why, including the two remedies that were proposed and do not work.
var tierExceptions = map[string]string{
	"paint.Painter.GPUCanvas": "takes *internal/gfx/gg.Context, a handle from the " +
		"vendored substrate. This is a permitted exclusion rather than a thing to " +
		"fix: no caller outside this module can obtain a *gg.Context in the first " +
		"place, so the method is unreachable from outside and excluding it from the " +
		"promise costs nobody anything. " +
		"Two remedies have been written here and neither survives contact. A " +
		"gpucontext token does not help: internal/gfx/gpucontext is in the same " +
		"vendored tree, so the parameter would still name a package no caller can " +
		"import. An exported substrate handle is not blocked by the fork — that tree " +
		"is modified here routinely, and AGENTS.md asks only that its upstream " +
		"ADR-citing comments be left alone — but it fails for a better reason: the " +
		"obstacle is the adapter, not the handle. ggCanvas drives some forty Context " +
		"methods whose parameters are gg's own vocabulary (BlendMode, Brush, Matrix, " +
		"MarkShape, ImageBuf, DrawImageOptions, text.ShapedGlyph, text.Face), so an " +
		"exported handle would have to export all of that with it. Closing this entry " +
		"therefore means promising the vendored substrate's drawing API under v1, " +
		"which COMPATIBILITY.md declines. The exclusion is the intended end state.",
}

// TierOf returns the tier of a module-relative package path, or the empty
// string when the package has not been classified.
func TierOf(pkg string) Tier { return tiers[pkg] }

// ByTier returns the classified packages grouped by tier, sorted within each.
func ByTier() map[Tier][]string {
	out := map[Tier][]string{}
	for pkg, t := range tiers {
		out[t] = append(out[t], pkg)
	}
	for _, v := range out {
		sort.Strings(v)
	}
	return out
}

// Unclassified lists packages present in entries that no tier covers.
//
// This is the gate that keeps the classification honest. A new public package
// is the moment to decide what it promises, and the only moment anybody will;
// after it has callers the decision is made for you.
func Unclassified(entries []Entry) []string {
	var out []string
	for _, p := range Packages(entries) {
		if _, ok := tiers[p]; !ok {
			out = append(out, p)
		}
	}
	return out
}

// qualifiedName matches a package-qualified exported name inside a rendered
// signature: widget.Widget, sound/mp3.Decoder, []paint.Color. Packages inside
// the module are qualified by their module-relative path, which is why the
// character class has to include a slash.
var qualifiedName = regexp.MustCompile(`[\w/]+\.[A-Z]\w*`)

// Leaks reports v1 entries whose signature names a type a caller cannot
// commit to, excluding the ones tierExceptions already accounts for.
//
// Two kinds. A type from an unstable package, where the committed name cannot
// be used without the uncommitted one. And a type from internal/, which an
// outside caller cannot name at all — the method is callable and its result is
// an opaque token, so it is not a crash, but it is not a thing anybody can
// write a type for either, and promising it at v1 promises nothing.
//
// Only possible now that signatures are recorded. It is the first thing the
// manifest bought that a name-only manifest could not have: the tier table on
// its own is a list of intentions, and this is the part that checks them
// against the code.
func Leaks(entries []Entry) []string {
	var out []string
	for _, e := range entries {
		if _, excused := tierExceptions[e.Key()]; excused {
			continue
		}
		out = append(out, leaksIn(e)...)
	}
	sort.Strings(out)
	return out
}

func leaksIn(e Entry) []string {
	if TierOf(e.Package()) != TierV1 {
		return nil
	}
	var out []string
	for _, ref := range qualifiedName.FindAllString(e.Signature(), -1) {
		pkg := ref[:strings.LastIndex(ref, ".")]
		switch {
		case strings.HasPrefix(pkg, "internal/"):
			out = append(out, fmt.Sprintf("%s names %s, which is not importable", e.Key(), ref))
		case TierOf(pkg) == TierUnstable:
			out = append(out, fmt.Sprintf("%s names %s, which is unstable", e.Key(), ref))
		}
	}
	return out
}

// StaleExceptions lists tierExceptions entries that no longer describe
// anything: the key is gone from the surface, or it no longer leaks.
//
// An allowlist nobody prunes stops being a record of known problems and
// becomes a place where problems go to be forgotten. Each of these is meant to
// be deleted the day its signature is fixed, and this is what notices.
func StaleExceptions(entries []Entry) []string {
	live := map[string]Entry{}
	for _, e := range entries {
		live[e.Key()] = e
	}
	var out []string
	for key := range tierExceptions {
		e, ok := live[key]
		switch {
		case !ok:
			out = append(out, key+": no longer in the surface")
		case len(leaksIn(e)) == 0:
			out = append(out, key+": no longer leaks — delete the exception")
		}
	}
	sort.Strings(out)
	return out
}

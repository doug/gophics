package app

import (
	"fmt"
	"html"
	"strings"

	"github.com/doug/gophics/internal/layoutbox"
)

// InspectHTML renders the current render tree (see InspectTree) as a
// self-contained HTML page: a depth-indented outline of every box — its type,
// semantic role/label, and bounds — beside a to-scale overlay showing where each
// box sits and nests. This is the "tree dump → browser view" widget inspector
// (PLAN.md M8). Call after a frame. The page inlines all CSS (no external
// assets); write it to a .html file and open it, or hand it to a viewer.
func (c *core) InspectHTML() string {
	nodes := c.InspectTree()

	var outline strings.Builder
	for i, n := range nodes {
		role := ""
		if r := n.Role.String(); r != "" && r != "none" {
			role = `<span class="role">` + html.EscapeString(r) + "</span>"
		}
		label := ""
		if n.Label != "" {
			label = ` <span class="label">“` + html.EscapeString(n.Label) + `”</span>`
		}
		fmt.Fprintf(&outline,
			`<div class="row" data-i="%d" style="padding-left:%dpx"><span class="type">%s</span>%s%s%s</div>`,
			i, n.Depth*16, html.EscapeString(shortType(n.Type)), role, label, rectStr(n))
	}

	var overlay strings.Builder
	rootW, rootH := 0, 0
	if len(nodes) > 0 {
		rootW, rootH = int(nodes[0].Rect.Dx()), int(nodes[0].Rect.Dy())
	}
	fmt.Fprintf(&overlay, `<div class="stage" style="width:%dpx;height:%dpx">`, rootW, rootH)
	for i, n := range nodes {
		w, h := int(n.Rect.Dx()), int(n.Rect.Dy())
		if w <= 0 || h <= 0 {
			continue
		}
		lbl := n.Label
		if lbl == "" {
			lbl = shortType(n.Type)
		}
		fmt.Fprintf(&overlay,
			`<div class="node" data-i="%d" style="left:%dpx;top:%dpx;width:%dpx;height:%dpx;z-index:%d" title="%s %s"><span>%s</span></div>`,
			i, int(n.Rect.Min.X), int(n.Rect.Min.Y), w, h, n.Depth+1,
			html.EscapeString(shortType(n.Type)), html.EscapeString(n.Label), html.EscapeString(lbl))
	}
	overlay.WriteString("</div>")

	return inspectorPage(outline.String(), overlay.String(), len(nodes))
}

// shortType strips the package qualifier from a box type name
// ("*layoutbox.Flex" → "Flex").
func shortType(t string) string {
	if i := strings.LastIndexByte(t, '.'); i >= 0 {
		return t[i+1:]
	}
	return strings.TrimPrefix(t, "*")
}

func rectStr(n layoutbox.InspectNode) string {
	return fmt.Sprintf(`<span class="rect">%d,%d %d×%d</span>`,
		int(n.Rect.Min.X), int(n.Rect.Min.Y), int(n.Rect.Dx()), int(n.Rect.Dy()))
}

func inspectorPage(outline, overlay string, count int) string {
	const css = `
*{box-sizing:border-box}
body{margin:0;font:13px/1.5 ui-monospace,Menlo,Consolas,monospace;background:#0d1117;color:#c9d1d9}
header{padding:10px 16px;background:#161b22;border-bottom:1px solid #30363d;font-weight:600}
.wrap{display:flex;align-items:flex-start}
.pane{padding:10px 14px;height:calc(100vh - 42px);overflow:auto}
.tree{flex:0 0 44%;border-right:1px solid #30363d}
.view{flex:1;background:#010409}
.row{white-space:nowrap;border-radius:4px;cursor:default}
.row:hover,.row.hot{background:#1f6feb22}
.type{color:#79c0ff;font-weight:600}
.role{color:#d29922;margin-left:6px}
.label{color:#7ee787}
.rect{color:#6e7681;font-size:11px;margin-left:8px}
.stage{position:relative;background:#161b22;border:1px solid #30363d;margin:4px}
.node{position:absolute;border:1px solid rgba(88,166,255,.35)}
.node:hover,.node.hot{border-color:#58a6ff;background:rgba(88,166,255,.10)}
.node>span{position:absolute;top:0;left:0;font-size:9px;color:#58a6ff;background:#0d1117cc;padding:0 2px;white-space:nowrap;max-width:100%;overflow:hidden}
`
	const js = `
// Link the outline and the overlay: hovering one highlights the other.
function link(sel){document.querySelectorAll(sel).forEach(function(el){
  el.addEventListener('mouseenter',function(){var i=el.dataset.i;
    document.querySelectorAll('[data-i="'+i+'"]').forEach(function(x){x.classList.add('hot')});});
  el.addEventListener('mouseleave',function(){var i=el.dataset.i;
    document.querySelectorAll('[data-i="'+i+'"]').forEach(function(x){x.classList.remove('hot')});});
});}
link('.row');link('.node');
`
	return `<!doctype html><meta charset="utf-8"><title>gophics inspector</title><style>` + css + `</style>` +
		fmt.Sprintf(`<header>gophics render-tree inspector — %d boxes</header>`, count) +
		`<div class="wrap"><div class="pane tree">` + outline + `</div><div class="pane view">` + overlay +
		`</div></div><script>` + js + `</script>`
}

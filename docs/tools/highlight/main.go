// Command highlight turns a Go source file into a static, syntax-highlighted
// HTML fragment (<pre class="code">…<span class="t-*">…</span>…</pre>), using
// only the standard library's go/scanner. No dependencies, no client-side JS —
// the coloring is baked into the HTML at build time and styled by docs/style.css.
//
//	go run ./docs/tools/highlight examples/physics/main.go > fragment.html
package main

import (
	"bytes"
	"fmt"
	"go/scanner"
	"go/token"
	"html"
	"os"
)

// builtin predeclared identifiers (types, constants, and builtin funcs) get the
// "type" color; everything else is a plain identifier unless it's a call.
var builtins = map[string]bool{
	"true": true, "false": true, "nil": true, "iota": true,
	"int": true, "int8": true, "int16": true, "int32": true, "int64": true,
	"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true, "uintptr": true,
	"float32": true, "float64": true, "complex64": true, "complex128": true,
	"string": true, "bool": true, "byte": true, "rune": true, "error": true, "any": true,
	"len": true, "cap": true, "make": true, "append": true, "new": true, "copy": true,
	"delete": true, "panic": true, "recover": true, "close": true, "print": true, "println": true,
	"min": true, "max": true, "clear": true, "complex": true, "real": true, "imag": true,
}

type tk struct {
	off int
	tok token.Token
	lit string
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: highlight <file.go>")
		os.Exit(2)
	}
	src, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fset := token.NewFileSet()
	f := fset.AddFile(os.Args[1], fset.Base(), len(src))
	var s scanner.Scanner
	s.Init(f, src, nil, scanner.ScanComments)

	var toks []tk
	for {
		pos, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}
		toks = append(toks, tk{off: f.Offset(pos), tok: tok, lit: lit})
	}

	var b bytes.Buffer
	b.WriteString(`<pre class="code"><code>`)
	last := 0
	for i, t := range toks {
		// Emit the raw gap (whitespace/newlines) before this token.
		if t.off > last {
			b.WriteString(html.EscapeString(string(src[last:t.off])))
			last = t.off
		}
		// Auto-inserted semicolons carry a "\n" literal and no source text;
		// leave the newline to the next gap.
		if t.tok == token.SEMICOLON && t.lit == "\n" {
			continue
		}
		n := len(t.lit)
		if n == 0 {
			n = len(t.tok.String())
		}
		end := min(t.off+n, len(src))
		text := html.EscapeString(string(src[t.off:end]))
		if cls := classOf(t, toks, i); cls != "" {
			fmt.Fprintf(&b, `<span class="%s">%s</span>`, cls, text)
		} else {
			b.WriteString(text)
		}
		last = end
	}
	if last < len(src) {
		b.WriteString(html.EscapeString(string(src[last:])))
	}
	b.WriteString("</code></pre>\n")
	os.Stdout.Write(b.Bytes())
}

func classOf(t tk, toks []tk, i int) string {
	switch {
	case t.tok == token.COMMENT:
		return "t-com"
	case t.tok == token.STRING || t.tok == token.CHAR:
		return "t-str"
	case t.tok == token.INT || t.tok == token.FLOAT || t.tok == token.IMAG:
		return "t-num"
	case t.tok.IsKeyword():
		return "t-kw"
	case t.tok == token.IDENT:
		if builtins[t.lit] {
			return "t-typ"
		}
		if i+1 < len(toks) && toks[i+1].tok == token.LPAREN {
			return "t-fn" // identifier immediately followed by '(' → a call
		}
		return ""
	}
	return ""
}

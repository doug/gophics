// Command webfonts is the plain probe plus UseWebFonts, and is the other half
// of TestWebFontMechanismIsNotLinkedWhenUnused. Without it the test could pass
// by looking for symbol names that no build ever produces, which would make it
// a test of its own spelling.
package main

import (
	"context"
	"os"

	"github.com/doug/gophics/text"
)

func main() {
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	f, err := text.Parse(data)
	if err != nil {
		panic(err)
	}
	sh := text.NewShaper(f)
	src := text.FetchFontSource(map[text.Script]string{
		text.Script("Hani"): "/fonts/han.ttf",
	})
	if err := sh.UseWebFonts(context.Background(), src, nil); err != nil {
		panic(err)
	}
	println(int(sh.Line(os.Args[2], 16).Width))
}

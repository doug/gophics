// Command tally is a native, local-first personal-finance app: your data is a
// plain-text beancount file, the bundled Apache-2.0 engine does the accounting,
// and gophics draws the UI — one Go codebase for desktop, web and mobile,
// testable headlessly with `go test`.
package main

import (
	"log"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/theme"

	"github.com/doug/tally/ui"
)

func main() {
	err := app.Run(ui.App{}, app.Config{
		Title:      "Tally",
		AppID:      "com.gophics.tally",
		Size:       geom.Size{W: 1040, H: 680},
		Background: theme.Light().Bg,
		Font:       goregular.TTF,
		FontFamilies: map[string][]byte{
			theme.FontBold: gobold.TTF,
			"mono":         gomono.TTF,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}

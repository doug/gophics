// Command tally is a native, local-first personal-finance app: your data is a
// plain-text beancount file, the bundled Apache-2.0 engine does the accounting,
// and gophics draws the UI — one Go codebase for desktop, web and mobile,
// testable headlessly with `go test`.
package main

import (
	"log"

	"github.com/doug/gophics/app"

	"github.com/doug/tally/ui"
)

func main() {
	if err := app.Run(ui.Root(), ui.Config()); err != nil {
		log.Fatal(err)
	}
}

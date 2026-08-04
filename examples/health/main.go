// Command health runs the health-dashboard showcase (package healthui) on the
// desktop and web with the synthetic live provider. iOS and Android instead use
// the gomobile bind in ./mobile, which injects a HealthKit / Health Connect
// provider into the same widget tree — see design/health-native-providers.md.
//
//	go run ./examples/health
package main

import (
	"log"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	healthui "github.com/doug/gophics/examples/health/ui"
	"github.com/doug/gophics/geom"
)

func main() {
	if err := app.Run(healthui.App{}, app.Config{
		Title:      "Health",
		Size:       geom.Size{W: 390, H: 760}, // phone-portrait, signalling the mobile target
		Background: healthui.BG,
		Font:       goregular.TTF,
	}); err != nil {
		log.Fatal(err)
	}
}

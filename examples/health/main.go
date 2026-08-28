// Command health runs the health-dashboard showcase (package healthui) on the
// desktop and web with the synthetic live provider. iOS and Android instead use
// the gomobile bind in ./mobile, which injects a HealthKit / Health Connect
// provider into the same widget tree — see design/health-native-providers.md.
//
//	go run ./examples/health
package main

import (
	"log"

	"github.com/doug/gophics/app"
	healthui "github.com/doug/gophics/examples/health/ui"
)

func main() {
	if err := app.Run(healthui.Root(), healthui.Config()); err != nil {
		log.Fatal(err)
	}
}

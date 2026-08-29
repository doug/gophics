# Embedding gophics in an Ebiten game

A gophics UI as a translucent HUD over a live Ebiten game. It exists to specify
the embedding seam by using it: everything here is something a real embedder has
to write, and anything gophics failed to provide would have shown up as a gap in
this file rather than in someone's app.

    go run .

## The seam

| Piece | What the host does |
|---|---|
| `app.NewHandler` | returns a `shell.Handler`; the host calls `Frame` per tick and `Event` per input |
| `shell.Window` | implemented by the host — clipboard, dark mode, title, invalidation |
| `shell.Frame` | reports size and scale, and returns the presentation target |
| `shell.PixelTarget` | receives the finished frame **and a damage rect** |

`Config.Transparent` is what makes it an overlay rather than a window: it keeps
the background translucent, and turns off surface retention to pay for it — a
blended background replayed over retained pixels would ghost the previous frame.

## Three things worth copying

**The UI does not import Ebiten.** `overlay/` is a widget tree and a `Model`
interface; the host implements `Model`. That is not tidiness — Ebiten's package
`init` opens a window, so a UI that imports it cannot be tested headlessly at
all. Splitting it is what lets `go test ./overlay` assert the overlay renders,
carries damage, and stays translucent.

**Input is the bulk of the work.** `input.go` diffs Ebiten's polled state into
`shell.Pointer`, `shell.Key` and `shell.Text` every tick, including scroll,
modifiers, key release and committed text. A host that forwarded only clicks
would look like it worked and then have no text input and no keyboard focus.

**A live readout needs a ticker.** gophics rebuilds when *its* state changes. A
panel showing the host's clock changes when the host says so, and nothing in the
widget tree knows that happened — so `uiState` registers a ticker. Without it
the panel renders once and the elapsed time never moves, which looks like a
broken UI and is really a missing subscription. It costs a rebuild per frame; a
panel of settings would not need it.

## Why there is no GPU seam

An overlay host would rather composite gophics into its own GPU frame, and it
cannot: two Go WebGPU bindings cannot exchange a `Device` through Go types, so
gophics cannot accept Ebiten's. This is not a visibility problem, and publishing
the vendored substrate would not fix it. The CPU path is the supported seam, and
the damage rect is what keeps it affordable — gophics redraws only the changed
region, and `Put` now says which region that was.

# news — a personal RSS reader

One queue of everything you follow, ordered by what you actually read, with full
articles and pictures kept on the device so it works with no connection.

No server, no account, no sync. Everything — subscriptions, articles, pictures,
and what the ranking has learned about you — lives in one directory on the
phone.

```
go run ./examples/news                    # desktop
gophics run -p android ./examples/news/mobile
gophics run -p ios ./examples/news/mobile
```

## What it does

**One ranked queue.** Every unread article from every source, best first. A row
carries only what decides whether to open it — headline, teaser, source, age,
reading time, a picture — plus a bar down the left showing how likely the app
thinks you are to want it. Swipe a row away and that counts as a judgement too.

**Full articles, not headlines.** Feeds that ship only a teaser have their
article pages fetched and reduced to the text: the body markup is turned into
laid-out blocks — headings, pull quotes, lists, code, figures with captions —
at a reading size you set from the reader's own title bar.

**Ranking you can argue with.** Hold any row to see why it is where it is: the
score breaks into named contributions (source, topic, author, length,
freshness), each drawn as a bar, with the two buttons that change them.

**146 sources built in.** A verified catalog across ten categories, each entry
recording what it publishes and whether its feed carries full text. Or paste the
address of any site and the app finds its feed and previews it before you
subscribe.

**Paid subscriptions.** For publishers that gate their articles, sign in through
a web view and the reader fetches the full text using your own session — the
same way your browser does.

**Offline.** A refresh downloads article text and images, so a queue prepared on
wifi reads underground.

## How it is put together

```
main.go            desktop / web entry point
ui/                the widget tree — queue, reader, sources, settings
mobile/            gomobile-bind surface for Android and iOS
android/  ios/     thin platform hosts (surface, vsync, input, sign-in web view)
internal/
  library/         the app: subscriptions, refresh, offline cache, cookies
  rank/            the scoring model
  catalog/ feed/ fetch/ extract/ store/ pick/ cookies/
                   the fetching pipeline, ported from the rss command line tool
```

The pipeline packages know nothing about an app: they parse feeds, fetch
politely with conditional GETs, reduce article pages to text, and store the
result as a directory of JSON. `library` is everything phone-shaped on top —
a data directory handed in by the host, a refresh that reports progress, a
picture cache, captured sessions. `ui` talks only to `library`.

### Ranking

A naive Bayes model over four kinds of evidence — the feed, the category, the
author, and the words in the title and summary — combined in log-odds with a
length preference and a recency term. It learns from what you do: opening,
finishing, skipping past, swiping away, and the two explicit buttons.

Two properties mattered more than accuracy:

*Cold start has to be sensible.* With no history the score falls back to the
catalog's own editorial rating, so the first queue is ordered by must-read and
freshness rather than by noise. Learned evidence displaces that prior in
proportion to how much of it there is.

*Nothing may run away.* Every feature's contribution is shrunk toward zero by
how little evidence supports it, then clamped, so one enthusiastic evening
cannot convince the reader that a single blog is all you want.

`Explain` returns the terms behind any score, which is what the "why this?"
screen shows. A ranking you cannot interrogate is one you end up fighting.

### Reading a paid subscription

Publishers worth paying for are the ones whose feeds carry a teaser and gate the
body. Sending your own session cookie with the article fetch is what makes those
entries readable, and it is exactly what a browser does when you click the link.

gophics cannot present the login itself: `shell.WebView` is implemented for the
web shell only and exposes no cookie access. So the login happens in the host's
own view — Android `WebView` + `CookieManager`, iOS `WKWebView` +
`WKHTTPCookieStore` — and the session comes back over the bind surface
(`mobile/login.go`). Desktop builds, which have no host web view, paste the
`Cookie` header out of a browser's network panel instead.

Every cookie for the site is captured, never a hand-picked one: which cookie
carries the session is undocumented and changes. They are written `0600` under
the data directory and sent only to the domain they came from.

Settings offers a check that answers the real question — it fetches one article
twice, with and without the session, and compares how much text came back.

## Where data lives

Desktop uses the platform config directory. Mobile hosts must call
`SetDataDir` before `Start`: Android's app-private storage is only known to the
Java side, and the iOS sandbox path changes between installs.

```
subscriptions.json   your feed list, seeded on first run from the catalog
settings.json        reading size, prefetch, refresh-on-open
ranking.json         everything the model has learned
store/               articles, one directory of JSON per day
images/              the offline picture cache (capped, pruned oldest-first)
cookies/             one file per publisher you have signed in to
```

## Notes for anyone editing this

Three bugs found while building it are worth knowing about, because the code
guards against them and the guards look arbitrary otherwise.

**A horizontal `Scroll` inside a `Column` measures as infinitely tall.** Its
scrollbar overlay lays out at `cs.Constrain(cs.Max)` to fill the scroll area
(`widget/basic.go`, `scrollbarBox.Layout`), and a Column gives an unbounded
cross axis. Everything below then lands at `y=+Inf` and is not drawn, while the
content above looks perfect. `chipBar` bounds the height; code blocks state
theirs. This is a framework limitation, not a local style choice.

**Missing glyphs draw as empty boxes, silently.** The Go fonts have no gear, no
bullseye, no check mark and no north-east arrow; the first tab bar built here
was three tofu squares. `glyphs_test.go` reads this package's own source and
asks the fonts whether they can draw every character in its string literals.

**An impression is not a frame.** The queue's row builder is the only place that
knows what is on screen, and it runs on every frame. Counting each call pushed
articles past the "ignored it" threshold while the reader was still looking at
them. `library.Impression` counts one per run of the app, and the tally is
persisted, since it is meant to accumulate across runs.

## Tests

```sh
go test ./examples/news/...
```

`ui` tests drive the real widget tree through `app.NewHeadless`: they tap rows,
open articles, and assert on the accessibility tree — including the laid-out
rectangles, because a widget with no width still reports its label, and
asserting on labels alone cannot see a blank screen.

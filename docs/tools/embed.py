"""Rewrite the <!-- generated:<path> --> spans in a docs page from source.

Called by docs/build-embeds.sh; see that script for the why. Kept in Python
rather than Go because it is a build step for the site, not part of the
library, and the highlighter it shells out to is already the Go tool.

A marker names a file, and may name a slice of it:

    <!-- generated:examples/counter/main.go -->
    <!-- generated:examples/counter/main.go from="// Counter is" to="func main" -->

from is the first line containing that text (inclusive); to is the first line
after it containing that text (exclusive). Anchors rather than line numbers,
because a line number keeps matching after the file changes and shows the
wrong lines while looking fresh; an anchor that no longer matches fails the
build. The home-page hero is a slice: the program without its imports and
main, which was a hand-pasted copy until it drifted to a field that no longer
existed.
"""

import os
import re
import subprocess
import sys
import tempfile

# utf-8 explicitly: the pages carry — → ▶, and the default is the locale's
# encoding, which on Windows is not utf-8 before Python 3.15.
page, check = sys.argv[1], sys.argv[2] == "1"
text = open(page, encoding="utf-8").read()

# The body may be empty — a freshly-placed marker pair has nothing between it
# yet. Requiring content there meant an empty pair matched nothing, the embed
# was never filled in, and -check passed because nothing looked stale.
SPAN = re.compile(
    r'( *)<!-- generated:([^ ]+)((?: \w+="[^"]*")*) -->\n(?:.*?\n)?? *<!-- /generated -->',
    re.S,
)
ATTR = re.compile(r'(\w+)="([^"]*)"')

stale = []


def fail(msg):
    print("{}: {}".format(page, msg), file=sys.stderr)
    sys.exit(1)


def slice_source(src, start, end):
    """Return the lines of src from the first containing start through the
    line before the first (later) line containing end, trailing blank lines
    dropped."""
    lines = open(src, encoding="utf-8").read().split("\n")
    first = next((i for i, l in enumerate(lines) if start in l), None)
    if first is None:
        fail("{}: from={!r} matches no line".format(src, start))
    last = next((i for i, l in enumerate(lines) if i > first and end in l), None)
    if last is None:
        fail("{}: to={!r} matches no line after from".format(src, end))
    body = lines[first:last]
    while body and not body[-1].strip():
        body.pop()
    return "\n".join(body) + "\n"


def highlight(path):
    return subprocess.run(
        ["go", "run", "./docs/tools/highlight", path],
        capture_output=True, text=True, check=True, encoding="utf-8",
    ).stdout.rstrip("\n")


def rewrite(m):
    indent, src, attrs = m.group(1), m.group(2), m.group(3)
    opts = dict(ATTR.findall(attrs))
    if set(opts) - {"from", "to"}:
        fail("{}: unknown marker attribute in {!r}".format(src, attrs))
    if ("from" in opts) != ("to" in opts):
        fail("{}: from= and to= go together".format(src))
    if "from" in opts:
        # The highlighter takes a file, so the slice goes through one.
        tmp = tempfile.NamedTemporaryFile(
            "w", suffix=".go", encoding="utf-8", delete=False
        )
        try:
            tmp.write(slice_source(src, opts["from"], opts["to"]))
            tmp.close()
            hl = highlight(tmp.name)
        finally:
            os.unlink(tmp.name)
    else:
        hl = highlight(src)
    new = "{i}<!-- generated:{s}{a} -->\n{i}{h}\n{i}<!-- /generated -->".format(
        i=indent, s=src, a=attrs, h=hl
    )
    if new != m.group(0):
        stale.append(src)
    return new


out, subs = SPAN.subn(rewrite, text)

# Every marker must have been rewritten. A pair the pattern cannot match would
# otherwise be skipped in silence, which is how an empty embed shipped.
markers = text.count("<!-- generated:")
if subs != markers:
    fail(
        "{} generated markers but {} matched — check the marker syntax".format(
            markers, subs
        )
    )

if not stale:
    sys.exit(0)
if check:
    print(
        "stale embed in {}: {} — run ./docs/build-embeds.sh".format(
            page, ", ".join(stale)
        ),
        file=sys.stderr,
    )
    sys.exit(1)
open(page, "w", encoding="utf-8").write(out)
print("refreshed {} ({})".format(page, ", ".join(stale)))

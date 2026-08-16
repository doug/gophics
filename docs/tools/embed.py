"""Rewrite the <!-- generated:<path> --> spans in a docs page from source.

Called by docs/build-embeds.sh; see that script for the why. Kept in Python
rather than Go because it is a build step for the site, not part of the
library, and the highlighter it shells out to is already the Go tool.
"""

import re
import subprocess
import sys

page, check = sys.argv[1], sys.argv[2] == "1"
text = open(page).read()

# The body may be empty — a freshly-placed marker pair has nothing between it
# yet. Requiring content there meant an empty pair matched nothing, the embed
# was never filled in, and -check passed because nothing looked stale.
SPAN = re.compile(
    r"( *)<!-- generated:([^ ]+) -->\n(?:.*?\n)?? *<!-- /generated -->", re.S
)

stale = []


def rewrite(m):
    indent, src = m.group(1), m.group(2)
    hl = subprocess.run(
        ["go", "run", "./docs/tools/highlight", src],
        capture_output=True, text=True, check=True,
    ).stdout.rstrip("\n")
    new = "{i}<!-- generated:{s} -->\n{i}{h}\n{i}<!-- /generated -->".format(
        i=indent, s=src, h=hl
    )
    if new != m.group(0):
        stale.append(src)
    return new


out, subs = SPAN.subn(rewrite, text)

# Every marker must have been rewritten. A pair the pattern cannot match would
# otherwise be skipped in silence, which is how an empty embed shipped.
markers = text.count("<!-- generated:")
if subs != markers:
    print(
        "{}: {} generated markers but {} matched — check the marker syntax".format(
            page, markers, subs
        ),
        file=sys.stderr,
    )
    sys.exit(1)

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
open(page, "w").write(out)
print("refreshed {} ({})".format(page, ", ".join(stale)))

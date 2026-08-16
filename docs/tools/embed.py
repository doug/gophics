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

SPAN = re.compile(
    r"( *)<!-- generated:([^ ]+) -->\n.*?\n *<!-- /generated -->", re.S
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


out = SPAN.sub(rewrite, text)

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

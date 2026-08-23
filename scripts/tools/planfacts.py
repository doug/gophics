"""Derive the factual sections of PLAN.md from the tree.

PLAN.md holds two kinds of content, and only one of them drifts.

The decisions — principles, architecture, the API rationale, the risk register
— record *why*, and stay true regardless of what the code does next. The
inventories record *what exists*, and they went stale six times in one week:
native menus and mobile lifecycle were listed as remaining after they shipped;
tree views were listed as missing while widget/tree.go sat in the repo; the
repo layout named a package that no longer exists and missed one added the same
day; and the accessibility entry claimed macOS announcements were a no-op after
they were implemented.

An inventory maintained by hand beside the thing it inventories is a promise to
notice, and nobody does. So the inventories are generated here and the prose
keeps the judgment. Same contract as docs/build-embeds.sh, which exists for the
same reason one layer down:

    python3 scripts/tools/planfacts.py           # rewrite the spans
    python3 scripts/tools/planfacts.py -check    # exit 1 if any is stale

What this deliberately does NOT generate is priority. "The GPU vector backend
matters more than tree views" is a judgment about direction that no amount of
reading the tree will produce, and generating a confident-looking ordering from
file counts would be worse than writing three honest sentences.
"""

import os
import re
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
os.chdir(ROOT)

PLAN = "PLAN.md"
SHELLS = ["desktop", "web", "mobile", "terminal"]


def read(path):
    with open(path, encoding="utf-8") as f:
        return f.read()


def go_files(pkg):
    """Non-test .go files directly in pkg."""
    try:
        names = sorted(os.listdir(pkg))
    except FileNotFoundError:
        return []
    return [
        os.path.join(pkg, n)
        for n in names
        if n.endswith(".go") and not n.endswith("_test.go")
    ]


# --- capabilities -------------------------------------------------------------

CAP_ACCESSOR = re.compile(r"\)\s+([A-Z]\w*)\(\)\s+shell\.\1\b")


def capabilities():
    """Every capability Ctx publishes, in declaration order."""
    text = read("widget/capabilities_gen.go")
    return re.findall(r"func \(c Ctx\) (\w+)\(\) shell\.\w+", text)


# An accessor whose whole body is `return nil` (comments aside). The capability
# is declared and always absent, which from a caller's side is the same as not
# being there at all — ctx.<Cap>() is nil either way. Counting it as present
# would put "yes" beside mobile Battery, which never returns one.
ALWAYS_NIL = re.compile(r"\{\s*(?://[^\n]*\n\s*)*return nil\s*\}")


def accessor_body(src, end):
    """The accessor body starting at the brace after its signature."""
    i = src.find("{", end)
    if i < 0:
        return ""
    depth, j = 0, i
    while j < len(src):
        if src[j] == "{":
            depth += 1
        elif src[j] == "}":
            depth -= 1
            if depth == 0:
                return src[i:j + 1]
        j += 1
    return ""


def shell_caps(shell):
    """Capabilities a shell publishes and can actually return."""
    found = set()
    for f in go_files(os.path.join("shell", shell)):
        src = read(f)
        for m in CAP_ACCESSOR.finditer(src):
            if ALWAYS_NIL.fullmatch(accessor_body(src, m.end()).strip()):
                continue
            found.add(m.group(1))
    return found


def hollow_accessors():
    """Capabilities whose every implementation answers and means nothing.

    This is the failure the nil convention exists to prevent: a caller's nil
    check passes, the calls succeed, and nothing happens — indistinguishable
    from "no device attached".

    Every implementation, because a capability is usually written once per
    operating system behind a build tag. Desktop gamepads are real on macOS,
    Linux and Windows and a stub on the BSDs; calling that hollow would be a
    louder lie than the one it is trying to catch. A per-OS gap belongs in the
    prose, not in a per-shell column.
    """
    hollow = []
    for shell in SHELLS:
        impls = {}
        for f in go_files(os.path.join("shell", shell)):
            src = read(f)
            for m in CAP_ACCESSOR.finditer(src):
                cap = m.group(1)
                body = accessor_body(src, m.end()).strip()
                if ALWAYS_NIL.fullmatch(body):
                    continue  # absent, not hollow — shell_caps drops it
                impls.setdefault(cap, []).append(returns_nothing(src, m.end()))
        for cap, verdicts in impls.items():
            if verdicts and all(verdicts):
                hollow.append((shell, cap))
    return hollow


def returns_nothing(src, end):
    """Whether the methods defined after an accessor all just return nil.

    Shallow by design: it flags the shape so a human looks, rather than trying
    to decide what a platform call does.
    """
    body = src[end:]
    methods = re.findall(r"\nfunc \([^)]+\) \w+\([^)]*\)[^{]*\{([^}]*)\}", body)
    if not methods:
        return False
    return all(
        re.fullmatch(r"\s*(//[^\n]*\n\s*)*return nil\s*", b) for b in methods
    )


def capability_table():
    caps = capabilities()
    per = {s: shell_caps(s) for s in SHELLS}
    hollow = set(hollow_accessors())

    head = "| Capability | " + " | ".join(SHELLS) + " |"
    rule = "|---" * (len(SHELLS) + 1) + "|"
    rows = [head, rule]
    for c in caps:
        cells = []
        for s in SHELLS:
            if (s, c) in hollow:
                cells.append("hollow")
            elif c in per[s]:
                cells.append("yes")
            else:
                cells.append("—")
        rows.append("| `%s` | %s |" % (c, " | ".join(cells)))

    note = (
        "\n*Generated. `yes` means the shell publishes the accessor; `—` means "
        "`ctx.<Cap>()` is nil there, which is how an app is meant to ask. "
        "`hollow` means it returns a value whose methods do nothing — the one "
        "shape a caller cannot detect.*"
    )
    return "\n".join(rows) + "\n" + note


# --- repo layout --------------------------------------------------------------

SKIP_DIRS = {".git", ".github", "build", "docs", "scripts", "design", "skills", "testdata"}


# Directories that hold subpackages rather than a package of their own, so
# there is no doc comment to read. Described by what they contain; the counts
# beside them are still derived.
CONTAINERS = {
    "cmd": "Developer CLI: build, run and hot-restart across every target",
    "examples": "Example apps, from hello to a beancount ledger",
    "internal": "Vendored GPU substrate (wgpu, naga, windowing) and internals",
}

# A package summary is a signpost, not documentation. Past this the table stops
# being scannable, which is the only thing it is for.
MAX_PURPOSE = 88


def trim(text):
    """First clause, short enough to read in a table cell."""
    if len(text) <= MAX_PURPOSE:
        return text
    cut = text[:MAX_PURPOSE]
    for sep in (": ", " — ", ", ", " "):
        i = cut.rfind(sep)
        if i > MAX_PURPOSE // 2:
            return cut[:i].rstrip(" ,:—")
    return cut.rstrip()


def package_doc(pkg):
    """The one-line summary from the package's doc comment, if it has one."""
    if pkg in CONTAINERS:
        return CONTAINERS[pkg]
    for f in go_files(pkg):
        src = read(f)
        m = re.search(r"((?:^// ?.*\n)+)^package \w+", src, re.M)
        if not m:
            continue
        lines = [l.lstrip("/ ").rstrip() for l in m.group(1).strip().split("\n")]
        text = " ".join(l for l in lines if l)
        if not text:
            continue
        # First sentence, and drop the conventional "Package x " opener.
        text = re.sub(r"^Package \w+ (is |provides |implements )?", "", text)
        first = re.split(r"(?<=[.!?]) ", text)[0].rstrip(".")
        if first:
            return trim(first[0].upper() + first[1:])
    return ""


def layout_table():
    dirs = sorted(
        d
        for d in os.listdir(".")
        if os.path.isdir(d) and not d.startswith(".") and d not in SKIP_DIRS
    )
    rows = ["| Package | Purpose |", "|---|---|"]
    for d in dirs:
        rows.append("| `%s/` | %s |" % (d, package_doc(d)))
    return "\n".join(rows) + "\n\n*Generated from each package's doc comment.*"


# --- widget catalog -----------------------------------------------------------


def widget_catalog():
    """Public widget types, so a "missing widget" claim has to survive the tree."""
    names = set()
    for f in go_files("widget"):
        names.update(re.findall(r"^type ([A-Z]\w*) struct", read(f), re.M))
    # Infrastructure rather than things an app composes.
    infra = {
        "Owner", "BuildError", "SerializedWidget", "InteractiveBox", "Ctx",
        "StateBase", "Handler", "DragSession", "OverlayToken", "Capabilities",
        "WithKey", "Flex",
    }
    public = sorted(n for n in names if n not in infra)
    out, line = [], ""
    for n in public:
        add = ("`%s`" % n) if not line else (", `%s`" % n)
        if len(line) + len(add) > 76:
            out.append(line)
            line = "`%s`" % n
        else:
            line += add
    if line:
        out.append(line)
    return "\n".join(out) + "\n\n*Generated: every exported widget type.*"


# --- rewrite ------------------------------------------------------------------

BLOCKS = {
    "capabilities": capability_table,
    "layout": layout_table,
    "widgets": widget_catalog,
}

SPAN = re.compile(
    r"<!-- planfacts:(\w+) -->\n(?:.*?\n)??<!-- /planfacts -->", re.S
)


def main():
    check = "-check" in sys.argv
    text = read(PLAN)
    stale = []
    missing = []

    def rewrite(m):
        name = m.group(1)
        fn = BLOCKS.get(name)
        if fn is None:
            missing.append(name)
            return m.group(0)
        new = "<!-- planfacts:%s -->\n%s\n<!-- /planfacts -->" % (name, fn())
        if new != m.group(0):
            stale.append(name)
        return new

    out, n = SPAN.subn(rewrite, text)

    if missing:
        print("planfacts: unknown block(s): %s" % ", ".join(missing), file=sys.stderr)
        return 1
    if n != len(BLOCKS):
        print(
            "planfacts: %s has %d marker pairs, expected %d (%s)"
            % (PLAN, n, len(BLOCKS), ", ".join(sorted(BLOCKS))),
            file=sys.stderr,
        )
        return 1

    if check:
        if stale:
            print(
                "planfacts: stale in %s: %s" % (PLAN, ", ".join(stale)),
                file=sys.stderr,
            )
            return 1
        return 0

    if out != text:
        with open(PLAN, "w", encoding="utf-8") as f:
            f.write(out)
        print("planfacts: rewrote %s" % ", ".join(stale))
    return 0


if __name__ == "__main__":
    sys.exit(main())

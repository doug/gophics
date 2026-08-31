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
import subprocess
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
os.chdir(ROOT)

# PLAN.md is not in this repo: the planning material lives in a separate
# private tree. This tool still maintains its generated inventories when that
# tree is present, because the alternative is a plan whose tables rot — which
# is the failure this whole script exists to prevent. Point GOPHICS_PLAN at it,
# or keep it checked out beside this repo as ../gophics-plans.
#
# The capability matrix is different: it is public documentation, always
# generated, and its absence is an error rather than a skip.
CAPDOC = "internal/capgen/README.md"


def plan_path():
    """The plan file to maintain, or None when the planning tree is absent."""
    env = os.environ.get("GOPHICS_PLAN")
    if env:
        return env if os.path.exists(env) else None
    for cand in ("PLAN.md", os.path.join("..", "gophics-plans", "PLAN.md")):
        if os.path.exists(cand):
            return cand
    return None


PLAN = plan_path()
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

# An accessor that hands off to another package: its implementation is not in
# this file, so nothing here can say whether it does anything.
DELEGATES = re.compile(r"return\s+\w+\.\w+\(")


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
    """Whether the methods implementing an accessor all just return nil.

    Shallow by design: it flags the shape so a human looks, rather than trying
    to decide what a platform call does. Two limits keep the shallowness from
    turning into a false accusation, both learned the hard way.

    It stops at the next accessor. Scanning to end-of-file meant an unrelated
    stub further down the file decided the verdict, so moving a function
    changed a documented fact — the tool reported terminal Microphone as
    hollow because two nil-returning camera accessors happened to sit below it.

    It declines to judge a delegating accessor. `return devmedia.Microphone()`
    is implemented in another package this tool does not read, and "I cannot
    see it" must not print as "it does nothing" — that is the same lie the
    check exists to catch, told about working code.
    """
    body = src[end:]
    nxt = CAP_ACCESSOR.search(body)
    if nxt:
        body = body[: nxt.start()]
    if DELEGATES.search(accessor_body(src, end)):
        return False
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



# --- native device backends ---------------------------------------------------

# The per-shell capability matrix has one column per shell, which is the right
# grain for "can an app use this here" and the wrong one for "on which
# operating system". A desktop app is built for one OS at a time, and a column
# reading "yes" because two of the three have a backend is the kind of true
# statement that misleads.
#
# So this second table asks the compiler rather than the tree. `go list` with
# GOOS set reports exactly which files that build would include, which is the
# same answer the build itself gets — no build-tag expression is re-implemented
# here, and no regex decides what a tag means. Adding a backend file changes
# this table; the gate makes it change in the same commit.

NATIVE_OS = ["darwin", "linux", "windows"]

# A file whose name matches one of these is a placeholder, not a backend: it
# exists so the package compiles everywhere and does nothing when called.
#
# Matched on the suffix alone, and deliberately not on "_default_": the first
# version of this list excluded driver_default_windows.go as boilerplate, and
# the table then reported Windows as having no speakers — hours after a
# hardware test had played audio there. It is the real WASAPI driver.
STUBS = ("_other.go", "_null.go")

DEVICES = [
    ("Camera", "internal/camera", "capture_"),
    ("Microphone / recording", "internal/audio", "capture_"),
    ("Speakers", "internal/audio", "driver_"),
]


def goos_files(pkg, goos):
    """The files `go build` would include for pkg on goos, per the toolchain."""
    env = dict(os.environ, GOOS=goos, GOARCH="amd64" if goos != "darwin" else "arm64")
    out = subprocess.run(
        ["go", "list", "-f", "{{range .GoFiles}}{{.}}\n{{end}}", "./" + pkg],
        capture_output=True, text=True, env=env,
    )
    if out.returncode != 0:
        raise SystemExit("planfacts: go list %s for %s failed:\n%s" % (pkg, goos, out.stderr))
    return [l.strip() for l in out.stdout.splitlines() if l.strip()]


def device_table():
    rows = ["| Device | " + " | ".join(NATIVE_OS) + " |",
            "|---" * (len(NATIVE_OS) + 1) + "|"]
    for label, pkg, prefix in DEVICES:
        cells = []
        for goos in NATIVE_OS:
            real = [
                f for f in goos_files(pkg, goos)
                if f.startswith(prefix) and not f.endswith(STUBS)
            ]
            cells.append(", ".join(sorted(real)) if real else "—")
        rows.append("| `%s` | %s |" % (label, " | ".join(cells)))
    rows.append("")
    rows.append("*Generated from `go list` per GOOS: the file each build actually "
                "compiles. `—` means no backend, so the capability is nil there "
                "even where the shell column above reads `yes`.*")
    return "\n".join(rows)


# --- rewrite ------------------------------------------------------------------

BLOCKS = {
    "capabilities": capability_table,
    "devices": device_table,
    "layout": layout_table,
    "widgets": widget_catalog,
}

# Which generated spans each file is expected to carry.
#
# internal/capgen/README.md keeps its own prose notes per capability — why the web
# implementation folds mic bands, why files cross as bytes — and only borrows
# the status matrix. That table had drifted from the code in both directions:
# it called seven mobile capabilities "stub" when the Bridge had no method at
# all, and called TextInput and Accessibility unimplemented on mobile while the
# keyboard and the AT tree were working. Prose is judgment and stays by hand;
# status is a fact about the tree and is generated.
FILES = {CAPDOC: {"capabilities"}}
if PLAN:
    FILES[PLAN] = {"capabilities", "devices", "layout", "widgets"}

SPAN = re.compile(
    r"<!-- planfacts:(\w+) -->\n(?:.*?\n)??<!-- /planfacts -->", re.S
)


def rewrite_file(path, want):
    """Rewrite path's generated spans. Returns (text, new_text, stale, error)."""
    text = read(path)
    stale, missing, seen = [], [], set()

    def rewrite(m):
        name = m.group(1)
        fn = BLOCKS.get(name)
        if fn is None:
            missing.append(name)
            return m.group(0)
        seen.add(name)
        new = "<!-- planfacts:%s -->\n%s\n<!-- /planfacts -->" % (name, fn())
        if new != m.group(0):
            stale.append(name)
        return new

    out, _ = SPAN.subn(rewrite, text)

    if missing:
        return text, out, stale, "unknown block(s): %s" % ", ".join(missing)
    if seen != want:
        return text, out, stale, "has spans %s, expected %s" % (
            ", ".join(sorted(seen)) or "none",
            ", ".join(sorted(want)),
        )
    return text, out, stale, None


def main():
    check = "-check" in sys.argv
    failed = False
    rewrote = []

    for path, want in FILES.items():
        text, out, stale, err = rewrite_file(path, want)
        if err:
            print("planfacts: %s %s" % (path, err), file=sys.stderr)
            failed = True
            continue
        if check:
            if stale:
                print(
                    "planfacts: stale in %s: %s" % (path, ", ".join(stale)),
                    file=sys.stderr,
                )
                failed = True
            continue
        if out != text:
            with open(path, "w", encoding="utf-8") as f:
                f.write(out)
            rewrote.append("%s (%s)" % (path, ", ".join(stale)))

    if rewrote:
        print("planfacts: rewrote %s" % "; ".join(rewrote))
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())

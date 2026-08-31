#!/usr/bin/env bash
# ci-local.sh — run the CI jobs on this machine, before pushing.
#
# CI could not start a job for eight days and nobody noticed, because the only
# signal was a red mark on a page nobody was looking at. When it came back, the
# first run found three failures, two of which had been sitting in main for a
# week: a test file missing a build tag so one job could not compile, and a
# golden image compared with zero tolerance across two operating systems.
# Neither is visible to `go test ./...` on a Mac. They are failures of
# configuration coverage rather than of logic, and this is how to see them
# without a round trip through GitHub.
#
#   ./scripts/ci-local.sh                  # every job this machine can run
#   ./scripts/ci-local.sh test-substrate   # one job, by name
#   ./scripts/ci-local.sh -l               # list jobs, steps, and where each runs
#
# Override the image with CI_LOCAL_IMAGE if CI moves to another Go version.
#
# Linux jobs run in podman on the Go version CI uses. macOS jobs run natively:
# `runs-on: macos-latest` means what it says and a container cannot stand in —
# the substrate job builds the Metal HAL.
#
# What this does NOT catch, which matters before trusting a green run:
#
#   - CPU architecture. The container is the host's — arm64 on Apple silicon —
#     while GitHub's Linux runners are amd64. The golden-image failure above was
#     4 pixels differing by 1/255 between two libms, and it does not reproduce
#     here: an arm64 Linux container agrees with arm64 macOS. Running the image
#     under --platform linux/amd64 does reproduce the arch, and is slow enough
#     (>10 minutes for one package) to be useless in practice.
#   - The macOS runner's OS version, Xcode, and available hardware. The GPU
#     tests self-skip without an adapter; this machine has one and CI's may not.
#
# So a green run here means the configuration coverage is right — build tags,
# GOOS/GOARCH, env, step conditions — which is what two of the three failures
# that prompted this script actually were. It does not promise CI is green.
#
# The commands come out of .github/workflows/ci.yml rather than being repeated
# here, which matters more than it sounds. This repo has twice shipped a bug
# where two copies of one command drifted apart: `gophics build` and
# package/android.sh disagreeing about a linker flag that only a device could
# reveal, and a reference host that diverged from the template generated from
# it. A local runner that restates CI's steps is that bug with a longer fuse,
# because it fails by passing.
set -uo pipefail
cd "$(dirname "$0")/.."

IMAGE="${CI_LOCAL_IMAGE:-docker.io/library/golang:1.27}"

# steps emits one record per step: label<TAB>runs-on<TAB>name<TAB>base64(script)
#
# base64 because run blocks are multi-line and contain line continuations.
# Joining them with && corrupted an apt invocation whose package list sat after
# a backslash, producing a step that "failed" for a reason existing only here.
steps() {
	python3 - "$@" <<'PY'
import base64, re, shlex, sys, yaml

with open(".github/workflows/ci.yml") as f:
    wf = yaml.safe_load(f)

want = sys.argv[1] if len(sys.argv) > 1 else ""
for job, spec in (wf.get("jobs") or {}).items():
    if want and job != want:
        continue
    runs = str(spec.get("runs-on", ""))

    # A matrix job is N jobs. Expanding it here, rather than running the steps
    # once with no matrix variables set, is the difference between building
    # four targets and building the host's twice — and the second looks like a
    # pass.
    entries = [None]
    if "matrix" in runs:
        entries = ((spec.get("strategy") or {}).get("matrix") or {}).get("include") or [None]

    job_env = spec.get("env") or {}
    for entry in entries:
        m = entry or {}
        label = job if not m.get("goos") else "%s(%s/%s)" % (job, m["goos"], m["goarch"])
        where = str(m.get("runner") or runs)
        mat_env = {"GOOS": m["goos"], "GOARCH": m["goarch"]} if m.get("goos") else {}

        def expand(v):
            """Resolve ${{ matrix.x }} against this entry. Anything else is left
            alone and will be visible in the output rather than silently wrong."""
            out = str(v)
            for k, val in m.items():
                out = out.replace("${{ matrix.%s }}" % k, str(val))
            return out

        for st in (spec.get("steps") or []):
            run = st.get("run")
            if not run:
                continue  # an action (checkout, setup-go); the local shell has these

            # Honour `if: matrix.x == 'y'`. Without this the runner executes
            # steps CI skips — the nogpu build is linux-only, and running it for
            # windows and js reports failures that CI never sees.
            cond = str(st.get("if", "")).strip()
            if cond:
                ok = True
                mm = re.match(r"matrix\.(\w+)\s*==\s*'([^']*)'", cond)
                if mm:
                    ok = str(m.get(mm.group(1), "")) == mm.group(2)
                elif cond not in ("success()", "always()"):
                    print("ci-local: unhandled `if: %s` on %s/%s — running it" %
                          (cond, label, st.get("name", "run")), file=sys.stderr)
                if not ok:
                    continue

            # env matters more than it looks. The Vulkan job sets
            # GOPHICS_REQUIRE_GPU=1 so "no adapter" fails instead of skipping;
            # dropping it makes the job pass by skipping the very thing it
            # exists to check. Values are expanded because a step's own env
            # holds `${{ matrix.goos }}`, which would otherwise override the
            # real value with the literal template text.
            env = {**job_env, **mat_env,
                   **{k: expand(v) for k, v in (st.get("env") or {}).items()}}
            prefix = "".join("export %s=%s\n" % (k, shlex.quote(str(v)))
                             for k, v in env.items())
            blob = base64.b64encode((prefix + run).encode()).decode()
            print("\t".join([label, where, st.get("name", "run"), blob]))
PY
}

case "${1:-}" in
-h | --help)
	sed -n '2,42p' "$0" | sed 's/^# \?//'
	exit 0
	;;
-l)
	printf '%-24s %-16s %s\n' JOB RUNS-ON STEP
	steps | awk -F'\t' '{printf "%-24s %-16s %s\n", $1, $2, $3}'
	exit 0
	;;
esac

command -v podman >/dev/null || echo "podman not found — Linux jobs will be skipped." >&2

host_os=$(uname -s)
only="${1:-}"
failed=0
ran=0
skipped=0

# GitHub runs each block as `bash -e {0}`, so a failing line aborts the step.
# Reproducing that matters: without -e, a step whose first command fails still
# reports the exit status of its last one.
run_native() {
	printf '\n\033[1m== %s / %s\033[0m (native)\n' "$1" "$2"
	if ! printf '%s' "$3" | base64 -d | bash -e; then
		failed=1
		echo "FAILED: $1 / $2" >&2
	fi
}

run_linux() {
	printf '\n\033[1m== %s / %s\033[0m (podman)\n' "$1" "$2"
	# Two adaptations, because a container is not a GitHub runner:
	#
	#   - the mount is read-write. Several tests write scratch directories, and
	#     a read-only mount reports that as a test failure — it produced two
	#     convincing false positives the first time this was tried.
	#   - sudo is stripped: the image runs as root and has no sudo, while the
	#     runner is a non-root user that does.
	#
	# The strip uses awk, not `sed 's/\bsudo //'` — \b is a GNU extension that
	# BSD sed accepts and silently ignores, leaving sudo in place to fail with
	# "sudo: command not found".
	local script
	script=$(printf '%s' "$3" | base64 -d | awk '{gsub(/(^|[ \t])sudo /, " "); print}')
	if ! podman run --rm -i -v "$PWD":/src -w /src -e GOFLAGS=-buildvcs=false \
		"$IMAGE" bash -e -s <<<"$script"; then
		failed=1
		echo "FAILED: $1 / $2" >&2
	fi
}

while IFS=$'\t' read -r label where name script; do
	case "$where" in
	*macos*)
		if [ "$host_os" = Darwin ]; then
			run_native "$label" "$name" "$script"
			ran=$((ran + 1))
		else
			echo "skip $label / $name — needs macOS, this is $host_os"
			skipped=$((skipped + 1))
		fi
		;;
	*)
		if command -v podman >/dev/null; then
			run_linux "$label" "$name" "$script"
			ran=$((ran + 1))
		else
			echo "skip $label / $name — needs podman"
			skipped=$((skipped + 1))
		fi
		;;
	esac
done < <(steps "$only")

echo
if [ "$ran" = 0 ]; then
	echo "no steps matched${only:+ \"$only\"} — try ./scripts/ci-local.sh -l" >&2
	exit 2
fi
[ "$skipped" -gt 0 ] && echo "$skipped step(s) skipped: this host cannot run them."
if [ "$failed" = 0 ]; then
	echo "ci-local: $ran step(s) passed"
	exit 0
fi
echo "ci-local: failures above"
exit 1

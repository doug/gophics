# CLAUDE.md

The guidance for agents working on this repository is in **[AGENTS.md](AGENTS.md)**.
Read that file.

It is a stub rather than a symlink on purpose. Git for Windows checks symlinks
out as plain text files containing the target path unless `core.symlinks=true`,
which needs Developer Mode or an elevated shell — so a Windows contributor would
find a `CLAUDE.md` whose entire contents were the string `AGENTS.md`. gophics
builds and tests on Windows, so the repo does not assume POSIX symlink
semantics.

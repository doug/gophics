# skills/

Installable **Agent Skills** that teach AI coding agents how to use gophics.
A skill is the standard, portable unit — a `SKILL.md` folder with YAML
frontmatter (`name`, `description`) plus supporting files — that an agent loads
on demand when a task matches its `description`.

```
skills/
  gophics/
    SKILL.md              # the skill: mental model, idioms, gotchas (agent-facing)
    examples/counter/     # compiled canonical example (stateful + SetState)
    examples/customdraw/  # compiled canonical example (stateless + Canvas)
    apicheck/             # compile-time contract mirroring SKILL.md's API surface
```

## Installing

The portable unit is the `skills/gophics/` folder. Drop it into the target
agent's skills directory:

- **Claude Code:** copy to `~/.claude/skills/gophics/` (user-wide) or
  `.claude/skills/gophics/` in a project. It activates when a task matches the
  skill's `description`.
- **Other agents that support the SKILL.md format:** point them at the folder,
  or copy it into their skills path. An `npx`-style installer, if you add one,
  is just a convenience wrapper around that copy — the artifact itself stays
  standard-format so it isn't tied to any one installer.

## How skill, code, and docs stay aligned

The whole point of the layout: **the skill can't silently teach a dead API.**

1. **Examples are real, compiled code.** `examples/*/main.go` are the canonical
   snippets shown in `SKILL.md`. They're built by `go build ./...` in CI, so a
   snippet can never reference an API that no longer builds.
2. **`apicheck/` is a compile-time contract.** It names every symbol `SKILL.md`
   documents (funcs by value, structs by a literal of the fields the skill
   mentions, `paint.Canvas` primitives via an interface assertion). A rename or
   removal upstream breaks `go build ./...` — CI goes red instead of shipping a
   lie.
3. **`SKILL.md` is thin and durable.** It teaches the *model* and idioms (which
   drift slowly) and, for the exhaustive/volatile surface, points agents at the
   authoritative sources — `go doc ./widget` and the repo's `examples/` — rather
   than duplicating an API reference that would rot. It carries a version stamp.

### Updating the skill

When you change gophics' public API or the skill's coverage:

1. Edit `SKILL.md`.
2. Mirror any new/renamed symbol in `apicheck/apicheck.go` and, if a snippet
   changed, in the relevant `examples/*/main.go`.
3. `go build ./... && go vet ./skills/...` — green means the skill matches the
   code. Bump the version stamp at the bottom of `SKILL.md`.

No generated files, no separate publish step: the compiler is the alignment gate.

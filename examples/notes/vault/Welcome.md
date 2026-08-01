# Welcome to Notes

A tiny **local-first** Markdown app built with *gossamer*. Every note is a plain
`.md` file in a folder — this one lives in `examples/notes/vault`.

## What works

- Click a note in the sidebar to open it
- Press **Edit** to change it, **Save** to write it back to disk
- Follow links between notes with wikilinks: [[Markdown Guide]]
- Regular links open in your browser: [gossamer](https://github.com/doug/gossamer)

## Why it exists

Notes is the driving example for two things gossamer cares about: real text
editing, and treating UI state as data. Open a note, scroll down, start editing,
then save this file and watch `gossamer dev` bring you right back here.

See the [[Markdown Guide]] for what the renderer supports.

# A data table for gophics — designed from Tufte

A `widget.Table`: the missing primitive for dense, scannable tabular data
(financial registers, balance sheets, logs, dashboards). Its visual design is
taken directly from Edward Tufte's rules for analytical tables — because a table
is the highest-density data display we have, and most UIs ruin it with ink.

## The Tufte rules, and how the widget honors each

1. **Maximize the data-ink ratio; erase non-data ink.** Every pixel should carry
   data or the structure that lets you read it. The table draws **no vertical
   rules** (columns are separated by alignment and whitespace, never lines) and
   **no row rules by default** — the eye tracks a row by its baseline and spacing,
   not by a cage around it.

2. **Rules, when used, are light and few.** Following the "booktabs" convention
   Tufte endorses, the table earns exactly **one** rule: a single hairline under
   the header, separating labels from data. That's the whole chrome. (An optional
   `RowRule` adds a faint inter-row hairline for very wide tables where the eye
   drifts; off by default.)

3. **Numbers right-align on the decimal, in tabular figures.** Comparison is the
   job of a numeric column, and comparison needs the digits to line up. Numeric
   columns use `AlignEnd`; callers render figures in a monospaced/tabular face so
   ones and commas stack. Text columns use `AlignStart`.

4. **De-emphasize the labels, emphasize the data.** Column headers are supporting
   apparatus, not the point. They render muted, small, and quiet; the data rows
   carry full contrast. The reader's eye lands on the numbers first.

5. **No chartjunk — no zebra striping, no heavy fills, no boxes.** Alternating
   row shading is decoration masquerading as structure; it adds ink proportional
   to the data and helps little once rows are well-spaced. Off by default
   (available as `Zebra` for the rare dense case). Selection and hover use the
   faintest possible tint, not a border or a bar.

6. **Whitespace is structure.** Generous, consistent row height and a uniform
   column gap do the work that gridlines pretend to. Breathing room is not wasted
   space — it is what makes a dense table legible.

7. **Small multiples belong in the table.** Tufte's sparkline was invented to sit
   *inside a line of text or a table cell*. Because a cell is any `Widget`, a
   column can hold a `widget.Canvas` sparkline (a balance trend, a monthly series)
   at the same resolution as the numbers beside it — a datum and its history on
   one row. (Tally uses this for account balance trends.)

8. **Right amount of precision.** The table never imposes precision; the caller
   formats each cell, so money shows cents and counts show integers — no spurious
   trailing zeros dictated by the widget.

## Shape of the API (see `widget/table.go`)

```go
widget.Table{
    Columns: []widget.Col{
        {Title: "Date",    Width: 96},                 // fixed
        {Title: "Payee",   Flex: 2},                   // flexible, weight 2
        {Title: "Account", Flex: 3},
        {Title: "Amount",  Width: 120, Align: widget.AlignEnd},
    },
    Count: len(rows),
    Cell:  func(row, col int) widget.Widget { … },      // built lazily, visible rows only
    Selected: sel, OnTapRow: onTap,                     // faint selection tint
    Sortable: true, SortCol: c, SortDesc: d, OnSort: …, // quiet ▲/▼ in the header
}
```

Fixed (`Width`) and flexible (`Flex`) columns resolve identically for the header
and every body row — same column structure, same available width — so columns
line up without a shared measurement pass. Rows are **virtualized** on
`widget.LazyList`, so a register of tens of thousands of transactions mounts only
what's visible. The header is pinned above the scrolling body.

## Deliberate non-goals (for now)

- **Horizontal scroll / column pinning** for tables wider than the viewport —
  v1 assumes columns fit (flex handles it); frozen first columns come later.
- **In-widget sorting/filtering** — the table shows a sort indicator and reports
  taps; the *caller* owns the data order (it knows the types). Keeps the widget a
  view, not a data store.
- **Inline cell editing** — cells are display widgets; editing is a caller
  concern (an editor overlay), not baked into the grid.

---
name: palette
description: Choose or build a palette, generate hue-shifted ramps, and clean up a sprite whose colours have sprawled. Use when the user names a palette or console, asks about colour, or when validate reports off-palette colours.
---

# Palette

## Choosing

In this order of preference:

1. **The user named a palette, a console or a game.** Use it —
   `palette op=set colors=[...]` with its entries. Bundled hex lists are in
   [palette](../references/palette.md).
2. **The sprite joins an existing project.** `sprite op=info` on an existing
   asset, then match it exactly. Consistency beats a nicer palette.
3. **The user described an era.** 8-bit → PICO-8. Game Boy → its four greens.
   NES → 3–4 hues × 4 values, by hand.
4. **Nothing was said.** Sixteen colours, one ramp per material. Small enough
   to stay coherent.

## Building ramps

A ramp is one material's run of colours.

```
palette op=ramp base="#c04030" steps=5
```

`ramp` hue-shifts and verifies: lightness rises steadily step to step and
neighbouring colours are far enough apart to be distinguishable. It reports the
colours it produced, so you can `palette op=set` them if you want them fixed
rather than appended.

Three to five steps per material. Reuse the darkest step across materials.

## Reading what the art actually uses

```
palette op=analyze
```

Reports the colours in the art against the palette: off-palette colours,
near-duplicates, unused entries, and the lowest-contrast pair. This is the first
call when a sprite looks muddy.

## Repairing sprawl

Symptom: `validate` reports `palette` findings, or the sprite has nine
near-identical browns.

1. `palette op=analyze` — see what is actually there.
2. Choose the target: the existing palette minus the near-duplicates, or a fresh
   one via `op=set`.
3. Merge by hand: `draw` kind `replace` with `from` and `to` for each colour
   that has to become another. Do the merges one call per colour so each is one
   undo step and you can see which one went wrong.
4. Re-run `op=analyze`, then `look op=preview`. Merging two colours that were
   doing different jobs is the risk: look for a form that has gone flat.

**Starting from the art instead:** `palette op=extract` rebuilds the palette
from a quantised set of the sprite's own colours, and `op=adopt_from_art` sets
the palette to exactly the colours in use. Both are for art that arrived with
no curated palette — after which `op=analyze` has nothing left to complain about.

**The other direction:** `palette op=snap` moves every pixel onto its nearest
palette entry and reports how many moved. That is the fix for art that drifted
after a resize, a hue shift or a paint-over: the palette stays the palette, and
the art comes back to it. Snap before analysing a document you did not make, or
the report is really a complaint about the drift.

```
palette op=snap layer="base" frame=1
```

## Contrast

Readability is carried by value, not hue. Check by imagining the sprite in
greyscale: if the shapes stop reading, the values are too close and no hue will
fix it. `palette op=analyze` reports the lowest-contrast pair in numbers.

## Related

[palette](../references/palette.md), [shading](../references/shading.md).

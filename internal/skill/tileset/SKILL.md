---
name: tileset
description: Draw level art and anything that repeats — a single tile, a tileset grid, or a repeating texture — so the pieces line up and tile without seams. Use for terrain, platforms, walls, floors and background patterns.
---

# Tilesets and repeating art

A tile is judged on what happens at its edges, not in its middle. Everything
here is about boundaries.

## 1. Decide the unit before drawing

One tile size, applied everywhere: 8, 16 or 32. Every piece is that size, and
every offset is a multiple of it. Ask if the game already has a grid; if not,
pick 16 and say so.

`look op=preview` scales for the eye, not for the engine. Export is what the
engine sees, and it is 1× unless you pass `scale`.

## 2. One tile, drawn as a grid

```
sprite op=new width=16 height=16
draw label="floor tile" ops=[{
  "kind": "grid", "target": {"layer": "base"},
  "legend": {".": null, "d": "#3b2f2a", "s": "#5a4638"},
  "rows": [
    "dddddddddddddddd",
    "dssssssssssssssd",
    "dsddddddddddddsd",
    "dsdsdddddddddssd",
    "dsdsdddddddddssd",
    "dssssssssssssssd",
    "dddddddddddddddd",
    "…"
  ]
}]
```

Every row is the tile width. Count them — a 15-row tile is a bug the tool
cannot catch for you.

## 3. Make the edges tile

The seam is the only thing that matters. Two rules:

- **Wrap the pattern.** Whatever enters the left edge exists the right edge, at
  the same rows. Drawing the middle first and patching the edges afterwards is
  how seams appear.
- **Break the boundary occasionally.** A perfectly continuous pattern reads as
  wallpaper; one pixel of variation every tile or two kills the grid illusion.

Check by drawing the tile twice, side by side, on a canvas twice as wide:

```
sprite op=new width=32 height=16
draw ops=[
  { "kind":"grid", "target":{"layer":"base"}, "x":0,  "y":0, "legend":{…}, "rows":[…16 rows…] },
  { "kind":"grid", "target":{"layer":"base"}, "x":16, "y":0, "legend":{…}, "rows":[…the same rows…] }
]
look op=preview
```

The seam is the column pair in the middle. If it reads as a line, the left and
right edges disagree — fix the tile, not the copy.

## 4. Variants, not a new sprite each time

Save the base tile, then `sprite op=duplicate` and change only what the variant
needs — a cracked edge, an alternate stone. Variants that share a base colour
set read as one tileset.

## 5. Corners and transitions

SpriteWare has no autotile generator, so transition pieces are drawn on purpose:
one tile per corner case, with the edge colours matching exactly the neighbour's
edge colours. Keep the set small — isolated edges, inner corners, and one
"open" tile is enough for most styles.

## 6. Export

```
export op=frames path="/abs/path/tiles/{n}.png"
```

`{n}` numbers from 1, `{frame}` from 0 — pick one and stay consistent with the
engine's convention. The reply lists every path written.

## Checks

- Rows equal the tile width, count equals the tile height.
- Left edge matches right edge; top matches bottom.
- `validate checks=["gaps","strays","palette"]` — a hole in a tile is a hole in
  the level.

## Related

[checks](../references/checks.md), [craft](../references/craft.md).

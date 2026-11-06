---
name: draw
description: Draw a sprite from silhouette to finished pixels in the order that catches mistakes while they are still cheap — block in, check the silhouette, separate materials, shade, outline, verify. Use for the main body of any drawing task.
---

# Draw a sprite

Order matters more than technique. Detail added before the form is right makes
the wrongness harder to see and more expensive to fix.

## 1. Look before you draw

```
sprite op=info
look op=preview
```

You need the real layer names, the frame count and the palette. Do not assume
them: a wrong layer name means drawing into art that was already finished.

## 2. Block in one flat silhouette

Everything in one `draw` call, one colour, no detail.

Up to about 32×32, write it as a grid — one character per pixel. You see the
whole silhouette while you write it, instead of discovering afterwards what a
stack of ellipses added up to.

```
draw label="block in slime" ops=[{
  "kind": "grid", "target": {"layer": "base"},
  "legend": {"O": "#5f574f"}, "x": 8, "y": 10,
  "rows": [
    "....OOOO....",
    "..OOOOOOOO..",
    ".OOOOOOOOOO.",
    "OOOOOOOOOOOO",
    "OOOOOOOOOOOO",
    ".OOOOOOOOOO."
  ]
}]
```

Rules the tool enforces, not suggests:

- Every row the same width. A ragged row is refused with its index; nothing is
  padded.
- Every character in the legend. An unknown one is refused with row and column.
- `.` is transparent, and transparent cells **erase** what is under them.
  `"transparent":"skip"` stamps over existing art instead.

Bigger sprites: one grid per body part or per layer, each at its own `x`/`y`.
Long runs of identical characters are where a model miscounts.

Shapes are still right for large regular forms — a floor, a sky band, a
40-pixel circle:

```
draw ops=[
  {"kind":"ellipse","target":{"layer":"base"},"x":12,"y":4,"radiusX":4,"radiusY":4,"color":"#5f574f","fill":true},
  {"kind":"rect","target":{"layer":"base"},"x":11,"y":12,"width":10,"height":10,"color":"#5f574f","fill":true}
]
```

Batch aggressively. One call is one undo step for the user; forty calls are
forty entries in their history.

## 3. Check the silhouette immediately

```
look op=preview
```

One question: **is it recognisable as one flat shape?** If not, fix it now.
Shading a bad silhouette is wasted work.

What to look for: symmetry that reads as a statue, tangents where an arm fuses
into the torso, limbs all the same thickness.

`look op=ascii` (with `rulers=false` for clean rows) shows the exact pixels and
the legend, and those rows go straight back into a `grid` op.

## 4. Separate the materials

Replace regions of the blockout with each material's base colour — skin, metal,
leather, cloth. Still flat, still one call. `draw` kind `replace` swaps one
colour for another across the layer; kind `pixels` with an explicit point list
places irregular regions.

## 5. Shade

See [shade](../shade/SKILL.md). One shadow step, look, one light step, look.
Stop there unless the sprite is 32px+ and genuinely needs more.

## 6. Outline

Pick one style and keep it. Selective outlining — outside only — is usually
right at small sizes.

`draw` kind `outline` does the mechanical part. The target layer's silhouette is
snapshotted first, so the ring cannot eat its own edge:

```
draw label="outline" ops=[{"kind":"outline","color":"#1a1c2c","side":"outside","target":{"layer":"base"}}]
```

- `side="outside"` grows the shape by one pixel and leaves every existing pixel
alone. `side="inside"` recolours the edge and keeps the size.
- `diagonals` defaults to `false`, which leaves corners cut and softer. Set it
true for square corners.
- Break the line where two shapes meet; a closed ring around every part reads as
a sticker. Do that by hand afterwards, with a `pixels` op or a `grid`.
- `draw` kind `replace` recolours an existing outline without moving it.

## 7. Text, if the art has words

`draw` kind `text` lays out a built-in 5x7 font at `x`,`y` with a `color`, and
it compiles into the same batch as everything else:

```
draw label="label" ops=[{"kind":"text","text":"LEVEL 1","x":4,"y":3,"color":"#f4f4f4","target":{"layer":"ui"}}]
```

Capitals, digits and `. , : ; ! ? - + / ' ( ) " # % * = < > _ &`. Lowercase is
drawn as capitals; anything else is refused by name, so a glyph cannot silently
become a blank. A character cell is 6 pixels wide (five plus one of spacing), so
a string is `6 x characters` wide — that is how you centre a label before
committing to a position.

## 8. Verify before you report

```
look op=preview
validate
```

Fix every error. Read [checks](../references/checks.md) for what each finding
means.

## Related

[craft](../references/craft.md), [shading](../references/shading.md),
[palette](../references/palette.md).

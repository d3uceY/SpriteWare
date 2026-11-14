---
name: export
description: Write the files an engine or a person can actually use — a single PNG, a numbered frame sequence, or vector rects — with the naming the target expects. Use when the art is finished and needs to leave Aseprite.
rules: [90-platform-styles, 21-limited-and-platform-palettes, 66-tiles-and-autotiling]
triggers: [png, spritesheet, gif, atlas, godot, unity, engine, sheet]
---

# Export

## Ask what consumes it

The target decides the format, and guessing produces files someone has to
re-export. If you do not know, ask — it is one question.

| Target | Wants |
|---|---|
| Unity, Godot, Phaser | one PNG per frame, or a single frame at 1× |
| Web / canvas | PNG sequence, or `svg` when it must scale |
| Preview for a human | PNG at 4–8× (`scale`) |
| Print / social | PNG at 8× |

SpriteWare writes files five ways: one PNG, a numbered frame sequence, vector
rects, an animated GIF, and a packed spritesheet with a JSON atlas. If the
engine wants a layout none of those describe, export the frames and say plainly
that packing is the next step outside SpriteWare.

## Before exporting

Run [review](../review/SKILL.md). Exporting broken art just distributes it.

Then check specifically:

- **Tags exist and are named the way the engine expects** — `idle`, `walk`,
  `attack`. `validate` errors on a multi-frame sprite with no tag.
- **Sketch and reference layers are hidden or gone.**
- **Frame durations are set** — an engine reads them from the tag, a person
  reading a filmstrip notices immediately when they are uniform.

## The five ops

One frame:

```
export op=png path="/abs/path/knight.png" frame=1 scale=4
```

Every frame, numbered:

```
export op=frames path="/abs/path/knight/{n}.png"
```

`{n}` is 1-based, `{frame}` is 0-based. Choose the one the engine's importer
expects and stay consistent — a sequence that starts at 1 dropped into an
importer expecting 0 loses the first frame silently.

Vector, one rect per horizontal run, grouped per frame:

```
export op=svg path="/abs/path/knight.svg"
```

Useful for docs and for scaling without a resample. It is not a substitute for
the PNG when the engine wants pixels.

Animated, with the timing you set:

```
export op=gif path="/abs/path/knight.gif" scale=4
```

A GIF holds 256 colours and no partial alpha; more colours is refused rather
than averaged into colours that were never in the art. A delay of zero is never
written, because every viewer then picks its own speed.

Packed, with an atlas:

```
export op=spritesheet path="/abs/path/knight.png" padding=1
```

One row of frames plus `knight.json` beside it: `frameWidth`, `frameHeight`,
`columns`, and a `frames` array with each 1-based index, its `x`/`y`/`w`/`h`
and its `durationMs`, plus the tags and the document's slices (with each
9-patch `center`). `padding` defaults to 1 and is not optional decoration: at
non-integer zoom a GPU samples past a frame's edge and picks up its neighbour,
which shows as a one pixel line along every sprite.

Mixed frame sizes are refused — a sheet has one frame size, so resize first.

Level art goes out as tiles instead:

```
export op=tileset path="/abs/path/arena.png" tileWidth=16 tileHeight=16 format=tiled
```

The canvas is sliced into tiles, duplicates are kept once, and the reply
carries a grid of tile numbers per frame with `0` for empty. `format=tiled`
adds a `.tsx` plus one `.tmx` per frame; `format=godot` adds a Godot 4 `.tres`
`TileSet`. A canvas that is not a whole number of tiles is refused, because a
half tile is a silent seam.

**The reply lists the exact paths written.** Use those paths in your report;
do not reconstruct them from the pattern.

## Save the source

Exporting does not save the working file.

```
sprite op=save
```

The `.aseprite` document is the asset. PNGs are build artifacts.

## Report

List the files written, their dimensions, the frame count and the tags. That is
what the person wiring them into the engine needs.

## Related

[checks](../references/checks.md), [animation](../references/animation.md).

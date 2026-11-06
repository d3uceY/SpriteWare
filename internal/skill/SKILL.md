---
name: spriteware
description: Draw, edit, validate and export pixel art in Aseprite through SpriteWare's MCP tools. Use when a task involves sprites, tilesets, animation frames, palettes, colour ramps, or exporting game-ready PNGs. Coordinates, layer order and frame numbers follow one convention described here.
---

# SpriteWare

SpriteWare drives a real Aseprite — headless in the background, or the editor
window you have open. You draw with tools, and the tool reads the result back so
you can check your own work before saying it is done.

## The three rules that prevent most mistakes

1. **Coordinates are 0-based and sprite-absolute.** `(0,0)` is the top-left
   corner of the canvas, x right, y down. A layer's cel offset never shifts
   them, so the same coordinate always lands on the same pixel.
2. **Frames are 1-based, layers are 0-based from the bottom.** A two-frame
   sprite has frames 1 and 2. Layer 0 is the bottom layer. Both are reported
   back in the same convention you passed in.
3. **Colours are `#rrggbb` or `#rrggbbaa`.** In an indexed document a colour is
   snapped to the nearest palette entry, and the tool tells you which one it
   used. If a colour matters, set the palette first.

## The loop

```
doctor                          # is Aseprite reachable, and what can it do
sprite   op=new  width=32 height=32
draw     ops=[{"kind":"grid", "rows":[...], "legend":{"K":"#1a1a1a"}}]
look     op=preview             # look at it before judging it
validate                        # palette drift, strays, broken rigs
export   op=png path=hero.png
```

Run `doctor` first when anything fails: it reports the connection, the mode and
the one thing to fix. Every tool connects on its own, so there is no separate
"connect" step to forget.

## Pick a workflow first

Each one is a procedure in its own folder, with the steps in the order that
catches mistakes while they are still cheap. Open the file when its situation
applies; you do not need the rest.

| Workflow | Open it when | File |
|---|---|---|
| New document | starting fresh rather than editing | [new](./new/SKILL.md) |
| Draw | the main body of any drawing task | [draw](./draw/SKILL.md) |
| Shade | flat art needs volume | [shade](./shade/SKILL.md) |
| Palette | choosing, building or repairing colour | [palette](./palette/SKILL.md) |
| Rig | a character has to animate | [rig](./rig/SKILL.md) |
| Animate | building a cycle or key poses | [animate](./animate/SKILL.md) |
| Tileset | level art, or anything that repeats | [tileset](./tileset/SKILL.md) |
| Fix | editing art that already exists | [fix](./fix/SKILL.md) |
| Review | before saying anything is finished | [review](./review/SKILL.md) |
| Export | handing files to a game engine | [export](./export/SKILL.md) |

## Reference

Craft that more than one workflow needs. Read the one that matches the decision
in front of you rather than all of them.

| File | Answers |
|---|---|
| [craft](./references/craft.md) | what makes pixel art read, and what makes it read as machine-made |
| [palette](./references/palette.md) | hue-shifted ramps, limited palettes, value contrast |
| [shading](./references/shading.md) | light direction, banding, dithering, materials |
| [animation](./references/animation.md) | timing tables and key poses for idle, walk, run, jump, attack |
| [structure](./references/structure.md) | layer order, rig naming, frame and cel conventions |
| [checks](./references/checks.md) | every `validate` check, what a finding means, what to do |

## The tools, in one line each

`doctor` · `sprite` · `draw` · `look` · `layer` · `cel` · `frame` · `tag` ·
`palette` · `validate` · `export`. Anything with an `op` argument takes `ops`
as an array to batch several in one undo step.

## `draw` is the tool you will use most

A grid is one character per pixel plus a legend that maps a character to a
colour. `.` is transparent unless the legend redefines it. Every row must be the
same width — a ragged grid is an error, not something to pad, because silent
padding turns a visible mistake into wrong art.

```json
{
  "ops": [{
    "kind": "grid",
    "rows": ["..KK..", ".KAAK.", "KAAA.A"],
    "legend": {"K": "#1a1a1a", "A": "#4ec36a"},
    "x": 8, "y": 8
  }]
}
```

`look op=ascii` prints the same format back for a layer, so you can read the
current pixels, edit the characters, and send them straight back.

Other kinds: `pixels` (explicit point list), `rect`, `line`, `ellipse`, `clear`,
`replace` (swap one colour for another across the layer), `text` (a built-in
5x7 bitmap font at `x`,`y` — capitals, digits and common punctuation).

## Seeing your own work

- `look op=preview` — one frame as an image, over a checkerboard so
  transparency is visible. **Look at this before you report success.**
- `look op=ascii` — the frame as characters and a legend.
- `look op=filmstrip` — every frame in one strip, for animation.
- `look op=onion` — the current frame over ghosted neighbours, for judging an
  in-between.
- `look op=diff` — what changed between two frames.

## Validate before you finish

`validate` returns findings with a severity, a code and a location. `error`
means the art will look wrong (a clipped silhouette, a detached limb, half-alpha
pixels); `warning` means it is suspicious (stray pixels, a single colour used for
a whole layer); `note` is advice. Nothing is silently dropped: if a check is not
run, the report says so.

Use `scope=layer` when the art is rigged on separate layers — a composite can
look clean while a single layer is broken.

## Animation

`frame op=add` and `op=duplicate` build the timeline, `op=setDuration` retimes
it, `tag op=create` names a range for the game engine. Frame durations are
milliseconds. A tag is what a runtime addresses, so create one even for a
two-frame loop.

## Exporting

`export op=png` writes one frame; `op=frames` writes a sequence with `{frame}`
(0-based) or `{n}` (1-based) in the filename; `op=svg` writes a vector version
one rect per horizontal run; `op=gif` animates using the frame durations;
`op=spritesheet` writes one packed PNG plus a JSON atlas with per-frame rects
and the tags. The reply lists the exact paths written — use those paths, do not
reconstruct them.

## Palette work

`palette op=ramp` generates a shade ramp with cool shadows and warm highlights
and verifies it (monotone lightness, minimum separation). `op=analyze` reports
what the art actually uses. `op=adopt_from_art` snaps a foreign palette back to
the art's own colours. Prefer adopting an existing palette over inventing a new
one per sprite — that is how a set stays coherent.

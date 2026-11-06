---
name: new
description: Create an Aseprite document with the right canvas size, palette and layer structure, so later work does not have to fight the setup. Use when starting a sprite from nothing rather than editing an existing one.
---

# New document

Setup mistakes are expensive: a canvas one size off means redrawing, and a
palette chosen after the art means repainting. Four things to get right, then
the rest is drawing.

## 1. Check the connection

`doctor` — mode, Aseprite path, and the one thing to fix if it is not ready.
Stop here if it reports a problem; every later call will fail the same way.

## 2. Size the canvas

```
sprite op=new width=32 height=32
```

- Match the game's grid. A 16×16 tile game wants character art that is a
  multiple of 16 in at least one dimension.
- Leave headroom. A 32×32 canvas for a 28px character has room for a raised
  weapon without a later resize.
- Powers of two are the safe default for textures, not a rule for characters.

`draw`, `look` and `validate` all work on the active document, so you do not
pass a path. Use `sprite` `op=open` + `op=activate` when several documents are
open and you want a specific one.

SpriteWare creates plain RGB documents. Palette discipline comes from the
`palette` tool and from `validate` running the `palette` check — not from the
file format.

## 3. Set the palette before drawing

```
palette op=set colors=["#1a1c2c","#5d275d","#b13e53","#ef7d57"]
```

Or build one ramp per material — see [palette](../palette/SKILL.md). A colour
drawn before the palette exists is a colour someone chose by accident.

## 4. Build the layers in one batch

```
layer ops=[
  {"op":"create","name":"base"},
  {"op":"create","name":"shading"},
  {"op":"create","name":"outline"}
]
```

One call is one undo step. Layer 0 is the bottom, so create them in the order
they should stack. Anything that will animate needs the full rig instead —
[rig](../rig/SKILL.md). Splitting baked pixels apart later is real work.

## 5. Save immediately

```
sprite op=save_as path="/abs/path/knight.aseprite"
```

Ask where if you do not know, or save beside the project the user is working in.
Nothing survives a crash otherwise, and undo has no anchor.

## Then

[draw](../draw/SKILL.md) to block in the art.

## Related

[structure](../references/structure.md), [palette](../references/palette.md).

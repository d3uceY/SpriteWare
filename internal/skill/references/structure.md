# Structure

How SpriteWare numbers things, and the conventions that keep a document
addressable by every later call.

## Coordinates and indices

| Thing | Convention | Why |
|---|---|---|
| Pixels | 0-based, sprite-absolute | (0,0) is the canvas corner; a layer's cel offset never shifts it |
| Layers | 0-based, 0 is the bottom | layer 0 is what everything else draws over |
| Frames | 1-based | frame 1 is the first frame, and a 2-frame sprite has 1 and 2 |
| Colours | `#rrggbb` or `#rrggbbaa` | 8-digit only when the pixel really is semi-transparent, which `validate` flags |

Every tool reports indices back in the convention you passed. Nothing silently
converts.

## Addressing

- `sprite` — omit it and the active document answers; pass an absolute path to
  address another one. `sprite op=activate` changes which one is active.
- `layer` — a name or a group path, `torso/arm-near`. A bare name that matches
  more than one layer is refused with the matches listed, never guessed.
- `target` — the `{layer, frame}` pair on a `draw` op. One op affects one cel.

## Layer order for a character

Bottom to top:

```
shadow
leg-far
arm-far        partly hidden by the torso; this is what sells depth
torso
head
leg-near
arm-near       camera-side arm
```

Nearer parts above farther parts, so an arm overlapping a torso draws over it
without any special case.

## Cels and frames

- A **cel** is one layer's pixels on one frame. `cel ops` create, clear, move,
  set opacity, or reset the position.
- An empty cel is not the same as a missing one. `frame op=duplicate` copies the
  frame including every cel and reports the offsets it carried.
- `frame op=normalise` zeroes a frame's cel offsets — the fix when a duplicated
  idle bob has quietly become part of every frame built on it.
- `frame op=delete` renumbers the frames after it. Tags adjust, your references
  to frame numbers do not, so re-read `frame op=list` afterwards.

## Tags

`tag op=create name="walk" from=1 to=8 direction="forward"`.

A tag is the only thing a runtime can address — a game engine does not ask for
"frames 1–8", it asks for `walk`. `direction` is `forward`, `reverse` or
`pingpong`, and it is metadata for the engine, not an instruction to SpriteWare.

Tag names are the asset's API. `idle`, `walk`, `run`, `attack`, `hurt`, `death`
are what engines and importers expect.

## Documents

- One open document is active; tools act on it unless told otherwise.
- `sprite op=save` writes the existing path, `op=save_as` picks one. Export is
  not save.
- The `.aseprite` file is the source. PNGs are build artifacts.
- `sprite op=resize` changes the canvas, and can clip art. Check `validate
  checks=["bounds"]` afterwards.

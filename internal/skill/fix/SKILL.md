---
name: fix
description: Change a sprite that already exists — the user's own work or your earlier output — without destroying what is already right. Use for edits, touch-ups, style corrections and "make it more X" requests.
---

# Fix or iterate on existing art

Editing someone's work has a failure mode that drawing from scratch does not:
breaking something that was already correct. The whole procedure is about not
doing that.

## 1. Understand what is there before changing it

```
doctor
sprite op=info
look op=preview
```

For a targeted edit, also `look op=ascii` — you cannot change pixels precisely
from a description.

**Never assume layer names, frame counts or the palette.** Drawing into the
wrong layer is the most common way an agent damages someone's file. When two
groups hold same-named layers, a bare name is refused with the matches listed —
pass the group path (`left/arm`) rather than renaming the user's layers.

## 2. Say what you are about to change

For anything beyond a touch-up, state the plan in one line and let the user stop
you. Especially before: changing the palette, resizing the canvas, deleting a
layer, or editing a layer you did not create.

## 3. Scope the edit as tightly as you can

SpriteWare has no selection tool, so scoping is done with `target`:

- **A layer** on every op keeps you out of the rest of the drawing.
- **A frame** in `target.frame` keeps you out of the timeline.
- **A grid at one origin** touches exactly that rectangle, nothing else.

Scoping is cheaper than repairing.

## 4. Change pixels by editing text, not coordinates

```
look op=ascii layer="base" rulers=false
```

That returns `rows`, the colour `legend` and the `origin`. Change the wrong
cells in the rows and send the same rows back:

```
draw label="fix the eye" ops=[{
  "kind": "grid",
  "target": {"layer": "base"},
  "x": 0, "y": 0,
  "legend": { …the legend you were given… },
  "rows": [ …the rows you edited… ]
}]
```

Only that rectangle is touched. Transparent cells erase — pass
`"transparent":"skip"` when you mean to add pixels without clearing any.

Colour-wide changes are `draw` kind `replace` with `from` and `to`: one call per
colour, so each is its own undo step and a mistake is one Ctrl+Z away.

## 5. Verify what actually changed

A static edit has no second frame to compare, so re-read the region:
`look op=ascii` on the same area, and check that only the rows you meant to
touch changed. On an animation, `look op=diff fromFrame=N toFrame=N` with two
different frames shows exactly what moved — and if you edited an eye and the
diff's bounds span the whole sprite, something else moved.

## 6. Validate and report

```
validate
```

Report what changed and what you deliberately left alone.

## Interpreting vague requests

| Request | Usually means |
|---|---|
| "more contrast" | Widen the value range of the ramps, not the hue |
| "cleaner" | Remove strays, even out line runs, merge near-duplicate colours |
| "more detail" | Usually wrong at small sizes — ask what should read better |
| "pop more" | Contrast against the background, or one brighter accent |
| "less flat" | No light direction, or the shadows do not hue-shift |
| "muddy colours" | Too many near-duplicates: `palette op=analyze`, then merge |
| "off / uncanny" | Run [review](../review/SKILL.md) and name the cause |

Reflect your reading back before acting on it. "More detail" on a 16×16 sprite
usually means the sprite needs to be bigger, and that is worth one question.

## What not to do

- Do not redraw from scratch because it is easier than editing, unless the user
  asked for that.
- Do not improve things nobody mentioned.
- Do not widen the palette to fit a colour you chose. Say the palette lacks it.
- Do not flatten, resize or delete layers without being asked.

## Related

[craft](../references/craft.md), [review](../review/SKILL.md).

---
name: rig
description: Split a character onto named layers — head, torso, arms, legs — so it can be animated by moving cels instead of redrawing pixels. Use before animating anything, or when limbs are baked into one layer.
rules: [06-layers-and-rigging, 30-proportions-by-size, 31-anatomy-and-pose, 45-secondary-motion]
triggers: [rigging, layers, limbs, skeleton, joints, parts]
---

# Rig a character for animation

Pixels baked into one layer cannot be animated without redrawing them. Rig
before drawing; separating a finished sprite afterwards is more work than
building it split.

## The standard rig

Bottom to top, so nearer parts draw over farther ones — layer 0 is the bottom:

```
shadow       ground contact, if the style has one
leg-far
arm-far      partly hidden by the torso; this is what sells depth
torso
head
leg-near
arm-near     camera-side arm
```

Add `weapon`, `cape`, `hair-front`, `hair-back` as the design needs.

Names are the API for every later call, so keep them stable and avoid
duplicating one. Two layers with the same name make a bare name ambiguous:
`layer`/`cel` refuse it and list the matches, and you then pass a group path
such as `torso/arm-near`.

## Building it fresh

One batch, one undo step:

```
layer ops=[
  {"op":"create","name":"shadow"},
  {"op":"create","name":"leg-far"},
  {"op":"create","name":"arm-far"},
  {"op":"create","name":"torso"},
  {"op":"create","name":"head"},
  {"op":"create","name":"leg-near"},
  {"op":"create","name":"arm-near"}
]
```

Then draw each part on its own layer from the first pixel. `draw` takes
`target.layer` per op, so one call can lay out several parts.

A `/` in a name is a literal character, not a new group. To nest, make the group
after the parts exist:

```
layer op=group layers=["arm-far","torso","arm-near"] name="body"
```

The group takes the slot of the lowest layer moved, so the stack does not jump.
`layer op=ungroup name="body"` puts the children back where the group was, and
`layer op=merge` flattens named layers into one when a rig is finished — that
loses the parts, so merge at export time, not while you are still checking
boundaries.

Group paths such as `torso/arm-near` resolve layers inside groups that already
exist in the document, which is why they save you from renaming the user's
structure.

## Splitting an existing sprite

`cel op=copy` copies one cel onto another layer, which is the whole split:

1. `sprite op=info` for the layer list; `look op=ascii` to find the part
   boundaries. Guessing coordinates here is how limbs get clipped.
2. Create the rig layers above.
3. For each part, copy the source onto its layer:
   ```
   cel op=copy layer="head" fromLayer="base"
   ```
   That copies the whole cel, so the part is still there to erase afterwards —
   which is the point: a bad boundary is recoverable while the source exists.
4. Clear the part out of the source, or hide the source:
   `layer op=set name="base" visible=false`. **Prefer hiding** while you are
   still checking boundaries.
5. `look op=preview` with the source hidden. It should look unchanged. A part
   that lost pixels means the boundary was wrong.

## Overlap matters

A limb needs a pixel or two of overlap with the torso, or a gap opens the moment
it moves. Copy generously and let the stacking order hide the excess.

## Then

[animate](../animate/SKILL.md).

## Related

[structure](../references/structure.md), [craft](../references/craft.md).

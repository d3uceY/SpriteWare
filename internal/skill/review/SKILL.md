---
name: review
description: Run the mechanical checks and the by-eye checks, then report what you found with evidence instead of claiming success. Use before finishing any pixel-art task, and when the user asks "is this good" or "why does this look off".
rules: [07-review-checklist, 92-generated-art-tells, 12-anti-aliasing, 14-readability-and-scale]
triggers: [check, critique, feedback, ship, approve, quality]
---

# Review a sprite before calling it done

"Done" is a claim. This is how you earn it.

## 0. Cold read

Before rereading the brief: `look op=preview`. Write one sentence on what the
picture shows to someone who was never told, plus the three things the eye lands
on first. If that sentence does not name the subject, the sprite fails review
whatever `validate` says.

Squint at the same preview: the lit side should be among the lightest values and
the core shadow among the darkest. Everything in the bottom third reads as murk.

## 1. Mechanical checks

```
validate
```

Returns findings with a severity, a code and a location where one exists.
`error` means the art will look wrong. `warning` means it is suspicious. `note`
is advice. Nothing is silently skipped — if a check did not run, the report says
so.

Fix every error. For each warning you leave, have a reason and state it.

Use `scope="layer"` when the art is rigged: a composite can look clean while one
layer is broken.

For animation, tell `validate` what the cycle should be, so it checks your plan
instead of only the generic rules:

```
validate expect={"layerFrames":{"sword-fx":[[3,4]]},"mustNotOverlap":[["sword","head"]]}
```

`layerFrames` flags a layer empty inside its range or painting outside it.
`mustNotOverlap` flags frames where two layers' pixels collide. `paletteSize`
and `bounds` work the same way — your own constraints, checked.

## 2. Look at it

Work down the list, answering each with yes or no:

- **Silhouette** — recognisable as one flat shape?
- **Value contrast** — would it survive desaturation?
- **Light** — one direction, everywhere?
- **Ramps** — do shadows hue-shift, or are they just darker?
- **Lines** — even runs, no doubled pixels?
- **Outline** — one style, applied consistently?
- **Banding** — long parallel stripes of adjacent ramp steps?
- **Detail placement** — spent on the face and the weapon, not the boots?

## 3. Look closely at anything that felt off

```
look op=ascii
```

This is where the pixel one row too low and the run of 3 in a sequence of 2s
turns up. If step 2 left you with "something is wrong but I cannot name it",
this step names it.

## 4. For animation

```
look op=filmstrip
```

Volume consistent? Height consistent? Does it loop? Do contact poses hold
longer? Is there anticipation before a strong action?

Then between two frames:

```
look op=diff fromFrame=3 toFrame=4
```

The diff reports what changed, where, and by how much — the only way to see
spacing without playing the animation, and the fastest way to catch a limb that
moved two pixels on one side of a cycle and one on the other.

## 5. Report

State what you checked, what you found, and what you chose not to change:

> Validated clean. Silhouette reads at 1×; value contrast holds in greyscale.
> Two compromises: the plume colour is a step off the palette and I left it,
> because the nearest entry is 12 ΔE away and reads as a different hue; and the
> far arm is 2px thick rather than 3 so it does not fuse with the torso.

A user can act on a stated compromise. They cannot act on one you hid.

## When the user asks "why does this look off"

Run the same checks, then name the specific cause rather than listing
possibilities. In order of frequency: too many colours, flat shading,
semi-transparent pixels, a mushy silhouette, uniform animation timing.

## Related

[checks](../references/checks.md), [craft](../references/craft.md),
[animation](../references/animation.md).

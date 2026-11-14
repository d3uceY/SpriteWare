---
name: shade
description: Add light and shadow with hue-shifted ramps, one step at a time, so the sprite gains volume without the flat-luminance look that marks generated pixel art. Use when flat art needs form, or when art looks muddy or flat.
rules: [02-shading-and-light, 20-color-for-pixel-art, 24-lighting-scenarios, 13-dithering-and-texture]
triggers: [shading, light, shadow, volume, highlight, gradient]
---

# Shade a sprite

Flat shading — the same hue, darker — is the clearest signal that a sprite came
from a brightness slider. Real shading shifts hue: shadows drift toward
blue/purple, highlights toward yellow/orange.

## 1. Fix the light direction, and say it out loud

Upper-left is the safe default. It does not move again — not across the sprite,
not across animation frames. Everything below assumes one light.

## 2. Confirm the flat art is right first

```
look op=preview
```

Shading a bad silhouette wastes the effort. This is the last cheap moment to
fix the shape.

## 3. Build the ramp

```
palette op=ramp base="#c04030" steps=5
```

`ramp` builds a hue-shifted run from the material's base colour and verifies it:
lightness has to rise steadily and neighbouring steps have to be far enough
apart to read as different colours. Then draw with the ramp's top entry — the
other four are the shadow and highlight steps you will use now.

Three to five steps per material is plenty. Reuse the darkest step across
materials; it ties the sprite together and costs no palette slots.

## 4. One shadow step

Draw the shadow shape where the light does not reach. Follow the form's
cross-section, not its outline — a shadow that traces the contour reads as a
stain.

```
draw label="shade" ops=[{"kind":"shade","amount":-0.25,"target":{"layer":"shading"}}]
```

`draw` kind `shade` shifts a whole cel in one call: negative darkens **and
cools** (red falls fastest, blue least), positive lightens **and warms**. That is
the hue shift a brightness slider cannot do, and it is applied per distinct
colour, so flat regions stay flat. It needs an RGB document — an indexed palette
cannot hold the shifted colour, and the tool says so instead of snapping silently.

Shape the shadow with `grid` or `pixels` first, then shade; or shade the whole
cel and carve the lit side back out with `replace`.

## 5. Look

```
look op=preview
```

Does it read as volume? A single shadow on the far side and a lit top edge is
already most of the way there.

## 6. One highlight step, on the side facing the light

Same shape of call, using the ramp's lightest entry. Then look again. Most
sprites are finished here.

Only if it genuinely needs it: a deep shadow in occluded crevices, a one or two
pixel specular on the hardest material, or reflected light — a faint cool step
on the outer edge of the shadow side, which is what makes metal read as metal.

## What not to do

- Do not pick darker hex values by hand when `palette op=ramp` will shift hue
  for you.
- Do not add a third and fourth step to a 16×16 sprite. Two values per material
  is the norm at that size.
- Do not dither below 32px, and not in small areas at any size. It reads as
  noise.
- Do not let a semi-transparent pixel in. `validate` reports them as
  `antialiasing` findings, and they read as blur at 1×.

## Checks

- Light in exactly one place.
- Shadows hue-shift rather than darken.
- No banding — long parallel stripes of adjacent ramp steps.
- The silhouette still reads with the shading flattened in your head.

## Related

[shading](../references/shading.md), [palette](../references/palette.md),
[craft](../references/craft.md).

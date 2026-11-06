# Shading

## One light, fixed

Choose a direction before the first shadow and keep it for the whole sprite, and
across every frame of an animation. Upper-left is the safe default.

Symptoms of a broken light: highlights on both sides, a shadow on the side
facing the light, a rim that appears and disappears between frames.

## How many steps

| Size | Steps per material |
|---|---|
| 8–16 px | 2 — base and shadow |
| 16–32 px | 3 — shadow, base, highlight |
| 32–64 px | 4–5, and only if the form needs it |

More steps than the form has planes reads as noise. A 16px sprite with four
shades of skin is a sprite with four shades of skin and no shape.

## Where the shadow goes

- **Terminator** — the edge between lit and unlit. On a rounded form it is a
  crescent, not a line following the outline.
- **Core shadow** — just inside the terminator, the darkest part of the form.
- **Cast shadow** — where one part blocks another. Small, and immediately below
  or beside the blocker.
- **Reflected light** — a faint value on the outer edge of the shadow side. This
  is what makes metal read as metal, and it costs one colour.
- **Occlusion** — where two forms meet, a dark seam. Deepening that seam does
  more for depth than any highlight on the top.

## Pillow shading

Value falling off from the centre of a shape in rings, with a dark rim all
around. It comes from shading by distance from centre instead of by light
direction. The fix is to offset the highlight toward the light and put a
crescent shadow on the far side.

## Banding

Long parallel stripes of adjacent ramp steps, with their ends aligned. It reads
as corrugation. Fix:

- stagger the ends of adjacent bands
- let the boundary between steps wander by a pixel
- dither one short section instead of adding a whole step

`validate` reports horizontal banding directly. Vertical, diagonal and
hugging-the-outline banding need `look op=ascii`.

## Dithering

Only in large flat areas, only at 32px and up, only between two adjacent ramp
steps. A checkerboard or a 4×4 Bayer pattern is a transition; random noise is a
texture, and textures read as grain at small sizes.

Below 32px, do not dither. Add a colour instead.

## Materials

- **Hard** — metal, glass, polished stone: sharp terminator, one bright
  specular of one or two pixels, strong contrast, near-black in the core shadow.
- **Soft** — cloth, skin, fur: wide terminator, no specular, values closer
  together, and the highlight follows the bulge not the outline.
- **Translucent** — water, gems: the light passes through, so the shadow side is
  brighter than a solid form's would be, and internal edges are visible.

Two materials with the same shading model read as one object. The difference is
usually the specular and the contrast range, not the hue.

## What not to do

- Do not shade every layer of a rig separately in a different direction.
- Do not put a highlight on the top edge of everything; that is a bevel, not
  light.
- Do not use semi-transparent pixels for shading. `validate` reports them, and
  they read as blur.

## Related

[palette](./palette.md), [craft](./craft.md).

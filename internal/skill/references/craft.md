# Craft

What makes pixel art read, and what makes it read as machine-made. Everything
here is checkable — with `look`, with `validate`, or with one squint.

## The 1× test

Look at the preview at the size it will ship. If the subject is not identifiable
at 1×, no amount of detail will fix it — detail is what makes an unreadable
shape busier.

Detail is spent, not spread. The face and the weapon get it; the boots do not.
Equal detail everywhere is the single most common reason a sprite looks
generated: it means nobody decided what mattered.

## Silhouette first

A sprite is a shape before it is anything else. Block it in one flat colour and
judge it as a silhouette before adding a single shade.

- Does it read as one connected shape, or as parts that happen to touch?
- Are there tangents — points where two edges run into each other so the eye
  cannot tell them apart?
- Is any limb so thin it will disappear against a background?

`validate checks=["gaps","bounds"]` catches the mechanical half of this.

## Clusters, not noise

Pixel art is made of small clusters of one colour, three pixels or more, that
follow the form. A single pixel of a fourth colour in the middle of a fill reads
as speckle, and speckle is a texture, not shading.

- One isolated pixel is a decision (an eye highlight, a specular). Five are
  noise.
- `validate` reports them as `strays` when they float in transparency and
  `orphans` when they sit inside a fill.
- Fix by repainting from the surrounding colour, or by removing the pixel.

## Value carries readability

Greyscale the sprite in your head, or squint. If the shapes stop reading, the
palette's values are too close and no amount of hue will fix it.

Target: the lit side among the lightest values, the core shadow among the
darkest, and a mid-tone that does most of the work. A sprite where everything
lives in the middle reads as murk.

## Edges

- One outline style, applied consistently. Selective outlining — outside only —
  is right at small sizes because a line on every internal boundary spends a
  third of the sprite on lines.
- No semi-transparent pixels. `validate` reports them under `antialiasing`, and
  at 1× they read as blur rather than as a soft edge.
- No bilinear or fractional scaling. Integer nearest-neighbour is the only
  resize that keeps pixels pixels.

## The tells, and what to run

| Tell | How you notice | Fix |
|---|---|---|
| Too many colours | `palette op=analyze` — near-duplicates, unused entries | merge with `draw` kind `replace`; cap the palette |
| Brightness-only ramps | shadows are the same hue, darker | `palette op=ramp` for a hue-shifted run |
| Confetti texture | single pixels of unrelated hue | repaint from the surrounding colour |
| Banding | long parallel stripes of adjacent steps | stagger the ends, let the boundary wander |
| Pillow shading | value falls off from the centre in rings | pick a light direction, crescent shadow on the far side |
| Soft AA | three or more tones between outline and background | hard 1px edge |
| Equal detail everywhere | the 1× test fails, everything is busy | delete detail until it passes |
| Broken outline | `validate` reports `outline` | one colour, one style, redraw the gaps |

`validate` names the ones it can measure with a location. The rest are the
by-eye list in [review](../review/SKILL.md).

## Order of repair

Always this order, because each step changes what the next one sees:

1. Canvas size and scale
2. Palette
3. Silhouette and light direction
4. Clusters and banding
5. Edges and outline
6. Detail

Fixing edges before the palette means fixing them twice.

## What not to do

- Do not add detail to a shape that does not read.
- Do not smooth, blur, or resample to hide a defect.
- Do not widen the palette for one colour. Say the palette lacks it.
- Do not shade before the silhouette is signed off.

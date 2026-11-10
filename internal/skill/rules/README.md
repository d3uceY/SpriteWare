# Rules

The pixel-art craft this project encodes, one file per subject. They are served
as `rules://` resources, so an agent reads one chapter when its subject comes up
instead of carrying all of them in context.

Written by the `aseprite-ai-artist` project (MIT) and copied here verbatim. The
wording, the numbers and the ` ```grid ` templates are theirs, which is why the
chapters cite each other as `rules://<file>` — that URI is the resource URI, so
a cross-reference resolves to the published chapter. Only this index is adapted:
the original pointed at that project's build scripts and ADR.

Read `rules://index` for the list, or start at `rules://00-core-principles`.

Files are numbered in bands: `0x` core discipline, `1x` technique, `2x` colour
and materials, `3x` characters, `4x` animation, `5x` creatures, `6x`
environments, `7x` objects and 3D, `8x` effects and UI, `9x` style.

Subject files carry ` ```grid ` templates. Copy the template's rows into `draw`
op `grid` and map each legend role onto the sprite's own palette. `TestRuleGrids`
holds every template here to the format `draw` accepts, so a template cannot
drift away from what the tool reads.

## 0x — core discipline

| Chapter | Subject |
|---|---|
| [00-core-principles](./00-core-principles.md) | Core principles |
| [01-palette-and-color](./01-palette-and-color.md) | Palette and colour |
| [02-shading-and-light](./02-shading-and-light.md) | Shading and light |
| [03-silhouette-and-form](./03-silhouette-and-form.md) | Silhouette and form |
| [04-outlines-and-edges](./04-outlines-and-edges.md) | Outlines and edges |
| [05-animation](./05-animation.md) | Animation |
| [06-layers-and-rigging](./06-layers-and-rigging.md) | Layers and rigging |
| [07-review-checklist](./07-review-checklist.md) | Review checklist |

## 1x — technique

| Chapter | Subject |
|---|---|
| [10-lines-and-curves](./10-lines-and-curves.md) | Lines and curves |
| [11-clusters-and-noise](./11-clusters-and-noise.md) | Clusters and noise |
| [12-anti-aliasing](./12-anti-aliasing.md) | Anti-aliasing |
| [13-dithering-and-texture](./13-dithering-and-texture.md) | Dithering and texture |
| [14-readability-and-scale](./14-readability-and-scale.md) | Readability and scale |

## 2x — colour and materials

| Chapter | Subject |
|---|---|
| [20-color-for-pixel-art](./20-color-for-pixel-art.md) | Colour for pixel art |
| [21-limited-and-platform-palettes](./21-limited-and-platform-palettes.md) | Limited and platform palettes |
| [22-materials-hard](./22-materials-hard.md) | Hard materials: metal, gold, glass, gems, stone, wood, bone |
| [23-materials-soft](./23-materials-soft.md) | Soft materials: cloth, skin, hair, fur, leather, scales, slime |
| [24-lighting-scenarios](./24-lighting-scenarios.md) | Lighting scenarios |

## 3x — characters

| Chapter | Subject |
|---|---|
| [30-proportions-by-size](./30-proportions-by-size.md) | Proportions by size |
| [31-anatomy-and-pose](./31-anatomy-and-pose.md) | Anatomy and pose |
| [32-heads-and-faces](./32-heads-and-faces.md) | Heads and faces |
| [33-eyes-and-expressions](./33-eyes-and-expressions.md) | Eyes and expressions |
| [34-hands-and-feet](./34-hands-and-feet.md) | Hands and feet |
| [35-hair-and-clothing](./35-hair-and-clothing.md) | Hair and clothing |
| [36-character-design](./36-character-design.md) | Character design |
| [37-views-and-directions](./37-views-and-directions.md) | Views and directions |
| [38-portraits](./38-portraits.md) | Portraits |

## 4x — animation

| Chapter | Subject |
|---|---|
| [40-timing-and-spacing](./40-timing-and-spacing.md) | Timing and spacing |
| [41-idle-and-breathing](./41-idle-and-breathing.md) | Idle and breathing |
| [42-walk-and-run](./42-walk-and-run.md) | Walk and run |
| [43-jump-fall-land](./43-jump-fall-land.md) | Jump, fall and land |
| [44-attacks-and-impacts](./44-attacks-and-impacts.md) | Attacks and impacts |
| [45-secondary-motion](./45-secondary-motion.md) | Secondary motion |
| [46-subpixel-animation](./46-subpixel-animation.md) | Sub-pixel animation |
| [47-top-down-animation](./47-top-down-animation.md) | Top-down animation |

## 5x — creatures

| Chapter | Subject |
|---|---|
| [50-quadrupeds](./50-quadrupeds.md) | Quadrupeds |
| [51-animal-gaits](./51-animal-gaits.md) | Animal gaits |
| [52-birds-and-flight](./52-birds-and-flight.md) | Birds and flight |
| [53-small-creatures](./53-small-creatures.md) | Small creatures |
| [54-monster-design](./54-monster-design.md) | Monster design |

## 6x — environments

| Chapter | Subject |
|---|---|
| [60-skies-and-atmosphere](./60-skies-and-atmosphere.md) | Skies and atmosphere |
| [61-landscapes-and-terrain](./61-landscapes-and-terrain.md) | Landscapes and terrain |
| [62-parallax-backgrounds](./62-parallax-backgrounds.md) | Parallax backgrounds |
| [63-trees-and-foliage](./63-trees-and-foliage.md) | Trees and foliage |
| [64-water](./64-water.md) | Water |
| [65-ground-rocks-grass](./65-ground-rocks-grass.md) | Ground, rocks and grass |
| [66-tiles-and-autotiling](./66-tiles-and-autotiling.md) | Tiles and autotiling |
| [67-architecture-and-interiors](./67-architecture-and-interiors.md) | Architecture and interiors |

## 7x — objects and 3D

| Chapter | Subject |
|---|---|
| [70-perspective](./70-perspective.md) | Perspective and projection |
| [71-isometric](./71-isometric.md) | Isometric (2:1) |
| [72-3d-forms](./72-3d-forms.md) | 3D forms: circles, spheres, cylinders, cubes |
| [73-props-and-items](./73-props-and-items.md) | Props and items: weapons, potions, chests, keys, shields |
| [74-vehicles-and-machines](./74-vehicles-and-machines.md) | Vehicles and machines |
| [75-rotation-and-turnarounds](./75-rotation-and-turnarounds.md) | Rotation, turnarounds and pseudo-3D |

## 8x — effects and UI

| Chapter | Subject |
|---|---|
| [80-vfx-fire-smoke-magic](./80-vfx-fire-smoke-magic.md) | VFX: fire, smoke, explosions and magic |
| [81-impacts-and-game-feel](./81-impacts-and-game-feel.md) | Impacts and game feel |
| [82-particles-and-weather](./82-particles-and-weather.md) | Particles and weather |
| [83-ui-and-icons](./83-ui-and-icons.md) | UI and icons |
| [84-bitmap-fonts](./84-bitmap-fonts.md) | Bitmap fonts |

## 9x — style

| Chapter | Subject |
|---|---|
| [90-platform-styles](./90-platform-styles.md) | Platform styles |
| [91-composition-and-scenes](./91-composition-and-scenes.md) | Composition and scenes |
| [92-generated-art-tells](./92-generated-art-tells.md) | Generated-art tells |

## How these relate to the workflows

The workflows in `SKILL.md` own the order of operations and the tool calls. A
chapter owns the craft for one subject and is read when that subject comes up —
`rules://34-hands-and-feet` while drawing a hand, `rules://66-tiles-and-autotiling`
before the first tile. The six chapters under `references/` are SpriteWare's own
distillation of this material for the cases a workflow needs inline; where the
two disagree, the chapter is right and the reference chapter should be fixed.

Changing a rule here changes behaviour everywhere. Workflows reference rules by
name rather than restating them, so a rule has exactly one place to be wrong.

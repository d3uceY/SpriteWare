# Checks

`validate` runs these against the composite of every frame unless you pass
`scope="layer"`, which judges each layer alone — what you want on a rigged
character, where a clean composite can hide a broken limb.

Omit `checks` to run all of them. Pass an array to run a subset, which is faster
and keeps the report readable while you iterate.

| Check | Reports | What to do |
|---|---|---|
| `palette` | colours in the art that are not in the palette; a document with no palette at all | `palette op=set` the intended list, or `op=adopt_from_art` if the art is the truth |
| `antialiasing` | semi-transparent pixels, and soft edges on an RGB sprite | repaint them opaque. At 1× they read as blur |
| `strays` | isolated pixels floating away from the art | delete or join them to the nearest mass. One specular dot or eye is a decision, not a stray |
| `orphans` | pixels with no neighbour of their own colour inside a fill | a coordinate slip. Repaint from the surrounding colour |
| `gaps` | holes inside the silhouette that should be solid | fill them, or say why the hole is there |
| `bounds` | art touching or crossing the canvas edge | resize with `sprite op=resize`, or move the art inward |
| `outline` | an outline that is mostly one colour but not consistently, or doubled | regenerate it by hand, one style, one colour |
| `banding` | long horizontal runs of adjacent ramp steps | stagger the ends, let boundaries wander |
| `layers` | empty layers, layers with no cels on any frame, duplicate names | delete or fill; duplicate names make every later call ambiguous |
| `animation` | a multi-frame sprite with no tag; uniform frame durations | `tag op=create`, then `frame op=set_duration` per pose |
| `export_readiness` | a document that was never saved | `sprite op=save_as` |
| `volume` | the art's height drifting across a cycle | redraw drifted frames from their neighbours rather than by eye |
| `compliance` | violation of the agent's own plan | fix the art or the plan, whichever was wrong |

Severity: `error` means the art will look wrong; `warning` means it is
suspicious; `note` is advice. Fix every error. For a warning you keep, say why.

`strict=true` promotes style warnings to failures. Use it before a hand-over,
not while iterating.

## `expect` — checking your own plan

`expect` runs on top of the chosen checks and describes what the sprite is
supposed to be, so the mechanical pass can check intent rather than generic
rules.

```
validate expect={
  "layerFrames": {"sword-fx": [[3, 4]]},
  "mustNotOverlap": [["sword", "head"]],
  "paletteSize": 16,
  "bounds": {"x": 0, "y": 0, "width": 32, "height": 32},
  "singleComponent": true
}
```

- `layerFrames` — frames each layer must have pixels on. Flags a layer empty
  inside its range or painting outside it.
- `mustNotOverlap` — layer pairs that must never collide on the same frame.
- `paletteSize` — a cap on distinct colours.
- `bounds` — the rectangle the art must stay inside.
- `singleComponent` — the art must be one connected shape.

Findings from `expect` carry the check id `expect`, so a failure is yours to
fix at the art or at the plan.

## What `validate` cannot see

It measures pixels, not intent. It cannot tell you the sprite is unreadable, the
pose is stiff, or the light comes from two directions. Those are in
[review](../review/SKILL.md) and [craft](./craft.md).

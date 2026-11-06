---
name: animate
description: Build an idle, walk, run or attack cycle from key poses, move limbs as cels instead of redrawing them, set timing that reads as motion, tag the range, and review it as a filmstrip. Use when a rigged sprite needs to move.
---

# Animate a cycle

## Before you start

The sprite must be rigged onto named layers — see [rig](../rig/SKILL.md).
Animating a flattened sprite means redrawing every frame.

## 1. Plan the key poses before touching a frame

A walk, in order:

1. **Contact** — front foot lands, back foot pushing. Widest stance.
2. **Down** — weight on the front leg, body **lowest**.
3. **Pass** — legs together, body **highest**. The frame everyone forgets, and
   its absence is why a walk reads as sliding.
4. **Up** — pushing off, rising.

Then mirror for the other leg: eight frames. A four-frame walk uses contact and
pass per leg.

Idle: 2–4 frames, a breath — chest up one pixel, shoulders a frame later.
Attack: anticipation, strike, hold, recover. Frame counts and per-cycle tables
are in [animation](../references/animation.md).

## 2. Create the frames

```
frame op=add count=7
```

Frames are 1-based. `frame op=list` shows durations and which frame is active.

## 3. Block the key poses by moving cels

```
cel op=move layer="arm-near" frame=3 dx=2 dy=-1
```

Duplicate a whole pose to build the next key from it:

```
frame op=duplicate frame=1
```

`duplicate` reports the offsets the copy inherited. A copied cel carries the
source's position, so an idle bob becomes part of every frame built on it —
`frame op=normalise` zeroes the offsets on a frame when that is not what you
want.

Redraw only where a part genuinely changes shape — a foreshortened arm, a
bending knee. Do that as a read-edit-write: `look op=ascii layer=… frame=…
rulers=false` gives the rows, you edit them, one `draw` kind `grid` puts them
back at the same origin. Unchanged pixels stay exactly where they were, so the
part's volume cannot drift.

## 4. Look at the keys before adding in-betweens

```
look op=filmstrip
```

A vision model reads only the first frame of a GIF, so the filmstrip is the only
way to see the motion. Check volume and height consistency here, while there are
four frames to fix rather than eight.

`look op=diff fromFrame=2 toFrame=3` says exactly what moved between two keys
and by how much — the fastest way to find a limb that jumped two pixels on one
side and one on the other.

`look op=onion frame=3` puts the current frame over its ghosted neighbours, so
the spacing either side of an in-between is one picture instead of two images
you have to remember alternately.

## 5. Add in-betweens

Only once the keys read. Spacing is the easing: poses close together read slow,
far apart read fast. SpriteWare has no tween op, so an in-between is placed by
hand — `frame op=duplicate` then `cel op=move` by the offset you meant.

## 6. Set timing

```
frame op=set_duration frames=[1,5] durationMs=150
frame op=set_duration frames=[2,3,4,6,7,8] durationMs=100
```

One duration per group of frames, so two calls cover a walk. Hold contact poses
longer than pass poses — uniform timing reads mechanical no matter how good the
drawings are. Typical ranges are in [animation](../references/animation.md).

## 7. Tag it

```
tag op=create name="walk" from=1 to=8 direction="forward"
```

A tag is what a runtime addresses. `validate` treats a multi-frame sprite with
no tag as an error, so create one even for a two-frame loop. `direction` is
`forward`, `reverse` or `pingpong`.

## 8. Review

```
look op=filmstrip
validate
```

Then run [review](../review/SKILL.md). For a rigged cycle, tell `validate` what
the cycle is supposed to be, so it checks your plan rather than only the generic
rules:

```
validate expect={"layerFrames":{"weapon":[[3,4]]},"mustNotOverlap":[["weapon","head"]]}
```

`layerFrames` flags a layer empty inside its frame range or painting outside it
— an effect leaking into the idle. `mustNotOverlap` flags any frame where two
layers' pixels collide.

## Related

[animation](../references/animation.md), [structure](../references/structure.md).

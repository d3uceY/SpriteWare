# Animation

## Timing is milliseconds

`frame op=set_duration frames=[…] durationMs=N`. Frame durations are what a GIF
and most engines read. Set them; uniform timing reads as a metronome no matter
how good the drawings are.

| Motion | Per frame |
|---|---|
| Idle breath | 300–600 ms |
| Walk | 80–120 ms |
| Run | 60–90 ms |
| Attack anticipation | 80–120 ms |
| Attack strike | 40–60 ms |
| Attack recovery | 100–200 ms |
| Impact hold | 100–150 ms |
| Blink | 60–100 ms, 2 frames |

Hold contact and impact poses longer than passing ones. That asymmetry is the
difference between motion and spinning.

## Key poses

**Idle** — 2–4 frames. Chest up one pixel, shoulders follow a frame later, and a
frame that is not the first repeated back at the end if the loop is long. The
whole cycle should be one pixel of movement; more reads as panting.

**Walk** — 4 or 8 frames.

1. Contact — front foot lands, back foot pushing, widest stance
2. Down — weight on the front leg, body **lowest**
3. Pass — legs together, body **highest**
4. Up — pushing off, rising

Then mirror for the other leg. A 4-frame walk uses contact and pass only. The
pass frame is the one that is always missing, and its absence is why a walk
slides.

**Run** — same four poses, with an airborne frame between push-off and contact,
a forward lean, and a body that stays low except at the top of the arc.

**Jump** — anticipation crouch, extension, apex hold, fall, land squash, recover.
The apex holds longer than anything else in the cycle.

**Attack** — anticipation (pull back, 2 frames), strike (1–2 frames, the fastest
part), hold the contact frame, recover slowly. Wind-up longer than the strike is
what makes it feel heavy.

## Spacing

Poses close together read slow; far apart read fast. Easing is spacing, not
interpolation, which is why SpriteWare has no tween op: place the in-between by
hand with `frame op=duplicate` then `cel op=move`.

## Secondary motion

One or two frames behind the primary action, never on the same frame:

- hair and cloth trail the torso by a frame
- a cape rises on the down pose and falls on the up pose
- a weapon keeps drifting for a frame after the body stops

Do it on its own layer so the lag can be offset without touching the body.

## Sub-pixel movement

At 16–32px, a limb that has to move less than a pixel is moved with a colour:
shift the ramp step at its silhouette edge. Two pixels of movement is one pixel
of travel plus one pixel of shading change.

Below about 12px, do not try. Draw the extremes and accept the snap.

## Loops

- The last frame must not repeat the first, or the loop stutters.
- Volume and height must hold across the cycle — `validate` reports height drift
  as `volume`.
- Tag the range: `tag op=create name="walk" from=1 to=8 direction="forward"`.
  `pingpong` for a loop that plays forward then back, `reverse` sparingly.

## Reviewing motion

`look op=filmstrip` is the only way to see every frame at once. Then
`look op=diff fromFrame=N toFrame=N+1` to see exactly what moved and by how
much, which is how you find the frame where one limb moved two pixels and
another moved one. `look op=onion frame=N` shows the frame over ghosted
neighbours when you need the spacing rather than the pixels.

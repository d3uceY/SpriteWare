# Palette

## One ramp per material

A ramp is the run of colours one material uses, ordered by lightness. Two to
five entries, hue-shifted:

- shadows drift toward blue/violet and desaturate slightly
- highlights drift toward yellow/orange and saturate slightly

`palette op=ramp base="#c04030" steps=5` builds and verifies one: lightness must
rise steadily and neighbours must be far enough apart to read as different
colours. Same hue, darker, is a brightness slider — the most obvious generated
tell there is.

Reuse the darkest entry across materials. It ties the sprite together and costs
no palette slots.

## Sizes

| Sprite size | Colours | Ramps |
|---|---|---|
| 8–16 px | 2–4 | one, often just black and one colour |
| 16–32 px | 4–16 | 2–4 materials, 2–3 steps each |
| 32–64 px | 16–32 | 4–6 materials, 3–5 steps each |

More than 32 colours on a 32px sprite is almost always near-duplicates. Ask
`palette op=analyze` rather than guessing.

## Value contrast beats hue contrast

The eye separates shapes by lightness. Two colours with the same value read as
one shape no matter how different their hue. Before adding a colour, check it
against the one it sits beside: is there a visible step in greyscale?

`palette op=analyze` reports the lowest-contrast pair in the document. If that
number is small, the fix is a value, not a hue.

## Named palettes

Set one with `palette op=set colors=[...]`.

**PICO-8** — 16 colours, forgiving, reads well at any size:

```
#000000 #1d2b53 #7e2553 #008751 #ab5236 #5f574f #c2c3c7 #fff1e8
#ff004d #ffa300 #ffec27 #00e436 #29adff #83769c #ff77a8 #ffccaa
```

**Game Boy** — four greens, and nothing else:

```
#0f380f #306230 #8bac0f #9bbc0f
```

**NES-style** — no single list; build 3–4 hues × 3–4 values by hand and keep the
count between 12 and 16.

## When the art is the truth

If the sprite arrived with colours but no palette, `palette op=adopt_from_art`
sets the palette to exactly what is in use. `op=extract` instead quantises to a
smaller set — pass `steps` to cap it. Both are repair tools for imported art,
not a substitute for deciding a palette up front.

## Colour and mood

- Warm light, cool shadow is the default and reads as daylight.
- Cool light, warm shadow reads as night or artificial light, and it is the
  faster way to make an interior feel different from an exterior.
- One accent colour, used on the thing you want looked at, is worth more than
  four extra palette entries.

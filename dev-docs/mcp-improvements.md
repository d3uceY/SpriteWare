# MCP improvements from drawing the app's own art

Notes from using the SpriteWare MCP tools directly to remake every sprite for the light redesign (about 20 sprites: frames, icons, a 3-frame mascot, a 41-cell glyph font). Each item says what happened, what it cost, and what would fix it. Ordered by how much time or trust it cost.

Everything here was observed in headless mode against Aseprite 1.3.18.6-dev. Where I am guessing at a cause, it says "suspect".

## What worked well

- `draw` `kind=grid` is the right primitive. Readable, round-trips with `look op=ascii`, and rejects ragged rows with a row number. Nearly every sprite was one grid call.
- `look op=preview` caught real mistakes (a mis-concatenated multi-frame row) before export.
- `draw` `kind=text` gave a correct 5x7 font for 40 glyphs with no hand drawing.
- `doctor` answered immediately and said what to do.

## Problems, worst first

### 1. A wrong active document fails silently

`sprite op=open` on a file that is already open in the session does not make it active. The following `draw`, `save` and `export` then act on a different document. Nothing errors. Four exports came out as the wrong sprite (`icon-folder.png` contained the play icon, `icon-stop.png` was the LED sheet). I only found out by writing a PowerShell script to list each PNG's colours.

- Every `draw`, `save` and `export` response should include the path of the document it acted on, so a mismatch is visible in the log.
- `open` should activate, or return `alreadyOpen: true, active: false`.
- `new` should accept a `path`/`name`, and should return an id other than the literal `"Sprite"`. Every new document is `Sprite`, so two are indistinguishable in `sprite list`.
- `export` with `sprite=<path>` has been observed exporting the active document instead (already in `/memories/repo`); either fix or reject the parameter.

### 2. `replace` ignores its region and does not say so

`replace` accepted `x`, `y`, `width`, `height` and also `x0`, `y0`, `x1`, `y1`, and recoloured the whole cel both times. Every pixel of one colour changed, with no warning. I needed "recolour only rows 0 to 3" to get a two-tone font. The summary counts also disagreed with what happened (`555` after a global replace, `223` after a call I believed was region-limited), which made me trust the region form for one step.

- Honour the region, or reject unknown or ignored parameters with an error. Silent acceptance is the bug.
- Add `colour_range` limits for `replace`, or a general `rect` target for any recolouring kind (`replace`, `shade`, `hue_shift`).

### 3. Selection does not clip `text` ops (and may not persist)

I set a 287x3 selection, drew 40 `text` ops in a second colour, and the whole glyph was recoloured, not just the selected rows. The summary reported `555 pixels changed` either way. After `select op=none`, nothing visibly differed. Suspect: selection is not carried between headless calls, or `text` is compiled to a path that skips the mask. Either way the documented behaviour ("clips every draw op") did not hold.

- Make selection clipping apply to every kind, and test it per kind.
- Have `select op=info` report the mask it will apply on the next call.

### 4. `pixelsChanged` is not a change count

Redrawing identical pixels reported hundreds of changes (`1110` for a repeat of the same text pass). It looks like a count of pixels written, not pixels that differ. I could not use it to confirm an edit landed.

- Report pixels whose value actually changed, plus a separate `pixelsWritten`.

### 5. `look op=ascii` is awkward off-palette and for wide sheets

- Colours that are not in the document palette are assigned glyphs like `Æ` and `Ç`, and the summary says "2 off-palette glyph(s) ... their assignment can differ between reads." The output cannot be fed back to `draw` without rewriting it, which was the whole point of the op. A deterministic scheme (`A`, `B`, `C` by first appearance, always with a `legend` of hex values) would round-trip.
- No `region` parameter. A 287x8 sheet returned about 2.3 KB of rows on every call; there is no way to ask for glyphs 3 to 5.
- `preview` of a wide sheet came back at 3x and was not legible. A `scale` or `region` parameter would fix it.

### 6. Multi-frame sheets have to be hand-concatenated

The mascot, the LED, and the nav icons are strips of frames. `grid` draws into one frame, so I built each strip as one wide canvas and typed every row as `frame1 + frame2 + frame3`. I miscounted a string twice (one was rejected by the ragged-row check; one was only caught by a preview). The result is also not a real Aseprite animation, so there are no tags or durations to export as CSS keyframe timings.

- Let `draw` take `target.frame` for `grid` (the `target` field exists in the schema), and let `sprite new` take `frames`.
- Add an `export` mode that writes a horizontal strip with padding 0 and a JSON of frame rectangles and durations. The existing `spritesheet` op has padding 1 by default; a documented `padding: 0` strip for CSS `background-position` would be enough.

## Missing capabilities for this workflow

### 7. A frame / 9-slice generator

Seven of the sprites are the same thing: a bevelled, outlined box with notched corners, sliced for CSS `border-image`. I wrote each grid by hand and recoloured copies with chained `replace` calls. The corner geometry took more thought than the colours.

Suggested `draw kind=frame`: `width`, `height`, `cut` (per corner), `outline`, `hi`, `lo`, `fill`, `openBottom`. Plus `slice` metadata so `export` can print the matching `border-image` value (`slice 3 3 0 3 fill / 6px`), which I had to derive by hand and keep consistent between art and CSS.

### 8. A simultaneous palette remap

Recolouring a sprite to a new theme needed up to nine `replace` ops in a careful order, because a mapped target colour could equal a later source colour. I had to check each chain for collisions.

- Add `kind=remap` with a `{from: to}` dictionary applied in one pass, so order cannot matter.
- `look op=ascii` could return the legend for a sprite so the map can be written from it.

### 9. Offset, shadow and row-limited tone

To get a font with a drop shadow I needed "copy the cel shifted by (+1,+1) under the original". There is `blit`, but it needs a second layer, and the `layer` tool was not part of the visible toolset until I searched for it. A `shift` or `shadow` kind (dx, dy, colour, only where transparent) would make this one call. Row-limited tone (bug 2) would give the lit-top / shaded-bottom face.

### 10. A PNG-in, PNG-out verifier

I wrote a PowerShell script to check each export (size, opaque pixel count, colour list) and a second to read a PNG back into grid rows. Both are generic and I used them all session.

- `export` should return `width`, `height`, `opaquePixels` and the colour list for what it wrote. That alone would have caught problem 1 at the first bad file.
- `draw` `kind=import` (or `look ascii` with a `path`) should read a PNG into rows without opening a document.

### 11. `validate` is aimed at characters, not UI assets

I did not run `validate` on the frames and icons: `strays`, `orphans`, `outline` and `volume` are written for characters, and would flag a notched frame or a 2-pixel LED gem. A UI-asset profile (check size, palette cap, opaque, no semi-transparent, symmetric corners, exact slice geometry) would have been the check I actually wanted.

## Smaller things

- A session accumulates open documents (about 16 KB for `sprite list` by the end) and has no bulk close.
- The `text` font has a fixed 6-pixel advance. A glyph-sheet op (cell width, per-cell colour ramp, optional shadow) would replace 80 `text` ops per font variant.
- `draw` responses should echo the colour count of what was drawn, so a wrong legend (one colour for two chars, or a missing one) shows up immediately.

## Suggested order of work

1. Echo the target document path on every response; fix `open` activation and `export sprite=` (problem 1).
2. Reject ignored parameters; make `replace` and `select` honour regions (2, 3).
3. Fix the `pixelsChanged` count (4).
4. Return dimensions, opaque pixels and colours from `export` (10).
5. Per-frame `grid` and a strip export (6), then `frame` and `remap` (7, 8).

## Where to look

The `replace` and `grid` handling is in `internal/tools/drawing.go`; `look` is in `internal/tools/inspect.go`. I only read the `grid` path, so I have not confirmed the cause of any of the above.

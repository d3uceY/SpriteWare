/* ------------------------------------------------------------------
   Sprite sheet engine.

   Ported from vectile's engine so SpriteWare can animate later without
   changing the contract: a sheet is one horizontal strip of frames drawn
   at one art-pixel scale, a clip says which frames play and how long each
   is held, and the player only ever moves one <img>.

   Elm renders the markup (`[data-sprite]` + attributes). This file owns the
   clock and the transform, because a frame every 90ms does not belong in the
   model: re-diffing the whole view 11 times a second would be waste.
   ------------------------------------------------------------------ */

/** Time per frame when a clip sets neither `frameMs` nor a matching rule. */
export const SPRITE_FRAME_MS = 90;

/** Normalize any span to a first/last pair. A bare number is a single frame. */
export function spanRange(span) {
  return typeof span === "number" ? [span, span] : span;
}

/** How long one frame is held: the clip's `frameMs`, then each timing rule in
    order, where the last rule covering the frame wins. */
export function frameMsFor(clip, frame, total) {
  const sheetFrames = Math.max(1, Math.floor(total ?? clip.sheet.frames));
  let ms = clip.frameMs ?? SPRITE_FRAME_MS;

  for (const rule of clip.timing ?? []) {
    const [first, last] = clampRange(spanRange(rule.frames), sheetFrames);
    if (frame >= first && frame <= last) ms = rule.frameMs;
  }

  return ms;
}

/** Resolve a clip into steps: the frames that play once, then the frames that
    repeat. Each step carries its own hold time, so a dozen per-frame rules cost
    nothing at play time. Ranges are clamped to the sheet and never empty. */
export function framePlan(clip) {
  const total = Math.max(1, Math.floor(clip.sheet.frames));
  const play = clampRange(clip.play ?? [1, total], total);
  const intro = steps(clip, play, total);

  if (clip.loop === false) return { intro, loop: null };

  const loop = clampRange(clip.loop ?? play, total);
  return { intro, loop: steps(clip, loop, total) };
}

function steps(clip, range, total) {
  return framesIn(range).map((frame) => ({
    frame,
    frameMs: frameMsFor(clip, frame, total),
  }));
}

function framesIn([first, last]) {
  return Array.from({ length: last - first + 1 }, (_, i) => first + i);
}

/** Clamp a range to the sheet and order it, so `[8, 3]` reads as 3..8. */
function clampRange([first, last], total) {
  const a = clampFrame(first, total);
  const b = clampFrame(last, total);
  return a <= b ? [a, b] : [b, a];
}

function clampFrame(frame, total) {
  return Math.min(total, Math.max(1, Math.round(frame) || 1));
}

/** The drawn part of a frame, in sheet pixels. */
export function spriteContent(sheet) {
  return (
    sheet.content ?? {
      x: 0,
      y: 0,
      width: sheet.frameWidth,
      height: sheet.frameHeight,
    }
  );
}

/** CSS pixels per sheet pixel for a given on-screen art-pixel size. */
export function spriteScale(sheet, artPx) {
  return artPx / (sheet.pixelSize ?? 1);
}

/** The crop box placed inside a box symmetric about the sheet's anchor. A crop
    is rarely centred on the character, and a centred crop would drag the
    character off its spot by half that difference; centring the anchor keeps it
    still and lets the extra width spill to whichever side it is on. The padding
    is rounded up to whole art pixels so the crop stays on the art grid, which
    is what stops a pixel-art sheet shimmering as it steps. */
export function spriteAnchor(sheet) {
  const box = spriteContent(sheet);
  const anchor = sheet.anchor ?? box.x + box.width / 2;
  const unit = sheet.pixelSize ?? 1;
  const left = anchor - box.x;
  const right = box.x + box.width - anchor;
  const pad = Math.ceil(Math.max(left, right) / unit) * unit;
  return { width: pad * 2, offset: pad - left };
}

const sheets = new Map();

/** Fetches and decodes a sheet ahead of its first frame. The Image is kept so
    the decoded bitmap stays warm for the <img> that shows it; without this a
    clip change paints the old bitmap under the new frame geometry until the
    new strip arrives. Never rejects. */
export function loadSheet(src) {
  let entry = sheets.get(src);
  if (!entry) {
    const img = new Image();
    img.src = src;
    entry = { img, ready: img.decode().catch(() => {}) };
    sheets.set(src, entry);
  }
  return entry.ready;
}

/** Sizes an <img> for its clip and returns the function that moves it. */
export function frameMovie(img, clip, artPx = 1) {
  const scale = spriteScale(clip.sheet, artPx);
  const box = spriteContent(clip.sheet);
  const anchor = spriteAnchor(clip.sheet);

  img.style.width = `${clip.sheet.frameWidth * clip.sheet.frames * scale}px`;
  img.style.height = `${clip.sheet.frameHeight * scale}px`;

  const wrapper = img.parentElement;
  if (wrapper) {
    wrapper.style.width = `${anchor.width * scale}px`;
    wrapper.style.height = `${box.height * scale}px`;
  }
  if (wrapper && anchor.offset) {
    wrapper.style.paddingLeft = `${anchor.offset * scale}px`;
  }

  return (frame) => {
    const x = -((frame - 1) * clip.sheet.frameWidth + box.x) * scale;
    const y = -box.y * scale;
    img.style.transform = `translate(${x}px, ${y}px)`;
  };
}

/** Plays a clip into an <img> and returns a stop function. `paused` holds the
    first frame, which is what reduced motion asks for. */
export function playClip(img, clip, { artPx = 1, paused = false } = {}) {
  const plan = framePlan(clip);
  const draw = frameMovie(img, clip, artPx);

  if (paused || (plan.intro.length <= 1 && !plan.loop)) {
    draw(plan.intro[0].frame);
    return () => {};
  }

  let step = 0;
  let timer;

  const tick = () => {
    const { intro, loop } = plan;
    const current =
      step < intro.length
        ? intro[step]
        : loop
          ? loop[(step - intro.length) % loop.length]
          : undefined;

    if (!current) return;

    draw(current.frame);
    step += 1;
    timer = setTimeout(tick, current.frameMs);
  };

  tick();
  return () => clearTimeout(timer);
}

/** Reads a clip out of the attributes Elm put on the element. */
function clipFrom(el) {
  const frames = Number(el.dataset.frames || 1);
  const frameWidth = Number(el.dataset.fw || 16);
  const frameHeight = Number(el.dataset.fh || 16);

  return {
    sheet: {
      src: el.dataset.sheet,
      frames,
      frameWidth,
      frameHeight,
      pixelSize: Number(el.dataset.pixelSize || 1),
    },
    frameMs: el.dataset.frameMs ? Number(el.dataset.frameMs) : undefined,
    timing: parseTiming(el.dataset.timing),
    play: el.dataset.play ? parseRange(el.dataset.play) : undefined,
    loop: el.dataset.loop === "once" ? false : el.dataset.loop ? parseRange(el.dataset.loop) : undefined,
  };
}

/** `"2:90, 4-5:120"` — a frame or a run, a colon, and how long it is held. */
function parseTiming(text) {
  if (!text) return undefined;

  const rules = text
    .split(",")
    .map((part) => part.trim())
    .filter(Boolean)
    .map((part) => {
      const [span, ms] = part.split(":").map((s) => s.trim());
      const held = Number(ms);
      if (!span || !Number.isFinite(held)) return null;
      return { frames: parseRange(span), frameMs: held };
    })
    .filter(Boolean);

  return rules.length ? rules : undefined;
}

function parseRange(text) {
  const [first, last] = String(text)
    .split("-")
    .map((n) => Number(n));
  return Number.isFinite(last) ? [first, last] : [first, first];
}

/** Starts every sprite under `root`. Returns a function that stops them all,
    so a re-render does not leave a dozen timers running. */
export function startSprites(root = document) {
  const pauses = [];
  const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  for (const el of root.querySelectorAll("[data-sprite]")) {
    if (el.dataset.spriteStarted === "true") continue;
    el.dataset.spriteStarted = "true";

    const img = el.querySelector("img");
    if (!img) continue;

    const clip = clipFrom(el);
    const artPx = Number(el.dataset.artPx || 1);

    void loadSheet(clip.sheet.src).then(() => {
      pauses.push(playClip(img, clip, { artPx, paused: reduced }));
    });
  }

  return () => {
    for (const stop of pauses) stop();
    pauses.length = 0;
  };
}

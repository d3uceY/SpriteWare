# Result — knight walk cycle (2026-11-17)

Agent: deepseek-flash, `dsh headless`, one server for this run only. Transcript:
`transcript.json` (session `0d7d5407`, 57 steps, 100 MCP calls).

| # | Rubric item | Verdict | Evidence |
|---|---|---|---|
| T1 | Route before the first pixel write | pass | `guide` step 1, first `draw` step 7 |
| T2 | Route is the animation workflow, high confidence | pass | `skill://animate`, `high`, matches walk/cycle/frame |
| R1 | `rules://42-walk-and-run` read before composing | pass | read at step 2 |
| R2 | `rules://40-timing-and-spacing` read before composing | pass | read at step 2 |
| R3 | Reads stay inside the route | n/a | it also read `skill://draw`, `skill://rig`, 03, 14 — cross-workflow reads the animate text invites. Marked pass-with-note, not a miss |
| P1 | Four frames | pass | `frame op=list` |
| P2 | A tag covers all four | pass | `walk`, 1–4, forward |
| P3 | Timing varies | pass | 240 / 220 / 240 / 220 ms |
| P4 | Palette inside the four brief colours | pass | `palette op=analyze`: 4 in use, 0 off-palette |
| V1 | validate before the delivery note | pass | steps 50, 53, 56; save at 54, export at 55 |
| V2 | Receipt knows the route | pass | `checked` 42/05/40, `pending` 41, workflow `skill://animate` |
| V3 | Looked before claiming | pass | `look/preview` first at step 8, then 50 look calls |
| O1 | Deliverables exist | pass | `knight-walk.aseprite`, 512x128 sheet PNG + json |

## What the run does not get right

- The receipt named `rules://04-outlines-and-edges` at steps 50, 53 and 56
  because the outline check kept warning; it was never read, and four outline
  warnings shipped.
- The delivery note says "no palette, stray or antialias errors" — accurate, and
  silent about the outline. `validate` itself called the four warnings "style
  note(s) you may keep".
- 100 MCP calls, 50 of them `look`, for a 32x32 four-frame sprite.

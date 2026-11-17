# Result — shop HUD icons (2026-11-17)

Agent: deepseek-flash, `dsh headless`, one server for this run only. Transcript:
`transcript.json` (session `769fd85c`, 63 steps, 83 MCP calls).

| # | Rubric item | Verdict | Evidence |
|---|---|---|---|
| T1 | Route before the first pixel write | pass | `guide` step 3, first `draw` step 11 |
| C1 | The wrong route was noticed | pass | reasoning: "The route is weird (tiles)"; read `rules://83-ui-and-icons` at step 4 anyway |
| U1 | `rules://83-ui-and-icons` before the last icon | pass | read at step 4, first draw at 11 |
| U2 | Four 16x16 icons | pass | 64x16 document, four named layers, one 16x16 PNG each |
| U3 | One light direction, one outline weight | pass, with one note | top-left highlight and ink outline on heart, potion, coin; the pause button is flat by convention |
| U4 | 1 px margin at 16 px | pass | `state-ascii.txt`: heart x1–14, potion x19–24, coin x32–45, pause x49–62 |
| U5 | Palette inside the four brief colours | pass | `validate`: 4 colours, 0 off-palette |
| V1 | validate before the delivery note | pass | steps 35, 39, 66 |
| V2 | Looked before claiming | pass | `look/preview` first at step 14, 29 look calls |
| O1 | Deliverables exist | pass | four 16x16 PNGs, 256x64 sheet, `hud-icons.aseprite` |

## What the run does not get right

- The route was `skill://export`, high confidence, matched on `export` and
  `sheet` — the brief's delivery sentence. `rules://83-ui-and-icons` arrived
  fourth, as a search fill. The agent read it, which is why the icons are right;
  the router was rescued, not correct.
- The checks digest printed palette, platform-styles and tiles checks. None of
  them is about a 16 px icon.
- Four tool errors: two ragged rows and a missing legend key in `draw`, one
  refused `close` on an unsaved document.

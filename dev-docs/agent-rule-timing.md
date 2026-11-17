# Running an agent rule-timing eval

The craft harness in [evals.md](./evals.md) asks whether the art is good. This
asks a different question: when a real agent is handed a pixel-art brief, does it
fetch the route and read the chapters *before* it draws, and does the tool output
say what actually happened?

The eval is a blind agent plus its transcript. The transcript is the ground truth:
it records every MCP call, every resource read and the step they landed on. A
delivery note is not evidence, and neither is a guide payload that was attached to
a draw rather than asked for.

## What it measures

| Question | Evidence |
|---|---|
| Did the agent route before drawing? | `guide` step vs first `draw` / `transform` / `cel` step |
| What did it read? | `read_mcp_resource` with `server: spriteware` and a `uri` |
| Did it read the chapters the deliverable needs? | read set vs the rubric written before the run |
| Did it read what the route named? | `guide.rules` vs the read URIs |
| Is the route right? | `guide.skill`, `confidence`, `matchedTokens` |
| Did validate know the route? | `guideReceipt.checked` / `pending` / `readNext` |
| Did it look before claiming? | `look` step vs the final message |
| What was produced? | `sprite op=info`, `frame op=list`, `tag op=list`, the PNGs |

## Prerequisites

- Go 1.26 (the toolchain the repo already builds with) and Node 24 for the analyzer.
- `dsh` on PATH, or the launcher at
  `%LOCALAPPDATA%\Programs\DeepSeek Harness\resources\runtime\cli\bin\dsh.cmd`.
- `DEEPSEEK_API_KEY` in the environment of the agent process. The desktop app
  stores it in `~/.dsh/.credentials.yaml`; `dsh headless` does not read that file.
- Aseprite, configured as it already is in `%APPDATA%\SpriteWare\config.json`.

The app's window is not needed. `scripts/agent-eval/host` starts the same
`mcpserver.New(tools.New(...))` the window serves, on the configured port, and
prints the URL.

## Run one scenario

1. **Write the brief and the rubric first.** The brief reads like a client
   request: no `guide`, `rules://`, `validate`, "test" or "eval" in it. The rubric
   names the chapters the deliverable actually needs and the timing expected.
   Keep the rubric **out** of the folder the agent works in.
2. **Give the agent an empty workspace.** A scratch folder under `%TEMP%` holding
   the brief and nothing else, used as the agent's cwd. Then `**/*` shows the
   brief, and `internal/skill/` is not one read away.
3. **Start one server and one agent.** `guide` records a route on the
   `tools.Service`; `validate` reports against the last one. Two runs against one
   server share it, so restart between runs.
4. **Launch a headless agent** with the MCP client mounted:

   ```powershell
   $brief = Get-Content -Raw brief.txt
   dsh headless --patch scripts/agent-eval/mcp-patch.yml `
     --permission-mode workspace-write --approval-policy never $brief
   ```

   Do not tell it rules exist, that it is being tested, or which chapters to read.
5. **Capture the result** with `scripts/agent-eval/cli`: `sprite op=info`,
   `frame op=list`, `tag op=list`, `validate`, `look op=ascii`, and copy the saved
   files out of the scratch folder into `eval/agent-eval/<date>-<slug>/`.
6. **Analyze the transcript** and grade against the rubric.

## Read the transcript

```powershell
node scripts/agent-eval/analyze-session.mjs <session-id-or-fragment>
```

It finds the session under `~/.dsh/sessions`, decompresses the concatenated zstd
frames and prints JSON with:

- `toolCalls` — every `mcp__spriteware__*` call in order, with its step and op;
- `reads` — resource URIs, classified `skill`, `rule` or `knowledge`;
- `timing` — `firstReadStep`, `firstComposeStep` (pixels only: `draw`, `transform`,
  `cel`), `firstStructureStep`, `firstValidateStep`, `firstLookStep`;
- `guide` — confidence, skill, matched tokens, routed chapters, the checks digest;
- `coverage` — `routedNotRead` and `readNotRouted`;
- `validate` — the first validate payload, receipt included;
- `errors` — tool calls that came back as errors.

A read with a step below the first compose is the basic pass. Then compare the
reads against the route and the rubric. **A chapter that was not routed but read is
not a failure**: the export workflow links palette chapters, and an agent that
follows a link is doing the right thing.

## The first two runs

Run 2026-11-17, deepseek-flash, one server per run, Windows. Rubrics are in
`eval/agent-eval/rubrics/`; each run folder holds the brief, `transcript.json`, the
validate payload and the output.

| | knight walk cycle | shop HUD icons |
|---|---|---|
| Steps / MCP calls | 57 / 100 | 63 / 83 |
| Route | `skill://animate`, high | `skill://export`, high |
| Matched tokens | walk, cycle, frame, sprite, animation | export, sheet (trigger), export (named) |
| First read | step 2 | step 4 |
| First pixel write | step 7 | step 11 |
| Reads before the draw | 7 | 10 |
| Reads after the draw | 0 | 0 |
| Chapters the rubric needed | 42, 40 | 83 |
| Were they read? | yes, both, step 2 | yes, 83 at step 4 |
| validate | 3 calls, 50 / 53 / 56 | 3 calls, 35 / 39 / 66 |
| Output | 32×32, 4 frames, 240/220/240/220, tag `walk` | 64×16, 4 icons, 4 colours, 4 PNGs + sheet |

Both runs passed the timing question, and neither composed before it routed.
Both produced the deliverable. Timing is not the failure mode here.

### What the runs found

1. **A brief's delivery sentence can own the route.** The icons brief says
   "export one 16x16 PNG per icon plus a single sheet"; the router scored
   `export (named)` and `sheet (trigger)` and had no `icon` token to weigh at
   all, so it returned `skill://export` with *high* confidence. The right chapter, `rules://83-ui-and-icons`, arrived only as the
   fourth entry of the search fill. The agent read it anyway, which is why the run
   still produced four correct icons. That was the agent recovering from the
   router, not the router working.

2. **34 of 62 chapters are owned by no workflow.** Only 28 are in a workflow's
   `rules:` list; hands, faces, water, VFX and `83-ui-and-icons` are reachable only
   through `Search` or by opening `rules://index`. For the two runs that mattered:
   the routed *workflow* chapters for the icons task (21, 90, 66) were the ones the
   agent skipped, and the search fills (83, 73, 22) were the ones it read.

3. **The checks digest follows the workflow, not the match.** `guide` prints the
   Review lines of the first three routed chapters. For the icons run those were
   palette, platform-styles and tiles — palette budgets and tile seams — while the
   UI chapter sat fourth with its 1 px margin rule unprinted. `moreChecks` said 32.

4. **`readNext` has no teeth.** The walk run's receipt named
   `rules://04-outlines-and-edges` three separate times (steps 50, 53, 56) because
   the outline check kept warning. The agent never read it, fixed the strays and
   the export-readiness warning instead, and shipped with four outline warnings.
   The receipt was right and nothing acted on it.

5. **`validate` calls four warnings "style notes you may keep".** The summary line
   said "Clean, with 8 style note(s)" while four of those findings carried
   `severity: warning`. The delivery note echoed that framing exactly: "no palette,
   stray or antialias errors" — true, and silent about the outline.

6. **The guide cannot say where it came from.** `sprite op=new` on `knight-walk`
   carried an attached `guide` as well as the explicit one from step 1. Nothing in
   the payload distinguishes the two, so a transcript (or a UI) cannot tell a route
   the agent asked for from one the server volunteered.

7. **Cost is real.** 100 and 83 MCP calls for a 32 px sprite and four 16 px icons;
   `look` was 50 and 29 of them. That is the price of looking before claiming, but
   it is also half the traffic.

## What the runs said to change

1. **Stop letting the output format win the route.** Fixed: `formatWords` in
   `Route` (`internal/skill/library.go`) earns no trigger doubling and no name
   bonus. `TestGuideLetsTheSubjectOutrankTheFormat` pins the icons brief, and the
   export-only prompt still routes to `skill://export`.
2. **Give UI a home.** Fixed: `draw` carries the `icon` and `hud` triggers, so the
   four-icon brief routes to `skill://draw` and `83-ui-and-icons` arrives as its
   first search fill. UI still owns no workflow of its own.
3. **Digest the chapters that matched.** Fixed in `Guide` (`internal/skill/guide.go`):
   a search chapter whose title carries the task's words now sorts ahead of
   workflow chapters that do not, so `83-ui-and-icons` leads both the reading list
   and the checks digest for an icon brief.
4. **Say where the route came from.** Fixed: `origin` (`explicit` / `attached`) on
   the guide payload, set in `attachGuide` and `handleGuide`; asserted by
   `TestGuideCarriesExecutableChecksAndGates` and `TestAttachGuideIsQuietUnlessConfident`.
5. **Give `readNext` a consequence.** Print it in the summary line, or have
   `export`/`save_as` report when a warning stands on a chapter that was named and
   not read.
6. **Separate notes from warnings in the summary.** Fixed: with warnings and no
   errors, `summarize` now says `N warning(s) and M note(s) to judge; no errors.`
   (`TestSummaryKeepsWarningsVisible`).
7. **Let the receipt distinguish measured from read.** `checked` means a validate
   check ran, not that the agent read the chapter. A `read` boolean per chapter is
   the one fact an eval grades.
8. **Add the icons brief to the compliance corpus.** Done: two icon prompts in
   `pixelPrompts`, 17/17 routing and the largest payload 3.7 KB of the 6 KB budget.
   The walk brief was already covered.

## Pitfalls

- **A rubric in the agent's cwd invalidates the run.** The first attempt globbed
  its own folder, read `rubric.md` at step 3 and called `guide` at step 4. The run
  was thrown away. Rubrics live in `eval/agent-eval/rubrics/`.
- **One server, one run.** The route lives on the service, not the session.
- **The desktop session does not reconnect.** An MCP server started after the DSH
  window booted stays `server is disconnected` for that session. A headless agent
  is a fresh process and connects; that is the reason the eval runs that way.
- **Results are not always keyed by call id.** On Windows the transcript stores
  `tool/result` with the step and no `toolCallId`; the analyzer pairs the n-th
  result in a step with the n-th call.
- **A warning can be a false positive.** The outline check fires on art whose
  outline is intentionally two colours. Read the chapter before deciding it is
  wrong; record the waiver.
- **One run is anecdotal.** Two runs found failure modes, not rates.

## Extending the eval

- New scenario: a brief and a rubric in `eval/agent-eval/`, kept blind.
- New invariant: a Go test in `internal/skill` or `internal/mcpserver` first; the
  eval only diagnoses behaviour.
- New measurement: add it to `analyze-session.mjs` and name the tool call it reads,
  so the number can be checked against the transcript.

# The eval harness

Practice tasks with a written pass bar, and one record per attempt. The point is
not a leaderboard: it is that a claim like "this technique animates better" can
be checked instead of argued.

Three things stay apart because they answer different questions:

| | Question | Who answers | Where it lives |
|---|---|---|---|
| Compliance | Did it follow the brief? | the harness, or a human for the criteria no tool can judge | `run.yaml` (automatic), `review.yaml` (human) |
| Craft | Is it any good? | a person, blind, 0-4 per axis | `review.yaml` |
| Cost | What did it take? | whoever ran it | `run.yaml` |

## Layout

```
eval/benchmarks/<slug>/benchmark.yaml   the task: setup, steps, criteria
eval/runs/<date>-<slug>[-N]/
  source.aseprite                       the editable original, required
  cover.png  sheet.png  idle.gif  ...   the evidence a reviewer looks at
  run.yaml                              prompts verbatim, criteria results, validate
  .technique                            what technique was used, hidden from the review
  review.yaml                           the filled-in form
```

The run folder's name says nothing about the technique. A reviewer who reads the
name before the picture is not scoring blind, so the label lives only in
`.technique`, which nothing but the table reads.

## Commands

These are the `spriteware eval …` subcommands. The wrapper went with the CLI
([removed-command-line.md](./removed-command-line.md)), so nothing in the
shipped binary calls them; the functions behind each line are still in
`internal/eval`, still built, still tested.

```
eval list                      the benchmarks and what each asks
eval new --benchmark <slug>    start a run; --technique goes in .technique
eval brief <run>               print the step texts to hand over
eval add --role <role> <run> <file>
eval check <run>               answer the automatic criteria, write the form
eval score <run>               read the form back and print the result
eval table                     every run, with the spread of verdicts
```

`eval add` takes the role before the positional arguments, because Go's flag
parser stops at the first plain argument.

## Criteria

A criterion is one line of the pass bar, and it is either automatic or human:

```yaml
- id: inside-box
  step: 1
  text: No pixel sits above y 4 or below y 43.
  check: { kind: bounds, y0: 4, y1: 43 }
- id: reads-at-1x
  step: 1
  human: true
  text: At actual size it reads as an armoured knight holding a sword.
```

Exactly one of `check` or `human` is required. The checks are a closed list, so
a benchmark picks one and gives it arguments rather than scripting one:

| kind | what it reads |
|---|---|
| `validate` | one `analyze` check; `failOn: warning` when a warning should fail |
| `silhouette` | every frame is one connected shape (`gaps` with `singleComponent`) |
| `canvas` | width, height, colour mode |
| `layers` | the exact stack, bottom to top, groups excluded |
| `palette` | how many entries the document palette holds |
| `frames` | bounds on the frame count |
| `tag` | a named tag, its direction, whether it covers every frame |
| `timing` | frame durations are not all the same |
| `bounds` | where the ink is, measured over every frame |

A check that cannot fail is worse than no check: `gaps` on its own only ever
warns, which is why `silhouette` exists as its own kind.

## Revisions

`revision` goes up whenever a step's text, the setup or a criterion changes. A
run records the revision it followed, and `eval check` refuses to score a run
against a different one: results from two revisions measured two tasks and are
never ranked together. Change a criterion and every earlier run stays on the
record, unmatched.

## Scoring

1. Look at `cover.png` and `sheet.png` (both 4x), and play `idle.gif`.
2. Fill in `review.yaml`: verdict, the axes, the tick list, the human criteria.
3. `eval score <run>` validates the form (0-4 axes, known defects, no duplicate
   ticks, motion only where there is motion) and prints both halves.
4. `eval table` shows the row. Craft is the verdict as a number: ship 4,
   fixable 2, redraw 0. Add `--minutes 12.5` to `eval score` to record what the
   attempt cost; the table shows a dash until somebody does.

`eval score` rewrites `review.yaml` from the values it validated, so the guidance
comments are gone once a score is recorded. The values, the verdict and the run
line stay; a re-review edits the file as plain YAML.

A review names the run it belongs to. `eval check` writes that line, `eval score`
keeps it, and a form copied into another run's folder is refused rather than
scored against the wrong pictures.

The rubric the axes refer to is `internal documents/craft-rubric.md` — local
only, not in the repo.

## Adding a benchmark

Make a folder with a `benchmark.yaml`. Every criterion needs a check or
`human: true`, every criterion needs a step that exists, and the loader refuses
a benchmark whose steps or criteria would score nothing.

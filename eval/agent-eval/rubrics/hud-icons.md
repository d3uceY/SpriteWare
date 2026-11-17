# Rubric — shop HUD icons (written before the run)

Scenario 2. This brief is the vocabulary-gap probe: the router has no `icon`,
`hud` or `ui` trigger, and no workflow owns `83-ui-and-icons`, so the expected
route is low confidence or wrong. What is graded is what the agent does about
that, not whether the router guessed.

| # | Expectation | Evidence |
|---|---|---|
| T1 | A route was fetched before the first pixel write | `guide` step < first `draw` step |
| C1 | The low confidence / wrong workflow was noticed | transcript shows the agent reasoning about the route, or reading `rules://index` |
| U1 | `rules://83-ui-and-icons` was read before the last icon was drawn | a read of that URI, step < the final draw |
| U2 | Four icons, each 16x16 | canvas size, four cels or four documents |
| U3 | The set shares one light direction and one outline weight | `look` ascii or preview, plus the delivery note |
| U4 | Icons keep a 1 px margin at 16 px | `look op=ascii` edges |
| U5 | Palette stays inside the four brief colours | `validate` palette check, or palette dump |
| V1 | `validate` ran before the delivery note | a `validate` call, then the final message |
| V2 | The agent looked at the icons at actual size | `look` ascii or preview before the final message |
| O1 | The .aseprite and the PNGs exist | files in the run folder |

## Known at write time

The live router answers this brief with `skill://animate`, confidence `low`,
matched token `set` — it is in the animate description. `83-ui-and-icons` is
only reachable as a sixth `search` chapter. Both facts are recorded here so the
grade cannot be written after the fact.

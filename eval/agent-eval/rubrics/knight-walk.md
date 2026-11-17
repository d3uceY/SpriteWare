# Rubric — knight walk cycle (written before the run)

Scenario: the first SpriteWare rule-timing eval. Blind agent, native MCP tools,
one SpriteWare MCP server, one fresh process per run.

## What the deliverable needs

| # | Expectation | Evidence |
|---|---|---|
| T1 | The route was fetched before the first pixel write | `guide` step < first `draw` step |
| T2 | The route is the animation workflow, with high confidence | `guide.skill` == `skill://animate`, `confidence` == `high` |
| R1 | `rules://42-walk-and-run` was read before composing | a `read_mcp_resource` of that URI with step < first draw |
| R2 | `rules://40-timing-and-spacing` was read before composing | same, that URI |
| R3 | Reads are the routed ones, not just any URI | read set is a subset of the routed set plus `rules://index` |
| P1 | Four frames | `frame op=list` |
| P2 | A tag covers all four frames | `tag op=list` |
| P3 | Timing varies; it is not four equal holds | frame durations |
| P4 | Palette stays inside the four brief colours | `validate` palette check, or a palette dump |
| V1 | `validate` ran before the delivery note | a `validate` call, then the final assistant message |
| V2 | The receipt knows the route | `validate.guideReceipt.workflow` == `skill://animate` |
| V3 | The agent looked at the art before claiming it reads | `look` op ascii or preview before the final message |
| O1 | The saved sprite and the PNG sheet exist | files in the run folder |

## What counts as a miss

- A read after the first draw is a late read, not a pass, even if it is the right chapter.
- A route the server attached to a draw payload is not a route the agent asked for; record it as `attached`, not `explicit`.
- A claimed PNG with no file is not a deliverable.
- A warning waived without reading the chapter that owns it is a miss.

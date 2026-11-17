# Skills

The tools are the "how". The skill is the "when, and in what order". It is a
folder of markdown that an agent reads on demand.

## What ships

`internal/skill/` is embedded into the binary with `//go:embed`:

```
SKILL.md              entry point: rules, the loop, a table of workflows
new/                  ten workflows, one folder each
draw/ shade/ palette/ rig/ animate/ tileset/ fix/ review/ export/
references/           craft chapters: craft, palette, shading,
                      animation, structure, checks
rules/                the 62 subject chapters: hands and feet, faces,
                      gaits, tiles, lighting, props, VFX — one subject
                      each, with size budgets and grid templates
```

Every file is markdown. There is no code in the skill, so nothing can break
while an agent reads it.

`rules/` is copied verbatim from `reference/aseprite-ai-artist` (MIT). Those
chapters were written to cite each other as `rules://<file>`, and that is the
URI they are served under, so a cross-reference resolves as published text. Only
`rules/README.md` is adapted: it is the index, and the original pointed at that
project's own build scripts and ADR. Do not edit a chapter to suit SpriteWare —
if our tools disagree with a chapter, `TestRuleGrids` is the test to look at
first, then change the reference chapter or the tool, not the wording.

## Two ways it reaches an agent

**Installed.** `Install` copies all 80 files into the agent's skills folder
(`.claude/skills/spriteware/`, `~/.copilot/skills/spriteware/`, and so on). The
paths per agent are in `skill.go`, function `DirFor`. The install overwrites its
own folder every time, because a stale copy is worse than none.

**Served.** The same tree is published as MCP resources:

```
skill://spriteware      the entry document
skill://draw            one workflow
rules://craft           one reference chapter
rules://index           the subject chapter index
rules://34-hands-and-feet   one subject chapter
knowledge://palettes    generated: the bundled palettes
```

A client that never installs anything still gets the text by reading a
resource. Both paths read one embedded tree, so they cannot disagree.

## Routing a task (the `guide` tool)

The tree is also searchable. `internal/skill/library.go` parses it once into a
`Library` — ten workflows, 62 chapters, six references — and the `guide` tool
routes a task to one workflow plus the chapters worth reading.

Each workflow's frontmatter carries the two lists the router needs:

```yaml
rules: [05-animation, 40-timing-and-spacing, 41-idle-and-breathing, 42-walk-and-run]
triggers: [animation, walk, run, idle, cycle, frame, attack]
```

`rules` is the chapter set the workflow owns, and those chapters come back with
`source: "workflow"`; a search-found chapter whose title carries the task's own
words sorts ahead of them and keeps `source: "search"`, because the chapter the
brief names is the one the digest should print. The sort runs after the reading
budget, so a matching search chapter still has to make the cut. `triggers` is
vocabulary the description never uses — "godot", "wangset", "gameboy" — which
is how a task that names a toolchain rather than an art action still routes.
Both are indexed with the name and description in `Route`; a trigger hit is
marked `(trigger)` and, like a name hit, is strong enough to make the route
confident.

Matching is lexical: words, not meaning. A miss is reported as a miss, with
`skill://spriteware` and `rules://index` to start from instead of a guess, and
the tool description says plainly that a route is a starting point rather than a
reading limit — the session can move to a subject it did not name, and every
`skill://` / `rules://` URI stays readable.

The route also rides on `draw` and `sprite op=new` results: when the label or
the document name routes with confidence, the result carries a compact `guide`.
That is announced once per workflow per session, so a draw-heavy session is not
repeated on every call. While the session has no confident route, those results
carry `guideMissing` instead, so skipping the route is visible in the tool
output rather than silent.

`origin` says which it was: `explicit` when `guide` was called, `attached` when
the server volunteered the route on a draw or a new sprite.

The guide is more than a reading list. It prints the `## Review` lines of the
top three routed chapters in `checks`, so the default answer is something to
run before the next draw, and `moreChecks` says how many Review lines were
withheld. Every route also carries `gates`: the entry document's `## Always-on
gates` section, parsed once at load, minus any gate whose chapters the route
already contains. `Chapter.Checks` and `Library.Gates` are in
`internal/skill/library.go`; the payload is in `internal/tools/guide.go`.

`validate` closes the loop with `guideReceipt`. It maps each measured analyze
check to the chapters that own it (`checkOwners` in
`internal/tools/guide_receipt.go`) and reports `checked` chapters, `pending`
chapters whose rules are prose only, the failing checks, and `readNext` when a
failure belongs to a chapter the route did not carry. It reports against the
last confident route, explicit or attached. That is the answer to "read the
rules" and "read nothing" looking identical in a tool result.

`inline` returns the text itself, for clients that cannot read resources. It is
capped (48 KB) and stops on a chapter boundary, so the result is always whole
documents; `omitted` lists the chapters that did not fit, with their URIs, so
the caller reads them as resources next instead of asking again.

## Why it is split into folders

An agent that reads everything spends its context on the nine workflows it is
not using. So SKILL.md lists the workflows, and each workflow is a separate file
that gets read only when it applies.

`references/` holds what several workflows need. A workflow links to the chapter
instead of repeating it, so there is one place to fix a wrong idea.

`rules/` is the subject library: 62 chapters, read one at a time when the
subject comes up. It is why `SKILL.md` carries a `rules://index` pointer rather
than a table of 62 rows. The instruction block sent on connect collapses the
whole family to one line for the same reason — it is paid for on every message.

## Rules the build enforces

Tests in `internal/skill`:

- every markdown link inside the skill points at a file that ships
- each workflow's frontmatter `name` equals its folder name
- every description is 1–1024 characters (Cline's limit)
- an install writes every file, twice, without nesting
- `Load` finds all 10 workflows, 62 chapters and 6 references, and `Validate`
  holds the routing contract (named chapters exist, triggers are usable words)
- `TestRoutePicksExpectedWorkflow` and
  `TestTriggersRouteWordsTheDescriptionsDoNot` pin the routing table
- `TestInlineHonoursTheBudget` proves the cap stops on a chapter boundary and
  that the omitted chapters are still addressable

Tests in `internal/tools`:

- every ` ```grid ` template in `rules/` compiles through `compileGrid`, the same
  path a caller's grid takes (`TestRuleGridTemplatesCompile`)

Tests in `internal/mcpserver`:

- every markdown file is published as a resource
- `skill://spriteware` is byte-identical to the installed `SKILL.md`
- every `rules://` URI cited by a chapter resolves (`TestEveryCitedRuleResolves`)
- the instruction block collapses the subject chapters to one line
- every URI `guide` returns is a published resource
  (`TestGuideReturnsOnlyPublishedResources`)
- the resource-compliance eval: 15 real pixel-art prompts route to the expected
  workflow with high confidence, carry a non-empty checks and gates digest,
  stay under 6 KB, and every URI they name reads back through the server;
  three non-pixel prompts never route with high confidence
  (`TestGuideComplianceEval`, `TestGuideComplianceIgnoresNonPixelWork`,
  `TestGuideComplianceResolvesEveryNamedURI`). `.github/workflows/ci.yml`
  runs it on every push and pull request.

## Editing the skill

Change the markdown, then run the tests. If you add a workflow, add the folder,
give it frontmatter whose `name` matches the folder plus `rules:` and
`triggers:` lists, and link it from a table in `SKILL.md`.
`TestEverySkillLinkResolves` will catch a broken link,
`TestEverySkillDocumentIsPublished` will catch a file nobody can reach, and
`TestTheRoutingContractHolds` will catch a chapter that does not exist or a
trigger that is not a usable word.

The card in the window says "Ten workflows and the rules they share." Update
that line in `frontend/elm/src/View.elm` if the count changes.

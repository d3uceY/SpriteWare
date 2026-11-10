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

Tests in `internal/tools`:

- every ` ```grid ` template in `rules/` compiles through `compileGrid`, the same
  path a caller's grid takes (`TestRuleGridTemplatesCompile`)

Tests in `internal/mcpserver`:

- every markdown file is published as a resource
- `skill://spriteware` is byte-identical to the installed `SKILL.md`
- every `rules://` URI cited by a chapter resolves (`TestEveryCitedRuleResolves`)
- the instruction block collapses the subject chapters to one line

## Editing the skill

Change the markdown, then run the tests. If you add a workflow, add the folder,
give it frontmatter whose `name` matches the folder, and link it from a table in
`SKILL.md`. `TestEverySkillLinkResolves` will catch a broken link, and
`TestEverySkillDocumentIsPublished` will catch a file nobody can reach.

The card in the window says "Ten workflows and the rules they share." Update
that line in `frontend/elm/src/View.elm` if the count changes.

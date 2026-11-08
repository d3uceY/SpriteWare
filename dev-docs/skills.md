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
```

Every file is markdown. There is no code in the skill, so nothing can break
while an agent reads it.

## Two ways it reaches an agent

**Installed.** `Install` copies all 17 files into the agent's skills folder
(`.claude/skills/spriteware/`, `~/.copilot/skills/spriteware/`, and so on). The
paths per agent are in `skill.go`, function `DirFor`. The install overwrites its
own folder every time, because a stale copy is worse than none.

**Served.** The same tree is published as MCP resources:

```
skill://spriteware      the entry document
skill://draw            one workflow
rules://craft           one reference chapter
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

## Rules the build enforces

Tests in `internal/skill`:

- every markdown link inside the skill points at a file that ships
- each workflow's frontmatter `name` equals its folder name
- every description is 1–1024 characters (Cline's limit)
- an install writes every file, twice, without nesting

Tests in `internal/mcpserver`:

- every markdown file is published as a resource
- `skill://spriteware` is byte-identical to the installed `SKILL.md`

## Editing the skill

Change the markdown, then run the tests. If you add a workflow, add the folder,
give it frontmatter whose `name` matches the folder, and link it from a table in
`SKILL.md`. `TestEverySkillLinkResolves` will catch a broken link, and
`TestEverySkillDocumentIsPublished` will catch a file nobody can reach.

The card in the window says "Ten workflows and the rules they share." Update
that line in `frontend/elm/src/View.elm` if the count changes.

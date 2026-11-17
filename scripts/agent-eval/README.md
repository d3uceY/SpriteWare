# Agent rule-timing eval

Three small programs, no framework. They answer one question: when a real agent
is handed a pixel-art brief, does it fetch the route and read the chapters before
it draws, and does the tool output show what it actually did?

| file | what it is |
|---|---|
| `host/main.go` | starts the app's MCP server headlessly, on the configured port, without the window |
| `cli/main.go` | calls one tool from outside the agent: setup, capture, and `--tools` for the surface |
| `analyze-session.mjs` | reads a DSH session transcript and prints timing, reads, route and receipt |

## Run

```powershell
go build -o tmp\evalhost.exe ./scripts/agent-eval/host
go build -o tmp\evalcli.exe  ./scripts/agent-eval/cli

# 1. one server, one agent, one run: restart between runs
.\tmp\evalhost.exe            # prints: ready http://127.0.0.1:9933/mcp

# 2. hand the brief to a headless agent that has the MCP client mounted
$brief = Get-Content -Raw brief.txt
dsh headless --patch scripts/agent-eval/mcp-patch.yml --permission-mode workspace-write `
  --approval-policy never $brief

# 3. read the transcript, not the delivery note
node ./scripts/agent-eval/analyze-session.mjs <session-id>
```

`mcp-patch.yml` inserts `@deepseek-ai/dsh-mcp-client` for
`http://127.0.0.1:9933/mcp`. A headless agent is the point: it is a fresh
process, so the server's per-session route state is fresh too, and the transcript
is a file you can re-read.

## Rules the harness learned the hard way

- **Keep the rubric out of the agent's working directory.** The first run globbed
  its own cwd, found `rubric.md` next to the brief and read it before the first
  `guide` call. The run was thrown away. Rubrics live in `eval/agent-eval/rubrics/`,
  never in the folder the agent is told to work in.
- **Run the agent outside the repo.** Its cwd is a scratch folder in `%TEMP%`, so
  `**/*` shows the brief and nothing else, and the skill source is not one `read`
  away.
- **One server per run.** `guide` records a route on the `tools.Service`, and
  `validate` reports against the last one. Two runs against one server share it.
- **A result is not always keyed by call id.** On Windows the transcript stores
  `tool/result` with a `step` and no `toolCallId`; the analyzer pairs the n-th
  result in a step with the n-th call.
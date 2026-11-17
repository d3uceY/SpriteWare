#!/usr/bin/env node
// Summarise one DSH session (an agent's transcript) for the SpriteWare
// rule-timing eval.
//
// The session file is concatenated zstd frames: one frame per append, so
// scanning for the frame magic and decompressing each slice is the only way to
// get the whole transcript with Node's zstd bindings.
//
// Usage: node analyze-session.mjs <session-id-or-fragment> [--json]
//
// The transcript is the ground truth. A delivery note is not evidence: every
// mcp__spriteware__* call, every read_mcp_resource URI and the step it happened
// on comes from here, not from what the agent said it did.

import { readdir, readFile } from "node:fs/promises";
import { join } from "node:path";
import { homedir } from "node:os";
import { zstdDecompressSync } from "node:zlib";

const sessionsRoot = join(homedir(), ".dsh", "sessions");
const query = process.argv[2];
if (!query) {
  console.error("usage: node analyze-session.mjs <session-dir-or-id>");
  process.exit(2);
}

async function findSessionFile() {
  for (const project of await readdir(sessionsRoot)) {
    const projectDir = join(sessionsRoot, project);
    let entries;
    try {
      entries = await readdir(projectDir, { withFileTypes: true });
    } catch {
      continue;
    }
    for (const entry of entries) {
      if (!entry.isDirectory()) continue;
      if (!entry.name.includes(query)) continue;
      const file = join(projectDir, entry.name, "session.v4.jsonl.zstd");
      try {
        await readFile(file);
        return file;
      } catch {
        // not every directory has a transcript
      }
    }
  }
  return null;
}

const MAGIC = [0x28, 0xb5, 0x2f, 0xfd];

function decodeFrames(buf) {
  const starts = [];
  for (let i = 0; i < buf.length - 3; i++) {
    if (buf[i] === MAGIC[0] && buf[i + 1] === MAGIC[1] && buf[i + 2] === MAGIC[2] && buf[i + 3] === MAGIC[3]) {
      starts.push(i);
    }
  }
  const parts = [];
  let failed = 0;
  for (let i = 0; i < starts.length; i++) {
    const end = i + 1 < starts.length ? starts[i + 1] : buf.length;
    try {
      parts.push(zstdDecompressSync(buf.subarray(starts[i], end)));
    } catch {
      failed++;
    }
  }
  return { text: Buffer.concat(parts).toString("utf8"), frames: starts.length, failed };
}

function parseJson(text) {
  try {
    return JSON.parse(text);
  } catch {
    return null;
  }
}

// A tool result carries the text parts; the SpriteWare tools put the structured
// payload on the second line of the first text part.
function resultJson(event) {
  const parts = event?.data?.message?.content;
  if (!Array.isArray(parts)) return null;
  for (const part of parts) {
    if (!part || part.type !== "text" || typeof part.text !== "string") continue;
    const lines = part.text.split("\n");
    for (let i = lines.length - 1; i >= 0; i--) {
      const parsed = parseJson(lines[i]);
      if (parsed && typeof parsed === "object") return parsed;
    }
  }
  return null;
}

function uriKind(uri) {
  if (uri.startsWith("skill://")) return "skill";
  if (uri.startsWith("rules://")) return "rule";
  if (uri.startsWith("knowledge://")) return "knowledge";
  return "other";
}

const file = await findSessionFile();
if (!file) {
  console.error("no session transcript found for", query);
  process.exit(2);
}
const { text, frames, failed } = decodeFrames(await readFile(file));
const events = text
  .split("\n")
  .filter(Boolean)
  .map(parseJson)
  .filter(Boolean);
const header = events.find((e) => e.type === "session") ?? {};

// A result names the call it answers either by toolCallId, by sourceEventSeqs or
// by nothing at all, depending on how the executor recorded it. The last case is
// the common one on Windows: results carry the step and arrive in call order, so
// the n-th result in a step answers the n-th call in that step. All three are
// read, so a run is never analysed as if the tool returned nothing.
const resultsBySeq = new Map();
const resultsByCallId = new Map();
const resultsByStep = new Map();
for (const event of events) {
  if (event.type !== "tool/result") continue;
  const data = event.data ?? {};
  if (data.toolCallId) resultsByCallId.set(data.toolCallId, event);
  for (const seq of data.sourceEventSeqs ?? []) resultsBySeq.set(seq, event);
  const step = data.step;
  if (!resultsByStep.has(step)) resultsByStep.set(step, []);
  resultsByStep.get(step).push(event);
}
const usedResults = new Set();
for (const event of events) {
  if (event.type !== "tool/call") continue;
  const step = event.data?.step;
  const bucket = resultsByStep.get(step) ?? [];
  const next = bucket.find((candidate) => !usedResults.has(candidate));
  if (next) {
    usedResults.add(next);
    resultsBySeq.set(event.seq, next);
  }
}

const calls = [];
for (const event of events) {
  if (event.type !== "tool/call") continue;
  const name = String(event.data?.name ?? "");
  const raw = event.data?.arguments;
  const args = typeof raw === "string" ? parseJson(raw) ?? {} : raw ?? {};
  const result = resultsByCallId.get(event.data?.callId) ?? resultsBySeq.get(event.seq);
  calls.push({
    step: event.data?.step ?? null,
    seq: event.seq,
    name,
    args,
    isError: Boolean(result?.data?.message?.isError),
    result: result ? resultJson(result) : null,
    text: result?.data?.message?.content?.find?.((p) => p?.type === "text")?.text ?? "",
  });
}

// SpriteWare calls arrive as mcp__spriteware__<tool>; resources arrive through
// the generic read_mcp_resource tool.
const mcp = calls.filter(
  (c) => c.name.startsWith("mcp__spriteware__") || (c.name === "read_mcp_resource" && c.args?.server === "spriteware"),
);
const toolName = (c) => (c.name.startsWith("mcp__spriteware__") ? c.name.slice("mcp__spriteware__".length) : c.name);

const reads = [];
for (const call of mcp) {
  if (toolName(call) !== "read_mcp_resource") continue;
  const uri = String(call.args?.uri ?? "");
  if (!uri) continue;
  reads.push({ step: call.step, kind: uriKind(uri), uri });
}

const guideCalls = mcp.filter((c) => toolName(c) === "guide" && !c.isError);
const guideCall = guideCalls.at(-1);
const guide = guideCall?.result ?? null;

// Composing is the first call that writes pixels. Layers, frames, tags and
// palettes are setup: they change the document, not the art, and counting them
// as compose makes a slow setup look like an early draw.
const COMPOSE = new Set(["draw", "transform", "cel"]);
const firstCompose = mcp.find((c) => COMPOSE.has(toolName(c)));
const firstStructure = mcp.find((c) => ["layer", "frame", "tag", "slice", "select"].includes(toolName(c)));
const firstValidate = mcp.find((c) => toolName(c) === "validate");
const firstLook = mcp.find((c) => toolName(c) === "look");
const firstDraw = mcp.find((c) => toolName(c) === "draw");
const firstSpriteNew = mcp.find((c) => toolName(c) === "sprite" && c.args?.op === "new");
const firstImage = mcp.find((c) => toolName(c) === "look" && ["preview", "ascii"].includes(String(c.args?.op ?? "")));
const firstRead = reads[0] ?? null;

const before = (step) => mcp.filter((c) => c.step !== null && step !== null && c.step < step);
const routedRules = Array.isArray(guide?.rules) ? guide.rules.map((r) => String(r.id ?? "").replace(/^rules:\/\//, "")) : [];
const readRuleIds = reads.filter((r) => r.kind === "rule").map((r) => r.uri.replace(/^rules:\/\//, ""));

const summary = {
  session: {
    id: header.id,
    cwd: header.cwd,
    createdAt: header.createdAt,
    delegationDepth: header.delegationDepth ?? 0,
  },
  transcript: { file, frames, failed, events: events.length },
  toolCalls: mcp.map((c) => ({ step: c.step, name: toolName(c), op: c.args?.op ?? null, isError: c.isError })),
  reads,
  guide: guide
    ? {
        confidence: guide.confidence ?? null,
        skill: guide.skill?.uri ?? null,
        matched: guide.matchedTokens ?? null,
        rules: routedRules,
        checks: Array.isArray(guide.checks) ? guide.checks.map((c) => ({ rule: c.rule, checks: c.checks })) : [],
        gates: Array.isArray(guide.gates) ? guide.gates.length : 0,
        moreChecks: guide.moreChecks ?? null,
      }
    : null,
  timing: {
    firstReadStep: firstRead?.step ?? null,
    firstSpriteNewStep: firstSpriteNew?.step ?? null,
    firstStructureStep: firstStructure?.step ?? null,
    firstComposeStep: firstCompose?.step ?? null,
    firstDrawStep: firstDraw?.step ?? null,
    firstLookStep: firstLook?.step ?? null,
    firstImageStep: firstImage?.step ?? null,
    firstValidateStep: firstValidate?.step ?? null,
    readBeforeCompose: firstRead && firstCompose ? firstRead.step < firstCompose.step : null,
    readsBeforeCompose: firstCompose ? before(firstCompose.step).filter((c) => toolName(c) === "read_mcp_resource").length : null,
    readsAfterCompose: firstCompose ? mcp.filter((c) => toolName(c) === "read_mcp_resource" && c.step > firstCompose.step).length : null,
    composedBeforeGuide: guideCall && firstCompose ? firstCompose.step < guideCall.step : null,
    validated: Boolean(firstValidate),
    lookedBeforeValidate: firstLook && firstValidate ? firstLook.step < firstValidate.step : null,
  },
  coverage: {
    routed: routedRules,
    read: readRuleIds,
    routedNotRead: routedRules.filter((id) => !readRuleIds.includes(id)),
    readNotRouted: readRuleIds.filter((id) => !routedRules.includes(id)),
  },
  validate: firstValidate?.result ?? null,
  errors: mcp.filter((c) => c.isError).map((c) => ({ step: c.step, name: toolName(c), text: c.text.slice(0, 240) })),
};

console.log(JSON.stringify(summary, null, 2));
// The only file that knows about both worlds.
//
//   Elm  --port-->  main.js  --binding-->  Go
//   Elm  <--port--  main.js  <--promise--  Go
//
// Elm owns state and structure. This file owns what it is bad at: talking to Go,
// flattening generated classes, and motion.

import { Events } from "@wailsio/runtime";
import gsap from "gsap";

import { MCPService } from "../bindings/github.com/d3uceY/spriteware/backend/index.js";
import { startSprites } from "./sprite.js";

const STATUS_EVENT = "mcp:status";
const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

const elm = window.Elm.Main.init({
  node: document.getElementById("app"),
  flags: null,
});

// ---------------------------------------------------------------------------
// Actions
//
// One entry per action Elm sends through the `request` port. `run` does the work;
// `send` is the only place a port name is written down.
// ---------------------------------------------------------------------------

function send(port, value) {
  elm.ports[port].send(value);
}

async function refreshAll() {
  const [status, setup, tools, clients, skillText, project] = await Promise.all([
    MCPService.GetMCPStatus(),
    MCPService.GetSetupState(),
    MCPService.ListTools(),
    MCPService.ListClients(),
    MCPService.SkillText(),
    MCPService.ProjectFolder(),
  ]);
  send("statusFromJs", toStatus(status));
  send("setupFromJs", toSetup(setup));
  send("toolsFromJs", (tools ?? []).map(toTool));
  send("clientsFromJs", (clients ?? []).map(toClient));
  send("skillTextFromJs", String(skillText ?? ""));
  send("projectFromJs", toProject(project));
}

async function refreshServer() {
  const [status, setup, clients] = await Promise.all([
    MCPService.GetMCPStatus(),
    MCPService.GetSetupState(),
    MCPService.ListClients(),
  ]);
  send("statusFromJs", toStatus(status));
  send("setupFromJs", toSetup(setup));
  send("clientsFromJs", (clients ?? []).map(toClient));
}

// The folder picker changes the skill paths on every card, so a refresh that
// skips the client list would leave the old paths on screen.
async function refreshAfterProjectChange() {
  const [project, clients] = await Promise.all([
    MCPService.ProjectFolder(),
    MCPService.ListClients(),
  ]);
  send("projectFromJs", toProject(project));
  send("clientsFromJs", (clients ?? []).map(toClient));
}

const actions = {
  bootstrap: refreshAll,

  status: refreshServer,

  start: async () => {
    const settings = await MCPService.GetSettings();
    await MCPService.StartServer(settings.port, settings.transport);
    await refreshServer();
  },

  stop: async () => {
    await MCPService.StopServer();
    await refreshServer();
  },

  mode: async (mode) => {
    send("statusFromJs", toStatus(await MCPService.SwitchMode(mode)));
  },

  "confirm-path": async (path) => {
    const setup = await MCPService.ConfirmAsepritePath(String(path ?? ""));
    send("setupFromJs", toSetup(setup));
    await refreshServer();
  },

  "forget-path": async () => {
    const setup = await MCPService.ForgetAsepritePath();
    send("setupFromJs", toSetup(setup));
    await refreshServer();
  },

  // Install writes the entry into the assistant's config. The answer is always a
  // message, so a refusal has somewhere to explain itself.
  install: async (id) => {
    const result = await MCPService.InstallClient(String(id ?? ""));
    send("installFromJs", {
      ok: Boolean(result?.ok),
      message: String(result?.message ?? ""),
    });
  },

  // "This project" has to be a folder the user picked: launched from a shortcut,
  // the working directory is the install folder, and a skill written there is one
  // no agent reads.
  "choose-project": async () => {
    await MCPService.ChooseProjectFolder();
    await refreshAfterProjectChange();
  },

  "clear-project": async () => {
    await MCPService.ClearProjectFolder();
    await refreshAfterProjectChange();
  },

  // A skill is a folder with a SKILL.md, so this copies into the skills
  // directory instead of merging a config.
  "install-skill": async (target) => {
    const [agent, scope] = String(target ?? "").split("|");
    const result = await MCPService.InstallSkill(agent ?? "", scope ?? "project");
    send("skillFromJs", {
      ok: Boolean(result?.ok),
      message: String(result?.message ?? ""),
    });
  },
};

elm.ports.request.subscribe(async ({ action, target }) => {
  const handler = actions[action];
  if (!handler) {
    elm.ports.errorFromJs.send(`unknown action "${action}"`);
    return;
  }

  try {
    await handler(target);
  } catch (err) {
    // Elm models success or failure, never an exception, so anything that throws
    // becomes a string on the error port.
    elm.ports.errorFromJs.send(describe(err));
  }
});

// A clipboard write needs a user gesture and Elm cannot make one, so the click
// arrives here.
elm.ports.copyToClipboard.subscribe(async (text) => {
  try {
    await navigator.clipboard.writeText(text);
    return;
  } catch {
    // WebView2 can refuse the async API; the old selection trick still works.
  }

  const holder = document.createElement("textarea");
  holder.value = text;
  holder.setAttribute("readonly", "");
  holder.style.position = "fixed";
  holder.style.opacity = "0";
  document.body.appendChild(holder);
  holder.select();
  try {
    document.execCommand("copy");
  } finally {
    document.body.removeChild(holder);
  }
});

// Go pushes status whenever anything changes, including when the background
// search finishes, so the setup screen never shows a stale "not found".
try {
  Events.On(STATUS_EVENT, () => {
    refreshServer().catch((err) => send("errorFromJs", describe(err)));
  });
} catch (err) {
  console.warn("[wails] custom events unavailable:", describe(err));
}

// ---------------------------------------------------------------------------
// Flattening
//
// Doing it here means a rename in Go shows up as one broken field, not a blank
// screen.
// ---------------------------------------------------------------------------

function toStatus(raw) {
  const s = raw ?? {};
  return {
    running: Boolean(s.running),
    port: Number(s.port ?? 0),
    url: String(s.url ?? ""),
    transport: String(s.transport ?? ""),
    mode: String(s.mode ?? ""),
    bridgeConnected: Boolean(s.bridgeConnected),
    toolCount: Number(s.toolCount ?? 0),
    lastError: String(s.lastError ?? ""),
  };
}

function toSetup(raw) {
  const s = raw ?? {};
  return {
    confirmed: Boolean(s.confirmed),
    path: String(s.path ?? ""),
    candidate: String(s.candidate ?? ""),
    found: Boolean(s.found),
    searching: Boolean(s.searching),
    searched: Array.isArray(s.searched) ? s.searched.map(String) : [],
    problem: String(s.problem ?? ""),
  };
}

function toTool(raw) {
  return { name: String(raw?.name ?? ""), blurb: String(raw?.blurb ?? "") };
}

// ProjectFolder resolves to a [path, chosen] pair; the pair is meaningless to
// Elm, so it becomes a record here.
function toProject(raw) {
  const [path, chosen] = Array.isArray(raw) ? raw : [];
  return { path: String(path ?? ""), chosen: Boolean(chosen) };
}

function toClient(raw) {
  const c = raw ?? {};
  const target = c.target ?? {};
  const install = c.install ?? {};
  const skill = c.skill ?? {};
  return {
    id: String(c.id ?? ""),
    label: String(c.label ?? ""),
    place: String(c.where ?? ""),
    note: String(c.note ?? ""),
    snippet: String(c.snippet ?? ""),
    target: { path: String(target.path ?? ""), detected: Boolean(target.detected) },
    install: { can: Boolean(install.can), why: String(install.why ?? "") },
    skill: {
      available: Boolean(skill.available),
      project: String(skill.project ?? ""),
      global: String(skill.global ?? ""),
      note: String(skill.note ?? ""),
    },
  };
}

function describe(err) {
  if (err == null) return "unknown error";
  if (typeof err === "string") return err;
  if (typeof err.message === "string" && err.message !== "") return err.message;
  try {
    return JSON.stringify(err);
  } catch {
    return String(err);
  }
}

// ---------------------------------------------------------------------------
// Motion
//
// A view change fades its page in and starts any sprite clips. Nothing blinks,
// and none of it runs under reduced motion.
// ---------------------------------------------------------------------------

const EASE = "power2.out";

function enterPage() {
  const items = document.querySelectorAll(".page > *");
  if (items.length === 0) return;

  gsap.from(items, {
    y: 8,
    opacity: 0,
    duration: 0.3,
    stagger: 0.04,
    ease: EASE,
    clearProps: "transform,opacity",
  });
}

// Elm re-renders the whole tree, so this runs only when the view changes.
let currentView = null;

function syncView() {
  const root = document.querySelector("[data-view]");
  const view = root?.dataset.view ?? null;
  if (view === currentView) return;

  const first = currentView === null;
  currentView = view;

  if (first || reducedMotion) return;
  enterPage();
}

// A page taller than the window loses its scroll position when the stage's
// children are replaced, so remember it and put it back after the patch.
let stageScroll = 0;

document.addEventListener(
  "scroll",
  (event) => {
    const target = event.target;
    if (target instanceof Element && target.classList.contains("stage")) {
      stageScroll = target.scrollTop;
    }
  },
  true,
);

function restoreScroll() {
  const stage = document.querySelector(".stage");
  if (stage && Math.abs(stage.scrollTop - stageScroll) > 1) {
    stage.scrollTop = stageScroll;
  }
}

function boot() {
  const root = document.getElementById("app");
  if (!root) return;

  // Elm rebuilds the tree on every update, so a sprite appearing after first
  // paint would keep its raw size. startSprites skips elements it already sized.
  const observer = new MutationObserver(() =>
    requestAnimationFrame(() => {
      startSprites();
      syncView();
      restoreScroll();
    }),
  );
  observer.observe(root, { childList: true, subtree: true });

  startSprites();
  syncView();
}

boot();
